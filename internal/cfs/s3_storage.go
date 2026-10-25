package cfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// DefaultS3Region は、リージョンをどこからも決められなかったときに使うリージョン
const DefaultS3Region = "ap-northeast-1"

// S3Storage は Amazon S3 のバケットをキャビネットにする。書き込み（Storage）と読み込み（Fetcher）の両方を行う
type S3Storage struct {
	BucketName string
	Region     string
	client     *s3.Client
}

func NewS3Storage(bucketName string) (*S3Storage, error) {
	return newS3Storage(bucketName)
}

// optFns はテストでエンドポイントを差し替えるために使う
func newS3Storage(bucketName string, optFns ...func(*s3.Options)) (*S3Storage, error) {
	ctx := context.Background()

	// 単純な PutObject にするため、チェックサムは必要な場合だけ付ける
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
	)
	if err != nil {
		return nil, err
	}

	// リージョンは .cfsenv の S3Region → バケットから自動判定 → AWS 標準の設定 → 既定値 の順に決める
	region := Option.S3Region
	if region == "" {
		region = detectBucketRegion(ctx, cfg, bucketName, optFns)
	}
	if region == "" {
		region = cfg.Region
	}
	if region == "" {
		region = DefaultS3Region
	}
	if Verbose {
		fmt.Printf("using S3 bucket '%s' in region '%s'\n", bucketName, region)
	}

	client := s3.NewFromConfig(cfg, append([]func(*s3.Options){func(o *s3.Options) { o.Region = region }}, optFns...)...)

	return &S3Storage{BucketName: bucketName, Region: region, client: client}, nil
}

// detectBucketRegion は、バケットのリージョンを HeadBucket の x-amz-bucket-region から調べる。
// このヘッダーは権限がなく 403 が返る場合にも付くので、認証なしで問い合わせる。分からなければ空を返す
func detectBucketRegion(ctx context.Context, cfg aws.Config, bucketName string, optFns []func(*s3.Options)) string {
	probeRegion := cfg.Region
	if probeRegion == "" {
		probeRegion = "us-east-1"
	}
	client := s3.NewFromConfig(cfg, append([]func(*s3.Options){func(o *s3.Options) {
		o.Region = probeRegion
		o.Credentials = aws.AnonymousCredentials{}
	}}, optFns...)...)

	out, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucketName)})
	if err == nil {
		return aws.ToString(out.BucketRegion)
	}
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.Response != nil {
		return re.Response.Header.Get("x-amz-bucket-region")
	}
	return ""
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

// Open は S3 からデータを直接読む（Fetcher）
func (s *S3Storage) Open(path string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(path),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) || s3StatusCode(err) == http.StatusNotFound {
			return nil, fmt.Errorf("%w: s3://%s/%s", ErrNotFound, s.BucketName, path)
		}
		return nil, fmt.Errorf("cannot read s3://%s/%s: %w", s.BucketName, path, err)
	}
	return out.Body, nil
}

// Exists は S3 にデータがあるかを返す（Fetcher）
func (s *S3Storage) Exists(path string) (bool, error) {
	_, err := s.client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(path),
	})
	if err == nil {
		return true, nil
	}
	// s3:ListBucket の権限がないと、ないオブジェクトへの HeadObject は 404 ではなく 403 になる
	switch s3StatusCode(err) {
	case http.StatusNotFound, http.StatusForbidden:
		return false, nil
	}
	return false, err
}

func s3StatusCode(err error) int {
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.Response != nil {
		return re.Response.StatusCode
	}
	return 0
}
