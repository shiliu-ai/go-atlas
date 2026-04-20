package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	aerrors "github.com/shiliu-ai/go-atlas/aether/errors"
)

// Package-level sentinel errors. All map to HTTP 401 via aether/response.
// Callers that need to differentiate token failure reasons (expired vs
// bad signature vs malformed) can do errors.Is against the jwt/v5 package
// sentinels (jwt.ErrTokenExpired etc.) — they are preserved in the cause
// chain.
var (
	ErrTokenMissing = aerrors.New(aerrors.CodeUnauthorized, "missing authorization token")
	ErrTokenInvalid = aerrors.New(aerrors.CodeUnauthorized, "invalid or expired token")
)

// Config holds JWT configuration.
type Config struct {
	Secret        string        `mapstructure:"secret"`
	Issuer        string        `mapstructure:"issuer"`
	AccessExpire  time.Duration `mapstructure:"access_expire"`
	RefreshExpire time.Duration `mapstructure:"refresh_expire"`
	SigningMethod string        `mapstructure:"signing_method"` // HS256 (default), HS384, HS512
	HeaderName    string        `mapstructure:"header_name"`    // custom header name, e.g. "X-Authorization-Token"; defaults to "Authorization" with Bearer prefix
}

// Claims extends jwt.RegisteredClaims with a custom UserID and optional metadata.
type Claims struct {
	UserID   string         `json:"uid"`
	Metadata map[string]any `json:"meta,omitempty"`
	jwt.RegisteredClaims
}

// JWT provides token signing and parsing.
type JWT struct {
	cfg    Config
	method jwt.SigningMethod
}

// TokenPair holds an access token and a refresh token.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// GeneratePair creates both an access token and a refresh token.
func (j *JWT) GeneratePair(userID string, metadata map[string]any) (*TokenPair, error) {
	now := time.Now()

	accessToken, err := j.generateToken(userID, metadata, now, j.cfg.AccessExpire)
	if err != nil {
		return nil, fmt.Errorf("auth: generate access token: %w", err)
	}

	refreshToken, err := j.generateToken(userID, metadata, now, j.cfg.RefreshExpire)
	if err != nil {
		return nil, fmt.Errorf("auth: generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    now.Add(j.cfg.AccessExpire).Unix(),
	}, nil
}

// GenerateAccess creates a single access token.
func (j *JWT) GenerateAccess(userID string, metadata map[string]any) (string, error) {
	return j.generateToken(userID, metadata, time.Now(), j.cfg.AccessExpire)
}

// Parse validates the token string and returns the claims. All validation
// failures (expired, bad signature, malformed, wrong claim shape) are
// returned as aerrors.CodeUnauthorized so aether/response maps them to
// HTTP 401. The underlying jwt/v5 sentinel is preserved via errors.Is —
// e.g. errors.Is(err, jwt.ErrTokenExpired) still works.
func (j *JWT) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != j.method.Alg() {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return []byte(j.cfg.Secret), nil
	})
	if err != nil {
		return nil, aerrors.Wrap(aerrors.CodeUnauthorized, err.Error(), err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

func (j *JWT) generateToken(userID string, metadata map[string]any, now time.Time, expire time.Duration) (string, error) {
	claims := Claims{
		UserID:   userID,
		Metadata: metadata,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expire)),
		},
	}

	token := jwt.NewWithClaims(j.method, claims)
	return token.SignedString([]byte(j.cfg.Secret))
}

func initJWT(cfg Config) *JWT {
	if cfg.AccessExpire == 0 {
		cfg.AccessExpire = 2 * time.Hour
	}
	if cfg.RefreshExpire == 0 {
		cfg.RefreshExpire = 7 * 24 * time.Hour
	}

	method := jwt.GetSigningMethod(cfg.SigningMethod)
	if method == nil {
		method = jwt.SigningMethodHS256
	}

	return &JWT{cfg: cfg, method: method}
}
