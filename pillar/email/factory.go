package email

import "fmt"

// DefaultName is the key used by Default().
const DefaultName = "default"

// Config is the unified email configuration for a single instance.
type Config struct {
	Driver  string        `mapstructure:"driver"` // "smtp" or "tencentcloud"
	SMTP    SMTPConfig    `mapstructure:"smtp"`
	Tencent TencentConfig `mapstructure:"tencent"`
}

// SMTPConfig holds SMTP driver configuration.
type SMTPConfig struct {
	Host     string `mapstructure:"host"`     // SMTP server address
	Port     int    `mapstructure:"port"`     // Port (25/465/587), defaults to 587
	Username string `mapstructure:"username"` // Auth username
	Password string `mapstructure:"password"` // Auth password
	From     string `mapstructure:"from"`     // Default sender address
	TLS      bool   `mapstructure:"tls"`      // Enable direct TLS (for port 465)
}

// TencentConfig holds Tencent Cloud SES configuration.
type TencentConfig struct {
	SecretID   string `mapstructure:"secret_id"`
	SecretKey  string `mapstructure:"secret_key"`
	From       string `mapstructure:"from"`        // Sender address (must be verified in SES console)
	TemplateID string `mapstructure:"template_id"` // Default template ID
	Subject    string `mapstructure:"subject"`     // Default subject line (required by SES even in template mode)
	Region     string `mapstructure:"region"`      // Default: "ap-hongkong"
}

// newEmail creates an Email instance based on the driver specified in Config.
func newEmail(cfg Config) (Email, error) {
	switch cfg.Driver {
	case "smtp":
		return NewSMTP(cfg.SMTP)
	case "tencentcloud":
		return NewTencent(cfg.Tencent)
	default:
		return nil, fmt.Errorf("email: unsupported driver %q", cfg.Driver)
	}
}
