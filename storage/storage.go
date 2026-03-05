package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Standard storage errors for unified error handling across providers.
var (
	ErrNotFound     = errors.New("storage: object not found")
	ErrAccessDenied = errors.New("storage: access denied")
)

// ObjectInfo represents metadata about a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	LastModified time.Time
}

// ListInput defines parameters for listing objects.
type ListInput struct {
	Prefix    string
	Marker    string // continuation token for pagination
	MaxKeys   int
	Delimiter string // e.g. "/" for directory-like listing
}

// ListOutput holds the result of a list operation.
type ListOutput struct {
	Objects        []ObjectInfo
	NextMarker     string // use as Marker in next call for pagination
	IsTruncated    bool
	CommonPrefixes []string // populated when Delimiter is set
}

// Storage is the object storage interface, compatible with S3-like services
// (AWS S3, Tencent COS, Aliyun OSS, Volcengine TOS, MinIO, etc.).
type Storage interface {
	// Put uploads an object.
	Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	// Get downloads an object.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes an object.
	Delete(ctx context.Context, key string) error
	// Head returns object metadata without downloading.
	Head(ctx context.Context, key string) (*ObjectInfo, error)
	// Exists checks whether an object exists.
	Exists(ctx context.Context, key string) (bool, error)
	// List lists objects with the given prefix and pagination.
	List(ctx context.Context, input *ListInput) (*ListOutput, error)
	// Copy copies an object from srcKey to dstKey server-side.
	Copy(ctx context.Context, srcKey, dstKey string) error
	// SignURL generates a pre-signed GET URL for temporary download access.
	SignURL(ctx context.Context, key string, expire time.Duration) (string, error)
	// SignPutURL generates a pre-signed PUT URL for temporary upload access.
	SignPutURL(ctx context.Context, key string, contentType string, expire time.Duration) (string, error)
	// WithBucket returns a new Storage instance bound to a different bucket,
	// sharing the underlying client connection.
	WithBucket(bucket string) Storage
	// Ping verifies connectivity to the storage backend.
	Ping(ctx context.Context) error
}
