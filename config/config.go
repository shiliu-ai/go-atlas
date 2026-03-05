package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Load reads configuration from file and environment variables.
// It searches for configName (without extension) in the given paths.
// Environment variables override file values; prefix is used for env var namespace.
func Load(configName string, paths []string, envPrefix string, target any) error {
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
			return fmt.Errorf("config: read config: %w", err)
		}
	}

	if target != nil {
		if err := v.Unmarshal(target); err != nil {
			return fmt.Errorf("config: unmarshal: %w", err)
		}
	}
	return nil
}

// LoadFromFile reads configuration from a specific file path.
func LoadFromFile(filePath string, target any) error {
	v := viper.New()
	v.SetConfigFile(filePath)

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("config: read file %s: %w", filePath, err)
	}

	if target != nil {
		if err := v.Unmarshal(target); err != nil {
			return fmt.Errorf("config: unmarshal: %w", err)
		}
	}
	return nil
}
