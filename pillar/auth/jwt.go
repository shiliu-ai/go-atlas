package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

// Parse validates the token string and returns the claims.
// It first tries structured Claims parsing; if that fails, it falls back
// to MapClaims to support legacy tokens (e.g. {"uid": 123, "expire": ...}).
func (j *JWT) Parse(tokenStr string) (*Claims, error) {
	keyFunc := func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != j.method.Alg() {
			return nil, fmt.Errorf("auth: unexpected signing method %s", t.Header["alg"])
		}
		return []byte(j.cfg.Secret), nil
	}

	// Try structured Claims first.
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, keyFunc)
	if err == nil {
		claims, ok := token.Claims.(*Claims)
		if ok && token.Valid {
			return claims, nil
		}
	}

	// Fallback: parse as MapClaims (skip exp validation for legacy tokens).
	mapToken, mapErr := jwt.Parse(tokenStr, keyFunc, jwt.WithoutClaimsValidation())
	if mapErr != nil {
		return nil, fmt.Errorf("auth: parse token: %w", mapErr)
	}
	mc, ok := mapToken.Claims.(jwt.MapClaims)
	if !ok || !mapToken.Valid {
		return nil, fmt.Errorf("auth: invalid token claims")
	}

	claims := &Claims{}

	// Extract uid (string or numeric).
	switch v := mc["uid"].(type) {
	case string:
		claims.UserID = v
	case float64:
		claims.UserID = fmt.Sprintf("%d", int64(v))
	}

	// Check legacy "expire" field (milliseconds timestamp).
	if exp, ok := mc["expire"].(float64); ok {
		expTime := time.UnixMilli(int64(exp))
		if time.Now().After(expTime) {
			return nil, fmt.Errorf("auth: parse token: token is expired")
		}
		claims.ExpiresAt = jwt.NewNumericDate(expTime)
	}

	// Collect remaining fields as metadata.
	meta := make(map[string]any)
	for k, v := range mc {
		if k == "uid" || k == "expire" {
			continue
		}
		meta[k] = v
	}
	if len(meta) > 0 {
		claims.Metadata = meta
	}

	return claims, nil
}

// Refresh takes a valid refresh token and returns a new token pair.
func (j *JWT) Refresh(refreshToken string) (*TokenPair, error) {
	claims, err := j.Parse(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid refresh token: %w", err)
	}
	return j.GeneratePair(claims.UserID, claims.Metadata)
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
