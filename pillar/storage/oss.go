package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

// OSSConfig holds Alibaba Cloud OSS configuration.
type OSSConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	AccessKeySecret string `mapstructure:"access_key_secret"`
}

// OSSStorage implements Storage using Alibaba Cloud OSS.
type OSSStorage struct {
	client *oss.Client
	bucket string
}

// NewOSS creates a new Alibaba Cloud OSS storage client.
func NewOSS(cfg OSSConfig) (*OSSStorage, error) {
	provider := credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret)
	ossCfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(provider).
		WithRegion(cfg.Region).
		WithEndpoint(cfg.Endpoint)

	client := oss.NewClient(ossCfg)

	return &OSSStorage{client: client, bucket: cfg.Bucket}, nil
}

func (o *OSSStorage) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	input := &oss.PutObjectRequest{
		Bucket:      oss.Ptr(o.bucket),
		Key:         oss.Ptr(key),
		Body:        reader,
		ContentType: oss.Ptr(contentType),
	}
	if size > 0 {
		input.ContentLength = oss.Ptr(size)
	}
	_, err := o.client.PutObject(ctx, input)
	return wrapOSSError(err)
}

func (o *OSSStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := o.client.GetObject(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(o.bucket),
		Key:    oss.Ptr(key),
	})
	if err != nil {
		return nil, wrapOSSError(err)
	}
	return out.Body, nil
}

func (o *OSSStorage) Delete(ctx context.Context, key string) error {
	_, err := o.client.DeleteObject(ctx, &oss.DeleteObjectRequest{
		Bucket: oss.Ptr(o.bucket),
		Key:    oss.Ptr(key),
	})
	return wrapOSSError(err)
}

func (o *OSSStorage) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	out, err := o.client.HeadObject(ctx, &oss.HeadObjectRequest{
		Bucket: oss.Ptr(o.bucket),
		Key:    oss.Ptr(key),
	})
	if err != nil {
		return nil, wrapOSSError(err)
	}
	info := &ObjectInfo{
		Key:         key,
		ContentType: oss.ToString(out.ContentType),
	}
	info.Size = out.ContentLength
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	return info, nil
}

func (o *OSSStorage) Exists(ctx context.Context, key string) (bool, error) {
	result, err := o.client.IsObjectExist(ctx, o.bucket, key)
	if err != nil {
		return false, wrapOSSError(err)
	}
	return result, nil
}

func (o *OSSStorage) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	in := &oss.ListObjectsV2Request{
		Bucket: oss.Ptr(o.bucket),
	}
	if input.Prefix != "" {
		in.Prefix = oss.Ptr(input.Prefix)
	}
	if input.Marker != "" {
		in.ContinuationToken = oss.Ptr(input.Marker)
	}
	if input.MaxKeys > 0 {
		in.MaxKeys = int32(input.MaxKeys)
	}
	if input.Delimiter != "" {
		in.Delimiter = oss.Ptr(input.Delimiter)
	}

	out, err := o.client.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, wrapOSSError(err)
	}

	result := &ListOutput{}
	result.IsTruncated = out.IsTruncated
	if out.NextContinuationToken != nil {
		result.NextMarker = *out.NextContinuationToken
	}
	for _, obj := range out.Contents {
		info := ObjectInfo{
			Key:  oss.ToString(obj.Key),
			Size: obj.Size,
		}
		if obj.LastModified != nil {
			info.LastModified = *obj.LastModified
		}
		result.Objects = append(result.Objects, info)
	}
	for _, p := range out.CommonPrefixes {
		result.CommonPrefixes = append(result.CommonPrefixes, oss.ToString(p.Prefix))
	}
	return result, nil
}

func (o *OSSStorage) Copy(ctx context.Context, srcKey, dstKey string) error {
	_, err := o.client.CopyObject(ctx, &oss.CopyObjectRequest{
		Bucket:       oss.Ptr(o.bucket),
		Key:          oss.Ptr(dstKey),
		SourceBucket: oss.Ptr(o.bucket),
		SourceKey:    oss.Ptr(srcKey),
	})
	return wrapOSSError(err)
}

func (o *OSSStorage) SignURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	result, err := o.client.Presign(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(o.bucket),
		Key:    oss.Ptr(key),
	}, oss.PresignExpires(expire))
	if err != nil {
		return "", fmt.Errorf("storage: oss presign: %w", err)
	}
	return result.URL, nil
}

func (o *OSSStorage) SignPutURL(ctx context.Context, key string, contentType string, expire time.Duration) (string, error) {
	input := &oss.PutObjectRequest{
		Bucket: oss.Ptr(o.bucket),
		Key:    oss.Ptr(key),
	}
	if contentType != "" {
		input.ContentType = oss.Ptr(contentType)
	}
	result, err := o.client.Presign(ctx, input, oss.PresignExpires(expire))
	if err != nil {
		return "", fmt.Errorf("storage: oss presign put: %w", err)
	}
	return result.URL, nil
}

func (o *OSSStorage) WithBucket(bucket string) Storage {
	return &OSSStorage{client: o.client, bucket: bucket}
}

func (o *OSSStorage) Ping(ctx context.Context) error {
	_, err := o.client.GetBucketInfo(ctx, &oss.GetBucketInfoRequest{
		Bucket: oss.Ptr(o.bucket),
	})
	return wrapOSSError(err)
}

func wrapOSSError(err error) error {
	if err == nil {
		return nil
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "NoSuchKey") || strings.Contains(errMsg, "StatusCode=404") {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if strings.Contains(errMsg, "AccessDenied") || strings.Contains(errMsg, "StatusCode=403") {
		return fmt.Errorf("%w: %v", ErrAccessDenied, err)
	}
	return err
}
