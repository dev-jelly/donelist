package websocket

import (
	"fmt"
	"strings"
)

// RoomType defines the type of room
type RoomType string

const (
	// RoomTypeUser is for user-specific updates
	RoomTypeUser RoomType = "user"

	// RoomTypeTeam is for team-wide updates
	RoomTypeTeam RoomType = "team"

	// RoomTypeDevice is for device-specific updates
	RoomTypeDevice RoomType = "device"

	// RoomTypeUserTeam is for user within a team
	RoomTypeUserTeam RoomType = "user_team"
)

// RoomKey represents a structured room identifier
type RoomKey struct {
	Type     RoomType
	UserID   string
	TeamID   string
	DeviceID string
}

// String returns the room ID string representation
func (rk *RoomKey) String() string {
	switch rk.Type {
	case RoomTypeUser:
		return fmt.Sprintf("user:%s", rk.UserID)
	case RoomTypeTeam:
		return fmt.Sprintf("team:%s", rk.TeamID)
	case RoomTypeDevice:
		return fmt.Sprintf("device:%s:%s", rk.UserID, rk.DeviceID)
	case RoomTypeUserTeam:
		return fmt.Sprintf("user:%s:team:%s", rk.UserID, rk.TeamID)
	default:
		return ""
	}
}

// ParseRoomKey parses a room ID string into a RoomKey
func ParseRoomKey(roomID string) (*RoomKey, error) {
	parts := strings.Split(roomID, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid room ID format: %s", roomID)
	}

	rk := &RoomKey{}

	switch parts[0] {
	case "user":
		if len(parts) == 2 {
			rk.Type = RoomTypeUser
			rk.UserID = parts[1]
		} else if len(parts) == 4 && parts[2] == "team" {
			rk.Type = RoomTypeUserTeam
			rk.UserID = parts[1]
			rk.TeamID = parts[3]
		} else {
			return nil, fmt.Errorf("invalid user room format: %s", roomID)
		}
	case "team":
		rk.Type = RoomTypeTeam
		rk.TeamID = parts[1]
	case "device":
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid device room format: %s", roomID)
		}
		rk.Type = RoomTypeDevice
		rk.UserID = parts[1]
		rk.DeviceID = parts[2]
	default:
		return nil, fmt.Errorf("unknown room type: %s", parts[0])
	}

	return rk, nil
}

// RoomTopology manages the room naming and subscription topology
type RoomTopology struct{}

// NewRoomTopology creates a new RoomTopology manager
func NewRoomTopology() *RoomTopology {
	return &RoomTopology{}
}

// GetUserRoom returns the room ID for a specific user
func (rt *RoomTopology) GetUserRoom(userID string) string {
	return (&RoomKey{Type: RoomTypeUser, UserID: userID}).String()
}

// GetTeamRoom returns the room ID for a specific team
func (rt *RoomTopology) GetTeamRoom(teamID string) string {
	return (&RoomKey{Type: RoomTypeTeam, TeamID: teamID}).String()
}

// GetDeviceRoom returns the room ID for a user's specific device
func (rt *RoomTopology) GetDeviceRoom(userID, deviceID string) string {
	return (&RoomKey{Type: RoomTypeDevice, UserID: userID, DeviceID: deviceID}).String()
}

// GetUserTeamRoom returns the room ID for a user within a team
func (rt *RoomTopology) GetUserTeamRoom(userID, teamID string) string {
	return (&RoomKey{Type: RoomTypeUserTeam, UserID: userID, TeamID: teamID}).String()
}

// GetUserRooms returns all room IDs a user should subscribe to
func (rt *RoomTopology) GetUserRooms(userID string, teamIDs []string, deviceID string) []string {
	rooms := []string{
		rt.GetUserRoom(userID),
	}

	// Add team rooms
	for _, teamID := range teamIDs {
		rooms = append(rooms, rt.GetTeamRoom(teamID))
		rooms = append(rooms, rt.GetUserTeamRoom(userID, teamID))
	}

	// Add device room if specified
	if deviceID != "" {
		rooms = append(rooms, rt.GetDeviceRoom(userID, deviceID))
	}

	return rooms
}

// ValidateRoomAccess checks if a user has access to a room
func (rt *RoomTopology) ValidateRoomAccess(userID string, teamIDs []string, roomID string) bool {
	roomKey, err := ParseRoomKey(roomID)
	if err != nil {
		return false
	}

	switch roomKey.Type {
	case RoomTypeUser:
		// User can only access their own user room
		return roomKey.UserID == userID

	case RoomTypeTeam:
		// User can access team rooms they belong to
		for _, teamID := range teamIDs {
			if teamID == roomKey.TeamID {
				return true
			}
		}
		return false

	case RoomTypeDevice:
		// User can only access their own device rooms
		return roomKey.UserID == userID

	case RoomTypeUserTeam:
		// User can access their own user-team rooms
		if roomKey.UserID != userID {
			return false
		}
		for _, teamID := range teamIDs {
			if teamID == roomKey.TeamID {
				return true
			}
		}
		return false

	default:
		return false
	}
}

// GetShardingKey returns a consistent sharding key for load balancing
func (rt *RoomTopology) GetShardingKey(roomID string) string {
	roomKey, err := ParseRoomKey(roomID)
	if err != nil {
		return roomID // Fallback to room ID itself
	}

	// Shard by user ID for user-centric rooms
	switch roomKey.Type {
	case RoomTypeUser, RoomTypeDevice, RoomTypeUserTeam:
		return roomKey.UserID
	case RoomTypeTeam:
		return roomKey.TeamID
	default:
		return roomID
	}
}
