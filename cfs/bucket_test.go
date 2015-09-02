package cfs

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"
)

var Printf = fmt.Printf // avoid error
var tempDir string
var timeCount = int64(0)
var initialized = false

func setupBucket() (*Bucket, string) {
	if !initialized {
		rand.Seed(time.Now().UnixNano())
		initialized = true
	}
	if os.Getenv("CFS_TEST_VERBOSE") != "" {
		Verbose = true
	}

	dir, err := ioutil.TempDir("", "cfs-test")
	if err != nil {
		panic(err)
	}

	if tempDir == "" {
		tempDir, err = ioutil.TempDir("", "cfs-work")
		if err != nil {
			panic(err)
		}
	}

	var uploader Uploader
	if os.Getenv("CFS_TEST_UPLOADER") == "s3" {
		uploader, err = CreateS3Uploader("cfs-dev")
		uploader.(*S3Uploader).base = fmt.Sprintf("test/%d/", rand.Int())
		if err != nil {
			panic(err)
		}
	} else {
		uploader, err = CreateFileUploader(tempDir)
	}

	bucket, err := BucketFromFile(dir, uploader)
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

	if b.uploader != nil {
		b.uploader.Stat().UploadCount = 0
	}
	return b, dir
}

func assertContents(t *testing.T, b *Bucket, n int) {
	if len(b.Contents) != n {
		t.Errorf("b.Contents must be %v but %v", n, len(b.Contents))
	}
}

func assertUploadCount(t *testing.T, b *Bucket, n int) {
	if b.uploader.Stat().UploadCount != n {
		t.Errorf("upload count must be %v but %v", n, b.uploader.Stat().UploadCount)
	}
}

func addFile(dir, file, content string) {
	fullpath := path.Join(dir, file)
	fulldir, _ := path.Split(fullpath)
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

	addFile(dir, "piyo/piyo", "piyo2")
	b.AddFiles(dir)

	assertUploadCount(t, b, 3)

	addFile(dir, "piyo/piyo", "piyo2")
	b.AddFiles(dir)

	assertContents(t, b, 3)
	assertUploadCount(t, b, 3)

}
