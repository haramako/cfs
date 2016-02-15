package cfs

import (
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"os/user"
	"path"
)

type SftpUploader struct {
	rootPath string
	conn     *sftp.Client
	stat     UploaderStat
	opt      *SftpOption
	count    int
}

type SftpOption struct {
	Host      string
	Port      int
	IdentFile string
	User      string
	Password  string
	RootPath  string
}

func (o *SftpOption) SetDefaults() error {
	if o.Port == 0 {
		o.Port = 22
	}

	if o.User == "" {
		u, err := user.Current()
		if err == nil {
			o.User = u.Username
		}
	}

	if o.RootPath == "" {
		return fmt.Errorf("invalid empty root path")
	}

	return nil
}

func loadIdentFile(file string) (ssh.Signer, error) {
	sshKey, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(sshKey)
	if err != nil {
		return nil, err
	}

	return signer, nil
}

func CreateSftpUploader(opt *SftpOption) (*SftpUploader, error) {

	sftpConn, err := connect(opt)
	if err != nil {
		return nil, err
	}

	u := new(SftpUploader)
	u.rootPath = opt.RootPath
	u.conn = sftpConn
	u.opt = opt

	return u, nil
}

func (u *SftpUploader) reconnect() error {
	u.count = 0
	u.conn.Close()
	var err error
	u.conn, err = connect(u.opt)
	return err
}

// 255回Createすると止まる問題対策
func connect(opt *SftpOption) (*sftp.Client, error) {
	err := opt.SetDefaults()
	if err != nil {
		return nil, err
	}

	authMethods := make([]ssh.AuthMethod, 0, 3)

	if opt.IdentFile != "" {
		signer, err := loadIdentFile(opt.IdentFile)
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	defaultIdent, err := homedir.Expand("~/.ssh/id_rsa")
	if err == nil {
		signer, err := loadIdentFile(defaultIdent)
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	if opt.Password != "" {
		authMethods = append(authMethods, ssh.Password(opt.Password))
	}

	config := &ssh.ClientConfig{
		User: opt.User,
		Auth: authMethods,
	}
	config.SetDefaults()

	sshConn, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", opt.Host, opt.Port), config)
	if err != nil {
		return nil, err
	}

	sftpConn, err := sftp.NewClient(sshConn)
	if err != nil {
		return nil, err
	}

	return sftpConn, err
}

func mkdirAll(c *sftp.Client, dir string) error {
	base, _ := path.Split(dir)
	if base != "/" && base != "" {
		mkdirAll(c, base[0:len(base)-1])
	}
	c.Mkdir(base)
	return nil
}

func (u *SftpUploader) Upload(_path string, body []byte, overwrite bool) error {
	fullpath := path.Join(u.rootPath, _path)
	dir, _ := path.Split(fullpath)

	if !overwrite {
		stat, err := u.conn.Lstat(fullpath)
		if err == nil && stat != nil {
			return nil
		}
	}

	err := mkdirAll(u.conn, dir)
	if err != nil {
		return err
	}

	u.stat.UploadCount++
	u.count++
	if u.count > 200 {
		err := u.reconnect()
		if err != nil {
			return err
		}
	}
	file, err := u.conn.Create(fullpath)
	if err != nil {
		return fmt.Errorf("Cannot create remote file '%s'", fullpath)
	}

	_, err = file.Write(body)
	if err != nil {
		return err
	}

	if Verbose {
		fmt.Printf("uploading '%s'\n", _path)
	}

	return nil
}

func (u *SftpUploader) Close() {
	u.conn.Close()
}

func (u *SftpUploader) Stat() *UploaderStat {
	return &u.stat
}
