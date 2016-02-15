package cfs

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"
)

var Printf = fmt.Printf // avoid error
var tempDir = ""
var timeCount = int64(0)

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UnixNano())
	if os.Getenv("CFS_TEST_VERBOSE") != "" {
		Verbose = true
	}
	var err error
	tempDir, err = ioutil.TempDir("", "cfs-work")
	if err != nil {
		panic(err)
	}
}

func setupBucket() (*Bucket, string) {

	dir, err := ioutil.TempDir("", "cfs-test")
	if err != nil {
		panic(err)
	}

	var uploader Uploader
	switch os.Getenv("CFS_TEST_UPLOADER") {
	case "s3":
		if Verbose {
			fmt.Println("using s3 uploder")
		}
		uploader, err = CreateS3Uploader("cfs-dev")
		if err != nil {
			panic(err)
		}
		uploader.(*S3Uploader).base = fmt.Sprintf("test/%d/", rand.Int())
	case "sftp":
		if Verbose {
			fmt.Println("using sftp uploder")
		}
		uploader, err = CreateSftpUploader(&SftpOption{
			Host:     "localhost",
			User:     "makoto",
			RootPath: fmt.Sprintf("cfs-test/%d", rand.Int()),
		})
		if err != nil {
			panic(err)
		}
	default:
		if uploader, err = CreateFileUploader(tempDir); err != nil {
			panic(err)
		}
	}

	bucket, err := BucketFromFile(dir+"/.bucket", uploader)
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

func setupBucketFromUrl(location string) *Bucket {
	switch os.Getenv("CFS_TEST_UPLOADER") {
	case "s3":
		// PENDING
	case "sftp":
		// PENDING
	default:
		bucket, err := BucketFromUrl("file://"+tempDir, location)
		if err != nil {
			panic(err)
		}
		return bucket
	}
	return nil
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

func TestCompress(t *testing.T) {
	b, dir := setupBucket()

	addFile(dir, "hoge", "piyo")
	addFile(dir, "fuga", "hage")
	addFile(dir, "piyo/piyo", "piyopiyo")
	b.AddFiles(dir)

	err := b.Finish()
	if err != nil {
		t.Errorf("cannot finish bucket")
		t.Error(err)
		return
	}

	b2 := setupBucketFromUrl("/data/" + b.Hash)

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
