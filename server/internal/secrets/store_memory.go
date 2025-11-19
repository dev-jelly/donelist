package secrets

import (
	"context"
	"fmt"
	"sync"
)

// MemorySecretStore implements in-memory secret storage for development
// WARNING: This is for development only. Use a database or vault in production.
type MemorySecretStore struct {
	secrets map[string]*Secret
	mu      sync.RWMutex
}

// NewMemorySecretStore creates a new in-memory secret store
func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{
		secrets: make(map[string]*Secret),
	}
}

// Store saves a secret to memory
func (s *MemorySecretStore) Store(ctx context.Context, secret *Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secrets[secret.Metadata.ID] = secret
	return nil
}

// Retrieve gets a secret by ID
func (s *MemorySecretStore) Retrieve(ctx context.Context, id string) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	secret, exists := s.secrets[id]
	if !exists {
		return nil, fmt.Errorf("secret not found: %s", id)
	}

	// Return a copy to prevent external modification
	secretCopy := *secret
	return &secretCopy, nil
}

// List lists all secrets matching criteria
func (s *MemorySecretStore) List(ctx context.Context, filter SecretFilter) ([]*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Secret

	for _, secret := range s.secrets {
		if s.matchesFilter(secret, filter) {
			secretCopy := *secret
			results = append(results, &secretCopy)
		}
	}

	return results, nil
}

// Delete removes a secret
func (s *MemorySecretStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.secrets[id]; !exists {
		return fmt.Errorf("secret not found: %s", id)
	}

	delete(s.secrets, id)
	return nil
}

// MarkActive marks a secret as active and deactivates others with the same name
func (s *MemorySecretStore) MarkActive(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	secret, exists := s.secrets[id]
	if !exists {
		return fmt.Errorf("secret not found: %s", id)
	}

	// Deactivate other secrets with the same name and environment
	for _, other := range s.secrets {
		if other.Metadata.Name == secret.Metadata.Name &&
			other.Metadata.Environment == secret.Metadata.Environment &&
			other.Metadata.ID != id {
			other.Metadata.Active = false
		}
	}

	// Activate this secret
	secret.Metadata.Active = true

	return nil
}

// GetActiveByName gets the active secret for a name and environment
func (s *MemorySecretStore) GetActiveByName(ctx context.Context, name string, env Environment) (*Secret, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, secret := range s.secrets {
		if secret.Metadata.Name == name &&
			secret.Metadata.Environment == env &&
			secret.Metadata.Active {
			secretCopy := *secret
			return &secretCopy, nil
		}
	}

	return nil, fmt.Errorf("no active secret found for name: %s, environment: %s", name, env)
}

// matchesFilter checks if a secret matches the filter criteria
func (s *MemorySecretStore) matchesFilter(secret *Secret, filter SecretFilter) bool {
	if filter.Name != "" && secret.Metadata.Name != filter.Name {
		return false
	}

	if filter.Type != "" && secret.Metadata.Type != filter.Type {
		return false
	}

	if filter.Environment != "" && secret.Metadata.Environment != filter.Environment {
		return false
	}

	if filter.Active != nil && secret.Metadata.Active != *filter.Active {
		return false
	}

	if filter.KeyID != "" && secret.Metadata.KeyID != filter.KeyID {
		return false
	}

	return true
}

// Count returns the number of secrets in the store
func (s *MemorySecretStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.secrets)
}

// Clear removes all secrets (for testing)
func (s *MemorySecretStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secrets = make(map[string]*Secret)
}
