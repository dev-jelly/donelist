package websocket

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoomKeyString(t *testing.T) {
	tests := []struct {
		name     string
		roomKey  RoomKey
		expected string
	}{
		{
			name:     "User room",
			roomKey:  RoomKey{Type: RoomTypeUser, UserID: "user-123"},
			expected: "user:user-123",
		},
		{
			name:     "Team room",
			roomKey:  RoomKey{Type: RoomTypeTeam, TeamID: "team-456"},
			expected: "team:team-456",
		},
		{
			name:     "Device room",
			roomKey:  RoomKey{Type: RoomTypeDevice, UserID: "user-123", DeviceID: "device-789"},
			expected: "device:user-123:device-789",
		},
		{
			name:     "User-Team room",
			roomKey:  RoomKey{Type: RoomTypeUserTeam, UserID: "user-123", TeamID: "team-456"},
			expected: "user:user-123:team:team-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.roomKey.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRoomKey(t *testing.T) {
	tests := []struct {
		name      string
		roomID    string
		expected  *RoomKey
		shouldErr bool
	}{
		{
			name:   "Valid user room",
			roomID: "user:user-123",
			expected: &RoomKey{
				Type:   RoomTypeUser,
				UserID: "user-123",
			},
			shouldErr: false,
		},
		{
			name:   "Valid team room",
			roomID: "team:team-456",
			expected: &RoomKey{
				Type:   RoomTypeTeam,
				TeamID: "team-456",
			},
			shouldErr: false,
		},
		{
			name:   "Valid device room",
			roomID: "device:user-123:device-789",
			expected: &RoomKey{
				Type:     RoomTypeDevice,
				UserID:   "user-123",
				DeviceID: "device-789",
			},
			shouldErr: false,
		},
		{
			name:   "Valid user-team room",
			roomID: "user:user-123:team:team-456",
			expected: &RoomKey{
				Type:   RoomTypeUserTeam,
				UserID: "user-123",
				TeamID: "team-456",
			},
			shouldErr: false,
		},
		{
			name:      "Invalid format",
			roomID:    "invalid",
			expected:  nil,
			shouldErr: true,
		},
		{
			name:      "Invalid device format",
			roomID:    "device:user-123",
			expected:  nil,
			shouldErr: true,
		},
		{
			name:      "Unknown room type",
			roomID:    "unknown:123",
			expected:  nil,
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRoomKey(tt.roomID)

			if tt.shouldErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected.Type, result.Type)
				assert.Equal(t, tt.expected.UserID, result.UserID)
				assert.Equal(t, tt.expected.TeamID, result.TeamID)
				assert.Equal(t, tt.expected.DeviceID, result.DeviceID)
			}
		})
	}
}

func TestRoomTopology_GetUserRooms(t *testing.T) {
	topology := NewRoomTopology()

	userID := "user-123"
	teamIDs := []string{"team-1", "team-2"}
	deviceID := "device-456"

	rooms := topology.GetUserRooms(userID, teamIDs, deviceID)

	// Should include:
	// 1. User room
	// 2. Team rooms (2)
	// 3. User-team rooms (2)
	// 4. Device room
	// Total: 6 rooms
	assert.Len(t, rooms, 6)

	// Verify specific rooms
	assert.Contains(t, rooms, "user:user-123")
	assert.Contains(t, rooms, "team:team-1")
	assert.Contains(t, rooms, "team:team-2")
	assert.Contains(t, rooms, "user:user-123:team:team-1")
	assert.Contains(t, rooms, "user:user-123:team:team-2")
	assert.Contains(t, rooms, "device:user-123:device-456")
}

func TestRoomTopology_GetUserRoomsWithoutDevice(t *testing.T) {
	topology := NewRoomTopology()

	userID := "user-123"
	teamIDs := []string{"team-1"}
	deviceID := ""

	rooms := topology.GetUserRooms(userID, teamIDs, deviceID)

	// Should include:
	// 1. User room
	// 2. Team room
	// 3. User-team room
	// Total: 3 rooms
	assert.Len(t, rooms, 3)
	assert.Contains(t, rooms, "user:user-123")
	assert.Contains(t, rooms, "team:team-1")
	assert.Contains(t, rooms, "user:user-123:team:team-1")
	assert.NotContains(t, rooms, "device:user-123:")
}

