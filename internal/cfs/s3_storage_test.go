package cfs

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3Put struct {
	Path        string
	ACL         string
	ContentType string
	Body        string
}

// fakeS3 は、cfs が使う S3 の操作だけを受け付ける S3 の偽物（パス形式の URL）
type fakeS3 struct {
	mutex       sync.Mutex
	region      string // HeadBucket で返すリージョン。空ならヘッダーを付けない
	objects     map[string][]byte
	lists       int
	headBuckets int
	puts        []fakeS3Put
	signRegions []string // 署名に使われたリージョン
}

var credentialRegion = regexp.MustCompile(`Credential=[^/]+/[0-9]+/([^/]+)/s3/`)

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if m := credentialRegion.FindStringSubmatch(r.Header.Get("Authorization")); m != nil {
		f.signRegions = append(f.signRegions, m[1])
	}

	key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/bkt"), "/")
	switch {
	case key == "" && r.Method == http.MethodHead:
		f.headBuckets++
		if f.region != "" {
			w.Header().Set("X-Amz-Bucket-Region", f.region)
		}
	case key == "" && r.Method == http.MethodGet:
		f.lists++
		prefix := r.URL.Query().Get("prefix")
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>bkt</Name>`)
		if _, ok := f.objects[prefix]; ok {
			io.WriteString(w, `<KeyCount>1</KeyCount><Contents><Key>`+prefix+`</Key></Contents>`)
		} else {
			io.WriteString(w, `<KeyCount>0</KeyCount>`)
		}
		io.WriteString(w, `</ListBucketResult>`)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		body, ok := f.objects[key]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			if r.Method == http.MethodGet {
				io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>not found</Message></Error>`)
			}
			return
		}
		if r.Method == http.MethodGet {
			w.Write(body)
		}
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body
		f.puts = append(f.puts, fakeS3Put{
			Path:        key,
			ACL:         r.Header.Get("X-Amz-Acl"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        string(body),
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func setupFakeS3(t *testing.T, region string) (*fakeS3, *S3Storage) {
	// 手元の AWS の設定に左右されないようにする
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "none"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "none"))

	fake := &fakeS3{region: region, objects: map[string][]byte{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	s, err := newS3Storage("bkt", func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
	})
	if err != nil {
		t.Fatal(err)
	}
	return fake, s
}

func TestS3Upload(t *testing.T) {
	fake, s := setupFakeS3(t, "")
	hash := "0123456789abcdef0123456789abcdef"

	if err := s.Upload("hoge", hash, []byte("hoge"), false); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 {
		t.Fatalf("expect 1 put but %d", len(fake.puts))
	}
	put := fake.puts[0]
	if put.Path != "data/01/23456789abcdef0123456789abcdef" || put.Body != "hoge" ||
		put.ACL != "bucket-owner-full-control" || put.ContentType != "binary/octet-stream" {
		t.Errorf("invalid put %+v", put)
	}

	// 既に存在するなら送信しない
	if err := s.Upload("hoge", hash, []byte("hoge"), false); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 {
		t.Errorf("uploaded existing object")
	}

	// overwrite なら存在確認せずに送信する
	lists := fake.lists
	if err := s.Upload("hoge", hash, []byte("hoge"), true); err != nil {
		t.Fatal(err)
	}
	if fake.lists != lists || len(fake.puts) != 2 {
		t.Errorf("overwrite must put without list")
	}
}

func TestS3UploadEmpty(t *testing.T) {
	fake, s := setupFakeS3(t, "")

	if err := s.Upload("empty", "d41d8cd98f00b204e9800998ecf8427e", []byte{}, false); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 0 {
		t.Errorf("empty file must not be uploaded")
	}
}

func TestS3UploadTag(t *testing.T) {
	fake, s := setupFakeS3(t, "")

	if err := s.UploadTag("v1", []byte("0123456789abcdef0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 || fake.puts[0].Path != "tag/v1" || fake.puts[0].Body != "0123456789abcdef0123456789abcdef" {
		t.Errorf("invalid put %+v", fake.puts)
	}
}

func TestS3Fetch(t *testing.T) {
	fake, s := setupFakeS3(t, "")
	fake.objects["tag/v1"] = []byte("hello")

	data, err := fetchAll(s, "tag/v1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("invalid data %q", data)
	}

	if _, err := s.Open("tag/nothing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing object must be ErrNotFound but %v", err)
	}

	if ok, err := s.Exists("tag/v1"); !ok || err != nil {
		t.Errorf("tag/v1 must exist (%v, %v)", ok, err)
	}
	if ok, err := s.Exists("tag/nothing"); ok || err != nil {
		t.Errorf("tag/nothing must not exist (%v, %v)", ok, err)
	}
}

func TestS3RegionFromBucket(t *testing.T) {
	fake, s := setupFakeS3(t, "us-west-2")

	if s.Region != "us-west-2" || fake.headBuckets != 1 {
		t.Fatalf("region must be detected from bucket but %q (head %d)", s.Region, fake.headBuckets)
	}

	// 判定したリージョンで署名すること
	fake.signRegions = nil
	if err := s.UploadTag("v1", []byte("x")); err != nil {
		t.Fatal(err)
	}
	for _, r := range fake.signRegions {
		if r != "us-west-2" {
			t.Errorf("request must be signed for us-west-2 but %s", r)
		}
	}
}

func TestS3RegionFromOption(t *testing.T) {
	saved := Option.S3Region
	defer func() { Option.S3Region = saved }()
	Option.S3Region = "eu-west-1"

	fake, s := setupFakeS3(t, "us-west-2")
	if s.Region != "eu-west-1" {
		t.Errorf("S3Region must take precedence but %q", s.Region)
	}
	if fake.headBuckets != 0 {
		t.Errorf("must not detect region when S3Region is set")
	}
}

func TestS3RegionFallback(t *testing.T) {
	// バケットからリージョンが分からなければ既定値
	_, s := setupFakeS3(t, "")
	if s.Region != DefaultS3Region {
		t.Errorf("region must fall back to %s but %q", DefaultS3Region, s.Region)
	}
}

func TestS3RegionFromEnv(t *testing.T) {
	// バケットからリージョンが分からなければ、AWS 標準の設定（AWS_REGION）を使う
	fake, _ := setupFakeS3(t, "")
	t.Setenv("AWS_REGION", "sa-east-1")
	server := httptest.NewServer(fake)
	defer server.Close()

	s, err := newS3Storage("bkt", func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Region != "sa-east-1" {
		t.Errorf("region must come from AWS_REGION but %q", s.Region)
	}
}

func TestS3RoundTrip(t *testing.T) {
	// S3 にアップロードしたものを、HTTP で公開せずに S3 から直接取得できること
	_, s := setupFakeS3(t, "")

	src := t.TempDir()
	addFile(src, "a.txt", "hello s3")
	addFile(src, "sub/b.raw", "raw data")
	addFile(src, "empty", "")

	manifest := NewManifest()
	manifest.Tag = "v1"
	client := &Client{Manifest: manifest, Storage: s}
	client.Init()
	if err := client.AddFiles(src); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(); err != nil {
		t.Fatal(err)
	}

	d := &Downloader{fetcher: s}
	loaded, err := d.LoadManifest("v1")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := d.Sync(loaded, out); err != nil {
		t.Fatal(err)
	}
	for path, expect := range map[string]string{"a.txt": "hello s3", "sub/b.raw": "raw data", "empty": ""} {
		data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(path)))
		if err != nil || string(data) != expect {
			t.Errorf("synced %s must be %q but %q (%v)", path, expect, data, err)
		}
	}
}

func TestS3FetcherFromURL(t *testing.T) {
	// s3:// の場所から Fetcher を作れること（実際の通信はしない）
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	saved := Option.S3Region
	defer func() { Option.S3Region = saved }()
	Option.S3Region = "ap-northeast-1"

	f, err := NewFetcher("s3://bkt")
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := f.(*S3Storage); !ok || s.BucketName != "bkt" {
		t.Errorf("s3:// must be fetched by S3Storage but %T", f)
	}
}
