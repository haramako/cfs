package cfs

import (
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"path"
)

type SftpUploader struct {
	rootPath string
	conn     *sftp.Client
	stat     UploaderStat
}

func CreateSftpUploader(info string) (*SftpUploader, error) {
	var signer ssh.Signer
	sshKeyFile, err := homedir.Expand("~/.ssh/id_rsa")
	if err == nil {
		sshKey, err := ioutil.ReadFile(sshKeyFile)
		if err == nil {
			signer, err = ssh.ParsePrivateKey(sshKey)
			if err != nil {
				return nil, err
			}
		}
	}

	config := &ssh.ClientConfig{
		User:            "tdadmin",
		HostKeyCallback: nil,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
			ssh.Password("Td121001"),
		},
	}
	config.SetDefaults()
	sshConn, err := ssh.Dial("tcp", "192.168.8.200:22", config)
	if err != nil {
		return nil, err
	}
	u := new(SftpUploader)
	sftpConn, err := sftp.NewClient(sshConn)
	if err != nil {
		return nil, err
	}
	u.rootPath = "/tmp/cfs-repo"
	u.conn = sftpConn
	return u, nil
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
	file, err := u.conn.Create(fullpath)
	if err != nil {
		return err
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
