// コンテントファイルシステム
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
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
		//panic(err)
		fmt.Println(err)
		os.Exit(1)
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
		check(err)
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
		cli.StringFlag{
			Name:  "output, o",
			Value: "",
			Usage: "hash output file",
		},
	},
}

func doUpload(c *cli.Context) {
	loadConfig(c)

	args := c.Args()
	if len(args) == 0 {
		args = []string{"."}
	}

	output := c.String("output")
	bucketPath := c.String("bucket")
	if bucketPath == "" {
		// bucketが指定されていない場合は、自動でパスを作成する
		cwd, err := filepath.Abs(".")
		check(err)

		absDirs := []string{}
		for _, dir := range args {
			absDir, err := filepath.Abs(dir)
			check(err)
			absDirs = append(absDirs, absDir)
		}
		filename := cfs.Option.Cabinet + ":" + cwd + ":" + strings.Join(absDirs, ":")
		filename = strings.Replace(filename, "/", "__", -1)
		bucketPath = filepath.Join(cfs.GlobalCacheDir(), filename)
	}

	bucket, err := cfs.BucketFromFile(bucketPath)
	check(err)
	bucket.Tag = c.String("tag")

	storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
	check(err)

	client := &cfs.Client{
		Storage: storage,
		Bucket:  bucket,
	}

	check(client.Init())

	for _, path := range args {
		check(client.AddFiles(path))
	}

	bucket.RemoveUntouched()
	check(client.Finish())

	if output != "" {
		check(ioutil.WriteFile(output, []byte(bucket.Hash), 0777))
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
		fmt.Println("need just 2 arguments")
		os.Exit(1)
	}

	location := args[0]
	dir := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	check(err)

	bucket, err := downloader.LoadBucket(location)
	check(err)

	check(downloader.Sync(bucket, dir))
}

var MergeCommand = cli.Command{
	Name:      "merge",
	Usage:     "merge buckets",
	Action:    doMerge,
	ArgsUsage: "output-tag location [...]",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output, o",
			Value: "",
			Usage: "hash output file",
		},
	},
}

func doMerge(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) < 2 {
		fmt.Println("need at least 2 arguments")
		os.Exit(1)
	}

	output := c.String("output")

	mergeTo := args[0]
	mergeFrom := args[1:]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	check(err)

	merged := &cfs.Bucket{
		Tag:      mergeTo,
		Contents: make(map[string]cfs.Content),
		HashType: "md5",
	}

	for _, location := range mergeFrom {
		bucket, err := downloader.LoadBucket(location)
		check(err)
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

	if output != "" {
		check(ioutil.WriteFile(output, []byte(merged.Hash), 0777))
	}
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
		fmt.Println("need 2 arguments")
		os.Exit(1)
	}

	location := args[0]
	filename := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	check(err)

	bucket, err := downloader.LoadBucket(location)
	check(err)

	content, ok := bucket.Contents[filename]
	if !ok {
		fmt.Println("file " + filename + " not found")
		os.Exit(1)
	}

	data, err := downloader.Fetch(content.Hash, content.Attr)
	check(err)

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
		fmt.Println("need just 1 arguments")
		os.Exit(1)
	}

	location := args[0]

	downloader, err := cfs.NewDownloader(getDownloaderUrl())
	check(err)

	bucket, err := downloader.LoadBucket(location)
	check(err)

	// To store the keys in slice in sorted order
	var keys []string
	for k := range bucket.Contents {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		file := bucket.Contents[k]
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
