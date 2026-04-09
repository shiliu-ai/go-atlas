package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"
)

// TOSConfig holds Volcengine TOS configuration.
//
// Endpoint is used for all data-plane operations (Put/Get/...). PublicEndpoint
// is optional: when set, pre-signed URLs are built against it instead of
// Endpoint. This supports the common deployment where the service reaches TOS
// over a private/internal endpoint for performance and cost, but must hand
// signed URLs to browsers or external clients that can only resolve the
// public endpoint.
type TOSConfig struct {
	Endpoint        string `mapstructure:"endpoint"`
	PublicEndpoint  string `mapstructure:"public_endpoint"`
	Region          string `mapstructure:"region"`
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
}

// TOSStorage implements Storage using Volcengine TOS.
type TOSStorage struct {
	client       *tos.ClientV2
	publicClient *tos.ClientV2 // nil when PublicEndpoint is unset; falls back to client
	bucket       string
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
	s := &TOSStorage{client: client, bucket: cfg.Bucket}
	if cfg.PublicEndpoint != "" && cfg.PublicEndpoint != cfg.Endpoint {
		publicClient, err := tos.NewClientV2(
			cfg.PublicEndpoint,
			tos.WithRegion(cfg.Region),
			tos.WithCredentials(tos.NewStaticCredentials(cfg.AccessKeyID, cfg.SecretAccessKey)),
		)
		if err != nil {
			return nil, fmt.Errorf("storage: create tos public client: %w", err)
		}
		s.publicClient = publicClient
	}
	return s, nil
}

// presignClient returns the client used for pre-signed URL generation. It
// falls back to the main client when PublicEndpoint is unset.
func (t *TOSStorage) presignClient() *tos.ClientV2 {
	if t.publicClient != nil {
		return t.publicClient
	}
	return t.client
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
	out, err := t.presignClient().PreSignedURL(&tos.PreSignedURLInput{
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
	out, err := t.presignClient().PreSignedURL(input)
	if err != nil {
		return "", wrapTOSError(err)
	}
	return out.SignedUrl, nil
}

func (t *TOSStorage) WithBucket(bucket string) Storage {
	return &TOSStorage{client: t.client, publicClient: t.publicClient, bucket: bucket}
}

func (t *TOSStorage) Ping(ctx context.Context) error {
	_, err := t.client.HeadBucket(ctx, &tos.HeadBucketInput{
		Bucket: t.bucket,
	})
	return wrapTOSError(err)
}

// wrapTOSError maps TOS SDK errors to the package's sentinel errors. It uses
// the SDK's typed helpers (tos.StatusCode / tos.Code) rather than matching on
// error strings, which is brittle and can misclassify errors whose message
// happens to contain substrings like "404".
func wrapTOSError(err error) error {
	if err == nil {
		return nil
	}
	// Prefer the server-side error Code when available — it's the most precise
	// signal (a 403 could be AccessDenied, SignatureDoesNotMatch, etc.).
	switch tos.Code(err) {
	case "NoSuchKey", "NoSuchBucket":
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case "AccessDenied":
		return fmt.Errorf("%w: %v", ErrAccessDenied, err)
	}
	// Fall back to HTTP status for errors without a parsed Code
	// (e.g. UnexpectedStatusCodeError).
	switch tos.StatusCode(err) {
	case 404:
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case 403:
		return fmt.Errorf("%w: %v", ErrAccessDenied, err)
	}
	return err
}
