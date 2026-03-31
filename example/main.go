// Package main demonstrates a complete Atlas service.
//
// Atlas organizes code into four domains (see README for details):
//
//	atlas/    — Core framework: config, server, lifecycle
//	aether/   — Built-in essentials: errors, response (always available, no setup)
//	pillar/   — Pluggable modules: auth, database, cache... (opt-in via Pillar())
//	artifact/ — Standalone utilities: crypto, id, pagination (no framework dependency)
package main

import (
	"fmt"

	"github.com/gin-gonic/gin"

	// Artifact — standalone utilities, usable in any Go project.
	"github.com/shiliu-ai/go-atlas/artifact/crypto"
	"github.com/shiliu-ai/go-atlas/artifact/id"
	"github.com/shiliu-ai/go-atlas/artifact/pagination"
	"github.com/shiliu-ai/go-atlas/artifact/validate"

	// Atlas core — the framework itself.
	"github.com/shiliu-ai/go-atlas/atlas"

	// Aether — built-in essentials, always available without registration.
	"github.com/shiliu-ai/go-atlas/aether/errors"
	"github.com/shiliu-ai/go-atlas/aether/response"

	// Pillar — pluggable infrastructure modules, registered via Pillar().
	"github.com/shiliu-ai/go-atlas/pillar/auth"
	"github.com/shiliu-ai/go-atlas/pillar/httpclient"
	"github.com/shiliu-ai/go-atlas/pillar/serviceclient"
)

func main() {
	// Step 1 — Register Pillars: declare which infrastructure modules this service needs.
	// Each xxx.Pillar() is an atlas.Option that registers a module.
	// Aether (errors, response, log) and Artifact (id, crypto) need no registration.
	a := atlas.New("example-service",
		atlas.WithConfigPaths(".", "./example"),
		auth.Pillar(),          // JWT authentication
		httpclient.Pillar(),    // production-ready HTTP client
		serviceclient.Pillar(), // typed inter-service RPC
	)

	// Step 2 — Retrieve instances: use xxx.Of(a) to get initialized Pillars.
	jwt := auth.Of(a)
	authMW := jwt.Middleware()
	hc := httpclient.Of(a)
	svcm := serviceclient.Of(a)

	// Artifact usage — no registration needed, works like any Go library.
	snowflake, err := id.NewSnowflake(1)
	if err != nil {
		panic(err)
	}

	// Step 3 — Define routes.
	a.Route(func(r *gin.RouterGroup) {
		v1 := r.Group("/v1")

		// --- Public routes ---

		v1.GET("/health", func(c *gin.Context) {
			// Aether: response is always available, no setup needed.
			response.OK(c, gin.H{"status": "ok"})
		})

		// Login: validate, hash-check password, return JWT token pair.
		type LoginReq struct {
			Username string `json:"username" binding:"required,min=3"`
			Password string `json:"password" binding:"required,min=6"`
		}
		v1.POST("/login", func(c *gin.Context) {
			var req LoginReq
			// Artifact: validate works standalone.
			if !validate.BindJSON(c, &req) {
				return
			}
			// Artifact: crypto works standalone.
			// In real app: crypto.CheckPassword(user.HashedPassword, req.Password)
			_ = crypto.CheckPassword
			// Pillar: auth generates JWT tokens.
			pair, err := jwt.GeneratePair(req.Username, nil)
			if err != nil {
				// Aether: errors provides structured error codes.
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
			pair, err := jwt.Refresh(req.RefreshToken)
			if err != nil {
				response.Fail(c, errors.CodeUnauthorized, "invalid refresh token")
				return
			}
			response.OK(c, pair)
		})

		// --- Protected routes (Pillar: auth middleware) ---

		authorized := v1.Group("/api", authMW)

		// Get current user info from JWT claims.
		authorized.GET("/me", func(c *gin.Context) {
			claims := auth.ClaimsFromContext(c.Request.Context())
			response.OK(c, gin.H{
				"user_id": claims.UserID,
			})
		})

		// Paginated list with snowflake IDs.
		authorized.GET("/items", func(c *gin.Context) {
			// Artifact: pagination works standalone.
			pg := pagination.FromContext(c)

			items := make([]gin.H, 0, pg.Size)
			for i := range pg.Size {
				items = append(items, gin.H{
					// Artifact: id.Snowflake works standalone.
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
			// Pillar: httpclient with retries and trace propagation.
			resp, err := hc.Get(c.Request.Context(), rawURL)
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

			// Pillar: serviceclient for typed inter-service RPC.
			userSvc := svcm.MustGet("user-service")

			// Typed call: automatically unwraps R{code, message, data}.
			var user struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if err := serviceclient.Get(c.Request.Context(), userSvc, "/v1/users/"+userID, &user); err != nil {
				response.Err(c, err)
				return
			}
			response.OK(c, user)
		})

		// ID generation demo.
		authorized.GET("/id", func(c *gin.Context) {
			// Artifact: all ID generators work standalone, no Pillar needed.
			response.OK(c, gin.H{
				"snowflake":  fmt.Sprintf("%d", snowflake.MustGenerate()),
				"uuid":       id.UUID(),
				"nanoid":     id.NanoID(),
				"short_id":   id.ShortID(),
				"numeric_id": id.NumericID(),
			})
		})
	})

	// Start the server.
	a.MustRun()
}
