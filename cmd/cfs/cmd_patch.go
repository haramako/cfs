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

func doPatch(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	var args = c.Args().Slice()
	if len(args) != 3 {
		fmt.Println("need just 3 arguments")
		os.Exit(1)
	}

	basepath := args[0]
	currentpath := args[1]
	packfile := args[2]

	// Read base pack
	basefile, err := os.Open(basepath)
	check(err)
	defer basefile.Close()

	basepack, err := pack.Parse(basefile)
	check(err)

	// Read current pack
	currentfile, err := os.Open(currentpath)
	check(err)
	defer currentfile.Close()

	currentpack, err := pack.Parse(currentfile)
	check(err)

	// Calculate diff
	patch, err := pack.Patch(basepack, currentpack)
	check(err)

	// Make patch pack
	w, err := os.OpenFile(packfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	defer w.Close()
	check(err)

	err = pack.Pack(w, patch, nil)
	check(err)
	return nil
}
