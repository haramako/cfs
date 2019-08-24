package main

import (
	"fmt"
	"os"

	"github.com/haramako/cfs/pack"
	"github.com/urfave/cli"
)

var unpackCommand = cli.Command{
	Name:      "unpack",
	Usage:     "unpack specified pack file",
	Action:    doUnpack,
	ArgsUsage: "packfile.cfspack -o output-dir",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output, o",
			Value: "",
			Usage: "output dir",
		},
	},
}

func doUnpack(c *cli.Context) {
	loadConfig(c)

	filter := c.GlobalString("filter-cmd")

	var args = c.Args()

	packfile := args[0]

	f, err := os.Open(packfile)
	check(err)

	pak, err := pack.Parse(f)
	check(err)

	if c.String("o") != "" {
		// TODO
		if len(args) != 2 {
			fmt.Println("need just 2 arguments")
			os.Exit(1)
		}
	} else {
		if len(args) != 1 {
			fmt.Println("need just 1 arguments")
			os.Exit(1)
		}

		entries := pak.Entries
		if filter != "" {
			files := make([]string, 0, len(entries))
			for _, e := range entries {
				files = append(files, e.Path)
			}
			files, err := runFilter(filter, files)
			check(err)

			fileDict := make(map[string]bool, len(entries))
			for _, f := range files {
				fileDict[f] = true
			}

			newEntries := make([]pack.Entry, 0, len(entries))
			for _, e := range entries {
				_, ok := fileDict[e.Path]
				if ok {
					newEntries = append(newEntries, e)
				}
			}
			entries = newEntries
		}

		for _, e := range entries {
			fmt.Printf("%s\t%d\t%s\n", e.Path, e.Size, e.Hash)
		}
	}

}
