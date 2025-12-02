/*
Copyright 2025.
*/

package auth

import (
	"context"
	"fmt"
	"os"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoadJWTSecret loads JWT secret from Kubernetes Secret or environment variable
func LoadJWTSecret(k8sClient client.Client, namespace string) ([]byte, error) {
	// First try environment variable (for development/testing)
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		return []byte(secret), nil
	}

	// Try to load from Kubernetes Secret
	if k8sClient != nil {
		secret := &v1.Secret{}
		err := k8sClient.Get(context.Background(), client.ObjectKey{
			Namespace: namespace,
			Name:      "kube-green-jwt-secret",
		}, secret)

		if err == nil {
			jwtSecret := secret.Data["jwt-secret"]
			if jwtSecret != nil && len(jwtSecret) > 0 {
				return jwtSecret, nil
			}
		}
	}

	return nil, fmt.Errorf("JWT_SECRET not found in environment or Kubernetes Secret")
}

// IsAuthEnabled checks if authentication is enabled
func IsAuthEnabled() bool {
	authEnabled := os.Getenv("AUTH_ENABLED")
	return authEnabled == "true" || authEnabled == "1"
}

