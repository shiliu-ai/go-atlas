package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config holds JWT configuration.
type Config struct {
	Secret          string        `mapstructure:"secret"`
	Issuer          string        `mapstructure:"issuer"`
	AccessExpire    time.Duration `mapstructure:"access_expire"`
	RefreshExpire   time.Duration `mapstructure:"refresh_expire"`
	SigningMethod   string        `mapstructure:"signing_method"` // HS256 (default), HS384, HS512
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

// New creates a JWT instance.
func New(cfg Config) *JWT {
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

	refreshToken, err := j.generateToken(userID, nil, now, j.cfg.RefreshExpire)
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
func (j *JWT) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != j.method.Alg() {
			return nil, fmt.Errorf("auth: unexpected signing method %s", t.Header["alg"])
		}
		return []byte(j.cfg.Secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("auth: parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("auth: invalid token claims")
	}
	return claims, nil
}

// Refresh takes a valid refresh token and returns a new token pair.
func (j *JWT) Refresh(refreshToken string) (*TokenPair, error) {
	claims, err := j.Parse(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid refresh token: %w", err)
	}
	return j.GeneratePair(claims.UserID, nil)
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
