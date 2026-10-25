// コンテントファイルシステム
package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/urfave/cli/v3"
)

var revision string

// エントリーポイント

func main() {

	app := &cli.Command{}
	app.Name = "cfs"
	app.Usage = "cfs client"
	app.Version = "0.0.0 " + revision
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"V"},
			Usage:   "verbose",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"C"},
			Value:   ".cfsenv",
			Usage:   "config file",
		},
		&cli.StringFlag{
			Name:    "setting",
			Aliases: []string{"S"},
			Value:   ".cfs_setting",
			Usage:   "setting file",
		},
		&cli.StringFlag{
			Name:    "cabinet",
			Aliases: []string{"c"},
			Value:   "",
			Usage:   "cabinet URL or path",
		},
		&cli.StringFlag{
			Name:  "url",
			Usage: "download location (URL or path)",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "force upload(use no cache)",
		},
		&cli.StringFlag{
			Name:  "filter-cmd",
			Usage: "command for filter files",
		},
	}
	app.Commands = []*cli.Command{
		uploadCommand,
		syncCommand,
		mergeCommand,
		catCommand,
		lsCommand,
		configCommand,
		settingCommand,
		serveCommand,
		packCommand,
		unpackCommand,
		packManifestCommand,
		patchCommand,
		gcCommand,
		tagCommand,
		cabinetCommand,
		keygenCommand,
	}

	err := app.Run(context.Background(), os.Args)
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

