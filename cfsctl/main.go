// コンテントファイルシステム
package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/haramako/cfs"
	"net/url"
	"os"
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
		FetchCommand,
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
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
			Value: ".bucket",
			Usage: "bucket file name",
		},
	},
}

func loadConfig(c *cli.Context) {
	cfs.Verbose = c.GlobalBool("V")
	cfs.LoadDefaultOptions(c.GlobalString("config"))
	if c.GlobalString("cabinet") != "" {
		cfs.Option.Cabinet = c.GlobalString("cabinet")
	}
}

func createStorage() cfs.Storage {
	if cfs.Option.GcsBucket != "" {
		return &cfs.GcsStorage{
			BucketName: cfs.Option.GcsBucket,
		}
	} else {
		uri, _ := url.Parse(cfs.Option.Cabinet)
		return &cfs.CfsStorage{
			CabinetUrl: uri,
		}
	}
}

func doUpload(c *cli.Context) {
	loadConfig(c)

	bucket, err := cfs.BucketFromFile(c.String("bucket"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bucket.Tag = c.String("tag")

	args := c.Args()
	if len(args) == 0 {
		args = []string{"."}
	}

	storage := createStorage()

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
	ArgsUsage: "base-url location output-dir",
	Flags:     []cli.Flag{},
}

func doSync(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) < 3 {
		panic("need 3 arguments")
	}

	baseUrl := args[0]
	location := args[1]
	dir := args[2]

	bucket, err := cfs.BucketFromUrl(baseUrl, location)
	if err != nil {
		panic(err)
	}

	err = bucket.Sync(dir)
	if err != nil {
		panic(err)
	}

}

var FetchCommand = cli.Command{
	Name:      "fetch",
	Usage:     "fetch a data from url (for debug)",
	Action:    doFetch,
	ArgsUsage: "base-url location",
	Flags:     []cli.Flag{},
}

func doFetch(c *cli.Context) {
	loadConfig(c)
	/*
		cfs.Verbose = c.GlobalBool("V")

		var args = c.Args()
		if len(args) < 2 {
			panic("need 2 arguments")
		}

		url := args[0]
		location := args[1]

		bucket, err := cfs.BucketFromUrl(url)
		if err != nil {
			panic(err)
		}

		data, err := bucket.Fetch(location, cfs.DefaultContentAttribute())
		if err != nil {
			panic(err)
		}

		print(string(data))
	*/
}
