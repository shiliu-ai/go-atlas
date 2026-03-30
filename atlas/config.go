package atlas

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// coreConfig holds only what Atlas Core needs (server, log, i18n, middleware).
type coreConfig struct {
	Server     serverConfig     `mapstructure:"server"`
	Log        logConfig        `mapstructure:"log"`
	I18n       i18nConfig       `mapstructure:"i18n"`
	Middleware middlewareConfig  `mapstructure:"middleware"`
}

// logConfig holds logging configuration.
type logConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// i18nConfig holds i18n configuration.
type i18nConfig struct {
	Default string `mapstructure:"default"`
}

// middlewareConfig holds middleware configuration.
type middlewareConfig struct {
	CORS      corsConfig      `mapstructure:"cors"`
	RateLimit rateLimitCfg    `mapstructure:"rate_limit"`
}

// rateLimitCfg holds rate limiter YAML-friendly configuration.
type rateLimitCfg struct {
	Rate   int           `mapstructure:"rate"`
	Window time.Duration `mapstructure:"window"`
}

// decoderWithSquash enables mapstructure squash so that embedded structs
// (tagged with `mapstructure:",squash"`) are flattened during decode.
func decoderWithSquash(dc *mapstructure.DecoderConfig) {
	dc.Squash = true
}

// loadConfig reads configuration from file and environment variables using Viper.
// It returns the raw viper instance for Pillar config access plus the decoded coreConfig.
func loadConfig(configName string, paths []string, envPrefix string) (*viper.Viper, coreConfig, error) {
	v := viper.New()
	v.SetConfigName(configName)

	for _, p := range paths {
		v.AddConfigPath(p)
	}

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, coreConfig{}, fmt.Errorf("atlas: read config: %w", err)
		}
	}

	var cfg coreConfig
	if err := v.Unmarshal(&cfg, decoderWithSquash); err != nil {
		return nil, coreConfig{}, fmt.Errorf("atlas: unmarshal core config: %w", err)
	}

	return v, cfg, nil
}
