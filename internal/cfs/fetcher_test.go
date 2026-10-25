package cfs

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testFetcher(t *testing.T, f Fetcher) {
	data, err := fetchAll(f, "tag/v1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("invalid data %q", data)
	}

	if _, err := f.Open("tag/nothing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing data must be ErrNotFound but %v", err)
	}

	if ok, err := f.Exists("tag/v1"); !ok || err != nil {
		t.Errorf("tag/v1 must exist (%v, %v)", ok, err)
	}
	if ok, _ := f.Exists("tag/nothing"); ok {
		t.Errorf("tag/nothing must not exist")
	}
}

func setupFetcherDir(t *testing.T) string {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tag"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tag", "v1"), []byte("hello"), 0666); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFileFetcherFromPath(t *testing.T) {
	f, err := NewFetcher(setupFetcherDir(t))
	if err != nil {
		t.Fatal(err)
	}
	testFetcher(t, f)
}

func TestFileFetcherFromURL(t *testing.T) {
	f, err := NewFetcher(PathToFileURL(setupFetcherDir(t)).String())
	if err != nil {
		t.Fatal(err)
	}
	testFetcher(t, f)
}

func TestHttpFetcher(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(setupFetcherDir(t))))
	defer server.Close()

	// 末尾の "/" がなくても使えること
	f, err := NewFetcher(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	testFetcher(t, f)
}

func TestHttpFetcherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	f, err := NewFetcher(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.Open("tag/v1")
	if err == nil {
		r.Close()
		t.Fatal("403 must be an error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("403 must not be ErrNotFound")
	}
}

func TestFileStorageConcurrentPut(t *testing.T) {
	// 同じ中身を同時に置いても失敗しないこと（Windows では置き換えが競合する）
	s, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := dataKey(SumHash([]byte("same")))
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() { errs <- s.Put(key, []byte("same"), false) }()
	}
	for i := 0; i < 16; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent put failed: %v", err)
		}
	}
	if ok, _ := s.Exists(key); !ok {
		t.Errorf("data must exist")
	}
}

func TestFileStorageListAndDelete(t *testing.T) {
	s, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"data/ab/c1", "data/ab/c2", "data/ad/c3", "tag/v1", "config.json"} {
		if err := s.Put(key, []byte(key), false); err != nil {
			t.Fatal(err)
		}
	}

	keys := func(prefix string) []string {
		t.Helper()
		list, err := s.List(prefix)
		if err != nil {
			t.Fatal(err)
		}
		var r []string
		for _, o := range list {
			r = append(r, o.Key)
			if o.Size != int64(len(o.Key)) || o.LastModified.IsZero() {
				t.Errorf("invalid object info %+v", o)
			}
		}
		return r
	}
	// prefix はフォルダの途中で切れていてもよい
	if got := strings.Join(keys("data/a"), ","); got != "data/ab/c1,data/ab/c2,data/ad/c3" {
		t.Errorf("List(data/a) = %s", got)
	}
	if got := strings.Join(keys("data/ab/"), ","); got != "data/ab/c1,data/ab/c2" {
		t.Errorf("List(data/ab/) = %s", got)
	}
	if got := len(keys("")); got != 5 {
		t.Errorf("List() must return all objects but %d", got)
	}
	if got := keys("manifest/"); len(got) != 0 {
		t.Errorf("missing folder must be empty but %v", got)
	}

	// 無いものを消しても失敗しない
	if err := s.Delete([]string{"data/ab/c1", "tag/v1", "data/zz/none"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(keys(""), ","); got != "config.json,data/ab/c2,data/ad/c3" {
		t.Errorf("after delete = %s", got)
	}
}

func TestNewFetcherUnsupported(t *testing.T) {
	if _, err := NewFetcher("ftp://example.com/"); err == nil {
		t.Error("ftp must be unsupported")
	}
}
