package cfs

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	modified    map[string]time.Time
	deleted     []string
	lists       int
	headBuckets int
	puts        []fakeS3Put
	signRegions []string // 署名に使われたリージョン

	url               string   // 偽サーバーの URL
	bucketOwner       string   // CreateBucket の結果。"" ならまだない、"me" なら自分の、"other" なら他人の
	createBucket      []string // CreateBucket の本文
	publicAccessBlock string
	ownershipControls string
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
		q := r.URL.Query()
		prefix, delimiter := q.Get("prefix"), q.Get("delimiter")
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, prefix) && (delimiter == "" || !strings.Contains(k[len(prefix):], delimiter)) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		if n, err := strconv.Atoi(q.Get("max-keys")); err == nil && n < len(keys) {
			keys = keys[:n]
		}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>bkt</Name><KeyCount>%d</KeyCount>`, len(keys))
		for _, k := range keys {
			fmt.Fprintf(w, `<Contents><Key>%s</Key><Size>%d</Size><LastModified>%s</LastModified></Contents>`,
				k, len(f.objects[k]), f.modified[k].UTC().Format(time.RFC3339))
		}
		io.WriteString(w, `</ListBucketResult>`)
	case key == "" && r.Method == http.MethodPut && r.URL.Query().Has("publicAccessBlock"):
		body, _ := io.ReadAll(r.Body)
		f.publicAccessBlock = string(body)
	case key == "" && r.Method == http.MethodPut && r.URL.Query().Has("ownershipControls"):
		body, _ := io.ReadAll(r.Body)
		f.ownershipControls = string(body)
	case key == "" && r.Method == http.MethodPut:
		// CreateBucket
		if f.bucketOwner != "" {
			code := "BucketAlreadyOwnedByYou"
			if f.bucketOwner == "other" {
				code = "BucketAlreadyExists"
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>exists</Message></Error>`, code)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.createBucket = append(f.createBucket, string(body))
		f.bucketOwner = "me"
	case key == "" && r.Method == http.MethodPost && r.URL.Query().Has("delete"):
		var req struct {
			Objects []struct{ Key string } `xml:"Object"`
		}
		xml.NewDecoder(r.Body).Decode(&req)
		for _, o := range req.Objects {
			delete(f.objects, o.Key)
			f.deleted = append(f.deleted, o.Key)
		}
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><DeleteResult></DeleteResult>`)
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
		f.modified[key] = time.Now()
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

	fake := &fakeS3{region: region, objects: map[string][]byte{}, modified: map[string]time.Time{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	fake.url = server.URL

	s, err := newS3Storage("bkt", func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
	})
	if err != nil {
		t.Fatal(err)
	}
	return fake, s
}

func TestS3Put(t *testing.T) {
	fake, s := setupFakeS3(t, "")
	key := dataKey(SumHash([]byte("hoge")))

	if err := s.Put(key, []byte("hoge"), false); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 {
		t.Fatalf("expect 1 put but %d", len(fake.puts))
	}
	put := fake.puts[0]
	if put.Path != key || put.Body != "hoge" ||
		put.ACL != "bucket-owner-full-control" || put.ContentType != "binary/octet-stream" {
		t.Errorf("invalid put %+v", put)
	}

	// 既に存在するなら送信しない
	if err := s.Put(key, []byte("hoge"), false); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 {
		t.Errorf("uploaded existing object")
	}

	// 名前の前方が一致するだけのものは、既にあるとみなさない
	if err := s.Put(key[:len(key)-1], []byte("other"), false); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 2 {
		t.Errorf("prefix match must not be treated as existing")
	}

	// overwrite なら存在確認せずに送信する
	lists := fake.lists
	if err := s.Put(key, []byte("hoge"), true); err != nil {
		t.Fatal(err)
	}
	if fake.lists != lists || len(fake.puts) != 3 {
		t.Errorf("overwrite must put without list")
	}
}

func TestS3ListAndDelete(t *testing.T) {
	fake, s := setupFakeS3(t, "")
	for _, k := range []string{"tag/v1", "tag/v2", "data/aa/1", "data/bb/2"} {
		if err := s.Put(k, []byte(k), true); err != nil {
			t.Fatal(err)
		}
	}

	objects, err := s.List("tag/")
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 || objects[0].Key != "tag/v1" || objects[1].Key != "tag/v2" || objects[0].Size != 6 || objects[0].LastModified.IsZero() {
		t.Errorf("invalid list %+v", objects)
	}

	if err := s.Delete([]string{"data/aa/1", "data/nothing"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.objects["data/aa/1"]; ok {
		t.Errorf("data/aa/1 must be deleted")
	}
	if _, ok := fake.objects["data/bb/2"]; !ok {
		t.Errorf("data/bb/2 must not be deleted")
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
	if err := s.Put("tag/v1", []byte("x"), true); err != nil {
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
