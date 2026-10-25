package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haramako/cfs/internal/cfs"
)

// setupServe は、手元のフォルダのキャビネットにファイルを上げ、それを返すサーバーを作る
func setupServe(t *testing.T, files map[string]string) (*server, *httptest.Server, *cfs.Manifest) {
	t.Helper()
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	saved := *cfs.Option
	t.Cleanup(func() { *cfs.Option = saved })
	cfs.Option.EncryptKey = "serve-test-key-1234567890"

	cabinet := t.TempDir()
	storage, err := cfs.NewFileStorage(cabinet)
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	for p, content := range files {
		full := filepath.Join(src, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0777)
		if err := os.WriteFile(full, []byte(content), 0666); err != nil {
			t.Fatal(err)
		}
	}
	m := cfs.NewManifest()
	m.Tag = "v1"
	client := &cfs.Client{Manifest: m, Storage: storage}
	if err := client.Init(); err != nil {
		t.Fatal(err)
	}
	if err := client.AddFiles(src); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(); err != nil {
		t.Fatal(err)
	}

	d, err := cfs.NewDownloader(cabinet)
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(d, time.Minute)
	hs := httptest.NewServer(s)
	t.Cleanup(hs.Close)
	return s, hs, m
}

func get(t *testing.T, client *http.Client, url string, header map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func TestServeFile(t *testing.T) {
	_, hs, _ := setupServe(t, map[string]string{"docs/index.html": "<h1>hello</h1>", "docs/a.css": "body{}", "raw.bin": "binary", "report.htm": "日本語", "README.md": "# 見出し"})

	res, body := get(t, noRedirect, hs.URL+"/v1/docs/a.css", nil)
	if res.StatusCode != 200 || body != "body{}" || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/css") {
		t.Errorf("invalid css response %d %q %q", res.StatusCode, body, res.Header.Get("Content-Type"))
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("invalid headers %v", res.Header)
	}

	// index.html を返す。末尾の / がなければリダイレクト
	res, _ = get(t, noRedirect, hs.URL+"/v1/docs", nil)
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/v1/docs/" {
		t.Errorf("must redirect to /v1/docs/ but %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	res, body = get(t, noRedirect, hs.URL+"/v1/docs/", nil)
	if res.StatusCode != 200 || body != "<h1>hello</h1>" {
		t.Errorf("must serve index.html but %d %q", res.StatusCode, body)
	}

	// 拡張子が分からないものは octet-stream
	res, _ = get(t, noRedirect, hs.URL+"/v1/raw.bin", nil)
	if res.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("unknown type must be octet-stream but %q", res.Header.Get("Content-Type"))
	}

	// Markdown は、ブラウザで読めるように UTF-8 のテキストとして返す
	res, _ = get(t, noRedirect, hs.URL+"/v1/README.md", nil)
	if res.Header.Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Errorf(".md must be text/markdown but %q", res.Header.Get("Content-Type"))
	}

	// .htm は特別扱いしない（Shift_JIS にしない）
	res, _ = get(t, noRedirect, hs.URL+"/v1/report.htm", nil)
	if strings.Contains(strings.ToLower(res.Header.Get("Content-Type")), "shift_jis") {
		t.Errorf(".htm must not be Shift_JIS: %q", res.Header.Get("Content-Type"))
	}
}

func TestServeNotFound(t *testing.T) {
	_, hs, _ := setupServe(t, map[string]string{"a.txt": "a"})
	for _, p := range []string{"/v1/nothing.txt", "/nosuchtag/a.txt", "/", "/../config.json", "/bad%2Ftag/a.txt"} {
		res, _ := get(t, noRedirect, hs.URL+p, nil)
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s must be 404 but %d", p, res.StatusCode)
		}
	}
	req, _ := http.NewRequest(http.MethodPost, hs.URL+"/v1/a.txt", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST must be 405 but %d", res.StatusCode)
	}
}

func TestServeDirectory(t *testing.T) {
	_, hs, _ := setupServe(t, map[string]string{"dir/a&b.txt": "x", "dir/b.txt": "b"})
	res, body := get(t, noRedirect, hs.URL+"/v1/dir/", nil)
	if res.StatusCode != 200 {
		t.Fatalf("directory listing must be 200 but %d", res.StatusCode)
	}
	if !strings.Contains(body, "b.txt") || !strings.Contains(body, "a&amp;b.txt") || !strings.Contains(body, "<h1>v1/dir/</h1>") {
		t.Errorf("listing must contain escaped names: %s", body)
	}
}

