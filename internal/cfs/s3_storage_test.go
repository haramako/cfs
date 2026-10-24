package cfs

import (
	"io"
	"net/http"
	"net/http/httptest"
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

// fakeS3 は ListObjectsV2 と PutObject だけを受け付ける S3 の偽物
type fakeS3 struct {
	mutex   sync.Mutex
	objects map[string]bool
	lists   int
	puts    []fakeS3Put
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	switch r.Method {
	case http.MethodGet:
		f.lists++
		prefix := r.URL.Query().Get("prefix")
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><Name>bkt</Name>`)
		if f.objects[prefix] {
			io.WriteString(w, `<KeyCount>1</KeyCount><Contents><Key>`+prefix+`</Key></Contents>`)
		} else {
			io.WriteString(w, `<KeyCount>0</KeyCount>`)
		}
		io.WriteString(w, `</ListBucketResult>`)
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		path := strings.TrimPrefix(r.URL.Path, "/bkt/")
		f.objects[path] = true
		f.puts = append(f.puts, fakeS3Put{
			Path:        path,
			ACL:         r.Header.Get("X-Amz-Acl"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        string(body),
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func setupFakeS3(t *testing.T) (*fakeS3, *S3Storage) {
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	fake := &fakeS3{objects: map[string]bool{}}
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
	fake, s := setupFakeS3(t)
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
	fake, s := setupFakeS3(t)

	if err := s.Upload("empty", "d41d8cd98f00b204e9800998ecf8427e", []byte{}, false); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 0 {
		t.Errorf("empty file must not be uploaded")
	}
}

func TestS3UploadTag(t *testing.T) {
	fake, s := setupFakeS3(t)

	if err := s.UploadTag("v1", []byte("0123456789abcdef0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 || fake.puts[0].Path != "tag/v1" || fake.puts[0].Body != "0123456789abcdef0123456789abcdef" {
		t.Errorf("invalid put %+v", fake.puts)
	}
}

func TestS3DownloaderUrl(t *testing.T) {
	_, s := setupFakeS3(t)

	if s.DownloaderUrl().String() != "http://bkt.s3-website-ap-northeast-1.amazonaws.com/" {
		t.Errorf("invalid url %v", s.DownloaderUrl())
	}
}
