// コンテントファイルシステム
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/urfave/cli/v3"
)

var revision string

// エントリーポイント

func main() {
	if err := newApp().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// newApp は、cfs のコマンド全体を作る
func newApp() *cli.Command {
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
	return app
}

// loadConfig は、.cfsenv と環境変数に、コマンドラインの指定を重ねた設定を作る
func loadConfig(c *cli.Command) (*cfs.Config, error) {
	cfg, err := cfs.LoadConfig(c.String("config"))
	if err != nil {
		return nil, err
	}
	cfg.Verbose = c.Bool("verbose")

	// コマンドラインで指定された相対パスは、今のフォルダを基準にする
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if s := c.String("cabinet"); s != "" {
		if cfg.Cabinet, err = cfs.ResolveLocation(s, cwd); err != nil {
			return nil, err
		}
	}
	if s := c.String("url"); s != "" {
		if cfg.Url, err = cfs.ResolveLocation(s, cwd); err != nil {
			return nil, err
		}
	}

	cfg.Logger().Printf("config: %s\n", cfg)
	return cfg, nil
}

// needArgs は、引数の数が min 以上 max 以下（max が負なら上限なし）かを確かめる
func needArgs(c *cli.Command, min, max int) ([]string, error) {
	args := c.Args().Slice()
	if len(args) < min || (max >= 0 && len(args) > max) {
		return nil, fmt.Errorf("usage: cfs %s %s", c.Name, c.ArgsUsage)
	}
	return args, nil
}

// openDownloader は、設定のダウンロード元から読む Downloader を作る
func openDownloader(cfg *cfs.Config) (*cfs.Downloader, error) {
	return cfs.NewDownloader(cfg.DownloadLocation(), cfg)
}

// loadManifest は、ダウンロード元からマニフェストを読み、--filter-cmd があれば絞り込む
func loadManifest(c *cli.Command, d *cfs.Downloader, location string) (*cfs.Manifest, error) {
	manifest, err := d.LoadManifest(location)
	if err != nil {
		return nil, err
	}
	return filterManifest(c.String("filter-cmd"), manifest)
}

var uploadCommand = &cli.Command{
	Name:      "upload",
	Usage:     "upload files to cabinet",
	Action:    doUpload,
	ArgsUsage: "[dir ...]",
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

// uploadCachePath は、前回アップロードしたマニフェストを置く手元のファイルを返す。
// キャビネット・今のフォルダ・上げるフォルダの組ごとに、別のファイルにする
func uploadCachePath(cfg *cfs.Config, dirs []string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, s := range append([]string{cfg.Cabinet, cwd}, dirs...) {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	cacheDir, err := cfg.ManifestCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, hex.EncodeToString(h.Sum(nil))), nil
}

func doUpload(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	log := cfg.Logger()

	args := c.Args().Slice()
	if len(args) == 0 {
		args = []string{"."}
	}

	tag := c.String("tag")
	if tag != "" {
		if err := cfs.ValidateTagName(tag); err != nil {
			return err
		}
	}

	manifestPath := c.String("manifest")
	if manifestPath == "" {
		absDirs := make([]string, 0, len(args))
		for _, dir := range args {
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			absDirs = append(absDirs, abs)
		}
		if manifestPath, err = uploadCachePath(cfg, absDirs); err != nil {
			return err
		}
	}

	storage, err := cfs.OpenStorage(cfg.Cabinet, cfg)
	if err != nil {
		return err
	}

	// 形式の版が違うキャビネットには書かない
	cabinetFetcher, err := cfs.OpenFetcher(cfg.Cabinet, cfg)
	if err != nil {
		return err
	}
	if err := cfs.CheckCabinetFormat(cabinetFetcher); err != nil {
		return err
	}

	var manifest *cfs.Manifest
	if !c.Bool("force") {
		if manifest, err = cfs.ManifestFromFile(manifestPath, log); err != nil {
			return err
		}
		// 前回のマニフェストが GC で消えていたら、中身も消えているかもしれないので、キャッシュを使わない
		ok, err := manifest.ValidateCache(storage)
		if err != nil {
			return err
		}
		if !ok {
			log.Printf("previous manifest %s is not in the cabinet, check all files\n", manifest.Hash)
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
		Config:   cfg,
	}
	if err := client.Init(); err != nil {
		return err
	}
	for _, path := range args {
		if err := client.AddFiles(path); err != nil {
			return err
		}
	}

	for _, path := range manifest.RemoveUntouched() {
		log.Printf("removed: %s\n", path)
	}
	if err := client.Finish(); err != nil {
		return err
	}

	if output := c.String("output"); output != "" {
		return os.WriteFile(output, []byte(manifest.Hash), 0666)
	}
	return nil
}

var syncCommand = &cli.Command{
	Name:      "sync",
	Usage:     "sync from cabinet",
	Action:    doSync,
	ArgsUsage: "location output-dir",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "delete",
			Usage: "delete files in output-dir that are not in the manifest",
		},
	},
}

func doSync(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	args, err := needArgs(c, 2, 2)
	if err != nil {
		return err
	}

	downloader, err := openDownloader(cfg)
	if err != nil {
		return err
	}
	manifest, err := loadManifest(c, downloader, args[0])
	if err != nil {
		return err
	}
	if err := downloader.Sync(manifest, args[1]); err != nil {
		return err
	}
	// 書き出しが全部うまくいってから、一覧にないものを消す
	if c.Bool("delete") {
		if _, err := downloader.RemoveUnlisted(manifest, args[1]); err != nil {
			return err
		}
	}
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
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	log := cfg.Logger()
	args, err := needArgs(c, 2, -1)
	if err != nil {
		return err
	}

	mergeTo := args[0]
	if err := cfs.ValidateTagName(mergeTo); err != nil {
		return err
	}

	downloader, err := openDownloader(cfg)
	if err != nil {
		return err
	}

	merged := cfs.NewManifest()
	merged.Tag = mergeTo
	for _, location := range args[1:] {
		manifest, err := downloader.LoadManifest(location)
		if err != nil {
			return err
		}
		log.Printf("%d files merged from %s\n", len(manifest.Contents), location)
		merged.Merge(manifest)
	}

	if merged, err = filterManifest(c.String("filter-cmd"), merged); err != nil {
		return err
	}
	log.Printf("total %d files into %s\n", len(merged.Contents), mergeTo)

	// マージしたマニフェストを書き込む
	storage, err := cfs.OpenStorage(cfg.Cabinet, cfg)
	if err != nil {
		return err
	}
	client := &cfs.Client{
		Storage:  storage,
		Manifest: merged,
		Config:   cfg,
	}
	if err := client.Init(); err != nil {
		return err
	}
	if err := client.Finish(); err != nil {
		return err
	}

	if output := c.String("output"); output != "" {
		return os.WriteFile(output, []byte(merged.Hash), 0666)
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
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	args, err := needArgs(c, 2, 2)
	if err != nil {
		return err
	}

	downloader, err := openDownloader(cfg)
	if err != nil {
		return err
	}
	manifest, err := downloader.LoadManifest(args[0])
	if err != nil {
		return err
	}
	content, ok := manifest.Contents[args[1]]
	if !ok {
		return fmt.Errorf("file %s not found in %s", args[1], args[0])
	}
	data, err := downloader.Fetch(content.Hash, content.Attr)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
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
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	args, err := needArgs(c, 1, 1)
	if err != nil {
		return err
	}

	downloader, err := openDownloader(cfg)
	if err != nil {
		return err
	}
	manifest, err := downloader.LoadManifest(args[0])
	if err != nil {
		return err
	}

	var paths []string
	for path := range manifest.Contents {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	if c.Bool("verify") {
		exists, err := downloader.ExistsAll(manifest)
		if err != nil {
			return err
		}
		missing := 0
		for _, path := range paths {
			status := "ok"
			if !exists[path] {
				status = "ng"
				missing++
			}
			fmt.Printf("%s\t%s\n", path, status)
		}
		if missing > 0 {
			return fmt.Errorf("%d files are missing in the cabinet", missing)
		}
		return nil
	}

	for _, path := range paths {
		file := manifest.Contents[path]
		fmt.Printf("%s\t%s\t%d\t%s\n", file.Path, file.OrigHash, file.OrigSize, file.Time.Format(time.RFC3339))
	}
	return nil
}

var configCommand = &cli.Command{
	Name:   "config",
	Usage:  "show current config",
	Action: doConfig,
}

func doConfig(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	cacheDir, err := cfg.DataCacheDir()
	if err != nil {
		return err
	}
	fmt.Printf("Cabinet       : %s\n", cfg.Cabinet)
	fmt.Printf("Downloader URL: %s\n", cfg.DownloadLocation())
	fmt.Printf("Cache         : %s\n", filepath.Dir(cacheDir))
	return nil
}
