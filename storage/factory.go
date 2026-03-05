package storage

import (
	"context"
	"fmt"
)

// Config is the unified storage configuration.
type Config struct {
	Driver string    `mapstructure:"driver"` // "s3", "cos", "oss", "tos"
	S3     S3Config  `mapstructure:"s3"`
	COS    COSConfig `mapstructure:"cos"`
	OSS    OSSConfig `mapstructure:"oss"`
	TOS    TOSConfig `mapstructure:"tos"`
}

// New creates a Storage instance based on the driver specified in Config.
func New(ctx context.Context, cfg Config) (Storage, error) {
	switch cfg.Driver {
	case "s3":
		return NewS3(ctx, cfg.S3)
	case "cos":
		return NewCOS(cfg.COS)
	case "oss":
		return NewOSS(cfg.OSS)
	case "tos":
		return NewTOS(cfg.TOS)
	default:
		return nil, fmt.Errorf("storage: unsupported driver %q", cfg.Driver)
	}
}

// Component wraps a Storage instance for app lifecycle integration.
type Component struct {
	Store Storage
}

func (c *Component) Name() string { return "storage" }
