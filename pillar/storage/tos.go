package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"
)

// TOSConfig holds Volcengine TOS configuration.
type TOSConfig struct {
	Endpoint       string `mapstructure:"endpoint"`
	Region         string `mapstructure:"region"`
	Bucket         string `mapstructure:"bucket"`
	AccessKeyID    string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
}

// TOSStorage implements Storage using Volcengine TOS.
type TOSStorage struct {
	client *tos.ClientV2
	bucket string
}

// NewTOS creates a new Volcengine TOS storage client.
func NewTOS(cfg TOSConfig) (*TOSStorage, error) {
	client, err := tos.NewClientV2(
		cfg.Endpoint,
		tos.WithRegion(cfg.Region),
		tos.WithCredentials(tos.NewStaticCredentials(cfg.AccessKeyID, cfg.SecretAccessKey)),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: create tos client: %w", err)
	}
	return &TOSStorage{client: client, bucket: cfg.Bucket}, nil
}

func (t *TOSStorage) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	input := &tos.PutObjectV2Input{
		PutObjectBasicInput: tos.PutObjectBasicInput{
			Bucket:      t.bucket,
			Key:         key,
			ContentType: contentType,
		},
		Content: reader,
	}
	if size > 0 {
		input.ContentLength = size
	}
	_, err := t.client.PutObjectV2(ctx, input)
	return wrapTOSError(err)
}

func (t *TOSStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := t.client.GetObjectV2(ctx, &tos.GetObjectV2Input{
		Bucket: t.bucket,
		Key:    key,
	})
	if err != nil {
		return nil, wrapTOSError(err)
	}
	return out.Content, nil
}

func (t *TOSStorage) Delete(ctx context.Context, key string) error {
	_, err := t.client.DeleteObjectV2(ctx, &tos.DeleteObjectV2Input{
		Bucket: t.bucket,
		Key:    key,
	})
	return wrapTOSError(err)
}

func (t *TOSStorage) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	out, err := t.client.HeadObjectV2(ctx, &tos.HeadObjectV2Input{
		Bucket: t.bucket,
		Key:    key,
	})
	if err != nil {
		return nil, wrapTOSError(err)
	}
	return &ObjectInfo{
		Key:          key,
		Size:         out.ContentLength,
		ContentType:  out.ContentType,
		LastModified: out.LastModified,
	}, nil
}

func (t *TOSStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := t.client.HeadObjectV2(ctx, &tos.HeadObjectV2Input{
		Bucket: t.bucket,
		Key:    key,
	})
	if err != nil {
		wrapped := wrapTOSError(err)
		if errors.Is(wrapped, ErrNotFound) {
			return false, nil
		}
		return false, wrapped
	}
	return true, nil
}

func (t *TOSStorage) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	in := &tos.ListObjectsV2Input{
		Bucket: t.bucket,
	}
	if input.Prefix != "" {
		in.Prefix = input.Prefix
	}
	if input.Marker != "" {
		in.Marker = input.Marker
	}
	if input.MaxKeys > 0 {
		in.MaxKeys = input.MaxKeys
	}
	if input.Delimiter != "" {
		in.Delimiter = input.Delimiter
	}

	out, err := t.client.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, wrapTOSError(err)
	}

	result := &ListOutput{
		IsTruncated: out.IsTruncated,
		NextMarker:  out.NextMarker,
	}
	for _, obj := range out.Contents {
		result.Objects = append(result.Objects, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
		})
	}
	for _, p := range out.CommonPrefixes {
		result.CommonPrefixes = append(result.CommonPrefixes, p.Prefix)
	}
	return result, nil
}

func (t *TOSStorage) Copy(ctx context.Context, srcKey, dstKey string) error {
	_, err := t.client.CopyObject(ctx, &tos.CopyObjectInput{
		Bucket:    t.bucket,
		Key:       dstKey,
		SrcBucket: t.bucket,
		SrcKey:    srcKey,
	})
	return wrapTOSError(err)
}

func (t *TOSStorage) SignURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	out, err := t.client.PreSignedURL(&tos.PreSignedURLInput{
		HTTPMethod: enum.HttpMethodGet,
		Bucket:     t.bucket,
		Key:        key,
		Expires:    int64(expire.Seconds()),
	})
	if err != nil {
		return "", wrapTOSError(err)
	}
	return out.SignedUrl, nil
}

func (t *TOSStorage) SignPutURL(ctx context.Context, key string, contentType string, expire time.Duration) (string, error) {
	input := &tos.PreSignedURLInput{
		HTTPMethod: enum.HttpMethodPut,
		Bucket:     t.bucket,
		Key:        key,
		Expires:    int64(expire.Seconds()),
	}
	if contentType != "" {
		input.Header = map[string]string{"Content-Type": contentType}
	}
	out, err := t.client.PreSignedURL(input)
	if err != nil {
		return "", wrapTOSError(err)
	}
	return out.SignedUrl, nil
}

func (t *TOSStorage) WithBucket(bucket string) Storage {
	return &TOSStorage{client: t.client, bucket: bucket}
}

func (t *TOSStorage) Ping(ctx context.Context) error {
	_, err := t.client.HeadBucket(ctx, &tos.HeadBucketInput{
		Bucket: t.bucket,
	})
	return wrapTOSError(err)
}

func wrapTOSError(err error) error {
	if err == nil {
		return nil
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "NoSuchKey") || strings.Contains(errMsg, "StatusCode=404") || strings.Contains(errMsg, "404") {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if strings.Contains(errMsg, "AccessDenied") || strings.Contains(errMsg, "StatusCode=403") || strings.Contains(errMsg, "403") {
		return fmt.Errorf("%w: %v", ErrAccessDenied, err)
	}
	return err
}
