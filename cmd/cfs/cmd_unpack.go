package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/haramako/cfs/internal/pack"
	"github.com/urfave/cli/v3"
)

var unpackCommand = &cli.Command{
	Name:      "unpack",
	Usage:     "unpack specified pack file (list files without -o). files deleted by a patch are removed from the output dir",
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
		for _, p := range pak.Deleted {
			fmt.Printf("%s\tdeleted\n", p)
		}
		return nil
	}

	verbose := c.Bool("verbose")
	for _, e := range pak.Entries {
		if verbose {
			fmt.Printf("%s\n", e.Path)
		}
		outPath, err := outputPath(outdir, e.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0777); err != nil {
			return err
		}
		if err := extractEntry(f, e, outPath); err != nil {
			return err
		}
	}
	// パッチで消えたファイルを消す（もうないものは無視する）
	for _, p := range pak.Deleted {
		if verbose {
			fmt.Printf("delete %s\n", p)
		}
		outPath, err := outputPath(outdir, p)
		if err != nil {
			return err
		}
		if err := os.Remove(outPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// outputPath は、パックの中のパスを、出力先のフォルダの中のパスにする。
// フォルダの外を指すパス（".." や絶対パス）はエラーにする
func outputPath(outdir string, p string) (string, error) {
	local := filepath.FromSlash(p)
	if p == "" || !filepath.IsLocal(local) {
		return "", fmt.Errorf("invalid path in pack file: %q", p)
	}
	return filepath.Join(outdir, local), nil
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
