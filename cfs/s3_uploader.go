package cfs

import (
	"fmt"
	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/s3"
)

type S3Uploader struct {
	bucket *s3.Bucket
	base   string
	stat   UploaderStat
}

type S3UploderOption struct {
	BucketName      *string
	AccessKeyId     *string
	SecretAccessKey *string
	Regin           *string
}

func NewS3() (*s3.S3, error) {
	auth, err := aws.EnvAuth()
	if err != nil {
		return nil, err
	}
	s := s3.New(auth, aws.GetRegion("ap-northeast-1"))
	return s, nil
}

func CreateS3Uploader(bucketName string) (*S3Uploader, error) {
	s, err := NewS3()
	if err != nil {
		return nil, err
	}
	return &S3Uploader{bucket: s.Bucket(bucketName)}, nil
}

func (u *S3Uploader) Upload(path string, body []byte, overwrite bool) error {
	if !overwrite {
		found, err := u.bucket.Exists(u.base + path)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
	}
	u.stat.UploadCount++
	err := u.bucket.Put(u.base+path, body, "binary/octet-stream", s3.BucketOwnerFull, s3.Options{})
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

func (u *S3Uploader) Stat() *UploaderStat {
	return &u.stat
}
