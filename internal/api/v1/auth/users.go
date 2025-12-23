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

// User represents a user with role
type User struct {
	Username     string
	PasswordHash string
	Role         string // "admin", "operacion", "lectura"
}

// UserStore manages user authentication data from Kubernetes Secrets
type UserStore struct {
	client     client.Client
	namespace  string
	secretName string
	users      map[string]*User // username -> User
	mu         sync.RWMutex
}

// Valid roles
const (
	RoleAdmin     = "admin"
	RoleOperacion = "operacion"
	RoleLectura   = "lectura"
)

// NewUserStore creates a new UserStore instance
func NewUserStore(k8sClient client.Client, namespace, secretName string) *UserStore {
	return &UserStore{
		client:     k8sClient,
		namespace:  namespace,
		secretName: secretName,
		users:      make(map[string]*User),
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

	// Parse users (format: username:password_hash:role, one per line)
	// Backward compatible: if no role, default to "admin" for existing users
	scanner := bufio.NewScanner(strings.NewReader(string(usersData)))
	us.mu.Lock()
	defer us.mu.Unlock()

	us.users = make(map[string]*User)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.Split(line, ":")
		if len(parts) >= 2 {
			username := strings.TrimSpace(parts[0])
			passwordHash := strings.TrimSpace(parts[1])
			role := RoleAdmin // Default role

			if len(parts) >= 3 {
				role = strings.TrimSpace(parts[2])
				// Validate role
				if role != RoleAdmin && role != RoleOperacion && role != RoleLectura {
					role = RoleAdmin // Default to admin if invalid
				}
			}

			if username != "" && passwordHash != "" {
				us.users[username] = &User{
					Username:     username,
					PasswordHash: passwordHash,
					Role:         role,
				}
			}
		}
	}

	return scanner.Err()
}

// ValidateUser validates a username and password, returns user if valid
func (us *UserStore) ValidateUser(username, password string) (*User, bool) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	user, exists := us.users[username]
	if !exists {
		return nil, false
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, false
	}

	return user, true
}

// GetUser returns a user by username
func (us *UserStore) GetUser(username string) (*User, bool) {
	us.mu.RLock()
	defer us.mu.RUnlock()
	user, exists := us.users[username]
	if !exists {
		return nil, false
	}
	// Return a copy to avoid race conditions
	return &User{
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Role:         user.Role,
	}, true
}

// CreateUser creates a new user (for admin operations)
func (us *UserStore) CreateUser(username, password, role string) error {
	// Validate role
	if role != RoleAdmin && role != RoleOperacion && role != RoleLectura {
		return fmt.Errorf("invalid role: %s. Valid roles are: admin, operacion, lectura", role)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	us.mu.Lock()
	us.users[username] = &User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
	}
	us.mu.Unlock()

	return us.saveUsers(context.Background())
}

// UpdateUserPassword updates a user's password
func (us *UserStore) UpdateUserPassword(username, newPassword string) error {
	us.mu.Lock()
	user, exists := us.users[username]
	if !exists {
		us.mu.Unlock()
		return fmt.Errorf("user not found: %s", username)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		us.mu.Unlock()
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user.PasswordHash = string(hash)
	us.mu.Unlock()
	return us.saveUsers(context.Background())
}

// UpdateUserRole updates a user's role
func (us *UserStore) UpdateUserRole(username, newRole string) error {
	// Validate role
	if newRole != RoleAdmin && newRole != RoleOperacion && newRole != RoleLectura {
		return fmt.Errorf("invalid role: %s. Valid roles are: admin, operacion, lectura", newRole)
	}

	us.mu.Lock()
	user, exists := us.users[username]
	if !exists {
		us.mu.Unlock()
		return fmt.Errorf("user not found: %s", username)
	}

	user.Role = newRole
	us.mu.Unlock()
	return us.saveUsers(context.Background())
}

// ListUsers returns a list of all users (without password hashes)
func (us *UserStore) ListUsers() []map[string]string {
	us.mu.RLock()
	defer us.mu.RUnlock()

	users := make([]map[string]string, 0, len(us.users))
	for username, user := range us.users {
		users = append(users, map[string]string{
			"username": username,
			"role":     user.Role,
		})
	}

	return users
}

// DeleteUser deletes a user
func (us *UserStore) DeleteUser(username string) error {
	us.mu.Lock()
	if _, exists := us.users[username]; !exists {
		us.mu.Unlock()
		return fmt.Errorf("user not found: %s", username)
	}

	delete(us.users, username)
	us.mu.Unlock()
	return us.saveUsers(context.Background())
}

// saveUsers saves users to Kubernetes Secret
func (us *UserStore) saveUsers(ctx context.Context) error {
	us.mu.RLock()
	var lines []string
	for username, user := range us.users {
		lines = append(lines, fmt.Sprintf("%s:%s:%s", username, user.PasswordHash, user.Role))
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
