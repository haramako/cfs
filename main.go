// コンテントファイルシステム
package main

import (
	"flag"
	"fmt"
	"github.com/haramako/cfs/cfs"
	// "github.com/davecgh/go-spew/spew"
	"os"
)

/*
type SftpUploader struct {
	rootPath string
	conn     *sftp.Client
}

func CreateSftpUploader(info string) (*SftpUploader, error) {
	config := &ssh.ClientConfig{
		User:            "makoto",
		HostKeyCallback: nil,
		Auth: []ssh.AuthMethod{
			ssh.Password("7teen'sMap"),
		},
	}
	config.SetDefaults()
	sshConn, err := ssh.Dial("tcp", "localhost:22", config)
	if err != nil {
		return nil, err
	}
	u := new(SftpUploader)
	sftpConn, err := sftp.NewClient(sshConn)
	if err != nil {
		return nil, err
	}
	rootPath = "/Users/makoto"
	u.conn = sftpConn
	return u, nil
}

func (u *EmptyUploader) Upload(path string, body []byte) error {
	u.conn.Create(path
	return nil
}

func (u *EmptyUploader) Close() {
	u.conn.Close()
}
*/

// エントリーポイント

var (
	optVerbose   = flag.Bool("v", false, "verbose")
	optHelp      = flag.Bool("h", false, "show this help")
	optFile      = flag.String("f", ".bucket", "bucket filename")
	optTag       = flag.String("tag", "", "tag name")
	optRecursive = flag.Bool("r", false, "recursive")
)

func showHelp() {
	fmt.Println("ContentFileSystem Tools")
	fmt.Println("Usage:")
	flag.PrintDefaults()
	os.Exit(0)
}

func main() {

	if len(os.Args) == 1 {
		showHelp()
	}

	subcommand := os.Args[1]
	os.Args = os.Args[1:]
	_ = subcommand

	flag.Parse()
	if *optHelp {
		showHelp()
	}
	cfs.Verbose = *optVerbose

	args := flag.Args()
	switch subcommand {
	case "upload":
		if len(args) == 0 {
			args = []string{"."}
		}
		doUpload(args)
	case "sync":
		if len(args) == 0 {
			args = []string{"cfs-sync"}
		}
		doUpload(args)
	default:
		fmt.Printf("unknown subcommand '%s'\n", subcommand)
		showHelp()
	}
}

func doUpload(files []string) {

	//u, _ := CreateEmptyUploader("")
	// u, _ := cfs.CreateS3Uploader("cfs-dev")
	u, _ := cfs.CreateFileUploader("/tmp/cfstmp")

	bucket, err := cfs.NewBucket(*optFile, u)
	if err != nil {
		panic(err)
	}
	bucket.Tag = *optTag

	for _, path := range files {
		bucket.AddFiles(path)
	}

	bucket.RemoveUntouched()
	err = bucket.Finish()
	if err != nil {
		panic(err)
	}
}

func doSync() {
}
