package main

import (
	"context"
	"os"

	"github.com/haramako/cfs/internal/pack"
	"github.com/urfave/cli/v3"
)

var packCommand = &cli.Command{
	Name:      "pack",
	Usage:     "pack specified dir",
	Action:    doPack,
	ArgsUsage: "packfile.cfspack dir",
}

func doPack(_ context.Context, c *cli.Command) error {
	args, err := needArgs(c, 2, 2)
	if err != nil {
		return err
	}

	pak, err := pack.NewPackFileFromDir(args[1])
	if err != nil {
		return err
	}
	if pak, err = filterPackFile(c.String("filter-cmd"), pak); err != nil {
		return err
	}
	return writePackFile(args[0], pak)
}

// writePackFile は、パックをファイルに書く
func writePackFile(path string, pak *pack.PackFile) error {
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := pack.Pack(w, pak, nil); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
