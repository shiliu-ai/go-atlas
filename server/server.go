package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Config holds HTTP server configuration.
type Config struct {
	Port            int           `mapstructure:"port"`
	Name            string        `mapstructure:"name"` // service name used as route prefix, e.g. "/myservice"
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	Mode            string        `mapstructure:"mode"` // debug, release, test
}

// Server wraps Gin engine and http.Server.
type Server struct {
	engine *gin.Engine
	srv    *http.Server
	cfg    Config
}

// New creates a new Server.
func New(cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	if cfg.Mode != "" {
		gin.SetMode(cfg.Mode)
	}

	engine := gin.New()

	return &Server{
		engine: engine,
		cfg:    cfg,
	}
}

// Engine returns the underlying Gin engine for route registration.
func (s *Server) Engine() *gin.Engine { return s.engine }

// Group returns a RouterGroup based on the configured BasePath.
// If BasePath is empty, it returns the root group "/".
func (s *Server) Group(middlewares ...gin.HandlerFunc) *gin.RouterGroup {
	basePath := s.cfg.Name
	if basePath == "" {
		basePath = "/"
	}
	return s.engine.Group(basePath, middlewares...)
}

// Start starts the HTTP server. It blocks until the server stops.
func (s *Server) Start() error {
	s.srv = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Port),
		Handler:      s.engine,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
	}

	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: listen: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.srv.Shutdown(shutdownCtx)
}