func TestRenderDirectoryEscapesNames(t *testing.T) {
	// Windows ではファイル名に使えない文字も、マニフェストには書けるので確かめる
	rec := httptest.NewRecorder()
	err := renderDirectory(rec, "v1", "dir", []cfs.Content{{Path: `dir/<script>alert(1)</script>".txt`}})
	if err != nil {
		t.Fatal(err)
	}
	if body := rec.Body.String(); strings.Contains(body, "<script>") {
		t.Errorf("file names must be escaped: %s", body)
	}
}

func TestServeETagAndRange(t *testing.T) {
	_, hs, m := setupServe(t, map[string]string{"a.txt": "0123456789"})
	res, _ := get(t, noRedirect, hs.URL+"/v1/a.txt", nil)
	etag := res.Header.Get("ETag")
	if etag != `"`+m.Contents["a.txt"].OrigHash+`"` {
		t.Fatalf("ETag must be the content hash but %q", etag)
	}

	res, _ = get(t, noRedirect, hs.URL+"/v1/a.txt", map[string]string{"If-None-Match": etag})
	if res.StatusCode != http.StatusNotModified {
		t.Errorf("same ETag must be 304 but %d", res.StatusCode)
	}

	res, body := get(t, noRedirect, hs.URL+"/v1/a.txt", map[string]string{"Range": "bytes=2-4"})
	if res.StatusCode != http.StatusPartialContent || body != "234" {
		t.Errorf("range must be served but %d %q", res.StatusCode, body)
	}

	// マニフェストのハッシュで指定した URL は、変わらないのでずっとキャッシュできる
	res, body = get(t, noRedirect, hs.URL+"/"+m.Hash+"/a.txt", nil)
	if res.StatusCode != 200 || body != "0123456789" || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("hash location must be immutable: %d %q %q", res.StatusCode, body, res.Header.Get("Cache-Control"))
	}
}

func TestServeCachesTag(t *testing.T) {
	s, hs, _ := setupServe(t, map[string]string{"a.txt": "a"})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	get(t, noRedirect, hs.URL+"/v1/a.txt", nil)
	if _, ok := s.tags["v1"]; !ok {
		t.Fatalf("tag must be cached")
	}
	// 期限内は覚えたハッシュを使う。期限が切れたら引き直す
	s.tags["v1"] = tagEntry{hash: s.tags["v1"].hash, expires: now.Add(time.Second)}
	if h, _, err := s.resolve("v1"); err != nil || h != s.tags["v1"].hash {
		t.Errorf("cached tag must be used (%v)", err)
	}
	now = now.Add(2 * time.Second)
	s.tags["v1"] = tagEntry{hash: "stale", expires: now.Add(-time.Second)}
	if h, _, err := s.resolve("v1"); err != nil || h == "stale" {
		t.Errorf("expired tag must be looked up again (%q, %v)", h, err)
	}
}

func TestGetTagAndPath(t *testing.T) {
	cases := []struct{ host, path, tag, rest string }{
		{"localhost:8000", "/v1/docs/a.html", "v1", "/docs/a.html"},
		{"127.0.0.1:8000", "/v1/", "v1", "/"},
		{"report-1.example.com", "/docs/a.html", "report-1", "/docs/a.html"},
		{"cfs.example.com", "/v1/a.html", "v1", "/a.html"},
		{"localhost", "/", "", "/"},
	}
	for _, c := range cases {
		tag, rest := getTagAndPath(c.host, c.path)
		if tag != c.tag || rest != c.rest {
			t.Errorf("getTagAndPath(%q, %q) = %q, %q", c.host, c.path, tag, rest)
		}
	}
}

func TestLRUCache(t *testing.T) {
	c := newLRUCache(2)
	c.put("a", 1)
	c.put("b", 2)
	c.get("a")
	c.put("c", 3) // b が一番古い
	if _, ok := c.get("b"); ok {
		t.Errorf("b must be evicted")
	}
	if v, ok := c.get("a"); !ok || v != 1 {
		t.Errorf("a must remain")
	}
}
