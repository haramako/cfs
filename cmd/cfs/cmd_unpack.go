package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/urfave/cli"
	"local.package/cfs"
	"local.package/cfs/pack"
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

	if len(args) != 1 {
		fmt.Println("need just 1 arguments")
		os.Exit(1)
	}

	packfile := args[0]

	f, err := os.Open(packfile)
	defer f.Close()
	check(err)

	pak, err := pack.Parse(f)
	check(err)

	pak, err = filterPackFile(filter, pak)
	check(err)

	if c.String("o") != "" {
		maxSize := 0
		for _, e := range pak.Entries {
			if e.Size > maxSize {
				maxSize = e.Size
			}
		}

		buf := make([]byte, maxSize)
		for _, e := range pak.Entries {
			if cfs.Verbose {
				fmt.Printf("%s\n", e.Path)
			}

			outdir := c.String("o")

			outPath := filepath.Join(outdir, e.Path)
			err := os.MkdirAll(filepath.Dir(outPath), 0777)
			check(err)

			_, err = f.Seek(int64(e.Pos), io.SeekStart)
			check(err)

			_, err = io.ReadFull(f, buf[:e.Size])
			check(err)

			err = ioutil.WriteFile(outPath, buf[:e.Size], 0777)
			check(err)
		}
	} else {
		for _, e := range pak.Entries {
			fmt.Printf("%s\t%d\t%s\n", e.Path, e.Size, e.Hash)
		}
	}

}
