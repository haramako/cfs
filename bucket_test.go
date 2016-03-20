package cfs

import (
	"fmt"
	"github.com/haramako/cfs/server"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var Printf = fmt.Printf // avoid error
var tempDir = ""
var timeCount = int64(0)

func TestMain(m *testing.M) {
	Option.EncryptKey = "12345678901234567890123456789012"
	Option.EncryptIv = "1234567890123456"
	Option.AdminUser = "admin"
	Option.AdminPass = "pass"
	Option.Cabinet = "http://localhost:9999/"

	rand.Seed(time.Now().UnixNano())
	if os.Getenv("CFS_TEST_VERBOSE") != "" {
		Verbose = true
	}
	var err error
	tempDir, err = ioutil.TempDir("", "cfs-work")
	if err != nil {
		panic(err)
	}

	tempDir = "/tmp/hogehoge"
	sv := &server.Server{
		FpRoot:    tempDir,
		Port:      9999,
		AdminUser: "admin",
		AdminPass: "pass",
	}
	err = sv.Init()
	if err != nil {
		panic(err)
	}
	go sv.Start()

	time.Sleep(100 * time.Millisecond) // サーバーが起動するまで待つ

	os.Exit(m.Run())
}

func setupBucket() (*Bucket, string) {

	dir, err := ioutil.TempDir("", "cfs-test")
	dir = filepath.FromSlash(dir)
	if err != nil {
		panic(err)
	}

	bucket, err := BucketFromFile(filepath.FromSlash(dir + "/.bucket"))
	if err != nil {
		panic(err)
	}

	return bucket, dir
}

func setupBucketWithFiles() (*Bucket, string) {
	b, dir := setupBucket()
	addFile(dir, "hoge", "hoge")
	addFile(dir, "fuga", "fuga")
	addFile(dir, "piyo/piyo", "piyo")

	b.AddFiles(dir)

	b.UploadCount = 0
	return b, dir
}

func setupBucketFromUrl(url string, location string) *Bucket {
	bucket, err := BucketFromUrl(url, location)
	if err != nil {
		panic(err)
	}
	return bucket
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
	b, dir := setupBucket()

	addFile(dir, "hoge", "fuga")
	b.AddFiles(dir)

	assertContents(t, b, 1)
	assertUploadCount(t, b, 1)

	addFile(dir, "fuga", "piyo")
	b.AddFiles(dir)

	assertContents(t, b, 2)
	assertUploadCount(t, b, 2)

	b.Finish()
	assertUploadCount(t, b, 3)
}

func TestOverwriteFile(t *testing.T) {
	b, dir := setupBucketWithFiles()

	addFile(dir, "hoge", "piyo")
	b.AddFiles(dir)

	assertContents(t, b, 3)
	assertUploadCount(t, b, 0)

	addFile(dir, "hoge", "fuga2")
	b.AddFiles(dir)

	assertUploadCount(t, b, 1)

	addFile(dir, "hoge", "fuga3")
	b.AddFiles(dir)

	assertUploadCount(t, b, 2)

	addFile(dir, "hoge", "fuga2")
	b.AddFiles(dir)

	assertUploadCount(t, b, 2)

	addFile(dir, filepath.FromSlash("piyo/piyo"), "piyo2")
	b.AddFiles(dir)

	assertUploadCount(t, b, 3)

	addFile(dir, filepath.FromSlash("piyo/piyo"), "piyo2")
	b.AddFiles(dir)

	assertContents(t, b, 3)
	assertUploadCount(t, b, 3)
}

func TestCompress(t *testing.T) {
	b, dir := setupBucket()

	addFile(dir, "hoge", "piyo")
	addFile(dir, "fuga.raw", "hage")
	addFile(dir, "piyo/piyo", "piyopiyo")
	b.AddFiles(dir)

	err := b.Finish()
	if err != nil {
		t.Errorf("cannot finish bucket")
		t.Error(err)
		return
	}

	b2 := setupBucketFromUrl(b.CabinetUrl.String(), b.Hash)
	if b2 == nil {
		return // なにもしない
	}

	temp, err := ioutil.TempDir("", "cfs-sync")
	if err != nil {
		panic(err)
	}

	err = b2.Sync(temp)
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
	b, dir := setupBucket()

	addFile(dir, "hoge.raw", "raw1")
	b.AddFiles(dir)
	if b.Contents["hoge.raw"].Attr != ContentAttribute(0) {
		t.Errorf("hoge.raw must be raw file")
	}

	addFile(dir, "hoge.raw", "raw1")
	b.AddFiles(dir)
	if b.Contents["hoge.raw"].Attr != ContentAttribute(0) {
		t.Errorf("hoge.raw must be raw file")
	}

	addFile(dir, "fuga.noraw", "raw2")
	b.AddFiles(dir)
	if b.Contents["fuga.noraw"].Attr == ContentAttribute(0) {
		t.Errorf("hoge.noraw must not be raw file")
	}

}
