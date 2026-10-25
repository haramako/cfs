package cfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// InitOptions は、キャビネットを作るときの設定
type InitOptions struct {
	Region    string // S3 のリージョン。空なら S3Region → AWS 標準の設定 → 既定値
	Retention string // GC の保持期間。空なら既存の設定（なければ既定値）のまま
	DryRun    bool   // 何もせず、何をするかだけ返す
}

// InitCabinet は、キャビネット（S3 のバケットか手元のフォルダ）を作り、config.json を置く。
// 既にあれば作らずに設定だけを確かめるので、何度実行してもよい。行ったこと（DryRun ならすること）の説明を返す
func InitCabinet(cabinet string, opt InitOptions) ([]string, error) {
	if IsURL(cabinet) {
		u, err := url.Parse(cabinet)
		if err != nil {
			return nil, err
		}
		switch u.Scheme {
		case "s3":
			return initS3Cabinet(u.Host, opt)
		case "file":
			path, err := FileURLToPath(u)
			if err != nil {
				return nil, err
			}
			return initFileCabinet(path, opt)
		default:
			return nil, fmt.Errorf("cannot init cabinet %v", cabinet)
		}
	}
	return initFileCabinet(cabinet, opt)
}

func initFileCabinet(path string, opt InitOptions) ([]string, error) {
	s, err := NewFileStorage(path)
	if err != nil {
		return nil, err
	}
	steps := []string{"create folder " + s.CabinetPath}
	if !opt.DryRun {
		if err := os.MkdirAll(s.CabinetPath, 0777); err != nil {
			return nil, err
		}
	}
	step, err := initCabinetConfig(s, &fileFetcher{root: s.CabinetPath}, opt)
	if err != nil {
		return nil, err
	}
	return append(steps, step), nil
}

// initCabinetConfig は、config.json に形式の版と保持期間を書く（既存の設定は残す）
func initCabinetConfig(s Storage, f Fetcher, opt InitOptions) (string, error) {
	cfg, err := LoadCabinetConfig(f)
	if err != nil {
		return "", err
	}
	if err := cfg.CheckFormat(); err != nil {
		return "", err
	}
	cfg.Format = CabinetFormat
	if opt.Retention != "" {
		cfg.Retention = opt.Retention
	}
	if cfg.Retention == "" {
		cfg.Retention = "30d"
	}
	if _, err := cfg.RetentionDuration(); err != nil {
		return "", err
	}
	step := fmt.Sprintf("write %s (format %d, retention %s)", cabinetConfigKey, cfg.Format, cfg.Retention)
	if opt.DryRun {
		return step, nil
	}
	return step, SaveCabinetConfig(s, cfg)
}

func initS3Cabinet(bucket string, opt InitOptions, optFns ...func(*s3.Options)) ([]string, error) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	)
	if err != nil {
		return nil, err
	}

	region := opt.Region
	if region == "" {
		region = Option.S3Region
	}
	if region == "" {
		region = cfg.Region
	}
	if region == "" {
		region = DefaultS3Region
	}
	client := s3.NewFromConfig(cfg, append([]func(*s3.Options){func(o *s3.Options) { o.Region = region }}, optFns...)...)

	steps := []string{
		fmt.Sprintf("create S3 bucket %s in %s (skip if it is already yours)", bucket, region),
		"block all public access",
		"disable ACLs (object ownership: bucket owner enforced)",
	}
	if opt.DryRun {
		step, err := initCabinetConfig(nil, &dryRunFetcher{}, opt)
		if err != nil {
			return nil, err
		}
		return append(steps, step), nil
	}

	// 1. バケットを作る（自分のバケットが既にあれば続ける）
	input := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}
	if _, err := client.CreateBucket(ctx, input); err != nil {
		var owned *types.BucketAlreadyOwnedByYou
		var exists *types.BucketAlreadyExists
		switch {
		case errors.As(err, &owned):
			steps[0] = fmt.Sprintf("S3 bucket %s already exists (yours), keep it", bucket)
		case errors.As(err, &exists):
			return nil, fmt.Errorf("bucket name %s is already used by someone else, choose another name", bucket)
		default:
			return nil, fmt.Errorf("cannot create bucket %s: %w", bucket, err)
		}
	}

	// 2. 非公開にする（cfs は S3 から直接読むので、公開は不要）
	if _, err := client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucket),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	}); err != nil {
		return nil, fmt.Errorf("cannot block public access of %s: %w", bucket, err)
	}

	// 3. ACL を無効にする（cfs が付ける bucket-owner-full-control は ACL 無効でも受け付けられる）
	if _, err := client.PutBucketOwnershipControls(ctx, &s3.PutBucketOwnershipControlsInput{
		Bucket: aws.String(bucket),
		OwnershipControls: &types.OwnershipControls{
			Rules: []types.OwnershipControlsRule{{ObjectOwnership: types.ObjectOwnershipBucketOwnerEnforced}},
		},
	}); err != nil {
		return nil, fmt.Errorf("cannot disable ACLs of %s: %w", bucket, err)
	}

	// 4. config.json を置く
	s := &S3Storage{BucketName: bucket, Region: region, client: client}
	step, err := initCabinetConfig(s, s, opt)
	if err != nil {
		return nil, err
	}
	return append(steps, step), nil
}

// dryRunFetcher は、まだないキャビネットを dry-run するときに使う、何も持たない Fetcher
type dryRunFetcher struct{}

func (dryRunFetcher) Open(path string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
}

func (dryRunFetcher) Exists(path string) (bool, error) { return false, nil }

// IAMPolicy は、用途（upload / download / gc / admin）に必要な IAM ポリシーの JSON を返す
func IAMPolicy(cabinet string, role string) ([]byte, error) {
	u, err := url.Parse(cabinet)
	if err != nil || u.Scheme != "s3" || u.Host == "" {
		return nil, fmt.Errorf("IAM policy is only for s3:// cabinets: %v", cabinet)
	}
	bucketArn := "arn:aws:s3:::" + u.Host
	objectArn := bucketArn + "/*"

	type statement struct {
		Effect   string
		Action   []string
		Resource string
	}
	var statements []statement
	switch role {
	case "download":
		statements = []statement{
			{"Allow", []string{"s3:GetObject"}, objectArn},
		}
	case "upload":
		statements = []statement{
			{"Allow", []string{"s3:ListBucket"}, bucketArn},
			{"Allow", []string{"s3:GetObject", "s3:PutObject", "s3:PutObjectAcl"}, objectArn},
		}
	case "gc":
		statements = []statement{
			{"Allow", []string{"s3:ListBucket"}, bucketArn},
			{"Allow", []string{"s3:GetObject", "s3:PutObject", "s3:PutObjectAcl", "s3:DeleteObject"}, objectArn},
		}
	case "admin":
		statements = []statement{
			{"Allow", []string{"s3:CreateBucket", "s3:PutBucketPublicAccessBlock", "s3:PutBucketOwnershipControls", "s3:ListBucket"}, bucketArn},
			{"Allow", []string{"s3:GetObject", "s3:PutObject", "s3:PutObjectAcl"}, objectArn},
		}
	default:
		return nil, fmt.Errorf("unknown role %q (upload, download, gc, admin)", role)
	}

	policy := struct {
		Version   string
		Statement []statement
	}{Version: "2012-10-17", Statement: statements}
	return json.MarshalIndent(policy, "", "  ")
}
