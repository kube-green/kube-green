/*
Copyright 2025.
*/

package v1

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kube-green/kube-green/internal/api/v1/auth"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// APIResponse represents a standard API response
// @Description Standard API response structure
type APIResponse struct {
	Success bool        `json:"success" example:"true"`                          // Indicates if the operation was successful
	Message string      `json:"message,omitempty" example:"Operation completed"` // Optional success message
	Data    interface{} `json:"data,omitempty"`                                  // Optional response data
	Error   string      `json:"error,omitempty"`                                 // Optional error message (if success is false)
}

// ErrorResponse represents an error response
// @Description Error response structure
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`         // Always false for error responses
	Error   string `json:"error" example:"Invalid request"` // Error message
	Code    int    `json:"code" example:"400"`              // HTTP status code
}

// handleHealth returns health status
// @Summary Health check endpoint
// @Description Returns the health status of the API server
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Router /health [get]
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "API server is healthy",
	})
}

// handleReady returns readiness status
// @Summary Readiness check endpoint
// @Description Returns the readiness status of the API server
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Router /ready [get]
func (s *Server) handleReady(c *gin.Context) {
	// TODO: Add actual readiness checks (e.g., Kubernetes client connectivity)
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "API server is ready",
	})
}

// handleInfo returns API information
// @Summary API information endpoint
// @Description Returns information about the API
// @Tags Info
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Router /api/v1/info [get]
func (s *Server) handleInfo(c *gin.Context) {
	info := map[string]interface{}{
		"version":    "1.0.0",
		"apiVersion": "v1",
		"name":       "kube-green REST API",
		"endpoints": []string{
			"GET    /api/v1/schedules",
			"GET    /api/v1/schedules/:tenant",
			"POST   /api/v1/schedules",
			"PUT    /api/v1/schedules/:tenant",
			"DELETE /api/v1/schedules/:tenant",
		},
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    info,
	})
}

// handleListTenants lists all discovered tenants
// @Summary List all tenants
// @Description Discovers all tenants by scanning namespaces that match the pattern {tenant}-{suffix}
// @Tags Tenants
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} APIResponse{data=TenantListResponse}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/tenants [get]
func (s *Server) handleListTenants(c *gin.Context) {
	tenants, err := s.scheduleService.ListTenants(c.Request.Context())
	if err != nil {
		s.logger.Error(err, "failed to list tenants")
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    tenants,
	})
}

// handleListSchedules lists all schedules
// @Summary List all schedules
// @Description Lists all SleepInfo schedules across all namespaces
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} APIResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/schedules [get]
func (s *Server) handleListSchedules(c *gin.Context) {
	schedules, err := s.scheduleService.ListSchedules(c.Request.Context())
	if err != nil {
		s.logger.Error(err, "failed to list schedules")
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    schedules,
	})
}

// handleGetSchedule gets schedule for a specific tenant
// @Summary Get schedule for tenant
// @Description Returns all SleepInfo configurations for a specific tenant, grouped by namespace. If namespace parameter is not provided, returns all namespaces. If namespace is provided (datastores, apps, rocket, intelligence, airflowsso), returns only that namespace.
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tenant path string true "Tenant name" example:"bdadevdat"
// @Param namespace query string false "Namespace suffix filter (datastores, apps, rocket, intelligence, airflowsso). Leave empty to get all namespaces" example:"datastores"
// @Success 200 {object} APIResponse{data=ScheduleResponse} "Schedule information with improved structure"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 404 {object} ErrorResponse "Schedule not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/schedules/{tenant} [get]
func (s *Server) handleGetSchedule(c *gin.Context) {
	tenant := c.Param("tenant")
	if tenant == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "tenant parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Get optional namespace filter from query parameter
	namespaceFilter := c.Query("namespace")

	// Namespaces are validated dynamically - any namespace that exists for the tenant is valid
	// No hardcoded validation - namespaces are discovered from the cluster

	schedule, err := s.scheduleService.GetSchedule(c.Request.Context(), tenant, namespaceFilter)
	if err != nil {
		if strings.Contains(err.Error(), "no schedules found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to get schedule", "tenant", tenant, "namespace", namespaceFilter)
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    schedule,
	})
}

// CreateScheduleRequest represents a request to create a schedule
// @Description Request to create a new sleep/wake schedule for a tenant
type CreateScheduleRequest struct {
	Tenant        string       `json:"tenant" binding:"required" example:"bdadevdat"`                      // Tenant name (e.g., bdadevdat, bdadevprd)
	Off           string       `json:"off" binding:"required" example:"22:00"`                             // Sleep time in local timezone (HH:MM format, 24-hour)
	On            string       `json:"on" binding:"required" example:"06:00"`                              // Wake time in local timezone (HH:MM format, 24-hour)
	Weekdays      string       `json:"weekdays,omitempty" example:"lunes-viernes"`                         // Days of week (human format: "lunes-viernes", or numeric: "1-5")
	SleepDays     string       `json:"sleepDays,omitempty" example:"viernes"`                              // Optional: specific days for sleep (overrides weekdays)
	WakeDays      string       `json:"wakeDays,omitempty" example:"lunes"`                                 // Optional: specific days for wake (overrides weekdays)
	WeekdaysSleep string       `json:"weekdaysSleep,omitempty" example:"viernes"`                          // Frontend format: specific days for sleep (mapped to SleepDays)
	WeekdaysWake  string       `json:"weekdaysWake,omitempty" example:"lunes"`                             // Frontend format: specific days for wake (mapped to WakeDays)
	Namespaces    []string     `json:"namespaces,omitempty" example:"datastores,apps"`                     // Optional: limit to specific namespaces (datastores, apps, rocket, intelligence, airflowsso)
	Delays        *DelayConfig `json:"delays,omitempty"`                                                   // Optional: custom delays for staggered wake-up (e.g., {"pgHdfsDelay": "0m", "pgbouncerDelay": "5m", "deploymentsDelay": "7m"})
	ScheduleName  string       `json:"scheduleName,omitempty" example:"horario-laboral"`                   // Optional: name to identify this schedule (allows multiple schedules per namespace)
	Description   string       `json:"description,omitempty" example:"Horario laboral de lunes a viernes"` // Optional: description of the schedule
	Apply         bool         `json:"apply,omitempty"`                                                    // Always applies to cluster (field is ignored but kept for compatibility)
}

// handleCreateSchedule creates a new schedule
// @Summary Create a new schedule
// @Description Creates SleepInfo configurations for a tenant. Automatically converts local time (America/Bogota) to UTC and handles timezone day shifts. Creates schedules for all namespaces (datastores, apps, rocket, intelligence, airflowsso) unless filtered.
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateScheduleRequest true "Schedule configuration"
// @Success 201 {object} APIResponse "Schedule created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/schedules [post]
func (s *Server) handleCreateSchedule(c *gin.Context) {
	// Check permissions
	role, exists := c.Get("role")
	if !exists || !auth.CanCreateSchedule(role.(string)) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Insufficient permissions. Only admin and operacion roles can create schedules",
			Code:    http.StatusForbidden,
		})
		return
	}

	var req CreateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate request
	if err := ValidateCreateSchedule(req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Map frontend format (weekdaysSleep/weekdaysWake) to backend format (sleepDays/wakeDays)
	// Frontend sends weekdaysSleep and weekdaysWake, but backend expects sleepDays and wakeDays
	sleepDays := req.SleepDays
	if sleepDays == "" && req.WeekdaysSleep != "" {
		sleepDays = req.WeekdaysSleep
		s.logger.Info("Mapped weekdaysSleep to sleepDays", "weekdaysSleep", req.WeekdaysSleep, "sleepDays", sleepDays)
	}

	wakeDays := req.WakeDays
	if wakeDays == "" && req.WeekdaysWake != "" {
		wakeDays = req.WeekdaysWake
		s.logger.Info("Mapped weekdaysWake to wakeDays", "weekdaysWake", req.WeekdaysWake, "wakeDays", wakeDays)
	}

	s.logger.Info("handleCreateSchedule: weekdays mapping",
		"weekdaysSleep", req.WeekdaysSleep,
		"weekdaysWake", req.WeekdaysWake,
		"sleepDays", sleepDays,
		"wakeDays", wakeDays,
		"weekdays", req.Weekdays)

	// Create schedule using service
	serviceReq := CreateScheduleRequest{
		Tenant:       req.Tenant,
		Off:          req.Off,
		On:           req.On,
		Weekdays:     req.Weekdays,
		SleepDays:    sleepDays,
		WakeDays:     wakeDays,
		Namespaces:   req.Namespaces,
		Delays:       req.Delays,
		ScheduleName: req.ScheduleName,
		Description:  req.Description,
	}

	if err := s.scheduleService.CreateSchedule(c.Request.Context(), serviceReq); err != nil {
		s.logger.Error(err, "failed to create schedule", "tenant", req.Tenant)
		if errors.Is(err, ErrScheduleOverlap) || errors.Is(err, ErrNamespaceAsleep) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusBadRequest,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create schedule: %v", err),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusCreated, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Schedule created successfully for tenant %s", req.Tenant),
	})
}

// UpdateScheduleRequest represents a request to update a schedule
// @Description Request to update an existing sleep/wake schedule for a tenant (all fields optional)
type UpdateScheduleRequest struct {
	Off        string   `json:"off,omitempty" example:"23:00"`         // Sleep time in local timezone (HH:MM format, 24-hour)
	On         string   `json:"on,omitempty" example:"07:00"`          // Wake time in local timezone (HH:MM format, 24-hour)
	Weekdays   string   `json:"weekdays,omitempty" example:"1-5"`      // Days of week (human format: "lunes-viernes", or numeric: "1-5")
	SleepDays  string   `json:"sleepDays,omitempty" example:"viernes"` // Optional: specific days for sleep (overrides weekdays)
	WakeDays   string   `json:"wakeDays,omitempty" example:"lunes"`    // Optional: specific days for wake (overrides weekdays)
	WeekdaysSleep string `json:"weekdaysSleep,omitempty" example:"viernes"` // Frontend format: specific days for sleep (mapped to sleepDays)
	WeekdaysWake  string `json:"weekdaysWake,omitempty" example:"lunes"`    // Frontend format: specific days for wake (mapped to wakeDays)
	Namespaces []string `json:"namespaces,omitempty" example:"apps"`   // Optional: limit to specific namespaces
	Apply      bool     `json:"apply,omitempty"`                       // Always applies to cluster (field is ignored)
}

// handleUpdateSchedule updates an existing schedule
// @Summary Update a schedule
// @Description Updates SleepInfo configurations for a tenant. Missing fields are extracted from existing schedule. At least 'off' or 'on' time must be provided.
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tenant path string true "Tenant name" example:"bdadevdat"
// @Param request body UpdateScheduleRequest true "Schedule configuration (all fields optional)"
// @Success 200 {object} APIResponse "Schedule updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 404 {object} ErrorResponse "Schedule not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/schedules/{tenant} [put]
func (s *Server) handleUpdateSchedule(c *gin.Context) {
	// Check permissions
	role, exists := c.Get("role")
	if !exists || !auth.CanCreateSchedule(role.(string)) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Insufficient permissions. Only admin and operacion roles can update schedules",
			Code:    http.StatusForbidden,
		})
		return
	}

	tenant := c.Param("tenant")
	if tenant == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "tenant parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req UpdateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate request
	if err := ValidateUpdateSchedule(req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Map frontend format (weekdaysSleep/weekdaysWake) to backend format (sleepDays/wakeDays)
	sleepDays := req.SleepDays
	if sleepDays == "" && req.WeekdaysSleep != "" {
		sleepDays = req.WeekdaysSleep
		s.logger.Info("Mapped weekdaysSleep to sleepDays (update)", "weekdaysSleep", req.WeekdaysSleep, "sleepDays", sleepDays)
	}

	wakeDays := req.WakeDays
	if wakeDays == "" && req.WeekdaysWake != "" {
		wakeDays = req.WeekdaysWake
		s.logger.Info("Mapped weekdaysWake to wakeDays (update)", "weekdaysWake", req.WeekdaysWake, "wakeDays", wakeDays)
	}

	// Convert UpdateScheduleRequest to CreateScheduleRequest
	createReq := CreateScheduleRequest{
		Tenant:     tenant,
		Off:        req.Off,
		On:         req.On,
		Weekdays:   req.Weekdays,
		SleepDays:  sleepDays,
		WakeDays:   wakeDays,
		Namespaces: req.Namespaces,
	}

	// Verify schedule exists before updating
	_, err := s.scheduleService.GetSchedule(c.Request.Context(), tenant)
	if err != nil {
		if strings.Contains(err.Error(), "no schedules found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   fmt.Sprintf("schedule not found for tenant: %s", tenant),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to get existing schedule", "tenant", tenant)
		handleKubernetesError(c, err)
		return
	}

	// Validate that at least off and on are provided (required for timezone conversion)
	if createReq.Off == "" && createReq.On == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "at least 'off' or 'on' time must be provided for update",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Update schedule
	if err := s.scheduleService.UpdateSchedule(c.Request.Context(), tenant, createReq); err != nil {
		s.logger.Error(err, "failed to update schedule", "tenant", tenant)
		if errors.Is(err, ErrScheduleOverlap) || errors.Is(err, ErrNamespaceAsleep) {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusBadRequest,
			})
			return
		}
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Schedule updated successfully for tenant %s", tenant),
	})
}

// handleDeleteSchedule deletes a schedule
// @Summary Delete a schedule
// @Description Deletes SleepInfo configurations and associated secrets for a tenant. Optional filters: namespace, scheduleName.
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tenant path string true "Tenant name" example:"bdadevdat"
// @Success 200 {object} APIResponse "Schedule deleted successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 404 {object} ErrorResponse "Schedule not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Param namespace query string false "Namespace suffix (optional)" example:"apps"
// @Param scheduleName query string false "Schedule name (optional)" example:"apagado-tenant-bdaqa"
// @Router /api/v1/schedules/{tenant} [delete]
func (s *Server) handleDeleteSchedule(c *gin.Context) {
	// Check permissions
	role, exists := c.Get("role")
	if !exists || !auth.CanDeleteSchedule(role.(string)) {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Insufficient permissions. Only admin and operacion roles can delete schedules",
			Code:    http.StatusForbidden,
		})
		return
	}

	tenant := c.Param("tenant")
	if tenant == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "tenant parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	filterNamespace := c.Query("namespace")
	scheduleName := c.Query("scheduleName")
	var err error
	if scheduleName != "" {
		err = s.scheduleService.DeleteScheduleByName(c.Request.Context(), tenant, scheduleName, filterNamespace)
	} else {
		err = s.scheduleService.DeleteSchedule(c.Request.Context(), tenant, filterNamespace)
	}

	if err != nil {
		if strings.Contains(err.Error(), "no schedules found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to delete schedule", "tenant", tenant)
		handleKubernetesError(c, err)
		return
	}

	if scheduleName != "" && filterNamespace != "" {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: fmt.Sprintf("Schedule deleted successfully for tenant %s (namespace %s, schedule %s)", tenant, filterNamespace, scheduleName),
		})
		return
	}
	if scheduleName != "" {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: fmt.Sprintf("Schedule deleted successfully for tenant %s (schedule %s)", tenant, scheduleName),
		})
		return
	}
	if filterNamespace != "" {
		c.JSON(http.StatusOK, APIResponse{
			Success: true,
			Message: fmt.Sprintf("Schedule deleted successfully for tenant %s (namespace %s)", tenant, filterNamespace),
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Schedule deleted successfully for tenant %s", tenant),
	})
}

// handleGetSuspendedServices gets currently suspended services for a tenant
// @Summary Get suspended services for tenant
// @Description Returns all currently suspended services (Deployments, StatefulSets, CronJobs) for a specific tenant
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tenant path string true "Tenant name" example:"bdadevdat"
// @Success 200 {object} APIResponse{data=SuspendedServicesResponse} "Suspended services information"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 404 {object} ErrorResponse "Tenant not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/schedules/{tenant}/suspended [get]
func (s *Server) handleGetSuspendedServices(c *gin.Context) {
	tenant := c.Param("tenant")
	if tenant == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "tenant parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	suspended, err := s.scheduleService.GetSuspendedServices(c.Request.Context(), tenant)
	if err != nil {
		if strings.Contains(err.Error(), "no schedules found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to get suspended services", "tenant", tenant)
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    suspended,
	})
}

// handleGetNextOperation gets the next scheduled operation for a tenant
// @Summary Get next scheduled operation for tenant
// @Description Returns the next scheduled sleep or wake operation for a specific tenant
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param tenant path string true "Tenant name" example:"bdadevdat"
// @Success 200 {object} APIResponse{data=NextOperationResponse} "Next operation information"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 404 {object} ErrorResponse "Tenant not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/schedules/{tenant}/next [get]
func (s *Server) handleGetNextOperation(c *gin.Context) {
	tenant := c.Param("tenant")
	if tenant == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "tenant parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	nextOp, err := s.scheduleService.GetNextOperation(c.Request.Context(), tenant)
	if err != nil {
		if strings.Contains(err.Error(), "no schedules found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to get next operation", "tenant", tenant)
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    nextOp,
	})
}

// handleGetAllSuspendedServices gets suspended services for all tenants
// @Summary Get all suspended services
// @Description Returns all currently suspended services across all tenants
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} APIResponse{data=[]SuspendedServiceInfo} "All suspended services"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/schedules/suspended [get]
func (s *Server) handleGetAllSuspendedServices(c *gin.Context) {
	suspended, err := s.scheduleService.GetAllSuspendedServices(c.Request.Context())
	if err != nil {
		s.logger.Error(err, "failed to get all suspended services")
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    suspended,
	})
}

// handleGetAllNextOperations gets the next operation across all tenants
// @Summary Get next operation across all tenants
// @Description Returns the earliest next scheduled operation across all tenants
// @Tags Schedules
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} APIResponse{data=NextOperationResponse} "Next operation information"
// @Failure 404 {object} ErrorResponse "No scheduled operations found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/schedules/next [get]
func (s *Server) handleGetAllNextOperations(c *gin.Context) {
	nextOp, err := s.scheduleService.GetAllNextOperations(c.Request.Context())
	if err != nil {
		if strings.Contains(err.Error(), "no scheduled operations found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to get all next operations")
		handleKubernetesError(c, err)
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    nextOp,
	})
}

// handleListUsers lists all users (admin only)
// @Summary List all users
// @Description Lists all users in the system. Requires admin role.
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} APIResponse{data=[]UserInfo} "List of users"
// @Failure 403 {object} ErrorResponse "Forbidden: Admin access required"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users [get]
func (s *Server) handleListUsers(c *gin.Context) {
	if s.userStore == nil {
		s.initializeAuth()
	}
	if s.userStore == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Success: false,
			Error:   "User management is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	// Check if user is admin
	role, exists := c.Get("role")
	if !exists || role.(string) != auth.RoleAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Admin access required",
			Code:    http.StatusForbidden,
		})
		return
	}

	users := s.userStore.ListUsers()
	// Convert to UserInfo (without password hash)
	userInfos := make([]UserInfo, 0, len(users))
	for _, userMap := range users {
		userInfos = append(userInfos, UserInfo{
			Username: userMap["username"],
			Role:     userMap["role"],
		})
	}
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    userInfos,
	})
}

// UserInfo represents user information (without password)
// @Description User information returned by the API
type UserInfo struct {
	Username string `json:"username" example:"admin"` // Username
	Role     string `json:"role" example:"admin"`     // User role: admin, operacion, or lectura
}

// CreateUserRequest represents a request to create a user
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
}

// handleCreateUser creates a new user (admin only)
// @Summary Create a new user
// @Description Creates a new user with a specified username, password, and role. Requires admin role.
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateUserRequest true "User creation request"
// @Success 201 {object} APIResponse "User created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 403 {object} ErrorResponse "Forbidden: Admin access required"
// @Failure 409 {object} ErrorResponse "Conflict: User already exists"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users [post]
func (s *Server) handleCreateUser(c *gin.Context) {
	if s.userStore == nil {
		s.initializeAuth()
	}
	if s.userStore == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Success: false,
			Error:   "User management is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	// Check if user is admin
	role, exists := c.Get("role")
	if !exists || role.(string) != auth.RoleAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Admin access required",
			Code:    http.StatusForbidden,
		})
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	if s.userStore.UserExists(req.Username) {
		c.JSON(http.StatusConflict, ErrorResponse{
			Success: false,
			Error:   "User already exists",
			Code:    http.StatusConflict,
		})
		return
	}

	if err := s.userStore.CreateUser(req.Username, req.Password, req.Role); err != nil {
		s.logger.Error(err, "failed to create user", "username", req.Username)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create user: %v", err),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusCreated, APIResponse{
		Success: true,
		Message: fmt.Sprintf("User %s created successfully", req.Username),
	})
}

// UpdatePasswordRequest represents a request to update a user's password
type UpdatePasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

// handleUpdateUserPassword updates a user's password (admin only)
// @Summary Update user password
// @Description Updates a user's password. Requires admin role.
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param username path string true "Username" example:"user1"
// @Param request body UpdatePasswordRequest true "Password update request"
// @Success 200 {object} APIResponse "Password updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 403 {object} ErrorResponse "Forbidden: Admin access required"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users/{username}/password [put]
func (s *Server) handleUpdateUserPassword(c *gin.Context) {
	if s.userStore == nil {
		s.initializeAuth()
	}
	if s.userStore == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Success: false,
			Error:   "User management is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	// Check if user is admin
	role, exists := c.Get("role")
	if !exists || role.(string) != auth.RoleAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Admin access required",
			Code:    http.StatusForbidden,
		})
		return
	}

	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Username parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := s.userStore.UpdateUserPassword(username, req.Password); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to update user password", "username", username)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to update password: %v", err),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Password updated successfully for user %s", username),
	})
}

// UpdateRoleRequest represents a request to update a user's role
type UpdateRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// handleUpdateUserRole updates a user's role (admin only)
// @Summary Update user role
// @Description Updates a user's role. Requires admin role.
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param username path string true "Username" example:"user1"
// @Param request body UpdateRoleRequest true "Role update request"
// @Success 200 {object} APIResponse "Role updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 403 {object} ErrorResponse "Forbidden: Admin access required"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users/{username}/role [put]
func (s *Server) handleUpdateUserRole(c *gin.Context) {
	if s.userStore == nil {
		s.initializeAuth()
	}
	if s.userStore == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Success: false,
			Error:   "User management is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	// Check if user is admin
	role, exists := c.Get("role")
	if !exists || role.(string) != auth.RoleAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Admin access required",
			Code:    http.StatusForbidden,
		})
		return
	}

	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Username parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := s.userStore.UpdateUserRole(username, req.Role); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to update user role", "username", username)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to update role: %v", err),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("Role updated successfully for user %s", username),
	})
}

// handleDeleteUser deletes a user (admin only)
// @Summary Delete user
// @Description Deletes a user from the system. Requires admin role.
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param username path string true "Username" example:"user1"
// @Success 200 {object} APIResponse "User deleted successfully"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 403 {object} ErrorResponse "Forbidden: Admin access required"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users/{username} [delete]
func (s *Server) handleDeleteUser(c *gin.Context) {
	if s.userStore == nil {
		s.initializeAuth()
	}
	if s.userStore == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Success: false,
			Error:   "User management is not configured",
			Code:    http.StatusServiceUnavailable,
		})
		return
	}

	// Check if user is admin
	role, exists := c.Get("role")
	if !exists || role.(string) != auth.RoleAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Success: false,
			Error:   "Admin access required",
			Code:    http.StatusForbidden,
		})
		return
	}

	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Username parameter is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Prevent deleting yourself
	currentUsername, _ := c.Get("username")
	if username == currentUsername {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Cannot delete your own account",
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := s.userStore.DeleteUser(username); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Success: false,
				Error:   err.Error(),
				Code:    http.StatusNotFound,
			})
			return
		}
		s.logger.Error(err, "failed to delete user", "username", username)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to delete user: %v", err),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: fmt.Sprintf("User %s deleted successfully", username),
	})
}

// handleKubernetesError converts Kubernetes API errors to HTTP responses
func handleKubernetesError(c *gin.Context, err error) {
	if k8serrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Success: false,
			Error:   err.Error(),
			Code:    http.StatusNotFound,
		})
		return
	}

	if k8serrors.IsConflict(err) {
		c.JSON(http.StatusConflict, ErrorResponse{
			Success: false,
			Error:   err.Error(),
			Code:    http.StatusConflict,
		})
		return
	}

	// Generic error
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Success: false,
		Error:   err.Error(),
		Code:    http.StatusInternalServerError,
	})
}

// ExclusionFilter represents a filter for excluding resources
type ExclusionFilter struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// DelayConfig represents delay configuration for staged wake-up
type DelayConfig struct {
	PgHdfsDelay      string `json:"pgHdfsDelay,omitempty"`      // Delay for PgCluster + HDFSCluster (e.g., "0m", "5m")
	PgbouncerDelay   string `json:"pgbouncerDelay,omitempty"`   // Delay for PgBouncer (e.g., "5m")
	DeploymentsDelay string `json:"deploymentsDelay,omitempty"` // Delay for Deployments (e.g., "7m")
}

// NamespaceScheduleRequest represents a request to create/update a schedule for a specific namespace
type NamespaceScheduleRequest struct {
	Tenant        string               `json:"tenant" binding:"required"`
	Namespace     string               `json:"namespace" binding:"required"`
	Off           string               `json:"off" binding:"required"`
	On            string               `json:"on" binding:"required"`
	Weekdays      string               `json:"weekdays,omitempty"`
	WeekdaysSleep string               `json:"weekdaysSleep,omitempty"`
	WeekdaysWake  string               `json:"weekdaysWake,omitempty"`
	ScheduleName  string               `json:"scheduleName,omitempty"`
	Description   string               `json:"description,omitempty"`
	Delays        *DelayConfig         `json:"delays,omitempty"`
	Exclusions    []NamespaceExclusion `json:"exclusions,omitempty"`
}

// NamespaceExclusion represents an exclusion for a specific namespace
type NamespaceExclusion struct {
	Namespace string          `json:"namespace"`
	Filter    ExclusionFilter `json:"filter"`
}
