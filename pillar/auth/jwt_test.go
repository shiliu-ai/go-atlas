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

// TestParse covers the happy path and every validation failure. All
// failures must surface as *aerrors.Error with CodeUnauthorized so the
// response layer maps them to HTTP 401.
func TestParse(t *testing.T) {
	j := newTestJWT()
	other := initJWT(Config{Secret: "different-secret"})

	cases := []struct {
		name    string
		build   func() string
		wantErr bool
		wantUID string
	}{
		{
			name: "valid token",
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
			name: "expired token",
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
			name: "bad signature",
			build: func() string {
				return signWith(t, other, Claims{
					UserID: "u1",
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					},
				})
			},
			wantErr: true,
		},
		{
			name:    "malformed",
			build:   func() string { return "not.a.jwt" },
			wantErr: true,
		},
		{
			// Any token shape without a standard "exp" claim must be rejected.
			// This covers legacy {"uid","expire"} tokens that predate the
			// structured Claims format — WithExpirationRequired kicks them
			// out so clients re-authenticate and get a current token.
			name: "no exp claim (legacy / never-expiring)",
			build: func() string {
				return signWith(t, j, jwt.MapClaims{"uid": "u1"})
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

// TestParse_PreservesJWTSentinelInChain verifies that the cause chain
// preserves the jwt/v5 sentinels — callers who want to differentiate
// expired vs bad-signature can still do so via errors.Is.
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
	if _, err := j.Parse(expired); !errors.Is(err, jwt.ErrTokenExpired) {
		t.Fatalf("errors.Is(err, jwt.ErrTokenExpired) = false; chain lost: %v", err)
	}

	badSig := signWith(t, other, Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	if _, err := j.Parse(badSig); !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		t.Fatalf("errors.Is(err, jwt.ErrTokenSignatureInvalid) = false; chain lost: %v", err)
	}
}
