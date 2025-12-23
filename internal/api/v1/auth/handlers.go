/*
Copyright 2025.
*/

package auth

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest represents a refresh token request
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	userStore     *UserStore
	jwtSecret     []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// GetJWTSecret returns the JWT secret
func (h *AuthHandler) GetJWTSecret() []byte {
	return h.jwtSecret
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(userStore *UserStore, jwtSecret []byte) *AuthHandler {
	// Get expiration times from environment or use defaults
	accessExpiryStr := os.Getenv("JWT_EXPIRATION")
	if accessExpiryStr == "" {
		accessExpiryStr = "24h"
	}
	accessExpiry, _ := time.ParseDuration(accessExpiryStr)

	refreshExpiryStr := os.Getenv("JWT_REFRESH_EXPIRATION")
	if refreshExpiryStr == "" {
		refreshExpiryStr = "168h" // 7 days
	}
	refreshExpiry, _ := time.ParseDuration(refreshExpiryStr)

	return &AuthHandler{
		userStore:     userStore,
		jwtSecret:     jwtSecret,
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
	}
}

// HandleLogin handles POST /api/v1/auth/login
// Note: Swagger annotations are in server.go handleAuthLogin, not here
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	var req LoginRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: username and password are required",
		})
		return
	}

	// Validate user credentials
	user, valid := h.userStore.ValidateUser(req.Username, req.Password)
	if !valid {
		// Don't log the actual password, but log that validation failed
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid username or password",
		})
		return
	}

	// Generate token pair with role
	tokenPair, err := GenerateTokenPair(user.Username, user.Role, h.jwtSecret, h.accessExpiry, h.refreshExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate tokens",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tokenPair,
	})
}

// HandleRefresh handles POST /api/v1/auth/refresh
// Note: Swagger annotations are in server.go handleAuthRefresh, not here
func (h *AuthHandler) HandleRefresh(c *gin.Context) {
	var req RefreshRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}
	if req.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: refreshToken is required",
		})
		return
	}

	// Validate refresh token
	claims, err := ValidateToken(req.RefreshToken, h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid or expired refresh token",
		})
		return
	}

	// Get user role from store (refresh token might have old role)
	user, exists := h.userStore.GetUser(claims.Username)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "User not found",
		})
		return
	}

	// Generate new token pair with current role
	tokenPair, err := GenerateTokenPair(claims.Username, user.Role, h.jwtSecret, h.accessExpiry, h.refreshExpiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate tokens",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tokenPair,
	})
}

// HandleMe handles GET /api/v1/auth/me
// Note: Swagger annotations are in server.go handleAuthMe, not here
func (h *AuthHandler) HandleMe(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Not authenticated",
		})
		return
	}

	role, _ := c.Get("role")
	if role == "" {
		// Try to get from user store
		if user, ok := h.userStore.GetUser(username.(string)); ok {
			role = user.Role
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"username": username,
			"role":     role,
		},
	})
}
