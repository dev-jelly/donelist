package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// SecretType represents the type of secret being managed
type SecretType string

const (
	SecretTypeJWT        SecretType = "jwt"
	SecretTypeAPIKey     SecretType = "api_key"
	SecretTypeDatabase   SecretType = "database"
	SecretTypeEncryption SecretType = "encryption"
	SecretTypeWebhook    SecretType = "webhook"
)

// Environment represents deployment environment
type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentStaging     Environment = "staging"
	EnvironmentProduction  Environment = "production"
)

// SecretMetadata contains metadata about a secret
type SecretMetadata struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Type         SecretType   `json:"type"`
	Environment  Environment  `json:"environment"`
	Version      int          `json:"version"`
	KeyID        string       `json:"key_id"`        // kid for key rotation
	Active       bool         `json:"active"`        // Is this the active key
	CreatedAt    time.Time    `json:"created_at"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	RotatedAt    *time.Time   `json:"rotated_at,omitempty"`
	RotationDue  *time.Time   `json:"rotation_due,omitempty"`
	LastAccessed *time.Time   `json:"last_accessed,omitempty"`
	Tags         []string     `json:"tags,omitempty"`
}

// Secret represents an encrypted secret with metadata
type Secret struct {
	Metadata      SecretMetadata `json:"metadata"`
	EncryptedData string         `json:"encrypted_data"`
	IV            string         `json:"iv"` // Initialization vector for AES
}

// SecretValue represents a decrypted secret value
type SecretValue struct {
	Value     string
	Metadata  SecretMetadata
	ExpiresAt *time.Time
}

// SecretStore defines the interface for secret storage backends
type SecretStore interface {
	// Store saves a secret to the backend
	Store(ctx context.Context, secret *Secret) error

	// Retrieve gets a secret by ID
	Retrieve(ctx context.Context, id string) (*Secret, error)

	// List lists all secrets matching criteria
	List(ctx context.Context, filter SecretFilter) ([]*Secret, error)

	// Delete removes a secret
	Delete(ctx context.Context, id string) error

	// MarkActive marks a secret as active
	MarkActive(ctx context.Context, id string) error

	// GetActiveByName gets the active secret for a name
	GetActiveByName(ctx context.Context, name string, env Environment) (*Secret, error)
}

// SecretFilter defines criteria for filtering secrets
type SecretFilter struct {
	Name        string
	Type        SecretType
	Environment Environment
	Active      *bool
	KeyID       string
}

// KMSProvider defines the interface for Key Management Service providers
type KMSProvider interface {
	// Encrypt encrypts data using KMS
	Encrypt(ctx context.Context, plaintext []byte, keyID string) ([]byte, error)

	// Decrypt decrypts data using KMS
	Decrypt(ctx context.Context, ciphertext []byte, keyID string) ([]byte, error)

	// GenerateDataKey generates a new data encryption key
	GenerateDataKey(ctx context.Context, keyID string) ([]byte, error)

	// RotateKey rotates a KMS key
	RotateKey(ctx context.Context, keyID string) (string, error)
}

// SecretManager manages secrets with encryption, rotation, and caching
type SecretManager struct {
	store       SecretStore
	kms         KMSProvider
	masterKeyID string
	environment Environment

	// In-memory cache for frequently accessed secrets
	cache      map[string]*cachedSecret
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration

	// Audit logger
	auditLog   AuditLogger

	// Rotation settings
	rotationInterval time.Duration
	rotationEnabled  bool
}

// cachedSecret represents a cached secret with expiration
type cachedSecret struct {
	value     string
	metadata  SecretMetadata
	expiresAt time.Time
}

// AuditLogger defines interface for audit logging
type AuditLogger interface {
	LogSecretAccess(ctx context.Context, secretID string, action string, success bool, metadata map[string]interface{})
}

// ManagerConfig configures the SecretManager
type ManagerConfig struct {
	Store            SecretStore
	KMS              KMSProvider
	MasterKeyID      string
	Environment      Environment
	CacheTTL         time.Duration
	RotationInterval time.Duration
	RotationEnabled  bool
	AuditLogger      AuditLogger
}

// NewSecretManager creates a new secret manager
func NewSecretManager(config ManagerConfig) (*SecretManager, error) {
	if config.Store == nil {
		return nil, fmt.Errorf("secret store is required")
	}
	if config.KMS == nil {
		return nil, fmt.Errorf("KMS provider is required")
	}
	if config.MasterKeyID == "" {
		return nil, fmt.Errorf("master key ID is required")
	}
	if config.Environment == "" {
		config.Environment = EnvironmentDevelopment
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.RotationInterval == 0 {
		config.RotationInterval = 90 * 24 * time.Hour // 90 days default
	}

	return &SecretManager{
		store:            config.Store,
		kms:              config.KMS,
		masterKeyID:      config.MasterKeyID,
		environment:      config.Environment,
		cache:            make(map[string]*cachedSecret),
		cacheTTL:         config.CacheTTL,
		rotationInterval: config.RotationInterval,
		rotationEnabled:  config.RotationEnabled,
		auditLog:         config.AuditLogger,
	}, nil
}

// StoreSecret stores a new secret with encryption
func (sm *SecretManager) StoreSecret(ctx context.Context, name string, value string, secretType SecretType, tags []string) (*SecretMetadata, error) {
	// Generate unique ID and key ID
	id := fmt.Sprintf("%s-%s-%d", name, sm.environment, time.Now().Unix())
	keyID := fmt.Sprintf("key-%s-%d", name, time.Now().Unix())

	// Encrypt the secret value
	encryptedData, iv, err := sm.encryptValue(ctx, value)
	if err != nil {
		sm.logAudit(ctx, id, "store", false, map[string]interface{}{
			"error": err.Error(),
			"name":  name,
		})
		return nil, fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Calculate rotation due date
	rotationDue := time.Now().Add(sm.rotationInterval)

	metadata := SecretMetadata{
		ID:          id,
		Name:        name,
		Type:        secretType,
		Environment: sm.environment,
		Version:     1,
		KeyID:       keyID,
		Active:      true,
		CreatedAt:   time.Now(),
		RotationDue: &rotationDue,
		Tags:        tags,
	}

	secret := &Secret{
		Metadata:      metadata,
		EncryptedData: encryptedData,
		IV:            iv,
	}

	// Store in backend
	if err := sm.store.Store(ctx, secret); err != nil {
		sm.logAudit(ctx, id, "store", false, map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to store secret: %w", err)
	}

	sm.logAudit(ctx, id, "store", true, map[string]interface{}{
		"name": name,
		"type": secretType,
	})

	return &metadata, nil
}

// GetSecret retrieves and decrypts a secret
func (sm *SecretManager) GetSecret(ctx context.Context, name string) (*SecretValue, error) {
	// Check cache first
	if cached := sm.getCached(name); cached != nil {
		sm.logAudit(ctx, cached.metadata.ID, "access", true, map[string]interface{}{
			"source": "cache",
			"name":   name,
		})
		return &SecretValue{
			Value:     cached.value,
			Metadata:  cached.metadata,
			ExpiresAt: &cached.expiresAt,
		}, nil
	}

	// Retrieve from store
	secret, err := sm.store.GetActiveByName(ctx, name, sm.environment)
	if err != nil {
		sm.logAudit(ctx, "", "access", false, map[string]interface{}{
			"error": err.Error(),
			"name":  name,
		})
		return nil, fmt.Errorf("failed to retrieve secret: %w", err)
	}

	// Decrypt the value
	value, err := sm.decryptValue(ctx, secret.EncryptedData, secret.IV)
	if err != nil {
		sm.logAudit(ctx, secret.Metadata.ID, "decrypt", false, map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to decrypt secret: %w", err)
	}

	// Update last accessed time
	now := time.Now()
	secret.Metadata.LastAccessed = &now

	// Cache the decrypted value
	sm.cacheSecret(name, value, secret.Metadata)

	sm.logAudit(ctx, secret.Metadata.ID, "access", true, map[string]interface{}{
		"source": "store",
		"name":   name,
	})

	return &SecretValue{
		Value:     value,
		Metadata:  secret.Metadata,
		ExpiresAt: secret.Metadata.ExpiresAt,
	}, nil
}

// RotateSecret rotates a secret by creating a new version
func (sm *SecretManager) RotateSecret(ctx context.Context, name string, newValue string) (*SecretMetadata, error) {
	// Get current active secret
	currentSecret, err := sm.store.GetActiveByName(ctx, name, sm.environment)
	if err != nil {
		sm.logAudit(ctx, "", "rotate", false, map[string]interface{}{
			"error": err.Error(),
			"name":  name,
		})
		return nil, fmt.Errorf("failed to get current secret: %w", err)
	}

	// Deactivate current secret
	currentSecret.Metadata.Active = false
	rotatedAt := time.Now()
	currentSecret.Metadata.RotatedAt = &rotatedAt
	if err := sm.store.Store(ctx, currentSecret); err != nil {
		return nil, fmt.Errorf("failed to deactivate old secret: %w", err)
	}

	// Create new version
	newVersion := currentSecret.Metadata.Version + 1
	newKeyID := fmt.Sprintf("key-%s-%d", name, time.Now().Unix())

	encryptedData, iv, err := sm.encryptValue(ctx, newValue)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt new secret: %w", err)
	}

	rotationDue := time.Now().Add(sm.rotationInterval)
	newMetadata := SecretMetadata{
		ID:          fmt.Sprintf("%s-%s-%d", name, sm.environment, time.Now().Unix()),
		Name:        name,
		Type:        currentSecret.Metadata.Type,
		Environment: sm.environment,
		Version:     newVersion,
		KeyID:       newKeyID,
		Active:      true,
		CreatedAt:   time.Now(),
		RotationDue: &rotationDue,
		Tags:        currentSecret.Metadata.Tags,
	}

	newSecret := &Secret{
		Metadata:      newMetadata,
		EncryptedData: encryptedData,
		IV:            iv,
	}

	if err := sm.store.Store(ctx, newSecret); err != nil {
		return nil, fmt.Errorf("failed to store new secret: %w", err)
	}

	// Invalidate cache
	sm.invalidateCache(name)

	sm.logAudit(ctx, newMetadata.ID, "rotate", true, map[string]interface{}{
		"name":        name,
		"old_version": currentSecret.Metadata.Version,
		"new_version": newVersion,
	})

	return &newMetadata, nil
}

// GetSecretsDueForRotation returns secrets that need rotation
func (sm *SecretManager) GetSecretsDueForRotation(ctx context.Context) ([]*SecretMetadata, error) {
	active := true
	secrets, err := sm.store.List(ctx, SecretFilter{
		Environment: sm.environment,
		Active:      &active,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var dueSecrets []*SecretMetadata
	now := time.Now()

	for _, secret := range secrets {
		if secret.Metadata.RotationDue != nil && secret.Metadata.RotationDue.Before(now) {
			dueSecrets = append(dueSecrets, &secret.Metadata)
		}
	}

	return dueSecrets, nil
}

// encryptValue encrypts a value using AES-256-GCM with KMS-derived key
func (sm *SecretManager) encryptValue(ctx context.Context, plaintext string) (string, string, error) {
	// Generate data encryption key from KMS
	dek, err := sm.kms.GenerateDataKey(ctx, sm.masterKeyID)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate data key: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random IV
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", "", fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(ciphertext),
		base64.StdEncoding.EncodeToString(iv),
		nil
}

// decryptValue decrypts a value using AES-256-GCM with KMS-derived key
func (sm *SecretManager) decryptValue(ctx context.Context, encryptedData, ivStr string) (string, error) {
	// Generate the same data encryption key
	dek, err := sm.kms.GenerateDataKey(ctx, sm.masterKeyID)
	if err != nil {
		return "", fmt.Errorf("failed to generate data key: %w", err)
	}

	// Decode base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	iv, err := base64.StdEncoding.DecodeString(ivStr)
	if err != nil {
		return "", fmt.Errorf("failed to decode IV: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// cacheSecret stores a decrypted secret in cache
func (sm *SecretManager) cacheSecret(name, value string, metadata SecretMetadata) {
	sm.cacheMu.Lock()
	defer sm.cacheMu.Unlock()

	sm.cache[name] = &cachedSecret{
		value:     value,
		metadata:  metadata,
		expiresAt: time.Now().Add(sm.cacheTTL),
	}
}

// getCached retrieves a secret from cache if valid
func (sm *SecretManager) getCached(name string) *cachedSecret {
	sm.cacheMu.RLock()
	defer sm.cacheMu.RUnlock()

	cached, exists := sm.cache[name]
	if !exists {
		return nil
	}

	if time.Now().After(cached.expiresAt) {
		return nil
	}

	return cached
}

// invalidateCache removes a secret from cache
func (sm *SecretManager) invalidateCache(name string) {
	sm.cacheMu.Lock()
	defer sm.cacheMu.Unlock()

	delete(sm.cache, name)
}

// ClearCache clears all cached secrets
func (sm *SecretManager) ClearCache() {
	sm.cacheMu.Lock()
	defer sm.cacheMu.Unlock()

	sm.cache = make(map[string]*cachedSecret)
}

// logAudit logs secret operations to audit log
func (sm *SecretManager) logAudit(ctx context.Context, secretID, action string, success bool, metadata map[string]interface{}) {
	if sm.auditLog != nil {
		sm.auditLog.LogSecretAccess(ctx, secretID, action, success, metadata)
	}
}

// ExportSecretMetadata exports secret metadata for backup (without sensitive data)
func (sm *SecretManager) ExportSecretMetadata(ctx context.Context) ([]byte, error) {
	secrets, err := sm.store.List(ctx, SecretFilter{
		Environment: sm.environment,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var metadata []SecretMetadata
	for _, secret := range secrets {
		metadata = append(metadata, secret.Metadata)
	}

	return json.Marshal(metadata)
}
