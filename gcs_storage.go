package cfs

import (
	"bytes"
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/storage/v1"
)

type GcsStorage struct {
	BucketName string
	service    *storage.Service
	cabinetUrl *url.URL
}

func NewGcsStorage(bucketName string) (*GcsStorage, error) {
	s := &GcsStorage{
		BucketName: bucketName,
	}

	client, err := google.DefaultClient(context.Background(), storage.DevstorageFullControlScope)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to get default gs client")
	}

	service, err := storage.New(client)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to create storage service")
	}

	s.service = service

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
	object := &storage.Object{Name: path}

	_, err := s.service.Objects.Get(s.BucketName, path).Do()
	if err == nil {
		// file already exists.
		return nil
	}

	// no file! lets make a file

	_, err = s.service.Objects.Insert(s.BucketName, object).IfGenerationMatch(0).Media(bytes.NewBuffer(body)).Do()
	if err != nil && googleapi.IsNotModified(err) {
		fmt.Println(err)
		return err
	}
	//if Verbose {
	fmt.Printf("uploading '%s' as '%s'\n", filename, hash)
	//}
	return nil
}

func (s *GcsStorage) UploadTag(filename string, body []byte) error {
	path := "tag/" + filename
	object := &storage.Object{Name: path}

	_, err := s.service.Objects.Insert(s.BucketName, object).Media(bytes.NewBuffer(body)).Do()
	if err != nil {
		fmt.Println(err)
		return err
	}

	fmt.Printf("uploading '%s'\n", filename)

	return nil
}
