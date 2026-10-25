package cfs

import (
	"fmt"
	"os"

	"path/filepath"

	"testing"
	"time"
)

var Printf = fmt.Printf // avoid error
var timeCount = int64(0)

// mainTestConfig は、このファイルの結合テストで使う設定
var mainTestConfig *Config

func TestMain(m *testing.M) {
	cacheDir, err := os.MkdirTemp("", "cfs-cache")
	if err != nil {
		panic(err)
	}
	mainTestConfig = DefaultConfig()
	mainTestConfig.EncryptKey = "12345678901234567890123456789012"
	mainTestConfig.CacheDir = cacheDir
	mainTestConfig.Verbose = os.Getenv("CFS_TEST_VERBOSE") != ""

	code := m.Run()
	os.RemoveAll(cacheDir)
	os.Exit(code)
}

func setupManifest() (*Client, *Manifest, string) {

	dir, err := os.MkdirTemp("", "cfs-test")
	dir = filepath.FromSlash(dir)
	if err != nil {
		panic(err)
	}

	manifest, err := ManifestFromFile(filepath.FromSlash(dir+"/.manifest"), nil)
	if err != nil {
		panic(err)
	}

	storage := newStorage(manifest)

	client := &Client{
		Manifest: manifest,
		Storage:  storage,
		Config:   mainTestConfig,
	}
	client.Init()

	return client, manifest, dir
}

// testCabinet は、最後に newStorage で作ったキャビネットの場所
var testCabinet string

func newStorage(manifest *Manifest) Storage {
	var uri string
	switch os.Getenv("CFS_TEST_STORAGE") {
	case "s3":
		// 本物の S3 で試すときのバケット名は、リポジトリに含めない（.cfs-test.env などから環境変数で渡す）
		bucket := os.Getenv("CFS_TEST_S3_BUCKET")
		if bucket == "" {
			panic("CFS_TEST_STORAGE=s3 needs CFS_TEST_S3_BUCKET (see docs/development.md)")
		}
		uri = "s3://" + bucket
	default:
		tempDir, err := os.MkdirTemp("", "cfs-file")
		if err != nil {
			panic(err)
		}
		uri = "file:///" + tempDir + "/"
	}

	storage, err := OpenStorage(uri, mainTestConfig)
	if err != nil {
		panic(err)
	}
	testCabinet = uri
	return storage
}

func setupManifestWithFiles() (*Client, *Manifest, string) {
	c, b, dir := setupManifest()
	addFile(dir, "hoge", "hoge")
	addFile(dir, "fuga", "fuga")
	addFile(dir, "piyo/piyo", "piyo")

	c.AddFiles(dir)

	b.UploadCount = 0
	return c, b, dir
}

func setupManifestFromCabinet(cabinet string, location string) (*Downloader, *Manifest) {
	downloader, err := NewDownloader(cabinet, mainTestConfig)
	if err != nil {
		panic(err)
	}

	manifest, err := downloader.LoadManifest(location)
	if err != nil {
		panic(err)
	}
	return downloader, manifest
}

func assertContents(t *testing.T, b *Manifest, n int) {
	if len(b.Contents) != n {
		t.Errorf("b.Contents must be %v but %v", n, len(b.Contents))
	}
}

func assertUploadCount(t *testing.T, b *Manifest, n int) {
	if b.UploadCount != n {
		// t.Errorf("upload count must be %v but %v", n, b.Uploader.UploadCount)
	}
}

func addFile(dir, file, content string) {
	fullpath := filepath.Join(filepath.FromSlash(dir), filepath.FromSlash(file))
	fulldir, _ := filepath.Split(fullpath)
	err := os.MkdirAll(fulldir, 0777)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(fullpath, []byte(content), 0666)
	if err != nil {
		panic(err)
	}
	t := time.Now().Add(time.Duration(int64(time.Second) * timeCount))
	timeCount++
	err = os.Chtimes(fullpath, t, t)
	if err != nil {
		panic(err)
	}
}

