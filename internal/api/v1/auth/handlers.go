/*
Copyright 2025.
*/

package auth

import (
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
	userStore      *UserStore
	jwtSecret      []byte
	accessExpiry   time.Duration
	refreshExpiry  time.Duration
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
func (h *AuthHandler) HandleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Validate user credentials
	if !h.userStore.ValidateUser(req.Username, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Invalid username or password",
		})
		return
	}

	// Generate token pair
	tokenPair, err := GenerateTokenPair(req.Username, h.jwtSecret, h.accessExpiry, h.refreshExpiry)
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
func (h *AuthHandler) HandleRefresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
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

	// Generate new token pair
	tokenPair, err := GenerateTokenPair(claims.Username, h.jwtSecret, h.accessExpiry, h.refreshExpiry)
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
func (h *AuthHandler) HandleMe(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Not authenticated",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"username": username,
		},
	})
}

