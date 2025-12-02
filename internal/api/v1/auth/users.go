/*
Copyright 2025.
*/

package auth

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UserStore manages user authentication data from Kubernetes Secrets
type UserStore struct {
	client     client.Client
	namespace  string
	secretName string
	users      map[string]string // username -> password_hash
	mu         sync.RWMutex
}

// NewUserStore creates a new UserStore instance
func NewUserStore(k8sClient client.Client, namespace, secretName string) *UserStore {
	return &UserStore{
		client:     k8sClient,
		namespace:  namespace,
		secretName: secretName,
		users:      make(map[string]string),
	}
}

// LoadUsers loads users from Kubernetes Secret
func (us *UserStore) LoadUsers(ctx context.Context) error {
	secret := &v1.Secret{}
	err := us.client.Get(ctx, client.ObjectKey{
		Namespace: us.namespace,
		Name:      us.secretName,
	}, secret)

	if err != nil {
		return fmt.Errorf("failed to load users secret: %w", err)
	}

	usersData := secret.Data["users"]
	if usersData == nil {
		return fmt.Errorf("users key not found in secret")
	}

	// Parse users (format: username:password_hash, one per line)
	scanner := bufio.NewScanner(strings.NewReader(string(usersData)))
	us.mu.Lock()
	defer us.mu.Unlock()

	us.users = make(map[string]string)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			username := strings.TrimSpace(parts[0])
			passwordHash := strings.TrimSpace(parts[1])
			if username != "" && passwordHash != "" {
				us.users[username] = passwordHash
			}
		}
	}

	return scanner.Err()
}

// ValidateUser validates a username and password
func (us *UserStore) ValidateUser(username, password string) bool {
	us.mu.RLock()
	defer us.mu.RUnlock()

	storedHash, exists := us.users[username]
	if !exists {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	return err == nil
}

// CreateUser creates a new user (for admin operations)
func (us *UserStore) CreateUser(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	us.mu.Lock()
	us.users[username] = string(hash)
	us.mu.Unlock()

	return us.saveUsers(context.Background())
}

// saveUsers saves users to Kubernetes Secret
func (us *UserStore) saveUsers(ctx context.Context) error {
	us.mu.RLock()
	var lines []string
	for username, hash := range us.users {
		lines = append(lines, fmt.Sprintf("%s:%s", username, hash))
	}
	us.mu.RUnlock()

	usersData := strings.Join(lines, "\n")

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      us.secretName,
			Namespace: us.namespace,
		},
		StringData: map[string]string{
			"users": usersData,
		},
	}

	// Try to update first
	existingSecret := &v1.Secret{}
	err := us.client.Get(ctx, client.ObjectKey{
		Namespace: us.namespace,
		Name:      us.secretName,
	}, existingSecret)

	if err != nil {
		// If doesn't exist, create
		return us.client.Create(ctx, secret)
	}

	// If exists, update
	existingSecret.StringData = secret.StringData
	return us.client.Update(ctx, existingSecret)
}

// UserExists checks if a user exists
func (us *UserStore) UserExists(username string) bool {
	us.mu.RLock()
	defer us.mu.RUnlock()
	_, exists := us.users[username]
	return exists
}

