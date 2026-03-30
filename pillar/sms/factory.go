package sms

import (
	"fmt"
)

const DefaultName = "default"

// Config is the unified SMS configuration.
type Config struct {
	Driver  string        `mapstructure:"driver"` // "tencentcloud"
	Tencent TencentConfig `mapstructure:"tencent"`
}

// TencentConfig holds Tencent Cloud SMS configuration.
type TencentConfig struct {
	SecretID  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
	AppID     string `mapstructure:"app_id"` // SmsSdkAppId
	Sign      string `mapstructure:"sign"`   // Default SMS signature
	Region    string `mapstructure:"region"` // Default: "ap-guangzhou"
}

// newSMS creates an SMS instance based on the driver specified in Config.
func newSMS(cfg Config) (SMS, error) {
	switch cfg.Driver {
	case "tencentcloud":
		return NewTencent(cfg.Tencent)
	default:
		return nil, fmt.Errorf("sms: unsupported driver %q", cfg.Driver)
	}
}
