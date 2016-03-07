// コンテントファイルシステム
package main

import (
	"github.com/codegangsta/cli"
	"github.com/haramako/cfs"
	"os"
)

// エントリーポイント

func main() {

	cfs.LoadDefaultOptions()

	app := cli.NewApp()
	app.Name = "cfs"
	app.HelpName = "cfs"
	app.Usage = "cfs hoge fuga"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "V",
			Usage: "verbose",
		},
	}
	app.Commands = []cli.Command{
		UploadCommand,
		SyncCommand,
		FetchCommand,
	}

	app.Run(os.Args)
}

var UploadCommand = cli.Command{
	Name:   "upload",
	Usage:  "upload files to cabinet",
	Action: doUpload,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "tag",
			Value: "",
			Usage: "specify tag name",
		},
	},
}

func doUpload(c *cli.Context) {
	cfs.Verbose = c.GlobalBool("V")

	u, err := cfs.CreateUploader("") // TODO
	if err != nil {
		panic(err)
	}

	bucket, err := cfs.BucketFromFile("", u)
	if err != nil {
		panic(err)
	}
	bucket.Tag = c.String("tag")
	println(bucket.Tag)

	args := c.Args()
	if len(args) == 0 {
		args = []string{"."}
	}

	for _, path := range args {
		bucket.AddFiles(path)
	}

	bucket.RemoveUntouched()
	err = bucket.Finish()
	if err != nil {
		panic(err)
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
	cfs.Verbose = c.GlobalBool("V")

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
