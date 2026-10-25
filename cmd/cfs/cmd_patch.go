package main

import (
	"context"
	"fmt"
	"os"

	"github.com/haramako/cfs/internal/pack"
	"github.com/urfave/cli/v3"
)

var patchCommand = &cli.Command{
	Name:      "patch",
	Usage:     "make patch package",
	Action:    doPatch,
	ArgsUsage: "base.tp current.tp output.tp",
}

// readPackFile は、パックのファイルを読む（中身も読む）
func readPackFile(path string) (*pack.PackFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return pack.Parse(f)
}

func doPatch(_ context.Context, c *cli.Command) error {
	args, err := needArgs(c, 3, 3)
	if err != nil {
		return err
	}

	base, err := readPackFile(args[0])
	if err != nil {
		return err
	}
	current, err := readPackFile(args[1])
	if err != nil {
		return err
	}

	patch, err := pack.Patch(base, current)
	if err != nil {
		return err
	}
	if c.Bool("verbose") {
		for _, e := range patch.Entries {
			fmt.Printf("changed %s\n", e.Path)
		}
	}
	return writePackFile(args[2], patch)
}
