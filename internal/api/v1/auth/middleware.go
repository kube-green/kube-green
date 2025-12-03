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

		path := c.Request.URL.Path
		method := c.Request.Method
		
		// Allow OPTIONS requests for CORS preflight
		if method == "OPTIONS" {
			c.Next()
			return
		}
		
		// Allow public paths - check this FIRST before any Authorization header validation
		publicPaths := []string{
			"/health",
			"/ready",
			"/api/v1/auth/login",
			"/api/v1/auth/refresh",
			"/swagger",
		}
		
		// Documentation paths - accessible without auth, but can accept token if provided
		documentationPaths := []string{
			"/docs",
			"/documentation",
		}
		
		// Check if path is public - this MUST be done before checking Authorization header
		// This allows login/refresh endpoints to work even if Swagger sends an Authorization header
		isPublic := false
		for _, publicPath := range publicPaths {
			if strings.HasPrefix(path, publicPath) {
				isPublic = true
				break
			}
		}
		
		if isPublic {
			// For public paths, skip authentication entirely - don't check Authorization header
			c.Next()
			return
		}
		
		// Check if path is documentation - allow access without auth, but validate token if provided
		isDocumentation := false
		for _, docPath := range documentationPaths {
			if path == docPath || strings.HasPrefix(path, docPath+"/") {
				isDocumentation = true
				break
			}
		}
		
		if isDocumentation {
			// For documentation, check if token is provided (optional)
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
				// Token provided - validate it and add user info to context
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					token := parts[1]
					if claims, err := ValidateToken(token, secret); err == nil {
						// Valid token - add user info to context
						c.Set("username", claims.Username)
						c.Set("role", claims.Role)
					}
					// If token is invalid, just continue without user info (still allow access)
				}
			}
			// Allow access to documentation regardless of token validity
			c.Next()
			return
		}

		// For protected paths, require Authorization header
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
		c.Set("role", claims.Role)
		c.Next()
	}
}

