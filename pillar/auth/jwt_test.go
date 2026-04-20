package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
