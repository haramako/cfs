package main

import (
	"context"
	"fmt"
	"os"

	"github.com/haramako/cfs/internal/pack"
	"github.com/urfave/cli/v3"
)

var packCommand = &cli.Command{
	Name:      "pack",
	Usage:     "pack specified dir",
	Action:    doPack,
	ArgsUsage: "packfile.cfspack dir [...]",
}

func doPack(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	var args = c.Args().Slice()
	if len(args) != 2 {
		fmt.Println("need just 2 arguments")
		os.Exit(1)
	}

	filter := c.String("filter-cmd")

	packfile := args[0]

	dir := args[1]

	w, err := os.OpenFile(packfile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	defer w.Close()
	check(err)

	pak, err := pack.NewPackFileFromDir(dir)
	check(err)

	if filter != "" {
		pak, err = filterPackFile(filter, pak)
		check(err)
	}

	err = pack.Pack(w, pak, nil)
	check(err)
	return nil
}
