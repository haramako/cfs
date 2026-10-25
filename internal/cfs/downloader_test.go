package cfs

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// testConfig は、手元のキャッシュを一時フォルダにした設定を返す
func testConfig(t *testing.T) *Config {
	cfg := DefaultConfig()
	cfg.CacheDir = t.TempDir()
	return cfg
}

// openTestDownloader は、f から読む Downloader を作る
func openTestDownloader(t *testing.T, f Fetcher, cfg *Config) *Downloader {
	t.Helper()
	if cfg == nil {
		cfg = testConfig(t)
	}
	d, err := newDownloader(f, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// newTestDownloader は、DummyStorage に上げたものを読む Downloader を作る
func newTestDownloader(t *testing.T, files map[string]string) (*DummyStorage, *Downloader, *Manifest) {
	t.Helper()
	s, _ := NewDummyStorage("")
	uploadTo(t, s, "v1", files)
	d := openTestDownloader(t, s, nil)
	m, err := d.LoadManifest("v1")
	if err != nil {
		t.Fatal(err)
	}
	return s, d, m
}

func TestDownloaderSync(t *testing.T) {
	files := map[string]string{"a.txt": "sync a", "dir/sub/b.bin": "sync b", "empty.txt": "", "raw.raw": "sync raw"}
	_, d, m := newTestDownloader(t, files)

	out := t.TempDir()
	if err := d.Sync(m, out); err != nil {
		t.Fatal(err)
	}
	for path, expect := range files {
		data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("cannot read synced %s: %v", path, err)
			continue
		}
		if string(data) != expect {
			t.Errorf("%s = %q, want %q", path, data, expect)
		}
	}
}

func TestDownloaderSyncFailsOnMissingData(t *testing.T) {
	s, d, m := newTestDownloader(t, map[string]string{"a.txt": "sync missing"})
	s.Delete([]string{dataKey(m.Contents["a.txt"].Hash)})

	if err := d.Sync(m, t.TempDir()); err == nil {
		t.Errorf("sync must fail when data is missing")
	}
}

func TestDownloaderExistsAll(t *testing.T) {
	s, d, m := newTestDownloader(t, map[string]string{"a.txt": "exists a", "b.txt": "exists b", "empty.txt": ""})
	s.Delete([]string{dataKey(m.Contents["b.txt"].Hash)})

	result, err := d.ExistsAll(m)
	if err != nil {
		t.Fatal(err)
	}
	// 0 バイトのデータは置かれないが、あるものとして扱う
	expect := map[string]bool{"a.txt": true, "b.txt": false, "empty.txt": true}
	for path, ok := range expect {
		if result[path] != ok {
			t.Errorf("ExistsAll[%s] = %v, want %v", path, result[path], ok)
		}
	}
}

func TestDownloaderFetchAll(t *testing.T) {
	s, d, m := newTestDownloader(t, map[string]string{"a.txt": "fetch all a", "b.txt": "fetch all b", "empty.txt": ""})
	if err := d.FetchAll(m); err != nil {
		t.Fatal(err)
	}
	// 取得したものは手元のキャッシュに入る
	for _, path := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(d.cacheDir, m.Contents[path].Hash)); err != nil {
			t.Errorf("%s must be cached: %v", path, err)
		}
	}

	// キャッシュになく、キャビネットにもなければ、やり直したうえで失敗する
	missing := uploadTo(t, s, "v2", map[string]string{"c.txt": "fetch all missing"})
	s.Delete([]string{dataKey(missing.Contents["c.txt"].Hash)})
	if err := d.FetchAll(missing); err == nil {
		t.Errorf("fetch all must fail when data is missing")
	}
}

func TestManifestMergeAndRemoveUntouched(t *testing.T) {
	m := NewManifest()
	m.Contents["a"] = Content{Path: "a", Hash: "old-a", Touched: true}
	m.Contents["b"] = Content{Path: "b", Hash: "old-b"}
	from := NewManifest()
	from.Contents["b"] = Content{Path: "b", Hash: "new-b", Touched: true}
	from.Contents["c"] = Content{Path: "c", Hash: "new-c"}

	// 両方にあるものは from で上書きする
	m.Merge(from)
	if len(m.Contents) != 3 || m.Contents["a"].Hash != "old-a" || m.Contents["b"].Hash != "new-b" || m.Contents["c"].Hash != "new-c" {
		t.Fatalf("invalid merge result %+v", m.Contents)
	}

	if removed := m.RemoveUntouched(); len(removed) != 1 || removed[0] != "c" {
		t.Errorf("removed must be [c] but %v", removed)
	}
	if len(m.Contents) != 2 || !m.Contents["a"].Touched || !m.Contents["b"].Touched {
		t.Errorf("only touched contents must remain: %+v", m.Contents)
	}
}

func TestManifestFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.ToSlash(filepath.Join(dir, "v1"))

	// ファイルがなければ空
	m, err := ManifestFromFile(path, nil)
	if err != nil || len(m.Contents) != 0 || m.Path != path {
		t.Fatalf("missing file must give an empty manifest: %+v, %v", m, err)
	}

	// 読めないもの（古い形式など）も空として扱う
	os.WriteFile(filepath.Join(dir, "v1"), []byte("old format"), 0666)
	if m, err := ManifestFromFile(path, nil); err != nil || len(m.Contents) != 0 {
		t.Errorf("unreadable cache must be ignored: %+v, %v", m, err)
	}

	// 正しいものと、そのハッシュを読む
	src := NewManifest()
	src.Contents["a.txt"] = Content{Path: "a.txt", Hash: SumHash([]byte("a")), OrigHash: SumHash([]byte("a")), Size: 1, OrigSize: 1}
	body := src.Dump(nil)
	os.WriteFile(filepath.Join(dir, "v1"), body, 0666)
	os.WriteFile(filepath.Join(dir, "v1.hash"), []byte(SumHash(body)+"\n"), 0666)
	m, err = ManifestFromFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Contents["a.txt"]; !ok || m.Hash != SumHash(body) {
		t.Errorf("cache must be loaded with its hash: %+v", m)
	}
}

func TestEmptyDataIsUploaded(t *testing.T) {
	// 圧縮も暗号化もしない空のファイルは、データも 0 バイトになる。それもキャビネットに置く
	s, d, m := newTestDownloader(t, map[string]string{"empty.raw": "", "a.txt": "a"})
	c := m.Contents["empty.raw"]
	if c.Size != 0 || c.Hash != SumHash(nil) {
		t.Fatalf("empty raw file must have 0-byte data: %+v", c)
	}
	if ok, _ := s.Exists(dataKey(c.Hash)); !ok {
		t.Errorf("0-byte data must be uploaded")
	}
	if exists, _ := d.ExistsAll(m); !exists["empty.raw"] {
		t.Errorf("ExistsAll must find 0-byte data")
	}

	// ls --verify では、ほかと同じく消えていれば ng になる
	s.Delete([]string{dataKey(c.Hash)})
	if exists, _ := d.ExistsAll(m); exists["empty.raw"] {
		t.Errorf("ExistsAll must report missing 0-byte data")
	}

	// 取得するときは、中身が 0 バイトなのはハッシュから分かるので、キャビネットになくても空のファイルを作れる
	out := t.TempDir()
	if err := d.Sync(m, out); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(out, "empty.raw")); err != nil || len(data) != 0 {
		t.Errorf("empty file must be synced: %q, %v", data, err)
	}
}

func TestRemoveUnlisted(t *testing.T) {
	_, d, m := newTestDownloader(t, map[string]string{"a.txt": "a", "sub/b.txt": "b"})
	out := t.TempDir()
	for p, content := range map[string]string{"extra.txt": "x", "sub/extra.txt": "x", "old/deep/x.txt": "x", "sub/b.txt": "old b"} {
		full := filepath.Join(out, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0777)
		os.WriteFile(full, []byte(content), 0666)
	}
	os.MkdirAll(filepath.Join(out, "empty"), 0777)

	if err := d.Sync(m, out); err != nil {
		t.Fatal(err)
	}
	removed, err := d.RemoveUnlisted(m, out)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	if got := strings.Join(removed, ","); got != "empty/,extra.txt,old/,old/deep/,old/deep/x.txt,sub/extra.txt" {
		t.Errorf("removed = %s", got)
	}

	var left []string
	filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if p != out {
			rel, _ := filepath.Rel(out, p)
			left = append(left, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(left)
	if got := strings.Join(left, ","); got != "a.txt,sub,sub/b.txt" {
		t.Errorf("only listed files must remain: %s", got)
	}
}

func TestRemoveUnlistedIgnoresCaseOnWindowsAndMac(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("file names are case sensitive")
	}
	_, d, m := newTestDownloader(t, map[string]string{"a.txt": "a"})
	out := t.TempDir()
	// 大文字の名前で置いてあるファイルに書き出すと、名前は大文字のまま残る。それを一覧にないものとして消さない
	os.WriteFile(filepath.Join(out, "A.txt"), []byte("old"), 0666)
	if err := d.Sync(m, out); err != nil {
		t.Fatal(err)
	}
	if removed, err := d.RemoveUnlisted(m, out); err != nil || len(removed) != 0 {
		t.Errorf("file differing only in case must not be removed: %v, %v", removed, err)
	}
}
