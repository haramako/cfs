// コンテントファイルシステム
package main

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli"
	"local.package/cfs"
)

var revision string

// エントリーポイント

func main() {

	app := cli.NewApp()
	app.Name = "cfs"
	app.HelpName = "cfs"
	app.Usage = "cfs client"
	app.Version = "0.0.0 " + revision
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
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "force upload(use no cache)",
		},
		cli.StringFlag{
			Name:  "filter-cmd",
			Usage: "command for filter files",
		},
	}
	app.Commands = []cli.Command{
		uploadCommand,
		syncCommand,
		mergeCommand,
		catCommand,
		lsCommand,
		configCommand,
		httpCommand,
		packCommand,
		unpackCommand,
		packBucketCommand,
		patchCommand,
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

	if c.GlobalBool("force") {
		cfs.Option.NoCache = true
	}
}

// ダウンロード用のURLを取得する
// Option.URL が指定されていない場合は、cabinetの情報から取得する
func getDownloaderURL() string {
	// 指定されているなら、それを返す
	if cfs.Option.Url != "" {
		return cfs.Option.Url
	}
	// cabinetからダウンロード用URLを取得する
	storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
	check(err)
	return storage.DownloaderUrl().String()
}

var uploadCommand = cli.Command{
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

var hex = []byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'A', 'B', 'C', 'D', 'E', 'F'}

// ファイル名に使える文字列に変換する
// "0-9A-Za-z_-$." はそのままで、それ以外はURLエンコードを行う
func escapeFilename(s string) string {
	r := make([]byte, 0, len(s)*3)
	for _, c := range []byte(s) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '-' || c == '_' || c == '.' || c == '$' {
			r = append(r, c)
		} else {
			r = append(r, '%')
			r = append(r, hex[(c>>4)&0x0f])
			r = append(r, hex[(c>>0)&0x0f])
		}
	}
	return string(r)
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
		filename := cfs.Option.Cabinet + "$" + cwd + "$" + strings.Join(absDirs, "$")
		filename = escapeFilename(filename)
		filename = fmt.Sprintf("%x", md5.Sum([]byte(filename)))
		// println("cachefile = " + filename)
		bucketPath = filepath.Join(cfs.GlobalCacheDir(), filename)
	}

	var err error
	var bucket *cfs.Bucket
	if cfs.Option.NoCache {
		bucket = cfs.NewBucket()
		bucket.Path = bucketPath
	} else {
		bucket, err = cfs.BucketFromFile(bucketPath)
		check(err)
	}
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

var syncCommand = cli.Command{
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

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	bucket, err := downloader.LoadBucket(location)
	check(err)

	filter := c.GlobalString("filter-cmd")

	if filter != "" {
		bucket, err = filterBucket(filter, bucket)
		check(err)
	}

	check(downloader.Sync(bucket, dir))
}

var mergeCommand = cli.Command{
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

	downloader, err := cfs.NewDownloader(getDownloaderURL())
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

	filter := c.GlobalString("filter-cmd")

	if filter != "" {
		merged, err = filterBucket(filter, merged)
		check(err)
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

var catCommand = cli.Command{
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

	downloader, err := cfs.NewDownloader(getDownloaderURL())
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

var lsCommand = cli.Command{
	Name:      "ls",
	Usage:     "list files in bucket",
	Action:    doLs,
	ArgsUsage: "location",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "verify",
			Usage: "Check for files on the server.",
		},
	},
}

func doLs(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) != 1 {
		fmt.Println("need just 1 arguments")
		os.Exit(1)
	}

	verify := c.Bool("verify")

	location := args[0]

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	bucket, err := downloader.LoadBucket(location)
	check(err)

	// To store the keys in slice in sorted order
	var keys []string
	for k := range bucket.Contents {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if verify {
		fileStatusList, err := downloader.ExistsAll(bucket)
		check(err)

		for _, k := range keys {
			file := bucket.Contents[k]
			status := "ng"
			if fileStatusList[k] {
				status = "ok"
			}
			fmt.Printf("%s\t%s\n", file.Path, status)
		}
	} else {
		for _, k := range keys {
			file := bucket.Contents[k]
			fmt.Printf("%s\t%s\t%d\t%s\n", file.Path, file.OrigHash, file.OrigSize, file.Time.Format(time.RFC3339))
		}
	}
}

var configCommand = cli.Command{
	Name:   "config",
	Usage:  "show current config",
	Action: doConfig,
}

func doConfig(c *cli.Context) {
	loadConfig(c)

	fmt.Printf("Cabinet       : %s\n", cfs.Option.Cabinet)
	fmt.Printf("Downloader URL: %s\n", getDownloaderURL())
}
