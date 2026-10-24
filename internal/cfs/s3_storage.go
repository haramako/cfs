package cfs

import (
	"bytes"
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Storage struct {
	BucketName string
	cabinetUrl *url.URL
	client     *s3.Client
}

func NewS3Storage(bucketName string) (*S3Storage, error) {
	return newS3Storage(bucketName)
}

// optFns はテストでエンドポイントを差し替えるために使う
func newS3Storage(bucketName string, optFns ...func(*s3.Options)) (*S3Storage, error) {
	s := &S3Storage{
		BucketName: bucketName,
	}

	// 単純な PutObject にするため、チェックサムは必要な場合だけ付ける
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("ap-northeast-1"),
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
	)
	if err != nil {
		return nil, err
	}

	s.client = s3.NewFromConfig(cfg, optFns...)

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
	ctx := context.Background()

	if !overwrite {
		found, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:    aws.String(s.BucketName),
			Prefix:    aws.String(path),
			Delimiter: aws.String("/"),
			MaxKeys:   aws.Int32(1),
		})
		if err != nil {
			return err
		}
		if len(found.Contents) > 0 {
			// already exists
			return nil
		}
	}

	// TODO: sizeが0だとエラーになるため、送信しないようにしている
	if len(body) > 0 {
		err := s.put(ctx, path, body)
		if err != nil {
			return err
		}
		if Verbose {
			fmt.Printf("uploading '%s'\n", path)
		}
	}

	return nil
}

func (s *S3Storage) UploadTag(filename string, body []byte) error {
	path := "tag/" + filename

	err := s.put(context.Background(), path, body)
	if err != nil {
		return err
	}
	if Verbose {
		fmt.Printf("uploading '%s'\n", path)
	}
	return nil
}

func (s *S3Storage) put(ctx context.Context, path string, body []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.BucketName),
		Key:         aws.String(path),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("binary/octet-stream"),
		ACL:         types.ObjectCannedACLBucketOwnerFullControl,
	})
	return err
}
