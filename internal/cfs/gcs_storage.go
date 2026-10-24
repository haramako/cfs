package cfs

import (
	"context"
	"fmt"
	"net/url"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

type GcsStorage struct {
	BucketName string
	bucket     *storage.BucketHandle
	cabinetUrl *url.URL
}

func NewGcsStorage(bucketName string) (*GcsStorage, error) {
	s := &GcsStorage{
		BucketName: bucketName,
	}

	client, err := storage.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("Unable to get default gs client: %w", err)
	}

	s.bucket = client.Bucket(bucketName)

	s.cabinetUrl, err = url.Parse("http://storage.googleapis.com/" + s.BucketName + "/")
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *GcsStorage) DownloaderUrl() *url.URL {
	return s.cabinetUrl
}

func (s *GcsStorage) Upload(filename string, hash string, body []byte, overwrite bool) error {
	path := "data/" + hashPath(hash)
	ctx := context.Background()
	obj := s.bucket.Object(path)

	_, err := obj.Attrs(ctx)
	if err == nil {
		// file already exists.
		return nil
	}

	// no file! lets make a file

	w := obj.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	w.Write(body)
	err = w.Close()
	// 従来の挙動を維持: 304 Not Modified 以外のエラーは無視している
	if err != nil && googleapi.IsNotModified(err) {
		fmt.Println(err)
		return err
	}
	if Verbose {
		fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	}
	return nil
}

func (s *GcsStorage) UploadTag(filename string, body []byte) error {
	path := "tag/" + filename

	w := s.bucket.Object(path).NewWriter(context.Background())
	w.Write(body)
	err := w.Close()
	if err != nil {
		fmt.Println(err)
		return err
	}

	fmt.Printf("uploading '%s'\n", filename)

	return nil
}
