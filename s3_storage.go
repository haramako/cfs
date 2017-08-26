package cfs

import (
	"fmt"
	"net/url"

	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/s3"
)

type S3Storage struct {
	BucketName string
	cabinetUrl *url.URL
	s3         *s3.S3
	bucket     *s3.Bucket
}

type S3UploderOption struct {
	BucketName      *string
	AccessKeyId     *string
	SecretAccessKey *string
	Regin           *string
}

func NewS3Storage(bucketName string) (*S3Storage, error) {
	s := &S3Storage{
		BucketName: bucketName,
	}

	auth, err := aws.EnvAuth()
	if err != nil {
		return nil, err
	}

	s.s3 = s3.New(auth, aws.GetRegion("ap-northeast-1"))
	s.bucket = s.s3.Bucket(bucketName)

	s.cabinetUrl, err = url.Parse("http://" + s.BucketName + ".s3-website-ap-northeast-1.amazonaws.com/")
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *S3Storage) DownloaderUrl() *url.URL {
	return s.cabinetUrl
}

func (s *S3Storage) Upload(filename string, hash string, body []byte, overwrite bool) error {
	path := "data/" + hashPath(hash)

	if !overwrite {
		found, err := s.bucket.List(path, "/", "", 1)
		if err != nil {
			return err
		}
		if len(found.Contents) > 0 {
			// already exists
			return nil
		}
	}

	err := s.bucket.Put(path, body, "binary/octet-stream", s3.BucketOwnerFull, s3.Options{})
	if err != nil {
		return err
	}
	if Verbose {
		fmt.Printf("uploading '%s'\n", path)
	}
	return nil
}

func (s *S3Storage) UploadTag(filename string, body []byte) error {
	path := "tag/" + filename

	err := s.bucket.Put(path, body, "binary/octet-stream", s3.BucketOwnerFull, s3.Options{})
	if err != nil {
		return err
	}
	if Verbose {
		fmt.Printf("uploading '%s'\n", path)
	}
	return nil
}
