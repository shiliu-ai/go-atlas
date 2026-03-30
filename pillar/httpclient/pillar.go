package httpclient

import (
	"context"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the httpclient Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		c := &Client{}
		for _, opt := range opts {
			opt(c)
		}
		a.Register(c)
	}
}

// Of retrieves the Client from an Atlas instance.
func Of(a *atlas.Atlas) *Client {
	return atlas.Use[*Client](a)
}

// Option configures the httpclient Pillar.
type Option func(*Client)

// Ensure interface compliance.
var _ atlas.Pillar = (*Client)(nil)

func (c *Client) Name() string { return "httpclient" }

func (c *Client) Init(core *atlas.Core) error {
	var cfg Config
	// httpclient config is optional — use defaults if missing.
	_ = core.Unmarshal("httpclient", &cfg)

	logger := core.Logger("httpclient")
	initialized := NewClient(cfg, logger)
	c.http = initialized.http
	c.cfg = initialized.cfg
	c.logger = initialized.logger
	return nil
}

func (c *Client) Stop(_ context.Context) error {
	return nil
}
