// Package v1 provides REST API endpoints for managing kube-green schedules
//
// @title           Kube-Green REST API
// @version         1.0
// @description     REST API for managing SleepInfo configurations in kube-green.
// @description     This API allows you to create, read, update, and delete schedules for automatic sleep/wake cycles of Kubernetes resources.
// @description     Authentication is required for most endpoints. Use the /api/v1/auth/login endpoint to obtain a JWT token, then click the "Authorize" button in Swagger UI to enter your Bearer token.

// @contact.name   Kube-Green Support
// @contact.email  support@kube-green.com

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /

// @schemes   http https

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token. Example: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

package v1
