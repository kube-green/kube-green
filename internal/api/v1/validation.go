/*
Copyright 2025.
*/

package v1

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// timePattern matches HH:MM format (24-hour)
	timePattern = regexp.MustCompile(`^([0-1]?[0-9]|2[0-3]):([0-5][0-9])$`)
)

// ValidateCreateSchedule validates a CreateScheduleRequest
func ValidateCreateSchedule(req CreateScheduleRequest) error {
	if req.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}

	if req.Off == "" {
		return fmt.Errorf("off time is required")
	}

	if !timePattern.MatchString(req.Off) {
		return fmt.Errorf("off time must be in HH:MM format (24-hour), got: %s", req.Off)
	}

	if req.On == "" {
		return fmt.Errorf("on time is required")
	}

	if !timePattern.MatchString(req.On) {
		return fmt.Errorf("on time must be in HH:MM format (24-hour), got: %s", req.On)
	}

	// Validate weekdays if provided
	if req.Weekdays != "" {
		if _, err := HumanWeekdaysToKube(req.Weekdays); err != nil {
			return fmt.Errorf("invalid weekdays: %w", err)
		}
	}

	// Validate sleepDays if provided
	if req.SleepDays != "" {
		if _, err := HumanWeekdaysToKube(req.SleepDays); err != nil {
			return fmt.Errorf("invalid sleepDays: %w", err)
		}
	}

	// Validate wakeDays if provided
	if req.WakeDays != "" {
		if _, err := HumanWeekdaysToKube(req.WakeDays); err != nil {
			return fmt.Errorf("invalid wakeDays: %w", err)
		}
	}

	// Validate namespaces if provided
	// Namespaces are validated dynamically - any namespace that exists for the tenant is valid
	// No hardcoded validation - namespaces are discovered from the cluster
	if len(req.Namespaces) > 0 {
		// Basic validation: namespace should not be empty
		for _, ns := range req.Namespaces {
			if strings.TrimSpace(ns) == "" {
				return fmt.Errorf("namespace cannot be empty")
			}
		}
	}

	return nil
}

// ValidateUpdateSchedule validates an UpdateScheduleRequest
func ValidateUpdateSchedule(req UpdateScheduleRequest) error {
	// At least one field must be provided
	if req.Off == "" && req.On == "" && req.Weekdays == "" && req.SleepDays == "" && req.WakeDays == "" && len(req.Namespaces) == 0 {
		return fmt.Errorf("at least one field must be provided for update")
	}

	// Validate time formats if provided
	if req.Off != "" && !timePattern.MatchString(req.Off) {
		return fmt.Errorf("off time must be in HH:MM format (24-hour), got: %s", req.Off)
	}

	if req.On != "" && !timePattern.MatchString(req.On) {
		return fmt.Errorf("on time must be in HH:MM format (24-hour), got: %s", req.On)
	}

	// Validate weekdays if provided
	if req.Weekdays != "" {
		if _, err := HumanWeekdaysToKube(req.Weekdays); err != nil {
			return fmt.Errorf("invalid weekdays: %w", err)
		}
	}

	// Validate sleepDays if provided
	if req.SleepDays != "" {
		if _, err := HumanWeekdaysToKube(req.SleepDays); err != nil {
			return fmt.Errorf("invalid sleepDays: %w", err)
		}
	}

	// Validate wakeDays if provided
	if req.WakeDays != "" {
		if _, err := HumanWeekdaysToKube(req.WakeDays); err != nil {
			return fmt.Errorf("invalid wakeDays: %w", err)
		}
	}

	// Validate namespaces if provided
	// Namespaces are validated dynamically - any namespace that exists for the tenant is valid
	// No hardcoded validation - namespaces are discovered from the cluster
	if len(req.Namespaces) > 0 {
		// Basic validation: namespace should not be empty
		for _, ns := range req.Namespaces {
			if strings.TrimSpace(ns) == "" {
				return fmt.Errorf("namespace cannot be empty")
			}
		}
	}

	return nil
}
