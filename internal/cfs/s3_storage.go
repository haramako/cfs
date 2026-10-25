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
	log        Logger
}

// NewS3Storage は、バケットを開く。region が空なら、バケットから自動で判定する
func NewS3Storage(bucketName string, region string, log Logger) (*S3Storage, error) {
	return newS3Storage(bucketName, region, log)
}

// optFns はテストでエンドポイントを差し替えるために使う
func newS3Storage(bucketName string, region string, log Logger, optFns ...func(*s3.Options)) (*S3Storage, error) {
	ctx := context.Background()

	// 単純な PutObject にするため、チェックサムは必要な場合だけ付ける（取得時の検証もしない。
	// 中身はハッシュで確かめる）
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		config.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	)
	if err != nil {
		return nil, err
	}

	// リージョンは .cfsenv の S3Region → バケットから自動判定 → AWS 標準の設定 → 既定値 の順に決める
	if region == "" {
		region = detectBucketRegion(ctx, cfg, bucketName, optFns)
	}
	if region == "" {
		region = cfg.Region
	}
	if region == "" {
		region = DefaultS3Region
	}
	log.Printf("using S3 bucket '%s' in region '%s'\n", bucketName, region)

	client := s3.NewFromConfig(cfg, append([]func(*s3.Options){func(o *s3.Options) { o.Region = region }}, optFns...)...)

	return &S3Storage{BucketName: bucketName, Region: region, client: client, log: log}, nil
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

func (s *S3Storage) Put(key string, body []byte, overwrite bool) error {
	ctx := context.Background()

	if !overwrite {
		// 既にあるかは ListObjectsV2 で確かめる（s3:ListBucket の権限で足りる）
		found, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:    aws.String(s.BucketName),
			Prefix:    aws.String(key),
			Delimiter: aws.String("/"),
			MaxKeys:   aws.Int32(1),
		})
		if err != nil {
			return err
		}
		if len(found.Contents) > 0 && aws.ToString(found.Contents[0].Key) == key {
			// already exists
			return nil
		}
	}

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.BucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("binary/octet-stream"),
		ACL:         types.ObjectCannedACLBucketOwnerFullControl,
	})
	if err != nil {
		return err
	}
	s.log.Printf("uploading '%s'\n", key)
	return nil
}

func (s *S3Storage) List(prefix string) ([]ObjectInfo, error) {
	var result []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.BucketName),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			result = append(result, ObjectInfo{
				Key:          aws.ToString(o.Key),
				Size:         aws.ToInt64(o.Size),
				LastModified: aws.ToTime(o.LastModified),
			})
		}
	}
	return result, nil
}

func (s *S3Storage) Delete(keys []string) error {
	// DeleteObjects は 1 回に 1000 件まで
	for start := 0; start < len(keys); start += 1000 {
		end := min(start+1000, len(keys))
		objects := make([]types.ObjectIdentifier, 0, end-start)
		for _, key := range keys[start:end] {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(key)})
		}
		out, err := s.client.DeleteObjects(context.Background(), &s3.DeleteObjectsInput{
			Bucket: aws.String(s.BucketName),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
		if len(out.Errors) > 0 {
			e := out.Errors[0]
			return fmt.Errorf("cannot delete s3://%s/%s: %s", s.BucketName, aws.ToString(e.Key), aws.ToString(e.Message))
		}
	}
	return nil
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
