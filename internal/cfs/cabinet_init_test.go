package cfs

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func fakeEndpoint(fake *fakeS3) func(*s3.Options) {
	return func(o *s3.Options) {
		o.BaseEndpoint = aws.String(fake.url)
		o.UsePathStyle = true
	}
}

func TestInitS3Cabinet(t *testing.T) {
	fake, _ := setupFakeS3(t, "")
	fake.createBucket = nil
	fake.bucketOwner = ""

	steps, err := initS3Cabinet("bkt", InitOptions{Region: "us-west-2", Retention: "14d"}, fakeEndpoint(fake))
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.createBucket) != 1 || !strings.Contains(fake.createBucket[0], "<LocationConstraint>us-west-2</LocationConstraint>") {
		t.Errorf("bucket must be created in us-west-2: %q", fake.createBucket)
	}
	for _, tag := range []string{"BlockPublicAcls", "IgnorePublicAcls", "BlockPublicPolicy", "RestrictPublicBuckets"} {
		if !strings.Contains(fake.publicAccessBlock, "<"+tag+">true</"+tag+">") {
			t.Errorf("%s must be true: %q", tag, fake.publicAccessBlock)
		}
	}
	if !strings.Contains(fake.ownershipControls, "BucketOwnerEnforced") {
		t.Errorf("ACLs must be disabled: %q", fake.ownershipControls)
	}

	cfg := &CabinetConfig{}
	if err := json.Unmarshal(fake.objects[cabinetConfigKey], cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Format != CabinetFormat || cfg.Retention != "14d" {
		t.Errorf("invalid config.json %+v", cfg)
	}
	if len(steps) != 4 {
		t.Errorf("steps must be reported: %v", steps)
	}

	// もう一度実行しても（自分のバケットがあっても）失敗せず、保持期間は残る
	if _, err := initS3Cabinet("bkt", InitOptions{Region: "us-west-2"}, fakeEndpoint(fake)); err != nil {
		t.Fatalf("init must be idempotent: %v", err)
	}
	json.Unmarshal(fake.objects[cabinetConfigKey], cfg)
	if cfg.Retention != "14d" {
		t.Errorf("existing retention must be kept but %q", cfg.Retention)
	}
}

func TestInitS3CabinetUSEast1(t *testing.T) {
	fake, _ := setupFakeS3(t, "")
	if _, err := initS3Cabinet("bkt", InitOptions{Region: "us-east-1"}, fakeEndpoint(fake)); err != nil {
		t.Fatal(err)
	}
	if len(fake.createBucket) != 1 || strings.Contains(fake.createBucket[0], "LocationConstraint") {
		t.Errorf("us-east-1 must not have LocationConstraint: %q", fake.createBucket)
	}
}

func TestInitS3CabinetOthersBucket(t *testing.T) {
	fake, _ := setupFakeS3(t, "")
	fake.bucketOwner = "other"
	if _, err := initS3Cabinet("bkt", InitOptions{Region: "ap-northeast-1"}, fakeEndpoint(fake)); err == nil || !strings.Contains(err.Error(), "someone else") {
		t.Errorf("bucket owned by others must be an error, but %v", err)
	}
	if fake.publicAccessBlock != "" {
		t.Errorf("must not change others' bucket")
	}
}

func TestInitS3CabinetDryRun(t *testing.T) {
	fake, _ := setupFakeS3(t, "")
	steps, err := initS3Cabinet("bkt", InitOptions{Region: "ap-northeast-1", DryRun: true}, fakeEndpoint(fake))
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.createBucket) != 0 || fake.publicAccessBlock != "" || len(fake.puts) != 0 {
		t.Errorf("dry run must not send requests")
	}
	if len(steps) != 4 {
		t.Errorf("dry run must report steps: %v", steps)
	}
}

func TestInitFileCabinet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "cabinet")
	if _, err := InitCabinet(dir, InitOptions{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCabinetConfig(&fileFetcher{root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Format != CabinetFormat || cfg.Retention != "30d" {
		t.Errorf("invalid config %+v", cfg)
	}

	// dry-run では何も作らない
	other := filepath.Join(t.TempDir(), "other")
	if _, err := InitCabinet(PathToFileURL(other).String(), InitOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := (&fileFetcher{root: other}).Exists(cabinetConfigKey); ok {
		t.Errorf("dry run must not write config")
	}
}

func TestCabinetFormatCheck(t *testing.T) {
	s, _ := NewDummyStorage("")
	uploadToDummy(t, s, "v1", map[string]string{"a.txt": "a"})
	s.Put(cabinetConfigKey, []byte(`{"format": 3}`), true)

	d := &Downloader{fetcher: s}
	if _, err := d.LoadManifest("v1"); err == nil || !strings.Contains(err.Error(), "unsupported cabinet format") {
		t.Errorf("newer cabinet format must be rejected, but %v", err)
	}
	if _, err := RunGC(s, s, GCOptions{DryRun: true}); err == nil {
		t.Errorf("GC must reject newer cabinet format")
	}
	if _, err := initCabinetConfig(s, s, InitOptions{}); err == nil {
		t.Errorf("init must not overwrite newer cabinet format")
	}

	// 版が書かれていない（init を使わずに作った）キャビネットは読める
	s.Put(cabinetConfigKey, []byte(`{"retention": "1d"}`), true)
	if _, err := (&Downloader{fetcher: s}).LoadManifest("v1"); err != nil {
		t.Errorf("cabinet without format must be readable: %v", err)
	}
}

func TestIAMPolicy(t *testing.T) {
	for role, expect := range map[string][]string{
		"download": {"s3:GetObject"},
		"upload":   {"s3:ListBucket", "s3:PutObject", "s3:PutObjectAcl", "s3:GetObject"},
		"gc":       {"s3:DeleteObject", "s3:ListBucket"},
		"admin":    {"s3:CreateBucket", "s3:PutBucketPublicAccessBlock", "s3:PutBucketOwnershipControls"},
	} {
		data, err := IAMPolicy("s3://my-bucket", role)
		if err != nil {
			t.Fatal(err)
		}
		var policy struct {
			Version   string
			Statement []struct {
				Effect   string
				Action   []string
				Resource string
			}
		}
		if err := json.Unmarshal(data, &policy); err != nil {
			t.Fatal(err)
		}
		all := ""
		for _, s := range policy.Statement {
			all += strings.Join(s.Action, ",") + " " + s.Resource + "\n"
		}
		for _, a := range expect {
			if !strings.Contains(all, a) {
				t.Errorf("%s policy must contain %s: %s", role, a, all)
			}
		}
		if !strings.Contains(all, "arn:aws:s3:::my-bucket") {
			t.Errorf("%s policy must target the bucket: %s", role, all)
		}
		if role == "download" && strings.Contains(all, "PutObject") {
			t.Errorf("download policy must be read only")
		}
	}

	if _, err := IAMPolicy("./local", "upload"); err == nil {
		t.Errorf("local cabinet must be rejected")
	}
	if _, err := IAMPolicy("s3://b", "root"); err == nil {
		t.Errorf("unknown role must be rejected")
	}
}