func TestRoomTopology_ValidateRoomAccess(t *testing.T) {
	topology := NewRoomTopology()

	tests := []struct {
		name       string
		userID     string
		teamIDs    []string
		roomID     string
		shouldPass bool
	}{
		{
			name:       "User accessing own user room",
			userID:     "user-123",
			teamIDs:    []string{},
			roomID:     "user:user-123",
			shouldPass: true,
		},
		{
			name:       "User accessing other user room",
			userID:     "user-123",
			teamIDs:    []string{},
			roomID:     "user:user-456",
			shouldPass: false,
		},
		{
			name:       "User accessing team room they belong to",
			userID:     "user-123",
			teamIDs:    []string{"team-1", "team-2"},
			roomID:     "team:team-1",
			shouldPass: true,
		},
		{
			name:       "User accessing team room they don't belong to",
			userID:     "user-123",
			teamIDs:    []string{"team-1"},
			roomID:     "team:team-2",
			shouldPass: false,
		},
		{
			name:       "User accessing own device room",
			userID:     "user-123",
			teamIDs:    []string{},
			roomID:     "device:user-123:device-456",
			shouldPass: true,
		},
		{
			name:       "User accessing other user's device room",
			userID:     "user-123",
			teamIDs:    []string{},
			roomID:     "device:user-456:device-789",
			shouldPass: false,
		},
		{
			name:       "User accessing own user-team room",
			userID:     "user-123",
			teamIDs:    []string{"team-1"},
			roomID:     "user:user-123:team:team-1",
			shouldPass: true,
		},
		{
			name:       "User accessing user-team room for team they don't belong to",
			userID:     "user-123",
			teamIDs:    []string{"team-1"},
			roomID:     "user:user-123:team:team-2",
			shouldPass: false,
		},
		{
			name:       "User accessing other user's user-team room",
			userID:     "user-123",
			teamIDs:    []string{"team-1"},
			roomID:     "user:user-456:team:team-1",
			shouldPass: false,
		},
		{
			name:       "Invalid room ID",
			userID:     "user-123",
			teamIDs:    []string{},
			roomID:     "invalid-room",
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := topology.ValidateRoomAccess(tt.userID, tt.teamIDs, tt.roomID)
			assert.Equal(t, tt.shouldPass, result)
		})
	}
}

func TestRoomTopology_GetShardingKey(t *testing.T) {
	topology := NewRoomTopology()

	tests := []struct {
		name     string
		roomID   string
		expected string
	}{
		{
			name:     "User room sharding",
			roomID:   "user:user-123",
			expected: "user-123",
		},
		{
			name:     "Team room sharding",
			roomID:   "team:team-456",
			expected: "team-456",
		},
		{
			name:     "Device room sharding",
			roomID:   "device:user-123:device-789",
			expected: "user-123",
		},
		{
			name:     "User-team room sharding",
			roomID:   "user:user-123:team:team-456",
			expected: "user-123",
		},
		{
			name:     "Invalid room ID sharding",
			roomID:   "invalid",
			expected: "invalid", // Fallback to room ID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := topology.GetShardingKey(tt.roomID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoomTopology_RoundTrip(t *testing.T) {
	// Test that we can convert a RoomKey to string and back
	original := RoomKey{
		Type:     RoomTypeDevice,
		UserID:   "user-123",
		DeviceID: "device-456",
	}

	roomID := original.String()
	assert.Equal(t, "device:user-123:device-456", roomID)

	parsed, err := ParseRoomKey(roomID)
	require.NoError(t, err)

	assert.Equal(t, original.Type, parsed.Type)
	assert.Equal(t, original.UserID, parsed.UserID)
	assert.Equal(t, original.DeviceID, parsed.DeviceID)
}

func TestRoomTopology_GetHelpers(t *testing.T) {
	topology := NewRoomTopology()

	t.Run("GetUserRoom", func(t *testing.T) {
		roomID := topology.GetUserRoom("user-123")
		assert.Equal(t, "user:user-123", roomID)
	})

	t.Run("GetTeamRoom", func(t *testing.T) {
		roomID := topology.GetTeamRoom("team-456")
		assert.Equal(t, "team:team-456", roomID)
	})

	t.Run("GetDeviceRoom", func(t *testing.T) {
		roomID := topology.GetDeviceRoom("user-123", "device-789")
		assert.Equal(t, "device:user-123:device-789", roomID)
	})

	t.Run("GetUserTeamRoom", func(t *testing.T) {
		roomID := topology.GetUserTeamRoom("user-123", "team-456")
		assert.Equal(t, "user:user-123:team:team-456", roomID)
	})
}
