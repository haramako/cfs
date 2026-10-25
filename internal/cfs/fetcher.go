package cfs

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound は、取得しようとしたものがないことを表す
var ErrNotFound = errors.New("not found")

// Fetcher は、キャビネットから "tag/v1" や "data/c5/9548..." のようなパスのデータを読む口
type Fetcher interface {
	// Open はデータを読み出す。ないときは ErrNotFound を包んだエラーを返す
	Open(path string) (io.ReadCloser, error)
	// Exists はデータがあるかを返す
	Exists(path string) (bool, error)
}

// NewFetcher は、場所の指定（URL か手元のパス）から Fetcher を作る
func NewFetcher(location string) (Fetcher, error) {
	if !IsURL(location) {
		abs, err := filepath.Abs(location)
		if err != nil {
			return nil, err
		}
		return &fileFetcher{root: abs}, nil
	}

	u, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https":
		// 相対パスで解決できるように、末尾を "/" にそろえる
		if !strings.HasSuffix(u.Path, "/") {
			u.Path += "/"
		}
		return &httpFetcher{base: u, client: &http.Client{}}, nil
	case "file":
		root, err := FileURLToPath(u)
		if err != nil {
			return nil, err
		}
		return &fileFetcher{root: root}, nil
	case "s3":
		// HTTP で公開しなくても、S3 から直接読む
		return NewS3Storage(u.Host)
	default:
		return nil, fmt.Errorf("unsupported download location %v", location)
	}
}

func fetchAll(f Fetcher, path string) ([]byte, error) {
	r, err := f.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// fileFetcher は手元のフォルダから読む
type fileFetcher struct {
	root string
}

func (f *fileFetcher) fullPath(path string) string {
	return filepath.Join(f.root, filepath.FromSlash(path))
}

func (f *fileFetcher) Open(path string) (io.ReadCloser, error) {
	r, err := os.Open(f.fullPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, f.fullPath(path))
	}
	return r, err
}

func (f *fileFetcher) Exists(path string) (bool, error) {
	_, err := os.Stat(f.fullPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// httpFetcher は HTTP(S) で読む
type httpFetcher struct {
	base   *url.URL
	client *http.Client
}

func (f *httpFetcher) Open(path string) (io.ReadCloser, error) {
	u, err := f.base.Parse(path)
	if err != nil {
		return nil, err
	}
	res, err := f.client.Get(u.String())
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		return nil, fmt.Errorf("%w: %v", ErrNotFound, u)
	}
	if res.StatusCode >= 400 {
		res.Body.Close()
		return nil, fmt.Errorf("bad response status code %d from %v", res.StatusCode, u)
	}
	return res.Body, nil
}

func (f *httpFetcher) Exists(path string) (bool, error) {
	u, err := f.base.Parse(path)
	if err != nil {
		return false, err
	}
	res, err := f.client.Head(u.String())
	if err != nil {
		return false, err
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK, nil
}
