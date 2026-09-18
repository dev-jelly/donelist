package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupTestConflictResolver(t *testing.T) (*ConflictResolver, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger, _ := zap.NewDevelopment()
	resolver := NewConflictResolver(StrategyLWW, redisClient, logger)

	return resolver, mr
}

func TestLWWConflictResolution(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	ctx := context.Background()

	// Create conflicting versions
	v1 := &VersionedData{
		ID:        "v1",
		ActorID:   "actor1",
		Version:   1,
		Timestamp: time.Now().Add(-2 * time.Second),
		Sequence:  1,
		Data:      json.RawMessage(`{"value": "version1"}`),
		Checksum:  "checksum1",
	}

	v2 := &VersionedData{
		ID:        "v2",
		ActorID:   "actor2",
		Version:   2,
		Timestamp: time.Now().Add(-1 * time.Second),
		Sequence:  2,
		Data:      json.RawMessage(`{"value": "version2"}`),
		Checksum:  "checksum2",
	}

	v3 := &VersionedData{
		ID:        "v3",
		ActorID:   "actor3",
		Version:   3,
		Timestamp: time.Now(), // Latest timestamp
		Sequence:  3,
		Data:      json.RawMessage(`{"value": "version3"}`),
		Checksum:  "checksum3",
	}

	// Test resolution
	winner, metadata, err := resolver.Resolve(ctx, v1, v2, v3)
	require.NoError(t, err)
	assert.NotNil(t, winner)
	assert.Equal(t, "v3", winner.ID) // v3 has the latest timestamp
	assert.Equal(t, StrategyLWW, metadata.Resolution)
	assert.Len(t, metadata.ConflictingIDs, 3)
	assert.Equal(t, "v3", metadata.WinnerID)
}

func TestLWWTiebreaking(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	ctx := context.Background()
	now := time.Now()

	// Create versions with same timestamp but different sequences
	v1 := &VersionedData{
		ID:        "v1",
		ActorID:   "actor1",
		Version:   1,
		Timestamp: now,
		Sequence:  10,
		Data:      json.RawMessage(`{"value": "version1"}`),
	}

	v2 := &VersionedData{
		ID:        "v2",
		ActorID:   "actor2",
		Version:   2,
		Timestamp: now,
		Sequence:  20, // Higher sequence
		Data:      json.RawMessage(`{"value": "version2"}`),
	}

	winner, _, err := resolver.Resolve(ctx, v1, v2)
	require.NoError(t, err)
	assert.Equal(t, "v2", winner.ID) // v2 has higher sequence

	// Test with same timestamp and sequence, different actor IDs
	v3 := &VersionedData{
		ID:        "v3",
		ActorID:   "actor_a",
		Version:   3,
		Timestamp: now,
		Sequence:  20,
		Data:      json.RawMessage(`{"value": "version3"}`),
	}

	v4 := &VersionedData{
		ID:        "v4",
		ActorID:   "actor_b", // Lexicographically higher
		Version:   4,
		Timestamp: now,
		Sequence:  20,
		Data:      json.RawMessage(`{"value": "version4"}`),
	}

	winner2, _, err := resolver.Resolve(ctx, v3, v4)
	require.NoError(t, err)
	assert.Equal(t, "v4", winner2.ID) // v4 has lexicographically higher actor ID
}

func TestVectorClockResolution(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	resolver.strategy = StrategyVectorClock
	ctx := context.Background()

	// Create versions with vector clocks
	v1 := &VersionedData{
		ID:      "v1",
		ActorID: "actor1",
		VectorClock: map[string]uint64{
			"actor1": 1,
			"actor2": 0,
		},
		Data: json.RawMessage(`{"value": "version1"}`),
	}

	v2 := &VersionedData{
		ID:      "v2",
		ActorID: "actor2",
		VectorClock: map[string]uint64{
			"actor1": 1,
			"actor2": 1, // v2 happened after v1
		},
		Data: json.RawMessage(`{"value": "version2"}`),
	}

	winner, metadata, err := resolver.Resolve(ctx, v1, v2)
	require.NoError(t, err)
	assert.Equal(t, "v2", winner.ID) // v2 happened after v1
	assert.Equal(t, StrategyVectorClock, metadata.Resolution)
}

