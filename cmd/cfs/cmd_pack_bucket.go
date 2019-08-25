package main

import (
	"fmt"
	"os"

	"github.com/haramako/cfs"
	"github.com/haramako/cfs/pack"
	"github.com/urfave/cli"
)

var packBucketCommand = cli.Command{
	Name:      "pack-bucket",
	Usage:     "pack specified bucket",
	Action:    doPackBucket,
	ArgsUsage: "location packfile.cfspack",
}

func packFromBucket(b *cfs.Bucket, d *cfs.Downloader) (*pack.PackFile, error) {

	err := d.FetchAll(b)
	check(err)

	entries := make([]pack.Entry, 0, len(b.Contents))
	for _, c := range b.Contents {
		data, err := d.Fetch(c.Hash, c.Attr)
		check(err)

		entries = append(entries, pack.Entry{
			Path: c.Path,
			Hash: c.OrigHash,
			Size: c.OrigSize,
			Data: data,
		})
	}

	return pack.NewPackFile(entries), nil
}

func doPackBucket(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) != 2 {
		fmt.Println("need just 2 arguments")
		os.Exit(1)
	}

	location := args[0]
	packfile := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	bucket, err := downloader.LoadBucket(location)
	check(err)

	filter := c.GlobalString("filter-cmd")

	if filter != "" {
		bucket, err = filterBucket(filter, bucket)
		check(err)
	}

	pak, err := packFromBucket(bucket, downloader)
	check(err)

	w, err := os.OpenFile(packfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	defer w.Close()
	check(err)

	pack.Pack(w, pak, nil)
}
