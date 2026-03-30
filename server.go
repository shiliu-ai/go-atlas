package atlas

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// serverConfig holds HTTP server configuration.
type serverConfig struct {
	Port            int           `mapstructure:"port"`
	Name            string        `mapstructure:"name"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	Mode            string        `mapstructure:"mode"`
}

// server is an internal HTTP server wrapping Gin.
type server struct {
	engine *gin.Engine
	srv    *http.Server
	cfg    serverConfig
}

func newServer(cfg serverConfig) *server {
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

	return &server{
		engine: engine,
		cfg:    cfg,
	}
}

func (s *server) start() error {
	s.srv = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Port),
		Handler:      s.engine,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
	}

	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("atlas: server listen: %w", err)
	}
	return nil
}

func (s *server) shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.srv.Shutdown(shutdownCtx)
}

func (s *server) group(middlewares ...gin.HandlerFunc) *gin.RouterGroup {
	basePath := s.cfg.Name
	if basePath == "" {
		basePath = "/"
	}
	return s.engine.Group(basePath, middlewares...)
}
