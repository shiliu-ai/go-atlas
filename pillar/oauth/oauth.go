package oauth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
)

// ProviderConfig holds OAuth2 provider configuration.
type ProviderConfig struct {
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	RedirectURL  string   `mapstructure:"redirect_url"`
	AuthURL      string   `mapstructure:"auth_url"`
	TokenURL     string   `mapstructure:"token_url"`
	Scopes       []string `mapstructure:"scopes"`
}

// Provider wraps an OAuth2 config for a specific identity provider.
type Provider struct {
	name   string
	config oauth2.Config
}

// NewProvider creates an OAuth2 provider.
func NewProvider(name string, cfg ProviderConfig) *Provider {
	return &Provider{
		name: name,
		config: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:  cfg.AuthURL,
				TokenURL: cfg.TokenURL,
			},
			Scopes: cfg.Scopes,
		},
	}
}

// AuthCodeURL returns the URL for the authorization code flow.
func (p *Provider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return p.config.AuthCodeURL(state, opts...)
}

// Exchange trades an authorization code for a token.
func (p *Provider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := p.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth: %s exchange: %w", p.name, err)
	}
	return token, nil
}

// Client returns an HTTP client with the given token for making authenticated API calls.
func (p *Provider) Client(ctx context.Context, token *oauth2.Token) *oauth2.Transport {
	return &oauth2.Transport{
		Source: p.config.TokenSource(ctx, token),
	}
}

// ProviderName returns the provider name.
func (p *Provider) ProviderName() string { return p.name }
