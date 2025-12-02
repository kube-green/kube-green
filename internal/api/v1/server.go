/*
Copyright 2025.
*/

package v1

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kube-green/kube-green/internal/api/v1/auth"
	_ "github.com/kube-green/kube-green/internal/api/v1/docs" // Swagger docs
)

// Server represents the REST API server
type Server struct {
	client          client.Client
	logger          logr.Logger
	router          *gin.Engine
	httpServer      *http.Server
	port            int
	scheduleService *ScheduleService
	authHandler     *auth.AuthHandler
	userStore       *auth.UserStore
}

// Config holds the configuration for the REST API server
type Config struct {
	Port       int
	Client     client.Client
	Logger     logr.Logger
	EnableCORS bool
	Namespace  string // Kubernetes namespace for loading secrets
}

// NewServer creates a new REST API server instance
func NewServer(config Config) *Server {
	// Set Gin to release mode if not in development
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())

	// Add logging middleware
	router.Use(ginLogger(config.Logger))

	// Enable CORS if requested
	if config.EnableCORS {
		router.Use(corsMiddleware())
	}

	server := &Server{
		client:          config.Client,
		logger:          config.Logger,
		router:          router,
		port:            config.Port,
		scheduleService: NewScheduleService(config.Client, config.Logger),
	}

	// Initialize authentication if enabled
	authEnabled := auth.IsAuthEnabled()
	var jwtSecret []byte
	if authEnabled {
		// Load JWT secret
		var err error
		jwtSecret, err = auth.LoadJWTSecret(config.Client, config.Namespace)
		if err != nil {
			config.Logger.Error(err, "Failed to load JWT secret, authentication disabled")
			authEnabled = false
		} else {
			// Initialize user store
			userStore := auth.NewUserStore(config.Client, config.Namespace, "kube-green-users")
			if err := userStore.LoadUsers(context.Background()); err != nil {
				config.Logger.Error(err, "Failed to load users, authentication disabled")
				authEnabled = false
			} else {
				server.userStore = userStore
				server.authHandler = auth.NewAuthHandler(userStore, jwtSecret)
				config.Logger.Info("Authentication enabled", "namespace", config.Namespace)
			}
		}
	} else {
		config.Logger.Info("Authentication disabled")
	}

	// Add JWT middleware if auth is enabled
	if authEnabled && jwtSecret != nil {
		router.Use(auth.JWTAuthMiddleware(jwtSecret, true))
	}

	// Setup routes
	server.setupRoutes()

	// Create HTTP server
	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	// Health and info endpoints
	s.router.GET("/health", s.handleHealth)
	s.router.GET("/ready", s.handleReady)
	s.router.GET("/api/v1/info", s.handleInfo)

	// Authentication endpoints (public, no auth required)
	if s.authHandler != nil {
		authGroup := s.router.Group("/api/v1/auth")
		{
			authGroup.POST("/login", s.authHandler.HandleLogin)
			authGroup.POST("/refresh", s.authHandler.HandleRefresh)
			authGroup.GET("/me", s.authHandler.HandleMe)
		}
	}

	// Tenant discovery endpoints
	s.router.GET("/api/v1/tenants", s.handleListTenants)

	// Namespace services endpoints
	s.router.GET("/api/v1/namespaces/:tenant/services", s.handleGetNamespaceServices)
	s.router.GET("/api/v1/namespaces/:tenant/resources", s.handleGetNamespaceResources)

	// Schedule management endpoints
	v1 := s.router.Group("/api/v1/schedules")
	{
		v1.GET("", s.handleListSchedules)
		v1.GET("/:tenant", s.handleGetSchedule)
		v1.GET("/:tenant/suspended", s.handleGetSuspendedServices)
		v1.POST("", s.handleCreateSchedule)
		v1.PUT("/:tenant", s.handleUpdateSchedule)
		v1.DELETE("/:tenant", s.handleDeleteSchedule)

		// Namespace-specific schedule endpoints
		v1.GET("/:tenant/:namespace", s.handleGetNamespaceSchedule)
		v1.POST("/:tenant/:namespace", s.handleCreateNamespaceSchedule)
		v1.PUT("/:tenant/:namespace", s.handleUpdateNamespaceSchedule)
		v1.DELETE("/:tenant/:namespace", s.handleDeleteNamespaceSchedule)
	}

	// Swagger documentation
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	s.router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting REST API server", "port", s.port)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		s.logger.Info("Shutting down REST API server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
		return nil
	}
}

// ginLogger creates a Gin middleware for logging
func ginLogger(logger logr.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Log request
		latency := time.Since(start)
		status := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info("HTTP request",
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency", latency,
			"client_ip", c.ClientIP(),
		)
	}
}

// corsMiddleware adds CORS headers
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
