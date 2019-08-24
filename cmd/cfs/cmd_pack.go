package main

import (
	"fmt"
	"os"

	"github.com/haramako/cfs/pack"
	"github.com/urfave/cli"
)

var packCommand = cli.Command{
	Name:      "pack",
	Usage:     "pack specified dir",
	Action:    doPack,
	ArgsUsage: "packfile.cfspack dir [...]",
}

func doPack(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) != 2 {
		fmt.Println("need just 2 arguments")
		os.Exit(1)
	}

	filter := c.GlobalString("filter")
	_ = filter

	packfile := args[0]
	_ = packfile

	dir := args[1]

	w, err := os.OpenFile(packfile, os.O_CREATE|os.O_WRONLY, 0777)
	defer w.Close()
	check(err)

	pak, err := pack.NewPackFileFromDir(dir)
	check(err)

	pack.Pack(w, pak, nil)
}
