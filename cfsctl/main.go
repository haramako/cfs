// コンテントファイルシステム
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codegangsta/cli"
	"github.com/haramako/cfs"
)

// エントリーポイント

func main() {

	app := cli.NewApp()
	app.Name = "cfs"
	app.HelpName = "cfs"
	app.Usage = "cfs client"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "verbose, V",
			Usage: "verbose",
		},
		cli.StringFlag{
			Name:  "config, C",
			Value: ".cfsenv",
			Usage: "config file",
		},
		cli.StringFlag{
			Name:  "cabinet, c",
			Value: "",
			Usage: "cabinet URL",
		},
	}
	app.Commands = []cli.Command{
		UploadCommand,
		SyncCommand,
		MergeCommand,
		CatCommand,
		LsCommand,
		ConfigCommand,
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func check(err error) {
	if err != nil {
		panic(err)
		//fmt.Println(err)
		//os.Exit(1)
	}
}

func loadConfig(c *cli.Context) {
	cfs.Verbose = c.GlobalBool("V")
	cfs.LoadDefaultOptions(c.GlobalString("config"))

	if c.GlobalString("cabinet") != "" {
		cfs.Option.Cabinet = c.GlobalString("cabinet")
	}

	if c.GlobalString("url") != "" {
		cfs.Option.Url = c.GlobalString("url")
	}
}

func getDownloaderUrl() string {
	if cfs.Option.Url != "" {
		return cfs.Option.Url
	} else {
		// cabinetからダウンロード用URLを取得する
		storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return storage.DownloaderUrl().String()
	}
}

var UploadCommand = cli.Command{
	Name:   "upload",
	Usage:  "upload files to cabinet",
	Action: doUpload,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "tag, t",
			Value: "",
			Usage: "tag name",
		},
		cli.StringFlag{
			Name:  "bucket, b",
			Value: "", // 指定されない場合は、自動で設定される
			Usage: "bucket file name",
		},
	},
}

func doUpload(c *cli.Context) {
	loadConfig(c)

	args := c.Args()
	if len(args) == 0 {
		args = []string{"."}
	}

	bucketPath := c.String("bucket")
	if bucketPath == "" {
		// bucketが指定されていない場合は、自動でパスを作成する
		absDirs := []string{}
		for _, dir := range args {
			absDir, err := filepath.Abs(dir)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			absDirs = append(absDirs, absDir)
		}
		filename := cfs.Option.Cabinet + ":" + strings.Join(absDirs, ":")
		filename = strings.Replace(filename, "/", "__", -1)
		bucketPath = filepath.Join(cfs.GlobalCacheDir(), filename)
	}

	bucket, err := cfs.BucketFromFile(bucketPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bucket.Tag = c.String("tag")

	storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	client := &cfs.Client{
		Storage: storage,
		Bucket:  bucket,
	}

	err = client.Init()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, path := range args {
		err = client.AddFiles(path)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	bucket.RemoveUntouched()
	err = client.Finish()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var SyncCommand = cli.Command{
	Name:      "sync",
	Usage:     "sync from cabinet",
	Action:    doSync,
	ArgsUsage: "location output-dir",
}

func doSync(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) != 2 {
		panic("need just 2 arguments")
	}

	location := args[0]
	dir := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	if err != nil {
		panic(err)
	}

	bucket, err := downloader.LoadBucket(location)
	if err != nil {
		panic(err)
	}

	err = downloader.Sync(bucket, dir)
	if err != nil {
		panic(err)
	}

}

var MergeCommand = cli.Command{
	Name:      "merge",
	Usage:     "merge buckets",
	Action:    doMerge,
	ArgsUsage: "output-tag location [...]",
}

func doMerge(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) < 2 {
		panic("need at least 2 arguments")
	}

	mergeTo := args[0]
	mergeFrom := args[1:]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	if err != nil {
		panic(err)
	}

	merged := &cfs.Bucket{
		Tag:      mergeTo,
		Contents: make(map[string]cfs.Content),
		HashType: "md5",
	}

	for _, location := range mergeFrom {
		bucket, err := downloader.LoadBucket(location)
		if err != nil {
			panic(err)
		}
		if cfs.Verbose {
			fmt.Printf("%d files merged from %s\n", len(bucket.Contents), location)
		}
		merged.Merge(bucket)
	}

	if cfs.Verbose {
		fmt.Printf("total %d files into %s\n", len(merged.Contents), mergeTo)
	}

	// マージしたバケットを書き込む
	storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
	check(err)

	client := &cfs.Client{
		Storage: storage,
		Bucket:  merged,
	}

	check(client.Init())

	check(client.UploadBucket())
}

var CatCommand = cli.Command{
	Name:      "cat",
	Usage:     "fetch a data from url (for debug)",
	Action:    doCat,
	ArgsUsage: "location filename",
}

func doCat(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) < 2 {
		panic("need 2 arguments")
	}

	location := args[0]
	filename := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	if err != nil {
		panic(err)
	}

	bucket, err := downloader.LoadBucket(location)
	if err != nil {
		panic(err)
	}

	content, ok := bucket.Contents[filename]
	if !ok {
		panic("file " + filename + " not found")
	}

	data, err := downloader.Fetch(content.Hash, content.Attr)
	if err != nil {
		panic(err)
	}

	fmt.Print(string(data))
}

var LsCommand = cli.Command{
	Name:      "ls",
	Usage:     "list files in bucket",
	Action:    doLs,
	ArgsUsage: "location",
}

func doLs(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) != 1 {
		panic("need just 1 arguments")
	}

	location := args[0]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	if err != nil {
		panic(err)
	}

	bucket, err := downloader.LoadBucket(location)
	if err != nil {
		panic(err)
	}

	for _, file := range bucket.Contents {
		fmt.Printf("%-40s %s %s\n", file.Path, file.Hash, file.Time.Format(time.RFC3339))
	}

}

var ConfigCommand = cli.Command{
	Name:   "config",
	Usage:  "show current config",
	Action: doConfig,
}

func doConfig(c *cli.Context) {
	loadConfig(c)

	fmt.Printf("Cabinet       : %s\n", cfs.Option.Cabinet)
	fmt.Printf("Downloader URL: %s\n", getDownloaderUrl())
}
