package cfs

import (
	"bytes"
	"fmt"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/storage/v1"
	"log"
)

type GcsStorage struct {
	BucketName string
	service    *storage.Service
}

func (s *GcsStorage) Init() error {
	//os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "gcs-key.json")

	client, err := google.DefaultClient(context.Background(), storage.DevstorageFullControlScope)
	if err != nil {
		log.Fatalf("Unable to get default client: %v", err)
	}

	service, err := storage.New(client)
	if err != nil {
		log.Fatalf("Unable to create storage service: %v", err)
	}

	s.service = service

	return nil
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
