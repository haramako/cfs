package main

import (
	"context"
	"fmt"
	"os"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/haramako/cfs/internal/pack"
	"github.com/urfave/cli/v3"
)

var packManifestCommand = &cli.Command{
	Name:      "pack-manifest",
	Usage:     "pack specified manifest",
	Action:    doPackManifest,
	ArgsUsage: "location packfile.cfspack",
}

func packFromManifest(b *cfs.Manifest, d *cfs.Downloader) (*pack.PackFile, error) {

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

func doPackManifest(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	var args = c.Args().Slice()
	if len(args) != 2 {
		fmt.Println("need just 2 arguments")
		os.Exit(1)
	}

	location := args[0]
	packfile := args[1]

	downloader, err := cfs.NewDownloader(getDownloaderURL())
	check(err)

	manifest, err := downloader.LoadManifest(location)
	check(err)

	filter := c.String("filter-cmd")

	if filter != "" {
		manifest, err = filterManifest(filter, manifest)
		check(err)
	}

	pak, err := packFromManifest(manifest, downloader)
	check(err)

	w, err := os.OpenFile(packfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	defer w.Close()
	check(err)

	err = pack.Pack(w, pak, nil)
	check(err)
	return nil
}
