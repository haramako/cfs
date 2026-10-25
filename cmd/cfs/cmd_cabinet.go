package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/urfave/cli/v3"
)

// キャビネットの管理（GC、タグ、キャビネットの設定、署名の鍵）のコマンド

func openCabinet() (cfs.Storage, cfs.Fetcher) {
	storage, err := cfs.StorageFromString(cfs.Option.Cabinet)
	check(err)
	fetcher, err := cfs.NewFetcher(cfs.Option.Cabinet)
	check(err)
	return storage, fetcher
}

var gcCommand = &cli.Command{
	Name:   "gc",
	Usage:  "delete data no longer used by any tag or recent manifest",
	Action: doGC,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be deleted without deleting",
		},
		&cli.StringFlag{
			Name:  "retention",
			Usage: "retention period like 30d (default: cabinet config, or 30d)",
		},
	},
}

func doGC(_ context.Context, c *cli.Command) error {
	loadConfig(c)

	opt := cfs.GCOptions{DryRun: c.Bool("dry-run")}
	if r := c.String("retention"); r != "" {
		d, err := cfs.ParseDuration(r)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		opt.Retention = d
	}

	storage, fetcher := openCabinet()
	result, err := cfs.RunGC(storage, fetcher, opt)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Printf("retention: %v\n", result.Retention)
	fmt.Printf("tags: %d, manifests: %d (live %d), data: %d\n", result.Tags, result.Manifests, result.LiveManifests, result.Data)
	fmt.Printf("candidates: %d manifests, %d data\n", len(result.CandidateManifests), len(result.CandidateData))
	if result.SkippedData {
		fmt.Println("some old manifests could not be read, data was not checked this time")
	}
	verb := "deleted"
	if opt.DryRun {
		verb = "would delete"
	}
	fmt.Printf("%s: %d (%d bytes)\n", verb, len(result.Deleted), result.DeletedBytes)
	if cfs.Verbose || opt.DryRun {
		for _, k := range result.Deleted {
			fmt.Printf("  %s\n", k)
		}
	}
	return nil
}

var tagCommand = &cli.Command{
	Name:  "tag",
	Usage: "manage tags",
	Commands: []*cli.Command{
		{
			Name:   "ls",
			Usage:  "list tags",
			Action: doTagLs,
		},
		{
			Name:      "rm",
			Usage:     "delete tags (manifests and data are deleted by later gc)",
			ArgsUsage: "tag [...]",
			Action:    doTagRm,
		},
	},
}

func doTagLs(_ context.Context, c *cli.Command) error {
	loadConfig(c)
	storage, fetcher := openCabinet()
	tags, err := cfs.ListTags(storage, fetcher)
	check(err)
	for _, t := range tags {
		fmt.Printf("%s\t%s\t%s\n", t.Name, t.Hash, t.LastModified.Format(time.RFC3339))
	}
	return nil
}

func doTagRm(_ context.Context, c *cli.Command) error {
	loadConfig(c)
	args := c.Args().Slice()
	if len(args) == 0 {
		fmt.Println("need at least 1 argument")
		os.Exit(1)
	}
	storage, _ := openCabinet()
	if err := cfs.DeleteTags(storage, args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}

var cabinetCommand = &cli.Command{
	Name:  "cabinet",
	Usage: "manage cabinet settings",
	Commands: []*cli.Command{
		{
			Name:   "config",
			Usage:  "show cabinet config",
			Action: doCabinetConfig,
			Commands: []*cli.Command{
				{
					Name:      "set",
					Usage:     "set cabinet config (keys: retention)",
					ArgsUsage: "key value",
					Action:    doCabinetConfigSet,
				},
			},
		},
	},
}

func doCabinetConfig(_ context.Context, c *cli.Command) error {
	loadConfig(c)
	_, fetcher := openCabinet()
	cfg, err := cfs.LoadCabinetConfig(fetcher)
	check(err)
	retention, err := cfg.RetentionDuration()
	check(err)
	fmt.Printf("retention: %v\n", retention)
	return nil
}

func doCabinetConfigSet(_ context.Context, c *cli.Command) error {
	loadConfig(c)
	args := c.Args().Slice()
	if len(args) != 2 {
		fmt.Println("need just 2 arguments")
		os.Exit(1)
	}
	storage, fetcher := openCabinet()
	cfg, err := cfs.LoadCabinetConfig(fetcher)
	check(err)
	switch args[0] {
	case "retention":
		cfg.Retention = args[1]
	default:
		fmt.Printf("unknown cabinet config key %q\n", args[0])
		os.Exit(1)
	}
	if err := cfs.SaveCabinetConfig(storage, cfg); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return nil
}

var keygenCommand = &cli.Command{
	Name:   "keygen",
	Usage:  "generate a key pair for signing manifests",
	Action: doKeygen,
}

func doKeygen(_ context.Context, c *cli.Command) error {
	private, public, err := cfs.GenerateSigningKey()
	check(err)
	fmt.Println("# Signing key: set to CFS_SIGNING_KEY on the uploading side. Keep it secret.")
	fmt.Printf("CFS_SIGNING_KEY=%s\n", private)
	fmt.Println("# Verify key: add to VerifyKeys in .cfsenv on the downloading side.")
	fmt.Printf("\"VerifyKeys\": [\"%s\"]\n", public)
	return nil
}
