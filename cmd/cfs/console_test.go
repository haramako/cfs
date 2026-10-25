package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haramako/cfs/internal/cfs"
)

// setupConsole は、手元のフォルダのキャビネットにタグを 2 つ上げ、それを見る管理画面を作る
func setupConsole(t *testing.T) (*httptest.Server, map[string]*cfs.Manifest, cfs.Storage) {
	t.Helper()
	cfg := cfs.DefaultConfig()
	cfg.EncryptKey = "console-test-key-1234567890"
	cfg.NoEncrypt = []string{"*.ab"}
	cfg.CacheDir = t.TempDir()
	cfg.Cabinet = t.TempDir()
	if _, err := cfs.InitCabinet(cfg.Cabinet, cfs.InitOptions{}); err != nil {
		t.Fatal(err)
	}
	storage, err := cfs.OpenStorage(cfg.Cabinet, cfg)
	if err != nil {
		t.Fatal(err)
	}

	manifests := map[string]*cfs.Manifest{}
	for tag, files := range map[string]map[string]string{
		"v1":  {"a.txt": "hello", "sub/b.ab": "bundle"},
		"dev": {"<script>.txt": "x"},
	} {
		m := cfs.NewManifest()
		m.Tag = tag
		client := &cfs.Client{Manifest: m, Storage: storage, Config: cfg}
		if err := client.Init(); err != nil {
			t.Fatal(err)
		}
		for p, content := range files {
			if _, err := client.AddContent(p, []byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := client.Finish(); err != nil {
			t.Fatal(err)
		}
		manifests[tag] = m
	}

	con, err := newConsole(cfg)
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(con)
	t.Cleanup(hs.Close)
	return hs, manifests, storage
}

func TestConsoleIndex(t *testing.T) {
	hs, manifests, _ := setupConsole(t)
	res, body := get(t, noRedirect, hs.URL+"/", nil)
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("index must be html 200 but %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	for _, want := range []string{`href="/tag/v1"`, `href="/tag/dev"`, `href="/manifest/` + manifests["v1"].Hash + `"`, "Encrypt key: set"} {
		if !strings.Contains(body, want) {
			t.Errorf("index must contain %q:\n%s", want, body)
		}
	}
	// タグは名前の順
	if strings.Index(body, "/tag/dev") > strings.Index(body, "/tag/v1") {
		t.Errorf("tags must be sorted by name")
	}
}

func TestConsoleManifest(t *testing.T) {
	hs, manifests, _ := setupConsole(t)
	for _, url := range []string{hs.URL + "/tag/v1", hs.URL + "/manifest/" + manifests["v1"].Hash} {
		res, body := get(t, noRedirect, url, nil)
		if res.StatusCode != 200 {
			t.Fatalf("%s must be 200 but %d", url, res.StatusCode)
		}
		// a.txt は圧縮と暗号化、sub/b.ab は NoEncrypt なので圧縮だけ
		for _, want := range []string{"a.txt", "sub/b.ab", `<span class="badge">zlib</span> <span class="badge">enc</span>`, `<td><span class="badge">zlib</span> </td>`, "2 files"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s must contain %q:\n%s", url, want, body)
			}
		}
	}

	// ファイルの中身は、復号して返す
	res, body := get(t, noRedirect, hs.URL+"/files/"+manifests["v1"].Hash+"/a.txt", nil)
	if res.StatusCode != 200 || body != "hello" {
		t.Errorf("file must be served decrypted: %d %q", res.StatusCode, body)
	}

	// ファイル名はエスケープする
	_, body = get(t, noRedirect, hs.URL+"/tag/dev", nil)
	if strings.Contains(body, "<script>.txt") || !strings.Contains(body, "&lt;script&gt;.txt") {
		t.Errorf("file names must be escaped:\n%s", body)
	}
}

func TestConsoleNotFoundAndBrokenManifest(t *testing.T) {
	hs, _, storage := setupConsole(t)
	if res, _ := get(t, noRedirect, hs.URL+"/tag/none", nil); res.StatusCode != 404 {
		t.Errorf("unknown tag must be 404 but %d", res.StatusCode)
	}
	if res, _ := get(t, noRedirect, hs.URL+"/manifest/short", nil); res.StatusCode != 404 {
		t.Errorf("invalid hash must be 404 but %d", res.StatusCode)
	}

	// 読めないマニフェストを指すタグがあっても、一覧は出して、その行にエラーを出す
	broken := cfs.SumHash([]byte("broken"))
	storage.Put("manifest/"+broken[:2]+"/"+broken[2:], []byte("broken"), true)
	storage.Put("tag/broken", []byte(broken), true)
	res, body := get(t, noRedirect, hs.URL+"/", nil)
	if res.StatusCode != 200 || !strings.Contains(body, `class="err"`) || !strings.Contains(body, "/tag/v1") {
		t.Errorf("index must show the broken tag with an error: %d\n%s", res.StatusCode, body)
	}
}

func TestConsoleRejectsOtherHost(t *testing.T) {
	hs, _, _ := setupConsole(t)
	// DNS リバインディングで、ほかの名前から来たものは断る
	req, _ := http.NewRequest(http.MethodGet, hs.URL+"/", nil)
	req.Host = "evil.example.com"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("other host must be 403 but %d", res.StatusCode)
	}
	for host, ok := range map[string]bool{"localhost:8001": true, "127.0.0.1": true, "[::1]:8001": true, "evil.example.com:8001": false, "127.0.0.1.evil.com": false} {
		if isLocalHost(host) != ok {
			t.Errorf("isLocalHost(%q) must be %v", host, ok)
		}
	}
}

func TestCheckLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{"127.0.0.1:8001": true, "localhost:8001": true, "[::1]:8001": true, "0.0.0.0:8001": false, ":8001": false, "192.168.0.1:8001": false} {
		if err := checkLoopback(addr); (err == nil) != ok {
			t.Errorf("checkLoopback(%q) = %v, want ok=%v", addr, err, ok)
		}
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 1536: "1.5 KiB", 5 * 1024 * 1024: "5.0 MiB"} {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", n, got, want)
		}
	}
}
