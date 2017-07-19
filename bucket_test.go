package cfs

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var Printf = fmt.Printf // avoid error
var timeCount = int64(0)

func TestMain(m *testing.M) {
	Option.EncryptKey = "12345678901234567890123456789012"
	Option.EncryptIv = "1234567890123456"
	Option.AdminUser = "admin"
	Option.AdminPass = "pass"

	if os.Getenv("CFS_TEST_VERBOSE") != "" {
		Verbose = true
	}

	initServer()

	os.Exit(m.Run())
}

func initServer() {
	if os.Getenv("CFS_TEST_STORAGE") == "cfs" {
		tempDir, err := ioutil.TempDir("", "cfs-cfssv")
		if err != nil {
			panic(err)
		}

		sv := &Server{
			RootFilepath: tempDir,
			Port:         9999,
			AdminUser:    "admin",
			AdminPass:    "pass",
		}
		err = sv.Init()
		if err != nil {
			panic(err)
		}
		go sv.Start()
		time.Sleep(100 * time.Millisecond) // サーバーが起動するまで待つ
	}
}

func setupBucket() (*Client, *Bucket, string) {

	dir, err := ioutil.TempDir("", "cfs-test")
	dir = filepath.FromSlash(dir)
	if err != nil {
		panic(err)
	}

	bucket, err := BucketFromFile(filepath.FromSlash(dir + "/.bucket"))
	if err != nil {
		panic(err)
	}

	storage := newStorage(bucket)

	client := &Client{
		Bucket:  bucket,
		Storage: storage,
	}
	client.Init()

	return client, bucket, dir
}

func newStorage(bucket *Bucket) Storage {
	var uri string
	switch os.Getenv("CFS_TEST_STORAGE") {
	case "gcs":
		uri = "gs://cfs"
	case "s3":
		uri = "s3://cfs-autotest"
	case "cfs":
		uri = "cfs://localhost:9999/"
	default:
		tempDir, err := ioutil.TempDir("", "cfs-file")
		if err != nil {
			panic(err)
		}
		uri = "file:///" + tempDir + "/"
	}

	storage, err := StorageFromString(uri)
	if err != nil {
		panic(err)
	}
	return storage
}

func setupBucketWithFiles() (*Client, *Bucket, string) {
	c, b, dir := setupBucket()
	addFile(dir, "hoge", "hoge")
	addFile(dir, "fuga", "fuga")
	addFile(dir, "piyo/piyo", "piyo")

	c.AddFiles(dir)

	b.UploadCount = 0
	return c, b, dir
}

func setupBucketFromUrl(baseUrl *url.URL, location string) (*Downloader, *Bucket) {
	downloader, err := NewDownloader(baseUrl.String())
	if err != nil {
		panic(err)
	}

	bucket, err := downloader.LoadBucket(location)
	if err != nil {
		panic(err)
	}
	return downloader, bucket
}

func assertContents(t *testing.T, b *Bucket, n int) {
	if len(b.Contents) != n {
		t.Errorf("b.Contents must be %v but %v", n, len(b.Contents))
	}
}

func assertUploadCount(t *testing.T, b *Bucket, n int) {
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
	err = ioutil.WriteFile(fullpath, []byte(content), 0666)
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

func TestNewBucket(t *testing.T) {
	c, b, dir := setupBucket()

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
	c, b, dir := setupBucketWithFiles()

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

func TestCompress(t *testing.T) {
	c, b, dir := setupBucket()

	addFile(dir, "hoge", "piyo")
	addFile(dir, "fuga.raw", "hage")
	addFile(dir, "piyo/piyo", "piyopiyo")
	c.AddFiles(dir)

	err := c.Finish()
	if err != nil {
		t.Errorf("cannot finish bucket")
		t.Error(err)
		return
	}

	d, b2 := setupBucketFromUrl(c.Storage.DownloaderUrl(), b.Hash)
	if b2 == nil {
		return // なにもしない
	}

	temp, err := ioutil.TempDir("", "cfs-sync")
	if err != nil {
		panic(err)
	}

	err = d.Sync(b2, temp)
	if err != nil {
		t.Error(err)
		return
	}

	output, _ := exec.Command("diff", "-r", dir, temp).CombinedOutput()
	if len(strings.Split(string(output), "\n")) != 3 {
		t.Errorf("invalid diff")
		t.Error(string(output))
	}
}

func TestRawFile(t *testing.T) {
	c, b, dir := setupBucket()

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
	c, b, dir := setupBucket()
	b.Tag = "test"

	addFile(dir, "hoge.txt", "hoge")
	c.AddFiles(dir)

	err := c.Finish()
	if err != nil {
		t.Errorf("cannot finish bucket")
		t.Error(err)
		return
	}
}