func TestConcurrentVectorClocks(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	resolver.strategy = StrategyVectorClock
	ctx := context.Background()

	// Create concurrent versions (no causal relationship)
	v1 := &VersionedData{
		ID:        "v1",
		ActorID:   "actor1",
		Timestamp: time.Now().Add(-1 * time.Second),
		VectorClock: map[string]uint64{
			"actor1": 2,
			"actor2": 0,
		},
		Data: json.RawMessage(`{"value": "version1"}`),
	}

	v2 := &VersionedData{
		ID:        "v2",
		ActorID:   "actor2",
		Timestamp: time.Now(), // Later timestamp
		VectorClock: map[string]uint64{
			"actor1": 0,
			"actor2": 2,
		},
		Data: json.RawMessage(`{"value": "version2"}`),
	}

	winner, _, err := resolver.Resolve(ctx, v1, v2)
	require.NoError(t, err)
	// Should fall back to LWW for concurrent versions
	assert.Equal(t, "v2", winner.ID) // v2 has later timestamp
}

func TestConflictDetection(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	// Test versions with same parent
	v1 := &VersionedData{
		ID:       "v1",
		ParentID: "parent1",
		Checksum: "checksum1",
	}

	v2 := &VersionedData{
		ID:       "v2",
		ParentID: "parent1", // Same parent
		Checksum: "checksum2", // Different content
	}

	hasConflict := resolver.DetectConflicts(v1, v2)
	assert.True(t, hasConflict)

	// Test versions with different parents
	v3 := &VersionedData{
		ID:       "v3",
		ParentID: "parent2",
		Checksum: "checksum3",
	}

	hasConflict = resolver.DetectConflicts(v1, v3)
	assert.False(t, hasConflict)
}

