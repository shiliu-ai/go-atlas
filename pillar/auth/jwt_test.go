package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	aerrors "github.com/shiliu-ai/go-atlas/aether/errors"
)

func newTestJWT() *JWT {
	return initJWT(Config{
		Secret:        "test-secret",
		Issuer:        "test",
		AccessExpire:  time.Hour,
		RefreshExpire: 24 * time.Hour,
	})
}

func signWith(t *testing.T, j *JWT, claims jwt.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(j.method, claims)
	s, err := tok.SignedString([]byte(j.cfg.Secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestParse(t *testing.T) {
	j := newTestJWT()
	otherSecret := initJWT(Config{Secret: "different-secret"})

	cases := []struct {
		name    string
		build   func() string
		wantErr bool
		wantUID string
	}{
		{
			name: "expired standard token",
			build: func() string {
				past := time.Now().Add(-time.Hour)
				return signWith(t, j, Claims{
					UserID: "u1",
					RegisteredClaims: jwt.RegisteredClaims{
						IssuedAt:  jwt.NewNumericDate(past.Add(-time.Hour)),
						ExpiresAt: jwt.NewNumericDate(past),
					},
				})
			},
			wantErr: true,
		},
		{
			name: "valid standard token",
			build: func() string {
				tok, err := j.GenerateAccess("u1", map[string]any{"role": "admin"})
				if err != nil {
					t.Fatalf("generate: %v", err)
				}
				return tok
			},
			wantUID: "u1",
		},
		{
			name: "valid legacy numeric uid",
			build: func() string {
				return signWith(t, j, jwt.MapClaims{
					"uid":    float64(123),
					"expire": float64(time.Now().Add(time.Hour).UnixMilli()),
				})
			},
			wantUID: "123",
		},
		{
			name: "expired legacy token",
			build: func() string {
				return signWith(t, j, jwt.MapClaims{
					"uid":    float64(123),
					"expire": float64(time.Now().Add(-time.Hour).UnixMilli()),
				})
			},
			wantErr: true,
		},
		{
			name: "bad signature",
			build: func() string {
				return signWith(t, otherSecret, Claims{
					UserID: "u1",
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					},
				})
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := j.Parse(tc.build())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				// Parse errors must be aerrors.*Error with CodeUnauthorized
				// so response.Err maps them to HTTP 401 instead of falling
				// into the unhandled-error (500) branch.
				var ae *aerrors.Error
				if !errors.As(err, &ae) {
					t.Fatalf("Parse error is not *aerrors.Error: %T (%v)", err, err)
				}
				if ae.Code() != aerrors.CodeUnauthorized {
					t.Fatalf("Parse error code = %d, want %d", ae.Code(), aerrors.CodeUnauthorized)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if claims.UserID != tc.wantUID {
				t.Fatalf("uid = %q, want %q", claims.UserID, tc.wantUID)
			}
		})
	}
}

// TestParse_PreservesJWTSentinelInChain verifies that callers who want
// to differentiate "expired" from "bad signature" can still do so via
// errors.Is against the jwt/v5 sentinels — the atlas wrap preserves
// the cause chain.
func TestParse_PreservesJWTSentinelInChain(t *testing.T) {
	j := newTestJWT()
	other := initJWT(Config{Secret: "different-secret"})

	expired := signWith(t, j, Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	})
	_, err := j.Parse(expired)
	if err == nil {
		t.Fatalf("expected error for expired token")
	}
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Fatalf("errors.Is(err, jwt.ErrTokenExpired) = false; chain lost")
	}

	badSig := signWith(t, other, Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	_, err = j.Parse(badSig)
	if err == nil {
		t.Fatalf("expected error for bad signature")
	}
	if !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		t.Fatalf("errors.Is(err, jwt.ErrTokenSignatureInvalid) = false; chain lost")
	}
}
