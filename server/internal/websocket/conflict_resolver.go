package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ConflictResolutionStrategy defines how conflicts are resolved
type ConflictResolutionStrategy string

const (
	// StrategyLWW uses Last Write Wins based on timestamp
	StrategyLWW ConflictResolutionStrategy = "lww"
	// StrategyVectorClock uses vector clocks for causality tracking
	StrategyVectorClock ConflictResolutionStrategy = "vector_clock"
	// StrategyCustom allows custom resolution logic
	StrategyCustom ConflictResolutionStrategy = "custom"
)

// ConflictMetadata contains information about a conflict
type ConflictMetadata struct {
	DetectedAt    time.Time              `json:"detected_at"`
	ConflictingIDs []string              `json:"conflicting_ids"`
	Resolution    ConflictResolutionStrategy `json:"resolution"`
	WinnerID      string                 `json:"winner_id"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// VersionedData represents data with version information for conflict detection
type VersionedData struct {
	ID         string                 `json:"id"`
	ActorID    string                 `json:"actor_id"`    // Client/user that made the change
	Version    uint64                 `json:"version"`     // Monotonic version number
	Timestamp  time.Time              `json:"timestamp"`   // Wall clock timestamp
	Sequence   uint64                 `json:"sequence"`    // Sequence number for tiebreaking
	Data       json.RawMessage        `json:"data"`        // The actual data
	Checksum   string                 `json:"checksum"`    // Data checksum for integrity
	VectorClock map[string]uint64     `json:"vector_clock,omitempty"` // Optional vector clock
	ParentID   string                 `json:"parent_id,omitempty"`    // Parent version for causality
	Metadata   map[string]interface{} `json:"metadata,omitempty"`     // Additional metadata
}

// ConflictResolver handles conflict resolution for concurrent updates
type ConflictResolver struct {
	strategy       ConflictResolutionStrategy
	redis          *redis.Client
	logger         *zap.Logger
	mu             sync.RWMutex

	// Conflict history for analysis
	conflictHistory []ConflictMetadata
	maxHistory      int

	// Custom resolution function
	customResolver func(a, b *VersionedData) (*VersionedData, ConflictMetadata)

	// Metrics
	conflictsResolved uint64
	conflictsDetected uint64
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(strategy ConflictResolutionStrategy, redis *redis.Client, logger *zap.Logger) *ConflictResolver {
	return &ConflictResolver{
		strategy:        strategy,
		redis:           redis,
		logger:          logger,
		conflictHistory: make([]ConflictMetadata, 0),
		maxHistory:      1000,
	}
}

// SetCustomResolver sets a custom conflict resolution function
func (cr *ConflictResolver) SetCustomResolver(resolver func(a, b *VersionedData) (*VersionedData, ConflictMetadata)) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.customResolver = resolver
}

// Resolve resolves a conflict between two versions using the configured strategy
func (cr *ConflictResolver) Resolve(ctx context.Context, versions ...*VersionedData) (*VersionedData, ConflictMetadata, error) {
	if len(versions) < 2 {
		return nil, ConflictMetadata{}, fmt.Errorf("at least 2 versions required for conflict resolution")
	}

	cr.mu.Lock()
	cr.conflictsDetected++
	cr.mu.Unlock()

	var winner *VersionedData
	var metadata ConflictMetadata

	switch cr.strategy {
	case StrategyLWW:
		winner, metadata = cr.resolveLWW(versions...)
	case StrategyVectorClock:
		winner, metadata = cr.resolveVectorClock(versions...)
	case StrategyCustom:
		if cr.customResolver == nil {
			return nil, ConflictMetadata{}, fmt.Errorf("custom resolver not configured")
		}
		// For simplicity, apply custom resolver pairwise
		winner = versions[0]
		for i := 1; i < len(versions); i++ {
			winner, metadata = cr.customResolver(winner, versions[i])
		}
	default:
		return nil, ConflictMetadata{}, fmt.Errorf("unknown conflict resolution strategy: %s", cr.strategy)
	}

	// Record conflict in history
	cr.recordConflict(metadata)

	// Store conflict resolution in Redis for audit
	if err := cr.storeConflictResolution(ctx, metadata); err != nil {
		cr.logger.Warn("Failed to store conflict resolution",
			zap.Error(err),
			zap.String("winner_id", winner.ID),
		)
	}

	cr.mu.Lock()
	cr.conflictsResolved++
	cr.mu.Unlock()

	cr.logger.Info("Conflict resolved",
		zap.String("strategy", string(cr.strategy)),
		zap.String("winner_id", winner.ID),
		zap.Int("candidates", len(versions)),
	)

	return winner, metadata, nil
}

// resolveLWW implements Last Write Wins conflict resolution
func (cr *ConflictResolver) resolveLWW(versions ...*VersionedData) (*VersionedData, ConflictMetadata) {
	// Sort by timestamp, then by sequence, then by actor ID for deterministic tiebreaking
	sort.Slice(versions, func(i, j int) bool {
		vi, vj := versions[i], versions[j]

		// First compare timestamps
		if !vi.Timestamp.Equal(vj.Timestamp) {
			return vi.Timestamp.After(vj.Timestamp) // Latest timestamp wins
		}

		// If timestamps are equal, compare sequence numbers
		if vi.Sequence != vj.Sequence {
			return vi.Sequence > vj.Sequence // Higher sequence wins
		}

		// If still tied, use actor ID for deterministic ordering
		return vi.ActorID > vj.ActorID // Lexicographically higher actor ID wins
	})

	winner := versions[0]

	// Build conflict metadata
	conflictingIDs := make([]string, len(versions))
	for i, v := range versions {
		conflictingIDs[i] = v.ID
	}

	metadata := ConflictMetadata{
		DetectedAt:     time.Now(),
		ConflictingIDs: conflictingIDs,
		Resolution:     StrategyLWW,
		WinnerID:       winner.ID,
		Details: map[string]interface{}{
			"winner_timestamp": winner.Timestamp,
			"winner_sequence":  winner.Sequence,
			"winner_actor":     winner.ActorID,
			"total_candidates": len(versions),
		},
	}

	return winner, metadata
}

// resolveVectorClock implements vector clock-based conflict resolution
func (cr *ConflictResolver) resolveVectorClock(versions ...*VersionedData) (*VersionedData, ConflictMetadata) {
	// Check for causal relationships using vector clocks
	var winner *VersionedData
	var concurrent []*VersionedData

	for _, v := range versions {
		if v.VectorClock == nil {
			// Fall back to LWW if no vector clock
			return cr.resolveLWW(versions...)
		}

		isConcurrent := true
		for _, other := range versions {
			if v.ID == other.ID {
				continue
			}

			if cr.happensBefore(v.VectorClock, other.VectorClock) {
				// v happened before other, so other is newer
				isConcurrent = false
				break
			} else if cr.happensBefore(other.VectorClock, v.VectorClock) {
				// other happened before v, so v is newer
				if winner == nil || cr.happensBefore(winner.VectorClock, v.VectorClock) {
					winner = v
				}
				isConcurrent = false
			}
		}

		if isConcurrent {
			concurrent = append(concurrent, v)
		}
	}

	// If we have concurrent versions, use LWW as tiebreaker
	if len(concurrent) > 0 {
		return cr.resolveLWW(concurrent...)
	}

	if winner == nil {
		// All versions are concurrent, use LWW
		return cr.resolveLWW(versions...)
	}

	conflictingIDs := make([]string, len(versions))
	for i, v := range versions {
		conflictingIDs[i] = v.ID
	}

	metadata := ConflictMetadata{
		DetectedAt:     time.Now(),
		ConflictingIDs: conflictingIDs,
		Resolution:     StrategyVectorClock,
		WinnerID:       winner.ID,
		Details: map[string]interface{}{
			"vector_clock": winner.VectorClock,
			"causality":    "happened-after",
		},
	}

	return winner, metadata
}

// happensBefore checks if vector clock a happened before b
func (cr *ConflictResolver) happensBefore(a, b map[string]uint64) bool {
	atLeastOneLess := false

	for actor, aVersion := range a {
		bVersion, exists := b[actor]
		if !exists {
			bVersion = 0
		}

		if aVersion > bVersion {
			return false // a has a component greater than b
		}
		if aVersion < bVersion {
			atLeastOneLess = true
		}
	}

	// Check if b has any actors that a doesn't have
	for actor := range b {
		if _, exists := a[actor]; !exists {
			atLeastOneLess = true
		}
	}

	return atLeastOneLess
}

// DetectConflicts checks if multiple versions are in conflict
func (cr *ConflictResolver) DetectConflicts(versions ...*VersionedData) bool {
	if len(versions) < 2 {
		return false
	}

	// Check if any versions have the same parent but different content
	parentMap := make(map[string][]*VersionedData)

	for _, v := range versions {
		if v.ParentID != "" {
			parentMap[v.ParentID] = append(parentMap[v.ParentID], v)
		}
	}

	// If multiple versions share the same parent, they're in conflict
	for _, siblings := range parentMap {
		if len(siblings) > 1 {
			return true
		}
	}

	// Also check for same resource ID with different versions
	resourceMap := make(map[string][]*VersionedData)
	for _, v := range versions {
		// Extract resource ID from metadata or use a convention
		resourceID := v.ID // This could be more sophisticated
		if meta, ok := v.Metadata["resource_id"].(string); ok {
			resourceID = meta
		}
		resourceMap[resourceID] = append(resourceMap[resourceID], v)
	}

	for _, versions := range resourceMap {
		if len(versions) > 1 {
			// Check if they're actually different versions
			checksums := make(map[string]bool)
			for _, v := range versions {
				checksums[v.Checksum] = true
			}
			if len(checksums) > 1 {
				return true // Different checksums = conflict
			}
		}
	}

	return false
}

// ApplyResolution applies the winning version and ensures idempotency
func (cr *ConflictResolver) ApplyResolution(ctx context.Context, winner *VersionedData, losers []*VersionedData) error {
	// Store the winner as the authoritative version
	key := fmt.Sprintf("version:%s:current", winner.ID)
	data, err := json.Marshal(winner)
	if err != nil {
		return fmt.Errorf("failed to marshal winner: %w", err)
	}

	// Use a transaction to ensure atomicity
	pipe := cr.redis.TxPipeline()

	// Set the winning version
	pipe.Set(ctx, key, data, 24*time.Hour)

	// Mark losers as superseded
	for _, loser := range losers {
		loserKey := fmt.Sprintf("version:%s:superseded", loser.ID)
		pipe.Set(ctx, loserKey, winner.ID, 24*time.Hour)
	}

	// Create a conflict resolution record
	resolutionKey := fmt.Sprintf("conflict:resolution:%d", time.Now().UnixNano())
	resolutionData := map[string]interface{}{
		"winner_id":  winner.ID,
		"loser_ids":  getVersionIDs(losers),
		"timestamp":  time.Now().Unix(),
		"strategy":   string(cr.strategy),
	}
	resolutionJSON, _ := json.Marshal(resolutionData)
	pipe.Set(ctx, resolutionKey, resolutionJSON, 7*24*time.Hour)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to apply resolution: %w", err)
	}

	cr.logger.Debug("Applied conflict resolution",
		zap.String("winner_id", winner.ID),
		zap.Int("losers", len(losers)),
	)

	return nil
}

// recordConflict adds a conflict to the history
func (cr *ConflictResolver) recordConflict(metadata ConflictMetadata) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	cr.conflictHistory = append(cr.conflictHistory, metadata)

	// Trim history if it exceeds max size
	if len(cr.conflictHistory) > cr.maxHistory {
		cr.conflictHistory = cr.conflictHistory[len(cr.conflictHistory)-cr.maxHistory:]
	}
}

// storeConflictResolution stores conflict resolution in Redis for audit
func (cr *ConflictResolver) storeConflictResolution(ctx context.Context, metadata ConflictMetadata) error {
	key := fmt.Sprintf("conflict:audit:%d", time.Now().UnixNano())

	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Store with 30-day retention for audit purposes
	return cr.redis.Set(ctx, key, data, 30*24*time.Hour).Err()
}

// GetConflictHistory returns recent conflict resolution history
func (cr *ConflictResolver) GetConflictHistory() []ConflictMetadata {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	history := make([]ConflictMetadata, len(cr.conflictHistory))
	copy(history, cr.conflictHistory)
	return history
}

// GetMetrics returns conflict resolution metrics
func (cr *ConflictResolver) GetMetrics() map[string]interface{} {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	return map[string]interface{}{
		"conflicts_detected":  cr.conflictsDetected,
		"conflicts_resolved":  cr.conflictsResolved,
		"history_size":        len(cr.conflictHistory),
		"strategy":            string(cr.strategy),
		"resolution_rate":     float64(cr.conflictsResolved) / float64(cr.conflictsDetected) * 100,
	}
}

// CreateSnapshot creates a consistent snapshot after conflict resolution
func (cr *ConflictResolver) CreateSnapshot(ctx context.Context, winner *VersionedData) error {
	snapshotKey := fmt.Sprintf("snapshot:%s:%d", winner.ID, winner.Version)

	snapshot := map[string]interface{}{
		"version":    winner.Version,
		"timestamp":  winner.Timestamp.Unix(),
		"actor_id":   winner.ActorID,
		"data":       winner.Data,
		"checksum":   winner.Checksum,
		"created_at": time.Now().Unix(),
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Store snapshot with expiration
	err = cr.redis.Set(ctx, snapshotKey, data, 7*24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to store snapshot: %w", err)
	}

	// Update latest snapshot pointer
	latestKey := fmt.Sprintf("snapshot:%s:latest", winner.ID)
	err = cr.redis.Set(ctx, latestKey, snapshotKey, 7*24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to update latest snapshot: %w", err)
	}

	return nil
}

// getVersionIDs extracts IDs from versioned data
func getVersionIDs(versions []*VersionedData) []string {
	ids := make([]string, len(versions))
	for i, v := range versions {
		ids[i] = v.ID
	}
	return ids
}

// MergeStrategy defines how to merge concurrent changes
type MergeStrategy interface {
	Merge(ctx context.Context, versions ...*VersionedData) (*VersionedData, error)
}

// OperationalTransform implements operational transformation for text
type OperationalTransform struct {
	logger *zap.Logger
}

// Transform transforms operations to handle concurrent edits
func (ot *OperationalTransform) Transform(op1, op2 json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	// This would implement OT algorithm for specific data types
	// For now, return as-is
	return op1, op2, nil
}

// CRDT represents a Conflict-free Replicated Data Type
type CRDT interface {
	Merge(other CRDT) CRDT
	Value() interface{}
}

// LWWRegister implements a Last-Write-Wins Register CRDT
type LWWRegister struct {
	value     interface{}
	timestamp time.Time
	actorID   string
}

// Merge merges two LWW registers
func (r *LWWRegister) Merge(other CRDT) CRDT {
	otherReg, ok := other.(*LWWRegister)
	if !ok {
		return r
	}

	// LWW: latest timestamp wins
	if otherReg.timestamp.After(r.timestamp) {
		return otherReg
	} else if otherReg.timestamp.Equal(r.timestamp) && otherReg.actorID > r.actorID {
		// Tiebreak with actor ID
		return otherReg
	}

	return r
}

// Value returns the register value
func (r *LWWRegister) Value() interface{} {
	return r.value
}