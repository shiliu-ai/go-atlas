package auth

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/atlas/errors"
	"github.com/shiliu-ai/go-atlas/atlas/response"
)

type contextKeyType struct{}

// Middleware returns a Gin middleware that validates the Authorization Bearer token,
// parses it with the given JWT instance, and injects Claims into the request context.
func (j *JWT) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractToken(c, j.cfg.HeaderName)
		if tokenStr == "" {
			response.Fail(c, errors.CodeUnauthorized, "missing authorization token")
			c.Abort()
			return
		}

		claims, err := j.Parse(tokenStr)
		if err != nil {
			response.Fail(c, errors.CodeUnauthorized, "invalid or expired token")
			c.Abort()
			return
		}

		ctx := context.WithValue(c.Request.Context(), contextKeyType{}, claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ClaimsFromContext extracts Claims from context. Returns nil if not present.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(contextKeyType{}).(*Claims)
	return claims
}

// UserIDFromContext is a convenience helper to extract UserID from context.
func UserIDFromContext(ctx context.Context) string {
	if claims := ClaimsFromContext(ctx); claims != nil {
		return claims.UserID
	}
	return ""
}

func extractToken(c *gin.Context, headerName string) string {
	if headerName != "" {
		return c.GetHeader(headerName)
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return ""
}
