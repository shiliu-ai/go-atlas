package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config holds S3-compatible storage configuration.
type S3Config struct {
	Endpoint       string `mapstructure:"endpoint"`
	Region         string `mapstructure:"region"`
	Bucket         string `mapstructure:"bucket"`
	AccessKeyID    string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	ForcePathStyle bool   `mapstructure:"force_path_style"`
}

// S3Storage implements Storage using S3-compatible API.
type S3Storage struct {
	client *s3.Client
	bucket string
}

// NewS3 creates a new S3-compatible storage client.
func NewS3(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.ForcePathStyle
	})

	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Storage) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentType),
	}
	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}
	_, err := s.client.PutObject(ctx, input)
	return wrapS3Error(err)
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, wrapS3Error(err)
	}
	return out.Body, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return wrapS3Error(err)
}

func (s *S3Storage) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, wrapS3Error(err)
	}
	return &ObjectInfo{
		Key:          key,
		Size:         aws.ToInt64(out.ContentLength),
		ContentType:  aws.ToString(out.ContentType),
		LastModified: aws.ToTime(out.LastModified),
	}, nil
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		wrapped := wrapS3Error(err)
		if errors.Is(wrapped, ErrNotFound) {
			return false, nil
		}
		return false, wrapped
	}
	return true, nil
}

func (s *S3Storage) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	in := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	}
	if input.Prefix != "" {
		in.Prefix = aws.String(input.Prefix)
	}
	if input.Marker != "" {
		in.ContinuationToken = aws.String(input.Marker)
	}
	if input.MaxKeys > 0 {
		in.MaxKeys = aws.Int32(int32(input.MaxKeys))
	}
	if input.Delimiter != "" {
		in.Delimiter = aws.String(input.Delimiter)
	}

	out, err := s.client.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, wrapS3Error(err)
	}

	result := &ListOutput{
		IsTruncated: aws.ToBool(out.IsTruncated),
	}
	if out.NextContinuationToken != nil {
		result.NextMarker = *out.NextContinuationToken
	}
	for _, obj := range out.Contents {
		result.Objects = append(result.Objects, ObjectInfo{
			Key:          aws.ToString(obj.Key),
			Size:         aws.ToInt64(obj.Size),
			LastModified: aws.ToTime(obj.LastModified),
		})
	}
	for _, p := range out.CommonPrefixes {
		result.CommonPrefixes = append(result.CommonPrefixes, aws.ToString(p.Prefix))
	}
	return result, nil
}

func (s *S3Storage) Copy(ctx context.Context, srcKey, dstKey string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(s.bucket + "/" + srcKey),
		Key:        aws.String(dstKey),
	})
	return wrapS3Error(err)
}

func (s *S3Storage) SignURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	presigner := s3.NewPresignClient(s.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expire))
	if err != nil {
		return "", wrapS3Error(err)
	}
	return req.URL, nil
}

func (s *S3Storage) SignPutURL(ctx context.Context, key string, contentType string, expire time.Duration) (string, error) {
	presigner := s3.NewPresignClient(s.client)
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	req, err := presigner.PresignPutObject(ctx, input, s3.WithPresignExpires(expire))
	if err != nil {
		return "", wrapS3Error(err)
	}
	return req.URL, nil
}

func (s *S3Storage) WithBucket(bucket string) Storage {
	return &S3Storage{client: s.client, bucket: bucket}
}

func (s *S3Storage) Ping(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	return wrapS3Error(err)
}

func wrapS3Error(err error) error {
	if err == nil {
		return nil
	}
	var notFound *types.NotFound
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}