func TestNewManifest(t *testing.T) {
	c, b, dir := setupManifest()

	addFile(dir, "hoge", "fuga")
	c.AddFiles(dir)

	assertContents(t, b, 1)
	assertUploadCount(t, b, 1)

	addFile(dir, "fuga", "piyo")
	c.AddFiles(dir)

	assertContents(t, b, 2)
	assertUploadCount(t, b, 2)

	c.Finish()
	assertUploadCount(t, b, 3)
}

func TestOverwriteFile(t *testing.T) {
	c, b, dir := setupManifestWithFiles()

	addFile(dir, "hoge", "piyo")
	c.AddFiles(dir)

	assertContents(t, b, 3)
	assertUploadCount(t, b, 0)

	addFile(dir, "hoge", "fuga2")
	c.AddFiles(dir)

	assertUploadCount(t, b, 1)

	addFile(dir, "hoge", "fuga3")
	c.AddFiles(dir)

	assertUploadCount(t, b, 2)

	addFile(dir, "hoge", "fuga2")
	c.AddFiles(dir)

	assertUploadCount(t, b, 2)

	addFile(dir, filepath.FromSlash("piyo/piyo"), "piyo2")
	c.AddFiles(dir)

	assertUploadCount(t, b, 3)

	addFile(dir, filepath.FromSlash("piyo/piyo"), "piyo2")
	c.AddFiles(dir)

	assertContents(t, b, 3)
	assertUploadCount(t, b, 3)
}

// アップロードしたものを、キャビネットから取得して元に戻せること
func TestCompress(t *testing.T) {
	c, b, dir := setupManifest()

	addFile(dir, "hoge", "piyo")
	addFile(dir, "fuga.raw", "hage")
	addFile(dir, "piyo/piyo", "piyopiyo")
	c.AddFiles(dir)

	err := c.Finish()
	if err != nil {
		t.Errorf("cannot finish manifest")
		t.Error(err)
		return
	}

	d, b2 := setupManifestFromCabinet(testCabinet, b.Hash)
	if b2 == nil {
		return // なにもしない
	}

	temp, err := os.MkdirTemp("", "cfs-sync")
	if err != nil {
		panic(err)
	}

	err = d.Sync(b2, temp)
	if err != nil {
		t.Error(err)
		return
	}

	for path, expect := range map[string]string{"hoge": "piyo", "fuga.raw": "hage", "piyo/piyo": "piyopiyo"} {
		data, err := os.ReadFile(filepath.Join(temp, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("cannot read synced %s: %v", path, err)
			continue
		}
		if string(data) != expect {
			t.Errorf("synced %s must be %q but %q", path, expect, data)
		}
	}
	if b2.Contents["fuga.raw"].Attr != ContentAttribute(0) || b2.Contents["hoge"].Attr == ContentAttribute(0) {
		t.Errorf("invalid attributes %+v", b2.Contents)
	}
}

func TestRawFile(t *testing.T) {
	c, b, dir := setupManifest()

	addFile(dir, "hoge.raw", "raw1")
	c.AddFiles(dir)
	if b.Contents["hoge.raw"].Attr != ContentAttribute(0) {
		t.Errorf("hoge.raw must be raw file")
	}

	addFile(dir, "hoge.raw", "raw1")
	c.AddFiles(dir)
	if b.Contents["hoge.raw"].Attr != ContentAttribute(0) {
		t.Errorf("hoge.raw must be raw file")
	}

	addFile(dir, "fuga.noraw", "raw2")
	c.AddFiles(dir)
	if b.Contents["fuga.noraw"].Attr == ContentAttribute(0) {
		t.Errorf("hoge.noraw must not be raw file")
	}

}

func TestTag(t *testing.T) {
	c, b, dir := setupManifest()
	b.Tag = "test"

	addFile(dir, "hoge.txt", "hoge")
	c.AddFiles(dir)

	err := c.Finish()
	if err != nil {
		t.Errorf("cannot finish manifest")
		t.Error(err)
		return
	}
}
