package secrets

import (
	"context"
	"fmt"
	"time"
)

// Config holds configuration for secret management integration
type Config struct {
	// Environment separation
	Environment Environment

	// KMS configuration
	KMSProvider  string // "local", "aws", "gcp", "azure"
	KMSMasterKey string
	KMSRegion    string
	KMSProjectID string

	// Storage configuration
	StoreType string // "memory", "postgres", "vault"
	StoreURL  string

	// Rotation configuration
	RotationEnabled  bool
	RotationInterval time.Duration
	CheckInterval    time.Duration

	// Caching
	CacheTTL time.Duration

	// Audit logging
	AuditLogPath string
}

// InitializeSecretManagement initializes the complete secret management system
func InitializeSecretManagement(cfg Config) (*SecretManager, *RotationScheduler, error) {
	// Initialize KMS provider
	kms, err := initializeKMS(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize KMS: %w", err)
	}

	// Initialize secret store
	store, err := initializeStore(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	// Initialize audit logger
	auditLogger, err := initializeAuditLogger(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize audit logger: %w", err)
	}

	// Create secret manager
	manager, err := NewSecretManager(ManagerConfig{
		Store:            store,
		KMS:              kms,
		MasterKeyID:      cfg.KMSMasterKey,
		Environment:      cfg.Environment,
		CacheTTL:         cfg.CacheTTL,
		RotationInterval: cfg.RotationInterval,
		RotationEnabled:  cfg.RotationEnabled,
		AuditLogger:      auditLogger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create secret manager: %w", err)
	}

	// Create rotation scheduler if enabled
	var scheduler *RotationScheduler
	if cfg.RotationEnabled {
		scheduler = NewRotationScheduler(manager, cfg.CheckInterval)
	}

	return manager, scheduler, nil
}

// initializeKMS initializes the KMS provider based on configuration
func initializeKMS(cfg Config) (KMSProvider, error) {
	switch cfg.KMSProvider {
	case "local":
		return NewLocalKMSProvider(cfg.KMSMasterKey)

	case "aws":
		return NewAWSKMS(cfg.KMSRegion)

	case "gcp":
		// Parse project ID and other GCP-specific config
		return NewGCPKMS(cfg.KMSProjectID, cfg.KMSRegion, "default")

	default:
		// Default to local KMS for development
		return NewLocalKMSProvider(cfg.KMSMasterKey)
	}
}

// initializeStore initializes the secret store based on configuration
func initializeStore(cfg Config) (SecretStore, error) {
	switch cfg.StoreType {
	case "memory":
		return NewMemorySecretStore(), nil

	case "postgres":
		// TODO: Implement PostgreSQL secret store
		return nil, fmt.Errorf("PostgreSQL store not yet implemented")

	case "vault":
		// TODO: Implement HashiCorp Vault store
		return nil, fmt.Errorf("Vault store not yet implemented")

	default:
		// Default to memory store for development
		return NewMemorySecretStore(), nil
	}
}

// initializeAuditLogger initializes the audit logger based on configuration
func initializeAuditLogger(cfg Config) (AuditLogger, error) {
	if cfg.AuditLogPath == "" {
		// Use memory logger for development
		return NewMemoryAuditLogger(cfg.Environment), nil
	}

	return NewFileAuditLogger(cfg.AuditLogPath, cfg.Environment)
}

// MigrateSecrets migrates secrets from one environment to another
func MigrateSecrets(ctx context.Context, sourceManager, targetManager *SecretManager, filter SecretFilter) error {
	// Export secrets from source
	secrets, err := sourceManager.store.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list source secrets: %w", err)
	}

	// Import to target
	for _, secret := range secrets {
		// Decrypt from source
		value, err := sourceManager.GetSecret(ctx, secret.Metadata.Name)
		if err != nil {
			return fmt.Errorf("failed to get secret %s: %w", secret.Metadata.Name, err)
		}

		// Re-encrypt and store in target
		_, err = targetManager.StoreSecret(ctx, secret.Metadata.Name, value.Value, secret.Metadata.Type, secret.Metadata.Tags)
		if err != nil {
			return fmt.Errorf("failed to store secret %s in target: %w", secret.Metadata.Name, err)
		}
	}

	return nil
}

// HealthCheck performs health checks on the secret management system
func HealthCheck(ctx context.Context, manager *SecretManager) error {
	// Test secret creation
	testName := "health-check-test"
	_, err := manager.StoreSecret(ctx, testName, "test-value", SecretTypeAPIKey, []string{"health-check"})
	if err != nil {
		return fmt.Errorf("failed to store test secret: %w", err)
	}

	// Test secret retrieval
	_, err = manager.GetSecret(ctx, testName)
	if err != nil {
		return fmt.Errorf("failed to retrieve test secret: %w", err)
	}

	// Clean up test secret
	secrets, err := manager.store.List(ctx, SecretFilter{Name: testName})
	if err != nil {
		return fmt.Errorf("failed to list test secrets: %w", err)
	}

	for _, secret := range secrets {
		if err := manager.store.Delete(ctx, secret.Metadata.ID); err != nil {
			return fmt.Errorf("failed to delete test secret: %w", err)
		}
	}

	return nil
}
