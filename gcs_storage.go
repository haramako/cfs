package cfs

import (
	"bytes"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/storage/v1"
	"log"
	"net/url"
	//"os"
)

type GcsStorage struct {
	BucketName string
	CabinetUrl *url.URL
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
	object := &storage.Object{Name: "data/" + hashPath(hash)}
	_, err := s.service.Objects.Insert(s.BucketName, object).Media(bytes.NewBuffer(body)).Do()
	if err != nil {
		return err
	}
	return nil
}

/*

func fatalf(service *storage.Service, errorMessage string, args ...interface{}) {
	log.Fatalf("Dying with error:\n"+errorMessage, args...)
}

func main() {
	projectId := "disco-beach-146723"
	bucketName := "cfs"

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "gcs-key.json")

	client, err := google.DefaultClient(context.Background(), storage.DevstorageFullControlScope)
	if err != nil {
		log.Fatalf("Unable to get default client: %v", err)
	}

	service, err := storage.New(client)
	if err != nil {
		log.Fatalf("Unable to create storage service: %v", err)
	}

	// If the bucket already exists and the user has access, warn the user, but don't try to create it.
	b, err := service.Buckets.Get(bucketName).Do()
	if err == nil {
		fmt.Printf("Bucket %s already exists - skipping buckets.insert call.\n", bucketName)
	} else {
		// Create a bucket.
		if res, err := service.Buckets.Insert(projectId, &storage.Bucket{Name: bucketName}).Do(); err == nil {
			fmt.Printf("Created bucket %v at location %v\n\n", res.Name, res.SelfLink)
		} else {
			fatalf(service, "Failed creating bucket %s: %v", bucketName, err)
		}
	}

	objectName := "Makefile"
	fileName := "Makefile"

	object := &storage.Object{Name: objectName}
	file, err := os.Open(fileName)
	if err != nil {
		fatalf(service, "Error opening %q: %v", fileName, err)
	}
	if res, err := service.Objects.Insert(bucketName, object).Media(file).Do(); err == nil {
		fmt.Printf("Created object %v at location %v\n\n", res.Name, res.SelfLink)
	} else {
		fatalf(service, "Objects.Insert failed: %v", err)
	}
}

*/
