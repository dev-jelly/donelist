package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ConflictResolver handles conflict detection and resolution
type ConflictResolver struct {
	checkinRepo CheckinRepository // Interface for checkin operations
}

// CheckinRepository defines the interface for checkin operations needed by conflict resolver
type CheckinRepository interface {
	GetByID(ctx context.Context, id, userID uuid.UUID) (CheckinData, error)
	GetVersion(ctx context.Context, id, userID uuid.UUID) (int, time.Time, error)
}

// CheckinData represents the minimal checkin data needed for conflict resolution
type CheckinData struct {
	ID              uuid.UUID  `json:"id"`
	Content         string     `json:"content"`
	CategoryID      *uuid.UUID `json:"category_id,omitempty"`
	Version         int        `json:"version"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CheckinTime     time.Time  `json:"checkin_time"`
	DurationMinutes int        `json:"duration_minutes"`
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(checkinRepo CheckinRepository) *ConflictResolver {
	return &ConflictResolver{
		checkinRepo: checkinRepo,
	}
}

// DetectConflict detects if an operation conflicts with server state
func (cr *ConflictResolver) DetectConflict(
	ctx context.Context,
	op *SyncOperationData,
	userID uuid.UUID,
) (*ConflictInfo, error) {
	switch op.ResourceType {
	case ResourceCheckin:
		return cr.detectCheckinConflict(ctx, op, userID)
	case ResourceCategory, ResourceTag:
		// For now, categories and tags use simpler conflict detection
		return cr.detectSimpleConflict(ctx, op, userID)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", op.ResourceType)
	}
}

// detectCheckinConflict detects conflicts for checkin operations
func (cr *ConflictResolver) detectCheckinConflict(
	ctx context.Context,
	op *SyncOperationData,
	userID uuid.UUID,
) (*ConflictInfo, error) {
	// For CREATE operations, check if resource already exists
	if op.OperationType == OperationCreate {
		checkin, err := cr.checkinRepo.GetByID(ctx, op.ResourceID, userID)
		if err == nil {
			// Resource already exists - conflict
			serverData, _ := json.Marshal(checkin)
			return &ConflictInfo{
				Type:              "already_exists",
				ServerTimestamp:   checkin.UpdatedAt,
				ClientTimestamp:   op.ClientTimestamp,
				ServerData:        serverData,
				RecommendedAction: "accept_server",
			}, nil
		}
		// No conflict - resource doesn't exist yet
		return nil, nil
	}

	// For UPDATE/DELETE operations, check version and timestamp
	if op.OperationType == OperationUpdate || op.OperationType == OperationDelete {
		serverVersion, serverTimestamp, err := cr.checkinRepo.GetVersion(ctx, op.ResourceID, userID)
		if err != nil {
			// Resource not found on server
			return &ConflictInfo{
				Type:              "deleted",
				ClientTimestamp:   op.ClientTimestamp,
				ServerTimestamp:   time.Now(),
				RecommendedAction: "accept_server",
			}, nil
		}

		// Parse client data to get version
		var clientData struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(op.Data, &clientData); err != nil {
			return nil, fmt.Errorf("failed to parse client data: %w", err)
		}

		// Check for version mismatch (concurrent edit)
		if clientData.Version != serverVersion {
			checkin, _ := cr.checkinRepo.GetByID(ctx, op.ResourceID, userID)
			serverData, _ := json.Marshal(checkin)

			conflictType := "version_mismatch"
			if op.ClientTimestamp.After(serverTimestamp) {
				conflictType = "concurrent_edit"
			}

			return &ConflictInfo{
				Type:              conflictType,
				ClientVersion:     &clientData.Version,
				ServerVersion:     &serverVersion,
				ClientTimestamp:   op.ClientTimestamp,
				ServerTimestamp:   serverTimestamp,
				ServerData:        serverData,
				RecommendedAction: cr.determineRecommendedAction(op.ClientTimestamp, serverTimestamp),
			}, nil
		}
	}

	// No conflict detected
	return nil, nil
}

// detectSimpleConflict detects conflicts for simple resources (categories, tags)
func (cr *ConflictResolver) detectSimpleConflict(
	ctx context.Context,
	op *SyncOperationData,
	userID uuid.UUID,
) (*ConflictInfo, error) {
	// For categories and tags, we use a simpler conflict detection
	// based on last-write-wins timestamp comparison
	// This could be enhanced with more specific repository methods

	// For now, assume no conflicts for categories and tags
	// In production, you'd implement proper conflict detection
	return nil, nil
}

// determineRecommendedAction determines the recommended action based on timestamps
func (cr *ConflictResolver) determineRecommendedAction(clientTime, serverTime time.Time) string {
	// Last-Write-Wins strategy: recommend accepting the more recent change
	if clientTime.After(serverTime) {
		return "force_client"
	}
	return "accept_server"
}

// ResolveConflict resolves a conflict based on the resolution strategy
func (cr *ConflictResolver) ResolveConflict(
	ctx context.Context,
	conflict *ConflictInfo,
	resolution string,
) (bool, error) {
	switch resolution {
	case "accept_server":
		// Client accepts server version - operation is skipped
		return false, nil

	case "force_client":
		// Client forces its version - operation proceeds
		return true, nil

	case "manual_merge":
		// Manual merge required - mark for user intervention
		return false, fmt.Errorf("manual merge required")

	default:
		return false, fmt.Errorf("unknown resolution strategy: %s", resolution)
	}
}

// ApplyLastWriteWins applies Last-Write-Wins conflict resolution
func (cr *ConflictResolver) ApplyLastWriteWins(
	conflict *ConflictInfo,
) string {
	// Compare timestamps and return resolution
	if conflict.ClientTimestamp.After(conflict.ServerTimestamp) {
		return "force_client"
	}
	return "accept_server"
}

// ConflictStrategy defines different conflict resolution strategies
type ConflictStrategy string

const (
	StrategyLastWriteWins   ConflictStrategy = "last_write_wins"
	StrategyClientWins      ConflictStrategy = "client_wins"
	StrategyServerWins      ConflictStrategy = "server_wins"
	StrategyManualMerge     ConflictStrategy = "manual_merge"
)

// ResolveWithStrategy resolves a conflict using a specific strategy
func (cr *ConflictResolver) ResolveWithStrategy(
	ctx context.Context,
	conflict *ConflictInfo,
	strategy ConflictStrategy,
) (bool, string, error) {
	switch strategy {
	case StrategyLastWriteWins:
		resolution := cr.ApplyLastWriteWins(conflict)
		shouldProceed, err := cr.ResolveConflict(ctx, conflict, resolution)
		return shouldProceed, string(strategy), err

	case StrategyClientWins:
		return true, string(strategy), nil

	case StrategyServerWins:
		return false, string(strategy), nil

	case StrategyManualMerge:
		return false, string(strategy), fmt.Errorf("manual merge required")

	default:
		return false, "", fmt.Errorf("unknown conflict strategy: %s", strategy)
	}
}

// MergeData performs a simple three-way merge for checkin data
func (cr *ConflictResolver) MergeData(
	baseData, clientData, serverData json.RawMessage,
) (json.RawMessage, error) {
	// This is a simplified merge implementation
	// In production, you'd want more sophisticated merging logic

	var base, client, server map[string]interface{}

	if err := json.Unmarshal(baseData, &base); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base data: %w", err)
	}
	if err := json.Unmarshal(clientData, &client); err != nil {
		return nil, fmt.Errorf("failed to unmarshal client data: %w", err)
	}
	if err := json.Unmarshal(serverData, &server); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server data: %w", err)
	}

	// Merge strategy: for each field, if client changed it from base, use client value,
	// otherwise use server value
	merged := make(map[string]interface{})

	for key, serverVal := range server {
		baseVal, baseHasKey := base[key]
		clientVal, clientHasKey := client[key]

		if !clientHasKey {
			merged[key] = serverVal
			continue
		}

		// If client changed the value from base, use client value
		if !baseHasKey || baseVal != clientVal {
			merged[key] = clientVal
		} else {
			merged[key] = serverVal
		}
	}

	// Add any client keys not in server
	for key, clientVal := range client {
		if _, exists := merged[key]; !exists {
			merged[key] = clientVal
		}
	}

	return json.Marshal(merged)
}

// ValidateConflictResolution validates that a conflict resolution is valid
func (cr *ConflictResolver) ValidateConflictResolution(
	conflict *ConflictInfo,
	resolution string,
	data json.RawMessage,
) error {
	switch resolution {
	case "accept_server":
		// No additional data needed
		return nil

	case "force_client":
		// Must provide data for force_client
		if len(data) == 0 {
			return fmt.Errorf("data required for force_client resolution")
		}
		return nil

	case "manual_merge":
		// Must provide merged data
		if len(data) == 0 {
			return fmt.Errorf("merged data required for manual_merge resolution")
		}
		return nil

	default:
		return fmt.Errorf("invalid resolution: %s", resolution)
	}
}
