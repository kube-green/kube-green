/*
Copyright 2025.
*/

package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTAuthMiddleware creates a middleware that validates JWT tokens
func JWTAuthMiddleware(secret []byte, enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// If auth is disabled, skip validation
		if !enabled {
			c.Next()
			return
		}

		// Allow public paths
		publicPaths := []string{
			"/health",
			"/ready",
			"/api/v1/auth/login",
			"/api/v1/auth/refresh",
			"/swagger",
		}

		path := c.Request.URL.Path
		for _, publicPath := range publicPaths {
			if strings.HasPrefix(path, publicPath) {
				c.Next()
				return
			}
		}

		// Extract token from header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header required",
			})
			c.Abort()
			return
		}

		// Format: "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid authorization header format. Expected: Bearer <token>",
			})
			c.Abort()
			return
		}

		token := parts[1]

		// Validate token
		claims, err := ValidateToken(token, secret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Add user information to context
		c.Set("username", claims.Username)
		c.Next()
	}
}

