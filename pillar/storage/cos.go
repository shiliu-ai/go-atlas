package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// COSConfig holds Tencent Cloud COS configuration.
type COSConfig struct {
	BucketURL  string        `mapstructure:"bucket_url"`
	ServiceURL string        `mapstructure:"service_url"`
	SecretID   string        `mapstructure:"secret_id"`
	SecretKey  string        `mapstructure:"secret_key"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

// COSStorage implements Storage using Tencent Cloud COS.
type COSStorage struct {
	client    *cos.Client
	cfg       COSConfig
	bucketURL *url.URL
}

// NewCOS creates a new Tencent Cloud COS storage client.
func NewCOS(cfg COSConfig) (*COSStorage, error) {
	bucketURL, err := url.Parse(cfg.BucketURL)
	if err != nil {
		return nil, fmt.Errorf("storage: parse cos bucket url: %w", err)
	}

	var serviceURL *url.URL
	if cfg.ServiceURL != "" {
		serviceURL, err = url.Parse(cfg.ServiceURL)
		if err != nil {
			return nil, fmt.Errorf("storage: parse cos service url: %w", err)
		}
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := cos.NewClient(
		&cos.BaseURL{BucketURL: bucketURL, ServiceURL: serviceURL},
		&http.Client{
			Timeout: timeout,
			Transport: &cos.AuthorizationTransport{
				SecretID:  cfg.SecretID,
				SecretKey: cfg.SecretKey,
			},
		},
	)

	return &COSStorage{
		client:    client,
		cfg:       cfg,
		bucketURL: bucketURL,
	}, nil
}

func (c *COSStorage) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType:   contentType,
			ContentLength: size,
		},
	}
	_, err := c.client.Object.Put(ctx, key, reader, opt)
	return wrapCOSError(err)
}

func (c *COSStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := c.client.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, wrapCOSError(err)
	}
	return resp.Body, nil
}

func (c *COSStorage) Delete(ctx context.Context, key string) error {
	_, err := c.client.Object.Delete(ctx, key)
	return wrapCOSError(err)
}

func (c *COSStorage) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	resp, err := c.client.Object.Head(ctx, key, nil)
	if err != nil {
		return nil, wrapCOSError(err)
	}
	info := &ObjectInfo{
		Key:         key,
		ContentType: resp.Header.Get("Content-Type"),
	}
	info.Size = resp.ContentLength
	if t, e := time.Parse(http.TimeFormat, resp.Header.Get("Last-Modified")); e == nil {
		info.LastModified = t
	}
	return info, nil
}

func (c *COSStorage) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := c.client.Object.IsExist(ctx, key)
	if err != nil {
		return false, wrapCOSError(err)
	}
	return ok, nil
}

func (c *COSStorage) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	opt := &cos.BucketGetOptions{
		Prefix:    input.Prefix,
		Marker:    input.Marker,
		Delimiter: input.Delimiter,
	}
	if input.MaxKeys > 0 {
		opt.MaxKeys = input.MaxKeys
	}

	out, _, err := c.client.Bucket.Get(ctx, opt)
	if err != nil {
		return nil, wrapCOSError(err)
	}

	result := &ListOutput{
		IsTruncated: out.IsTruncated,
		NextMarker:  out.NextMarker,
	}
	for _, obj := range out.Contents {
		lm, _ := time.Parse(time.RFC3339, obj.LastModified)
		result.Objects = append(result.Objects, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: lm,
		})
	}
	result.CommonPrefixes = out.CommonPrefixes
	return result, nil
}

func (c *COSStorage) Copy(ctx context.Context, srcKey, dstKey string) error {
	source := fmt.Sprintf("%s/%s", c.bucketURL.Host, srcKey)
	_, _, err := c.client.Object.Copy(ctx, dstKey, source, nil)
	return wrapCOSError(err)
}

func (c *COSStorage) SignURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	presignedURL, err := c.client.Object.GetPresignedURL(ctx, http.MethodGet, key, c.cfg.SecretID, c.cfg.SecretKey, expire, nil)
	if err != nil {
		return "", wrapCOSError(err)
	}
	return presignedURL.String(), nil
}

func (c *COSStorage) SignPutURL(ctx context.Context, key string, contentType string, expire time.Duration) (string, error) {
	opt := &cos.PresignedURLOptions{}
	if contentType != "" {
		opt.Header = &http.Header{}
		opt.Header.Set("Content-Type", contentType)
	}
	presignedURL, err := c.client.Object.GetPresignedURL(ctx, http.MethodPut, key, c.cfg.SecretID, c.cfg.SecretKey, expire, opt)
	if err != nil {
		return "", wrapCOSError(err)
	}
	return presignedURL.String(), nil
}

func (c *COSStorage) WithBucket(bucket string) Storage {
	// COS bucket URL format: https://<bucket>-<appid>.cos.<region>.myqcloud.com
	// Extract appid and region from existing URL, replace bucket name.
	host := c.bucketURL.Host
	// host = "<bucket>-<appid>.cos.<region>.myqcloud.com"
	parts := strings.SplitN(host, ".cos.", 2)
	if len(parts) != 2 {
		// fallback: cannot parse, return a copy with new bucket in URL
		newCfg := c.cfg
		newCfg.BucketURL = strings.Replace(c.cfg.BucketURL, c.bucketURL.Host, bucket+".cos."+host, 1)
		s, err := NewCOS(newCfg)
		if err != nil {
			panic(fmt.Sprintf("storage: WithBucket(%q): %v", bucket, err))
		}
		return s
	}
	// parts[0] = "<bucket>-<appid>", parts[1] = "<region>.myqcloud.com"
	bucketAppID := parts[0]
	// find last "-" to separate bucket from appid
	idx := strings.LastIndex(bucketAppID, "-")
	if idx < 0 {
		panic(fmt.Sprintf("storage: WithBucket(%q): cannot parse appid from host %q", bucket, host))
	}
	appID := bucketAppID[idx+1:]
	newHost := fmt.Sprintf("%s-%s.cos.%s", bucket, appID, parts[1])
	newCfg := c.cfg
	newCfg.BucketURL = fmt.Sprintf("%s://%s", c.bucketURL.Scheme, newHost)
	s, err := NewCOS(newCfg)
	if err != nil {
		panic(fmt.Sprintf("storage: WithBucket(%q): %v", bucket, err))
	}
	return s
}

func (c *COSStorage) Ping(ctx context.Context) error {
	_, err := c.client.Bucket.Head(ctx)
	return wrapCOSError(err)
}

func wrapCOSError(err error) error {
	if err == nil {
		return nil
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "NoSuchKey") || strings.Contains(errMsg, "404") {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if strings.Contains(errMsg, "AccessDenied") || strings.Contains(errMsg, "403") {
		return fmt.Errorf("%w: %v", ErrAccessDenied, err)
	}
	return err
}