func TestApplyResolution(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	ctx := context.Background()

	winner := &VersionedData{
		ID:        "winner",
		ActorID:   "actor1",
		Version:   3,
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"value": "winning_version"}`),
	}

	losers := []*VersionedData{
		{
			ID:      "loser1",
			ActorID: "actor2",
			Version: 1,
		},
		{
			ID:      "loser2",
			ActorID: "actor3",
			Version: 2,
		},
	}

	err := resolver.ApplyResolution(ctx, winner, losers)
	require.NoError(t, err)

	// Verify winner is stored
	key := "version:winner:current"
	exists := mr.Exists(key)
	assert.True(t, exists)

	// Verify losers are marked as superseded
	assert.True(t, mr.Exists("version:loser1:superseded"))
	assert.True(t, mr.Exists("version:loser2:superseded"))
}

func TestCustomResolver(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	resolver.strategy = StrategyCustom
	ctx := context.Background()

	// Set custom resolver that always picks the version with highest version number
	resolver.SetCustomResolver(func(a, b *VersionedData) (*VersionedData, ConflictMetadata) {
		winner := a
		if b.Version > a.Version {
			winner = b
		}

		return winner, ConflictMetadata{
			DetectedAt:     time.Now(),
			ConflictingIDs: []string{a.ID, b.ID},
			Resolution:     StrategyCustom,
			WinnerID:       winner.ID,
		}
	})

	v1 := &VersionedData{
		ID:      "v1",
		Version: 10, // Higher version number
		Data:    json.RawMessage(`{"value": "version1"}`),
	}

	v2 := &VersionedData{
		ID:      "v2",
		Version: 5,
		Data:    json.RawMessage(`{"value": "version2"}`),
	}

	winner, metadata, err := resolver.Resolve(ctx, v1, v2)
	require.NoError(t, err)
	assert.Equal(t, "v1", winner.ID) // v1 has higher version number
	assert.Equal(t, StrategyCustom, metadata.Resolution)
}

func TestCreateSnapshot(t *testing.T) {
	resolver, mr := setupTestConflictResolver(t)
	defer mr.Close()

	ctx := context.Background()

	version := &VersionedData{
		ID:        "test_id",
		ActorID:   "actor1",
		Version:   5,
		Timestamp: time.Now(),
		Data:      json.RawMessage(`{"value": "test_data"}`),
		Checksum:  "test_checksum",
	}

	err := resolver.CreateSnapshot(ctx, version)
	require.NoError(t, err)

	// Verify snapshot is created
	snapshotKey := "snapshot:test_id:5"
	exists := mr.Exists(snapshotKey)
	assert.True(t, exists)

	// Verify latest pointer is updated
	latestKey := "snapshot:test_id:latest"
	exists = mr.Exists(latestKey)
	assert.True(t, exists)
}

func TestConflictHistory(t *testing.T) {
	resolver, _ := setupTestConflictResolver(t)

	// Record some conflicts
	for i := 0; i < 5; i++ {
		metadata := ConflictMetadata{
			DetectedAt:     time.Now(),
			ConflictingIDs: []string{"id1", "id2"},
			Resolution:     StrategyLWW,
			WinnerID:       "id1",
		}
		resolver.recordConflict(metadata)
	}

	history := resolver.GetConflictHistory()
	assert.Len(t, history, 5)

	metrics := resolver.GetMetrics()
	assert.Equal(t, 5, metrics["history_size"])
}

func TestLWWRegisterCRDT(t *testing.T) {
	now := time.Now()

	reg1 := &LWWRegister{
		value:     "value1",
		timestamp: now,
		actorID:   "actor1",
	}

	reg2 := &LWWRegister{
		value:     "value2",
		timestamp: now.Add(1 * time.Second), // Later timestamp
		actorID:   "actor2",
	}

	merged := reg1.Merge(reg2).(*LWWRegister)
	assert.Equal(t, "value2", merged.Value())

	// Test tiebreaking with actor ID
	reg3 := &LWWRegister{
		value:     "value3",
		timestamp: now,
		actorID:   "actor_b", // Lexicographically higher
	}

	reg4 := &LWWRegister{
		value:     "value4",
		timestamp: now,
		actorID:   "actor_a",
	}

	merged2 := reg3.Merge(reg4).(*LWWRegister)
	assert.Equal(t, "value3", merged2.Value())
}

func TestHappensBefore(t *testing.T) {
	resolver, _ := setupTestConflictResolver(t)

	// Test clear happens-before relationship
	vc1 := map[string]uint64{
		"actor1": 1,
		"actor2": 2,
	}

	vc2 := map[string]uint64{
		"actor1": 2,
		"actor2": 3,
	}

	assert.True(t, resolver.happensBefore(vc1, vc2))
	assert.False(t, resolver.happensBefore(vc2, vc1))

	// Test concurrent vector clocks
	vc3 := map[string]uint64{
		"actor1": 3,
		"actor2": 1,
	}

	vc4 := map[string]uint64{
		"actor1": 1,
		"actor2": 3,
	}

	assert.False(t, resolver.happensBefore(vc3, vc4))
	assert.False(t, resolver.happensBefore(vc4, vc3))
}

func BenchmarkLWWResolution(b *testing.B) {
	resolver, mr := setupTestConflictResolver(b)
	defer mr.Close()

	ctx := context.Background()
	versions := make([]*VersionedData, 10)

	for i := 0; i < 10; i++ {
		versions[i] = &VersionedData{
			ID:        string(rune(i)),
			ActorID:   string(rune(i)),
			Version:   uint64(i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Sequence:  uint64(i),
			Data:      json.RawMessage(`{"value": "test"}`),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = resolver.Resolve(ctx, versions...)
	}
}