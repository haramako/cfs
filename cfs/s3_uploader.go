package cfs

import (
	"fmt"
	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/s3"
)

type S3Uploader struct {
	bucket *s3.Bucket
}

func CreateS3Uploader(bucketName string) (*S3Uploader, error) {
	auth, err := aws.EnvAuth()
	if err != nil {
		panic(err)
	}
	s := s3.New(auth, aws.GetRegion("ap-northeast-1"))
	return &S3Uploader{bucket: s.Bucket(bucketName)}, nil
}

func (u *S3Uploader) Upload(path string, body []byte, overwrite bool) error {
	if !overwrite {
		found, err := u.bucket.Exists(path)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
	}
	err := u.bucket.Put(path, body, "binary/octet-stream", s3.BucketOwnerFull, s3.Options{})
	if err != nil {
		return err
	}
	if Verbose {
		fmt.Printf("uploading '%s'\n", path)
	}
	return nil
}

func (u *S3Uploader) Close() {
}
