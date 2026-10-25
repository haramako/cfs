package main

import (
	"context"
	"fmt"
	"time"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/urfave/cli/v3"
)

// キャビネットの管理（GC、タグ、キャビネットの設定、署名の鍵）のコマンド

// openCabinet は、設定のキャビネットの書き込み口と読み込み口を開く
func openCabinet(cfg *cfs.Config) (cfs.Storage, cfs.Fetcher, error) {
	storage, err := cfs.OpenStorage(cfg.Cabinet, cfg)
	if err != nil {
		return nil, nil, err
	}
	fetcher, err := cfs.OpenFetcher(cfg.Cabinet, cfg)
	if err != nil {
		return nil, nil, err
	}
	return storage, fetcher, nil
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
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	keys, err := cfg.KeyRing()
	if err != nil {
		return err
	}

	opt := cfs.GCOptions{DryRun: c.Bool("dry-run"), Keys: keys, Log: cfg.Logger()}
	if r := c.String("retention"); r != "" {
		if opt.Retention, err = cfs.ParseDuration(r); err != nil {
			return err
		}
	}

	storage, fetcher, err := openCabinet(cfg)
	if err != nil {
		return err
	}
	result, err := cfs.RunGC(storage, fetcher, opt)
	if err != nil {
		return err
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
	if cfg.Verbose || opt.DryRun {
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
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	storage, fetcher, err := openCabinet(cfg)
	if err != nil {
		return err
	}
	tags, err := cfs.ListTags(storage, fetcher)
	if err != nil {
		return err
	}
	for _, t := range tags {
		fmt.Printf("%s\t%s\t%s\n", t.Name, t.Hash, t.LastModified.Format(time.RFC3339))
	}
	return nil
}

func doTagRm(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	args, err := needArgs(c, 1, -1)
	if err != nil {
		return err
	}
	storage, _, err := openCabinet(cfg)
	if err != nil {
		return err
	}
	return cfs.DeleteTags(storage, args)
}

var cabinetCommand = &cli.Command{
	Name:  "cabinet",
	Usage: "manage cabinet settings",
	Commands: []*cli.Command{
		{
			Name:   "init",
			Usage:  "create the cabinet (S3 bucket or local folder) and write config.json",
			Action: doCabinetInit,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "region",
					Usage: "S3 region (default: S3Region in .cfsenv, AWS_REGION or profile, then ap-northeast-1)",
				},
				&cli.StringFlag{
					Name:  "retention",
					Usage: "GC retention period like 30d (default: keep the current setting, or 30d)",
				},
				&cli.BoolFlag{
					Name:  "dry-run",
					Usage: "show what would be done without doing it",
				},
			},
		},
		{
			Name:      "policy",
			Usage:     "print an IAM policy for an S3 cabinet (roles: upload, download, gc, admin)",
			ArgsUsage: "role",
			Action:    doCabinetPolicy,
		},
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
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	_, fetcher, err := openCabinet(cfg)
	if err != nil {
		return err
	}
	cabinetCfg, err := cfs.LoadCabinetConfig(fetcher)
	if err != nil {
		return err
	}
	retention, err := cabinetCfg.RetentionDuration()
	if err != nil {
		return err
	}
	fmt.Printf("retention: %v\n", retention)
	return nil
}

func doCabinetConfigSet(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	args, err := needArgs(c, 2, 2)
	if err != nil {
		return err
	}
	storage, fetcher, err := openCabinet(cfg)
	if err != nil {
		return err
	}
	cabinetCfg, err := cfs.LoadCabinetConfig(fetcher)
	if err != nil {
		return err
	}
	switch args[0] {
	case "retention":
		cabinetCfg.Retention = args[1]
	default:
		return fmt.Errorf("unknown cabinet config key %q", args[0])
	}
	return cfs.SaveCabinetConfig(storage, cabinetCfg)
}

func doCabinetInit(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	opt := cfs.InitOptions{
		Region:    c.String("region"),
		Retention: c.String("retention"),
		DryRun:    c.Bool("dry-run"),
	}
	if opt.Region == "" {
		opt.Region = cfg.S3Region
	}
	steps, err := cfs.InitCabinet(cfg.Cabinet, opt)
	if err != nil {
		return err
	}
	if opt.DryRun {
		fmt.Printf("would init cabinet %s:\n", cfg.Cabinet)
	} else {
		fmt.Printf("init cabinet %s:\n", cfg.Cabinet)
	}
	for _, s := range steps {
		fmt.Printf("  %s\n", s)
	}
	return nil
}

func doCabinetPolicy(_ context.Context, c *cli.Command) error {
	cfg, err := loadConfig(c)
	if err != nil {
		return err
	}
	args, err := needArgs(c, 1, 1)
	if err != nil {
		return err
	}
	policy, err := cfs.IAMPolicy(cfg.Cabinet, args[0])
	if err != nil {
		return err
	}
	fmt.Println(string(policy))
	return nil
}

var keygenCommand = &cli.Command{
	Name:   "keygen",
	Usage:  "generate a key pair for signing manifests",
	Action: doKeygen,
}

func doKeygen(_ context.Context, c *cli.Command) error {
	private, public, err := cfs.GenerateSigningKey()
	if err != nil {
		return err
	}
	fmt.Println("# Signing key: set to CFS_SIGNING_KEY on the uploading side. Keep it secret.")
	fmt.Printf("CFS_SIGNING_KEY=%s\n", private)
	fmt.Println("# Verify key: add to VerifyKeys in .cfsenv on the downloading side.")
	fmt.Printf("\"VerifyKeys\": [\"%s\"]\n", public)
	return nil
}
