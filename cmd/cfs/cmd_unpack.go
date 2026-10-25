package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/haramako/cfs/internal/pack"
	"github.com/urfave/cli/v3"
)

var unpackCommand = &cli.Command{
	Name:      "unpack",
	Usage:     "unpack specified pack file (list files without -o)",
	Action:    doUnpack,
	ArgsUsage: "packfile.cfspack",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Value:   "",
			Usage:   "output dir",
		},
	},
}

func doUnpack(_ context.Context, c *cli.Command) error {
	args, err := needArgs(c, 1, 1)
	if err != nil {
		return err
	}

	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer f.Close()

	pak, err := pack.Parse(f)
	if err != nil {
		return err
	}
	if pak, err = filterPackFile(c.String("filter-cmd"), pak); err != nil {
		return err
	}

	outdir := c.String("output")
	if outdir == "" {
		for _, e := range pak.Entries {
			fmt.Printf("%s\t%d\t%s\n", e.Path, e.Size, e.Hash)
		}
		return nil
	}

	verbose := c.Bool("verbose")
	for _, e := range pak.Entries {
		if verbose {
			fmt.Printf("%s\n", e.Path)
		}
		outPath := filepath.Join(outdir, filepath.FromSlash(e.Path))
		if err := os.MkdirAll(filepath.Dir(outPath), 0777); err != nil {
			return err
		}
		if err := extractEntry(f, e, outPath); err != nil {
			return err
		}
	}
	return nil
}

// extractEntry は、パックのファイルの 1 つを outPath に書き出す
func extractEntry(f *os.File, e pack.Entry, outPath string) error {
	w, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, io.NewSectionReader(f, int64(e.Pos), int64(e.Size))); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
