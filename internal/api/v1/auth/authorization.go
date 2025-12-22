/*
Copyright 2025.
*/

package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireRole creates a middleware that requires a specific role
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "Role not found in token",
			})
			c.Abort()
			return
		}

		roleStr := role.(string)
		for _, allowedRole := range allowedRoles {
			if roleStr == allowedRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "Insufficient permissions",
		})
		c.Abort()
	}
}

// RequireAdmin requires admin role
func RequireAdmin() gin.HandlerFunc {
	return RequireRole(RoleAdmin)
}

// RequireAdminOrOperacion requires admin or operacion role
func RequireAdminOrOperacion() gin.HandlerFunc {
	return RequireRole(RoleAdmin, RoleOperacion)
}

// CanCreateSchedule checks if user can create schedules
func CanCreateSchedule(role string) bool {
	return role == RoleAdmin || role == RoleOperacion
}

// CanDeleteSchedule checks if user can delete schedules
func CanDeleteSchedule(role string) bool {
	return role == RoleAdmin || role == RoleOperacion
}

// CanManageUsers checks if user can manage users
func CanManageUsers(role string) bool {
	return role == RoleAdmin
}


