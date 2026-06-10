package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

var ErrNoObject = errors.New("media: object not found in storage")

type StoreConfig struct {
	Endpoint     string
	Region       string
	AccessKey    string
	SecretKey    string
	Bucket       string
	UsePathStyle bool // true for MinIO; false for Yandex Object Storage
}

// Store wraps the S3 API. The app talks to an endpoint variable, so MinIO
// (dev) vs Object Storage (deployed) is config, not a component swap (§6).
type Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewStore(ctx context.Context, c StoreConfig) (*Store, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(c.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(c.Endpoint)
		o.UsePathStyle = c.UsePathStyle
	})
	return &Store{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  c.Bucket,
	}, nil
}

// EnsureBucket creates the bucket if missing (dev/test convenience;
// deployed buckets come from Terraform).
func (s *Store) EnsureBucket(ctx context.Context) error {
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	})
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) &&
		(apiErr.ErrorCode() == "BucketAlreadyOwnedByYou" || apiErr.ErrorCode() == "BucketAlreadyExists") {
		return nil
	}
	return err
}

// PresignPut returns a 5-minute pre-signed PUT scoped to exactly key (§2.5).
func (s *Store) PresignPut(ctx context.Context, key, contentType string) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(5*time.Minute))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Head returns the object size, or ErrNoObject.
func (s *Store) Head(ctx context.Context, key string) (int64, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotFound" {
		return 0, ErrNoObject
	}
	if err != nil {
		return 0, err
	}
	return aws.ToInt64(out.ContentLength), nil
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close() //nolint:errcheck
	return io.ReadAll(out.Body)
}

func (s *Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	return err
}
