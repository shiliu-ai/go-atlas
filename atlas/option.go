package atlas

import (
	"github.com/gin-gonic/gin"

	"github.com/shiliu-ai/go-atlas/atlas/log"
)

// Option configures the Atlas instance before initialization.
type Option func(*Atlas)

// WithConfigName sets the config file name (without extension). Default: "config".
func WithConfigName(name string) Option {
	return func(a *Atlas) { a.configName = name }
}

// WithConfigPaths sets the directories to search for the config file.
// Default: ["."].
func WithConfigPaths(paths ...string) Option {
	return func(a *Atlas) { a.configPaths = paths }
}

// WithEnvPrefix sets the environment variable prefix. Default: "APP".
func WithEnvPrefix(prefix string) Option {
	return func(a *Atlas) { a.envPrefix = prefix }
}

// WithLogger overrides the auto-created logger.
func WithLogger(l log.Logger) Option {
	return func(a *Atlas) { a.logger = l }
}

// WithMiddleware appends additional Gin middleware to the default stack.
func WithMiddleware(mw ...gin.HandlerFunc) Option {
	return func(a *Atlas) { a.extraMiddleware = append(a.extraMiddleware, mw...) }
}

// WithoutDefaultMiddleware disables all default middleware registration.
// Useful when the service wants full control over middleware ordering.
func WithoutDefaultMiddleware() Option {
	return func(a *Atlas) { a.skipDefaultMW = true }
}
