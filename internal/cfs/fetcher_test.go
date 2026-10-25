package cfs

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestNewFetcherUnsupported(t *testing.T) {
	if _, err := NewFetcher("ftp://example.com/"); err == nil {
		t.Error("ftp must be unsupported")
	}
}
