package main

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/shiliu-ai/go-atlas/atlas"
	"github.com/shiliu-ai/go-atlas/auth"
	"github.com/shiliu-ai/go-atlas/crypto"
	"github.com/shiliu-ai/go-atlas/errors"
	"github.com/shiliu-ai/go-atlas/id"
	"github.com/shiliu-ai/go-atlas/pagination"
	"github.com/shiliu-ai/go-atlas/response"
	"github.com/shiliu-ai/go-atlas/serviceclient"
	"github.com/shiliu-ai/go-atlas/validate"
)

func main() {
	a := atlas.New("example-service",
		atlas.WithConfigPaths(".", "./example"),
	)

	// Snowflake ID generator.
	snowflake, err := id.NewSnowflake(1)
	if err != nil {
		panic(err)
	}

	// Base route group: /<name>/v1
	v1 := a.Group().Group("/v1")

	// --- Public routes ---

	v1.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"status": "ok"})
	})

	// Login: validate, hash-check password, return JWT token pair.
	type LoginReq struct {
		Username string `json:"username" binding:"required,min=3"`
		Password string `json:"password" binding:"required,min=6"`
	}
	v1.POST("/login", func(c *gin.Context) {
		var req LoginReq
		if !validate.BindJSON(c, &req) {
			return
		}
		// In real app, fetch hashed password from DB and verify:
		//   crypto.CheckPassword(user.HashedPassword, req.Password)
		_ = crypto.CheckPassword
		pair, err := a.Auth().GeneratePair(req.Username, nil)
		if err != nil {
			response.Fail(c, errors.CodeInternal, "token generation failed")
			return
		}
		response.OK(c, pair)
	})

	// Refresh token.
	type RefreshReq struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	v1.POST("/refresh", func(c *gin.Context) {
		var req RefreshReq
		if !validate.BindJSON(c, &req) {
			return
		}
		pair, err := a.Auth().Refresh(req.RefreshToken)
		if err != nil {
			response.Fail(c, errors.CodeUnauthorized, "invalid refresh token")
			return
		}
		response.OK(c, pair)
	})

	// --- Protected routes ---

	authorized := v1.Group("/api", auth.Middleware(a.Auth()))

	// Get current user info from JWT claims.
	authorized.GET("/me", func(c *gin.Context) {
		claims := auth.ClaimsFromContext(c.Request.Context())
		response.OK(c, gin.H{
			"user_id": claims.UserID,
		})
	})

	// Paginated list with snowflake IDs.
	authorized.GET("/items", func(c *gin.Context) {
		pg := pagination.FromContext(c)

		items := make([]gin.H, 0, pg.Size)
		for i := range pg.Size {
			items = append(items, gin.H{
				"id":   fmt.Sprintf("%d", snowflake.MustGenerate()),
				"name": fmt.Sprintf("item-%d", pg.Offset()+i+1),
			})
		}
		response.OK(c, pagination.NewResponse(items, 100, pg))
	})

	// Proxy: call an external API via httpclient.
	authorized.GET("/proxy", func(c *gin.Context) {
		rawURL := c.Query("url")
		if rawURL == "" {
			response.Fail(c, errors.CodeBadRequest, "url is required")
			return
		}
		resp, err := a.HTTPClient().Get(c.Request.Context(), rawURL)
		if err != nil {
			response.Fail(c, errors.CodeBadGateway, "upstream request failed")
			return
		}
		response.OK(c, gin.H{
			"status": resp.StatusCode,
			"body":   resp.String(),
		})
	})

	// Inter-service call demo: call another atlas-based service.
	// Requires "services.user-service" in config.yaml:
	//   services:
	//     user-service:
	//       base_url: "http://user-service:8080/user-service"
	authorized.GET("/user/:id", func(c *gin.Context) {
		userID := c.Param("id")

		// Typed call: automatically unwraps R{code, message, data}.
		var user struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := serviceclient.GetJSON(c.Request.Context(), a.Service("user-service"), "/v1/users/"+userID, &user); err != nil {
			response.Err(c, err)
			return
		}
		response.OK(c, user)
	})

	// ID generation demo.
	authorized.GET("/id", func(c *gin.Context) {
		response.OK(c, gin.H{
			"snowflake": fmt.Sprintf("%d", snowflake.MustGenerate()),
			"uuid":      id.UUID(),
			"nanoid":    id.NanoID(),
		})
	})

	a.MustRun()
}
