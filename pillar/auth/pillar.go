package auth

import (
	"context"
	"fmt"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the auth Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		j := &JWT{}
		for _, opt := range opts {
			opt(j)
		}
		a.Register(j)
	}
}

// Of retrieves the JWT from an Atlas instance.
func Of(a *atlas.Atlas) *JWT {
	return atlas.Use[*JWT](a)
}

// Option configures the auth Pillar.
type Option func(*JWT)

// Ensure interface compliance.
var _ atlas.Pillar = (*JWT)(nil)

func (j *JWT) Name() string { return "auth" }

func (j *JWT) Init(core *atlas.Core) error {
	var cfg Config
	if err := core.Unmarshal("auth", &cfg); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	initialized := initJWT(cfg)
	j.cfg = initialized.cfg
	j.method = initialized.method
	return nil
}

func (j *JWT) Stop(_ context.Context) error {
	return nil
}