func loadConfig(c *cli.Command) {
	cfs.InitDefaultSetting()

	cfs.Verbose = c.Bool("verbose")
	err := cfs.LoadDefaultOptions(c.String("config"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// コマンドラインで指定された相対パスは、今のフォルダを基準にする
	cwd, err := os.Getwd()
	check(err)

	if c.String("cabinet") != "" {
		cfs.Option.Cabinet, err = cfs.ResolveLocation(c.String("cabinet"), cwd)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if c.String("url") != "" {
		cfs.Option.Url, err = cfs.ResolveLocation(c.String("url"), cwd)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if c.Bool("force") {
		cfs.Option.NoCache = true
	}
}

// ダウンロード元を取得する
// Option.Url が指定されていない場合は、キャビネット（手元のフォルダや S3）から直接読む
func getDownloaderURL() string {
	if cfs.Option.Url != "" {
		return cfs.Option.Url
	}
	return cfs.Option.Cabinet
}

var uploadCommand = &cli.Command{
	Name:   "upload",
	Usage:  "upload files to cabinet",
	Action: doUpload,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "tag",
			Aliases: []string{"t"},
			Value:   "",
			Usage:   "tag name",
		},
		&cli.StringFlag{
			Name:    "manifest",
			Aliases: []string{"m"},
			Value:   "", // 指定されない場合は、自動で設定される
			Usage:   "local manifest file (cache of the last upload)",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Value:   "",
			Usage:   "hash output file",
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

func doUpload(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	args := c.Args().Slice()
	if len(args) == 0 {
		args = []string{"."}
	}

	output := c.String("output")
	manifestPath := c.String("manifest")
	if manifestPath == "" {
		// manifestが指定されていない場合は、自動でパスを作成する
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
		manifestPath = filepath.Join(cfs.GlobalCacheDir(), filename)
	}

	tag := c.String("tag")
	if tag != "" {
		if err := cfs.ValidateTagName(tag); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
	check(err)

	// 形式の版が違うキャビネットには書かない
	cabinetFetcher, err := cfs.NewFetcher(cfs.Option.Cabinet)
	check(err)
	if err := cfs.CheckCabinetFormat(cabinetFetcher); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var manifest *cfs.Manifest
	if !cfs.Option.NoCache {
		manifest, err = cfs.ManifestFromFile(manifestPath)
		check(err)
		// 前回のマニフェストが GC で消えていたら、中身も消えているかもしれないので、キャッシュを使わない
		ok, err := manifest.ValidateCache(storage)
		check(err)
		if !ok {
			if cfs.Verbose {
				fmt.Printf("previous manifest %s is not in the cabinet, check all files\n", manifest.Hash)
			}
			manifest = nil
		}
	}
	if manifest == nil {
		manifest = cfs.NewManifest()
		manifest.Path = manifestPath
	}
	manifest.Tag = tag

	client := &cfs.Client{
		Storage:  storage,
		Manifest: manifest,
	}

	check(client.Init())

	for _, path := range args {
		check(client.AddFiles(path))
	}

	manifest.RemoveUntouched()
	check(client.Finish())

	if output != "" {
		check(os.WriteFile(output, []byte(manifest.Hash), 0777))
	}
	return nil
}

var syncCommand = &cli.Command{
	Name:      "sync",
	Usage:     "sync from cabinet",
	Action:    doSync,
	ArgsUsage: "location output-dir",
}

func doSync(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	var args = c.Args().Slice()
	if len(args) != 2 {
		fmt.Println("need just 2 arguments")
		os.Exit(1)
	}

	location := args[0]
	dir := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	manifest, err := downloader.LoadManifest(location)
	check(err)

	filter := c.String("filter-cmd")

	if filter != "" {
		manifest, err = filterManifest(filter, manifest)
		check(err)
	}

	check(downloader.Sync(manifest, dir))
	return nil
}

var mergeCommand = &cli.Command{
	Name:      "merge",
	Usage:     "merge manifests",
	Action:    doMerge,
	ArgsUsage: "output-tag location [...]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Value:   "",
			Usage:   "hash output file",
		},
	},
}

func doMerge(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	var args = c.Args().Slice()
	if len(args) < 2 {
		fmt.Println("need at least 2 arguments")
		os.Exit(1)
	}

	output := c.String("output")

	mergeTo := args[0]
	mergeFrom := args[1:]

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	if err := cfs.ValidateTagName(mergeTo); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	merged := cfs.NewManifest()
	merged.Tag = mergeTo

	for _, location := range mergeFrom {
		manifest, err := downloader.LoadManifest(location)
		check(err)
		if cfs.Verbose {
			fmt.Printf("%d files merged from %s\n", len(manifest.Contents), location)
		}
		merged.Merge(manifest)
	}

	filter := c.String("filter-cmd")

	if filter != "" {
		merged, err = filterManifest(filter, merged)
		check(err)
	}

	if cfs.Verbose {
		fmt.Printf("total %d files into %s\n", len(merged.Contents), mergeTo)
	}

	// マージしたマニフェストを書き込む
	storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
	check(err)

	client := &cfs.Client{
		Storage:  storage,
		Manifest: merged,
	}

	check(client.Init())

	check(client.UploadManifest())

	if output != "" {
		check(os.WriteFile(output, []byte(merged.Hash), 0777))
	}
	return nil
}

var catCommand = &cli.Command{
	Name:      "cat",
	Usage:     "fetch a data from url (for debug)",
	Action:    doCat,
	ArgsUsage: "location filename",
}

func doCat(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	var args = c.Args().Slice()
	if len(args) < 2 {
		fmt.Println("need 2 arguments")
		os.Exit(1)
	}

	location := args[0]
	filename := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	manifest, err := downloader.LoadManifest(location)
	check(err)

	content, ok := manifest.Contents[filename]
	if !ok {
		fmt.Println("file " + filename + " not found")
		os.Exit(1)
	}

	data, err := downloader.Fetch(content.Hash, content.Attr)
	check(err)

	fmt.Print(string(data))
	return nil
}

var lsCommand = &cli.Command{
	Name:      "ls",
	Usage:     "list files in manifest",
	Action:    doLs,
	ArgsUsage: "location",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "verify",
			Usage: "Check for files on the server.",
		},
	},
}

func doLs(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	var args = c.Args().Slice()
	if len(args) != 1 {
		fmt.Println("need just 1 arguments")
		os.Exit(1)
	}

	verify := c.Bool("verify")

	location := args[0]

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	manifest, err := downloader.LoadManifest(location)
	check(err)

	// To store the keys in slice in sorted order
	var keys []string
	for k := range manifest.Contents {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if verify {
		fileStatusList, err := downloader.ExistsAll(manifest)
		check(err)

		for _, k := range keys {
			file := manifest.Contents[k]
			status := "ng"
			if fileStatusList[k] {
				status = "ok"
			}
			fmt.Printf("%s\t%s\n", file.Path, status)
		}
	} else {
		for _, k := range keys {
			file := manifest.Contents[k]
			fmt.Printf("%s\t%s\t%d\t%s\n", file.Path, file.OrigHash, file.OrigSize, file.Time.Format(time.RFC3339))
		}
	}
	return nil
}

var configCommand = &cli.Command{
	Name:   "config",
	Usage:  "show current config",
	Action: doConfig,
}

func doConfig(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	fmt.Printf("Cabinet       : %s\n", cfs.Option.Cabinet)
	fmt.Printf("Downloader URL: %s\n", getDownloaderURL())
	return nil
}

var settingCommand = &cli.Command{
	Name:   "setting",
	Usage:  "show current setting",
	Action: doSetting,
}

func doSetting(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	fmt.Printf("HomeRoot       : %s\n", cfs.Setting.HomeRoot)
	return nil
}
