package secrets

import (
	"context"
	"testing"
	"time"
)

func TestSecretManager_StoreAndRetrieve(t *testing.T) {
	// Setup
	kms, err := NewLocalKMSProvider("test-master-key-minimum-32-chars")
	if err != nil {
		t.Fatalf("failed to create KMS: %v", err)
	}

	store := NewMemorySecretStore()
	auditLog := NewMemoryAuditLogger(EnvironmentDevelopment)

	manager, err := NewSecretManager(ManagerConfig{
		Store:            store,
		KMS:              kms,
		MasterKeyID:      "master-key-1",
		Environment:      EnvironmentDevelopment,
		CacheTTL:         5 * time.Minute,
		RotationInterval: 90 * 24 * time.Hour,
		RotationEnabled:  true,
		AuditLogger:      auditLog,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Test storing a secret
	metadata, err := manager.StoreSecret(ctx, "jwt-secret", "my-super-secret-jwt-key", SecretTypeJWT, []string{"auth"})
	if err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	if metadata.Name != "jwt-secret" {
		t.Errorf("expected name 'jwt-secret', got %s", metadata.Name)
	}

	if metadata.Type != SecretTypeJWT {
		t.Errorf("expected type JWT, got %s", metadata.Type)
	}

	if !metadata.Active {
		t.Error("expected secret to be active")
	}

	// Test retrieving the secret
	value, err := manager.GetSecret(ctx, "jwt-secret")
	if err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}

	if value.Value != "my-super-secret-jwt-key" {
		t.Errorf("expected value 'my-super-secret-jwt-key', got %s", value.Value)
	}

	// Verify audit logs
	events := auditLog.GetEvents()
	if len(events) < 2 {
		t.Errorf("expected at least 2 audit events, got %d", len(events))
	}

	// Test cache hit
	value2, err := manager.GetSecret(ctx, "jwt-secret")
	if err != nil {
		t.Fatalf("failed to get cached secret: %v", err)
	}

	if value2.Value != value.Value {
		t.Error("cached value doesn't match original")
	}

	// Verify cache was used (should have additional audit event with source=cache)
	events = auditLog.GetEvents()
	cachedEvents := 0
	for _, event := range events {
		if metadata, ok := event.Metadata["source"]; ok && metadata == "cache" {
			cachedEvents++
		}
	}
	if cachedEvents == 0 {
		t.Error("expected at least one cached access event")
	}
}

func TestSecretManager_RotateSecret(t *testing.T) {
	// Setup
	kms, _ := NewLocalKMSProvider("test-master-key-minimum-32-chars")
	store := NewMemorySecretStore()
	auditLog := NewMemoryAuditLogger(EnvironmentDevelopment)

	manager, _ := NewSecretManager(ManagerConfig{
		Store:            store,
		KMS:              kms,
		MasterKeyID:      "master-key-1",
		Environment:      EnvironmentDevelopment,
		CacheTTL:         5 * time.Minute,
		RotationInterval: 90 * 24 * time.Hour,
		AuditLogger:      auditLog,
	})

	ctx := context.Background()

	// Store initial secret
	metadata1, _ := manager.StoreSecret(ctx, "api-key", "old-api-key-value", SecretTypeAPIKey, nil)

	// Rotate the secret
	metadata2, err := manager.RotateSecret(ctx, "api-key", "new-api-key-value")
	if err != nil {
		t.Fatalf("failed to rotate secret: %v", err)
	}

	if metadata2.Version != metadata1.Version+1 {
		t.Errorf("expected version %d, got %d", metadata1.Version+1, metadata2.Version)
	}

	if !metadata2.Active {
		t.Error("expected new secret to be active")
	}

	// Verify old secret is inactive
	secrets, _ := store.List(ctx, SecretFilter{Name: "api-key"})
	activeCount := 0
	for _, secret := range secrets {
		if secret.Metadata.Active {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active secret, got %d", activeCount)
	}

	// Verify new value is retrieved
	value, _ := manager.GetSecret(ctx, "api-key")
	if value.Value != "new-api-key-value" {
		t.Errorf("expected 'new-api-key-value', got %s", value.Value)
	}

	// Verify rotation was audited
	rotateEvents := auditLog.GetEventsByAction("rotate")
	if len(rotateEvents) == 0 {
		t.Error("expected rotation to be audited")
	}
}

func TestSecretManager_GetSecretsDueForRotation(t *testing.T) {
	// Setup
	kms, _ := NewLocalKMSProvider("test-master-key-minimum-32-chars")
	store := NewMemorySecretStore()

	manager, _ := NewSecretManager(ManagerConfig{
		Store:            store,
		KMS:              kms,
		MasterKeyID:      "master-key-1",
		Environment:      EnvironmentDevelopment,
		RotationInterval: 1 * time.Millisecond, // Very short interval for testing
	})

	ctx := context.Background()

	// Store a secret
	manager.StoreSecret(ctx, "short-lived", "value", SecretTypeJWT, nil)

	// Wait for rotation to be due
	time.Sleep(10 * time.Millisecond)

	// Get secrets due for rotation
	dueSecrets, err := manager.GetSecretsDueForRotation(ctx)
	if err != nil {
		t.Fatalf("failed to get due secrets: %v", err)
	}

	if len(dueSecrets) != 1 {
		t.Errorf("expected 1 secret due for rotation, got %d", len(dueSecrets))
	}

	if len(dueSecrets) > 0 && dueSecrets[0].Name != "short-lived" {
		t.Errorf("expected secret 'short-lived', got %s", dueSecrets[0].Name)
	}
}

func TestSecretManager_EnvironmentSeparation(t *testing.T) {
	// Setup
	kms, _ := NewLocalKMSProvider("test-master-key-minimum-32-chars")
	store := NewMemorySecretStore()

	devManager, _ := NewSecretManager(ManagerConfig{
		Store:       store,
		KMS:         kms,
		MasterKeyID: "master-key-1",
		Environment: EnvironmentDevelopment,
	})

	prodManager, _ := NewSecretManager(ManagerConfig{
		Store:       store,
		KMS:         kms,
		MasterKeyID: "master-key-1",
		Environment: EnvironmentProduction,
	})

	ctx := context.Background()

	// Store secret in dev
	devManager.StoreSecret(ctx, "db-password", "dev-password", SecretTypeDatabase, nil)

	// Store secret in prod
	prodManager.StoreSecret(ctx, "db-password", "prod-password", SecretTypeDatabase, nil)

	// Verify dev gets dev secret
	devValue, _ := devManager.GetSecret(ctx, "db-password")
	if devValue.Value != "dev-password" {
		t.Errorf("dev manager got wrong value: %s", devValue.Value)
	}

	// Verify prod gets prod secret
	prodValue, _ := prodManager.GetSecret(ctx, "db-password")
	if prodValue.Value != "prod-password" {
		t.Errorf("prod manager got wrong value: %s", prodValue.Value)
	}
}

func TestSecretManager_CacheInvalidation(t *testing.T) {
	// Setup
	kms, _ := NewLocalKMSProvider("test-master-key-minimum-32-chars")
	store := NewMemorySecretStore()

	manager, _ := NewSecretManager(ManagerConfig{
		Store:       store,
		KMS:         kms,
		MasterKeyID: "master-key-1",
		Environment: EnvironmentDevelopment,
		CacheTTL:    1 * time.Hour,
	})

	ctx := context.Background()

	// Store and retrieve (caches)
	manager.StoreSecret(ctx, "cached-secret", "value1", SecretTypeAPIKey, nil)
	manager.GetSecret(ctx, "cached-secret")

	// Rotate (should invalidate cache)
	manager.RotateSecret(ctx, "cached-secret", "value2")

	// Get again (should fetch new value from store, not cache)
	value, _ := manager.GetSecret(ctx, "cached-secret")
	if value.Value != "value2" {
		t.Errorf("expected 'value2', got %s (cache not invalidated)", value.Value)
	}
}

func TestSecretManager_ExportMetadata(t *testing.T) {
	// Setup
	kms, _ := NewLocalKMSProvider("test-master-key-minimum-32-chars")
	store := NewMemorySecretStore()

	manager, _ := NewSecretManager(ManagerConfig{
		Store:       store,
		KMS:         kms,
		MasterKeyID: "master-key-1",
		Environment: EnvironmentDevelopment,
	})

	ctx := context.Background()

	// Store multiple secrets
	manager.StoreSecret(ctx, "secret1", "value1", SecretTypeJWT, []string{"tag1"})
	manager.StoreSecret(ctx, "secret2", "value2", SecretTypeAPIKey, []string{"tag2"})

	// Export metadata
	data, err := manager.ExportSecretMetadata(ctx)
	if err != nil {
		t.Fatalf("failed to export metadata: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected metadata to be exported")
	}

	// Verify it's valid JSON
	var metadata []SecretMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}

	if len(metadata) != 2 {
		t.Errorf("expected 2 metadata entries, got %d", len(metadata))
	}
}

func BenchmarkSecretManager_GetSecret(b *testing.B) {
	kms, _ := NewLocalKMSProvider("test-master-key-minimum-32-chars")
	store := NewMemorySecretStore()

	manager, _ := NewSecretManager(ManagerConfig{
		Store:       store,
		KMS:         kms,
		MasterKeyID: "master-key-1",
		Environment: EnvironmentDevelopment,
		CacheTTL:    5 * time.Minute,
	})

	ctx := context.Background()
	manager.StoreSecret(ctx, "benchmark-secret", "value", SecretTypeJWT, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetSecret(ctx, "benchmark-secret")
	}
}
