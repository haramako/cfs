package main

import (
	"context"

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

// packFromManifest は、マニフェストのファイルを全部取得して、パックにする
func packFromManifest(m *cfs.Manifest, d *cfs.Downloader) (*pack.PackFile, error) {
	// 先に並列で手元のキャッシュに取得しておく
	if err := d.FetchAll(m); err != nil {
		return nil, err
	}
	entries := make([]pack.Entry, 0, len(m.Contents))
	for _, c := range m.Contents {
		data, err := d.Fetch(c.Hash, c.Attr)
		if err != nil {
			return nil, err
		}
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
	pak, err := packFromManifest(manifest, downloader)
	if err != nil {
		return err
	}
	return writePackFile(args[1], pak)
}
