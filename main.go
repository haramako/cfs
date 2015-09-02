// コンテントファイルシステム
package main

import (
	"flag"
	"fmt"
	"github.com/haramako/cfs/cfs"
	"os"
)

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
		if len(args) != 3 {
			fmt.Println("invalid argument number")
			showHelp()
		}
		doSync(args[0], args[1], args[2])
	default:
		fmt.Printf("unknown subcommand '%s'\n", subcommand)
		showHelp()
	}
}

func doUpload(files []string) {

	//u, _ := CreateEmptyUploader("")
	// u, _ := cfs.CreateS3Uploader("cfs-dev")
	u, _ := cfs.CreateFileUploader("/tmp/cfstmp")

	bucket, err := cfs.BucketFromFile(*optFile, u)
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

func doSync(baseUrl string, location string, dir string) {

	bucket, err := cfs.BucketFromUrl(baseUrl, location)
	if err != nil {
		panic(err)
	}

	err = bucket.Sync(dir)
	if err != nil {
		panic(err)
	}

}
