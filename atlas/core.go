package atlas

import (
	"fmt"

	"github.com/shiliu-ai/go-atlas/atlas/log"
	"github.com/spf13/viper"
)

// Core is the limited view of Atlas exposed to Pillars during Init.
// It provides access to configuration and logging without exposing
// the full Atlas instance.
type Core struct {
	config *viper.Viper
	logger log.Logger
}

// newCore creates a Core from a viper config and logger.
// This is intended for internal use by Atlas.
func newCore(config *viper.Viper, logger log.Logger) *Core {
	return &Core{config: config, logger: logger}
}

// Unmarshal deserializes the config section at key into target.
func (c *Core) Unmarshal(key string, target any) error {
	sub := c.config.Sub(key)
	if sub == nil {
		return fmt.Errorf("atlas: config section %q not found", key)
	}
	return sub.Unmarshal(target)
}

// Logger returns a sub-logger with the given name prefix.
func (c *Core) Logger(name string) log.Logger {
	return c.logger.WithFields(log.F("pillar", name))
}
