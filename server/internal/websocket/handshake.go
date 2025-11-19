package websocket

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// HandshakeRequest represents the initial handshake from the client
type HandshakeRequest struct {
	// ClientVersion is the client application version
	ClientVersion string `json:"client_version" validate:"required,max=32"`

	// SupportedFeatures lists the features the client supports
	SupportedFeatures []string `json:"supported_features" validate:"max=50,dive,min=1,max=64"`

	// LastAckSeq is the last acknowledged sequence number (for reconnection)
	LastAckSeq int64 `json:"last_ack_seq,omitempty" validate:"min=0"`

	// Capabilities describes client capabilities (compression, encryption, etc.)
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`

	// SessionID for reconnection (empty for new connections)
	SessionID string `json:"session_id,omitempty" validate:"omitempty,uuid"`

	// Platform information
	Platform string `json:"platform,omitempty" validate:"omitempty,max=32"`

	// Device information
	DeviceID string `json:"device_id,omitempty" validate:"omitempty,max=128"`
}

// HandshakeResponse represents the server's response to the handshake
type HandshakeResponse struct {
	// SessionID is the unique session identifier issued by the server
	SessionID string `json:"session_id" validate:"required,uuid"`

	// ServerVersion is the server version
	ServerVersion string `json:"server_version" validate:"required,max=32"`

	// SupportedFeatures lists the features the server supports
	SupportedFeatures []string `json:"supported_features" validate:"max=50,dive,min=1,max=64"`

	// EnabledFeatures lists the features enabled for this session (intersection of client/server)
	EnabledFeatures []string `json:"enabled_features" validate:"max=50,dive,min=1,max=64"`

	// HeartbeatInterval is the interval for sending heartbeat pings (milliseconds)
	HeartbeatInterval int64 `json:"heartbeat_interval" validate:"required,min=1000"`

	// HeartbeatTimeout is the timeout for heartbeat responses (milliseconds)
	HeartbeatTimeout int64 `json:"heartbeat_timeout" validate:"required,min=1000"`

	// MaxMessageSize is the maximum message size in bytes
	MaxMessageSize int `json:"max_message_size" validate:"required,min=512"`

	// ResumeFromSeq is the sequence number to resume from (for reconnection)
	ResumeFromSeq int64 `json:"resume_from_seq,omitempty" validate:"min=0"`

	// ServerCapabilities describes server capabilities
	ServerCapabilities map[string]interface{} `json:"server_capabilities,omitempty"`

	// Status indicates handshake result
	Status string `json:"status" validate:"required,oneof=accepted rejected"`

	// Message provides additional information
	Message string `json:"message,omitempty" validate:"omitempty,max=500"`
}

// ConnectionState represents the state of a WebSocket connection
type ConnectionState string

const (
	// StateConnecting indicates the connection is being established
	StateConnecting ConnectionState = "connecting"

	// StateHandshaking indicates the handshake is in progress
	StateHandshaking ConnectionState = "handshaking"

	// StateConnected indicates the connection is fully established
	StateConnected ConnectionState = "connected"

	// StateStale indicates the connection is stale (no heartbeat response)
	StateStale ConnectionState = "stale"

	// StateReconnecting indicates the connection is attempting to reconnect
	StateReconnecting ConnectionState = "reconnecting"

	// StateDisconnected indicates the connection is closed
	StateDisconnected ConnectionState = "disconnected"
)

// SessionInfo tracks information about a WebSocket session
type SessionInfo struct {
	// SessionID is the unique session identifier
	SessionID string

	// UserID is the user associated with this session
	UserID string

	// ClientVersion is the client version
	ClientVersion string

	// Platform is the client platform
	Platform string

	// DeviceID is the device identifier
	DeviceID string

	// EnabledFeatures are the features enabled for this session
	EnabledFeatures []string

	// State is the current connection state
	State ConnectionState

	// CreatedAt is when the session was created
	CreatedAt time.Time

	// LastHeartbeat is the last heartbeat received
	LastHeartbeat time.Time

	// LastActivity is the last activity timestamp
	LastActivity time.Time

	// HeartbeatMissCount tracks consecutive missed heartbeats
	HeartbeatMissCount int

	// LastAckSeq is the last acknowledged sequence number
	LastAckSeq int64

	// Metadata stores additional session metadata
	Metadata map[string]interface{}
}

// SessionManager manages WebSocket sessions and handshakes
type SessionManager struct {
	sessions              map[string]*SessionInfo
	userSessions          map[string][]string // userID -> []sessionID
	mu                    sync.RWMutex
	logger                *zap.Logger
	serverVersion         string
	supportedFeatures     []string
	heartbeatInterval     time.Duration
	heartbeatTimeout      time.Duration
	staleSessionTimeout   time.Duration
	maxMessageSize        int
}

// NewSessionManager creates a new session manager
func NewSessionManager(logger *zap.Logger) *SessionManager {
	return &SessionManager{
		sessions:            make(map[string]*SessionInfo),
		userSessions:        make(map[string][]string),
		logger:              logger,
		serverVersion:       "1.0.0",
		supportedFeatures:   []string{"ping", "pong", "rooms", "presence", "typing", "offline-queue"},
		heartbeatInterval:   30 * time.Second,
		heartbeatTimeout:    60 * time.Second,
		staleSessionTimeout: 5 * time.Minute,
		maxMessageSize:      512 * 1024, // 512KB
	}
}

// HandleHandshake processes a handshake request and returns a response
func (sm *SessionManager) HandleHandshake(userID string, req *HandshakeRequest) (*HandshakeResponse, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var sessionID string
	var resumeFromSeq int64
	var isReconnection bool

	// Check if this is a reconnection attempt
	if req.SessionID != "" {
		if session, exists := sm.sessions[req.SessionID]; exists && session.UserID == userID {
			// Valid reconnection
			sessionID = req.SessionID
			resumeFromSeq = session.LastAckSeq
			isReconnection = true

			// Update session state
			session.State = StateConnected
			session.LastHeartbeat = time.Now()
			session.LastActivity = time.Now()
			session.HeartbeatMissCount = 0

			sm.logger.Info("Session reconnected",
				zap.String("session_id", sessionID),
				zap.String("user_id", userID),
				zap.Int64("resume_from_seq", resumeFromSeq),
			)
		} else {
			// Invalid session ID or user mismatch
			sm.logger.Warn("Invalid reconnection attempt",
				zap.String("session_id", req.SessionID),
				zap.String("user_id", userID),
			)
			// Treat as new connection
			req.SessionID = ""
		}
	}

	// Create new session if not reconnecting
	if req.SessionID == "" {
		sessionID = uuid.New().String()

		// Determine enabled features (intersection of client and server)
		enabledFeatures := sm.intersectFeatures(req.SupportedFeatures)

		session := &SessionInfo{
			SessionID:          sessionID,
			UserID:             userID,
			ClientVersion:      req.ClientVersion,
			Platform:           req.Platform,
			DeviceID:           req.DeviceID,
			EnabledFeatures:    enabledFeatures,
			State:              StateConnected,
			CreatedAt:          time.Now(),
			LastHeartbeat:      time.Now(),
			LastActivity:       time.Now(),
			HeartbeatMissCount: 0,
			LastAckSeq:         req.LastAckSeq,
			Metadata:           make(map[string]interface{}),
		}

		sm.sessions[sessionID] = session

		// Add to user sessions map
		sm.userSessions[userID] = append(sm.userSessions[userID], sessionID)

		sm.logger.Info("New session created",
			zap.String("session_id", sessionID),
			zap.String("user_id", userID),
			zap.String("client_version", req.ClientVersion),
			zap.Strings("enabled_features", enabledFeatures),
		)
	}

	// Build handshake response
	response := &HandshakeResponse{
		SessionID:          sessionID,
		ServerVersion:      sm.serverVersion,
		SupportedFeatures:  sm.supportedFeatures,
		EnabledFeatures:    sm.getEnabledFeatures(sessionID),
		HeartbeatInterval:  sm.heartbeatInterval.Milliseconds(),
		HeartbeatTimeout:   sm.heartbeatTimeout.Milliseconds(),
		MaxMessageSize:     sm.maxMessageSize,
		ResumeFromSeq:      resumeFromSeq,
		ServerCapabilities: map[string]interface{}{
			"compression":     false,
			"encryption":      false,
			"binary_messages": false,
		},
		Status:  "accepted",
		Message: "Handshake successful",
	}

	if isReconnection {
		response.Message = fmt.Sprintf("Reconnected to existing session, resuming from seq %d", resumeFromSeq)
	}

	return response, nil
}

// RecordHeartbeat updates the heartbeat timestamp for a session
func (sm *SessionManager) RecordHeartbeat(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.LastHeartbeat = time.Now()
	session.LastActivity = time.Now()
	session.HeartbeatMissCount = 0

	// Update state if it was stale
	if session.State == StateStale {
		session.State = StateConnected
		sm.logger.Info("Session recovered from stale state",
			zap.String("session_id", sessionID),
			zap.String("user_id", session.UserID),
		)
	}

	return nil
}

// RecordMissedHeartbeat increments the missed heartbeat count
func (sm *SessionManager) RecordMissedHeartbeat(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.HeartbeatMissCount++

	// Mark as stale after 2 missed heartbeats
	if session.HeartbeatMissCount >= 2 {
		session.State = StateStale
		sm.logger.Warn("Session marked as stale",
			zap.String("session_id", sessionID),
			zap.String("user_id", session.UserID),
			zap.Int("missed_count", session.HeartbeatMissCount),
		)
	}

	return nil
}

// UpdateLastAckSeq updates the last acknowledged sequence number
func (sm *SessionManager) UpdateLastAckSeq(sessionID string, seq int64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.LastAckSeq = seq
	session.LastActivity = time.Now()

	return nil
}

// GetSession returns session information
func (sm *SessionManager) GetSession(sessionID string) (*SessionInfo, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Return a copy to avoid race conditions
	sessionCopy := *session
	return &sessionCopy, nil
}

// GetUserSessions returns all sessions for a user
func (sm *SessionManager) GetUserSessions(userID string) []*SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessionIDs := sm.userSessions[userID]
	sessions := make([]*SessionInfo, 0, len(sessionIDs))

	for _, sid := range sessionIDs {
		if session, exists := sm.sessions[sid]; exists {
			sessionCopy := *session
			sessions = append(sessions, &sessionCopy)
		}
	}

	return sessions
}

// CloseSession closes a session but keeps it for potential reconnection
func (sm *SessionManager) CloseSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Update state but keep session in map for potential reconnection
	session.State = StateDisconnected

	// Note: We don't remove from user sessions to allow reconnection
	// Session will be cleaned up by CleanupStaleSessions after timeout

	sm.logger.Info("Session closed",
		zap.String("session_id", sessionID),
		zap.String("user_id", session.UserID),
	)

	return nil
}

// CleanupStaleSessions removes stale sessions
func (sm *SessionManager) CleanupStaleSessions() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	staleSessionIDs := make([]string, 0)

	for sessionID, session := range sm.sessions {
		// Check if session is stale
		if session.State == StateDisconnected && now.Sub(session.LastActivity) > sm.staleSessionTimeout {
			staleSessionIDs = append(staleSessionIDs, sessionID)
		} else if session.State == StateStale && now.Sub(session.LastHeartbeat) > sm.staleSessionTimeout {
			staleSessionIDs = append(staleSessionIDs, sessionID)
		}
	}

	// Remove stale sessions
	for _, sessionID := range staleSessionIDs {
		session := sm.sessions[sessionID]
		userID := session.UserID

		// Remove from sessions map
		delete(sm.sessions, sessionID)

		// Remove from user sessions
		if sessionIDs, exists := sm.userSessions[userID]; exists {
			filtered := make([]string, 0, len(sessionIDs))
			for _, sid := range sessionIDs {
				if sid != sessionID {
					filtered = append(filtered, sid)
				}
			}
			if len(filtered) > 0 {
				sm.userSessions[userID] = filtered
			} else {
				delete(sm.userSessions, userID)
			}
		}

		sm.logger.Debug("Cleaned up stale session",
			zap.String("session_id", sessionID),
			zap.String("user_id", userID),
		)
	}

	if len(staleSessionIDs) > 0 {
		sm.logger.Info("Cleaned up stale sessions",
			zap.Int("count", len(staleSessionIDs)),
		)
	}

	return len(staleSessionIDs)
}

// GetSessionStats returns statistics about sessions
func (sm *SessionManager) GetSessionStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stateCount := make(map[ConnectionState]int)
	for _, session := range sm.sessions {
		stateCount[session.State]++
	}

	return map[string]interface{}{
		"total_sessions":     len(sm.sessions),
		"unique_users":       len(sm.userSessions),
		"connected":          stateCount[StateConnected],
		"stale":              stateCount[StateStale],
		"reconnecting":       stateCount[StateReconnecting],
		"disconnected":       stateCount[StateDisconnected],
	}
}

// Helper methods

func (sm *SessionManager) intersectFeatures(clientFeatures []string) []string {
	serverFeaturesMap := make(map[string]bool)
	for _, f := range sm.supportedFeatures {
		serverFeaturesMap[f] = true
	}

	enabled := make([]string, 0, len(clientFeatures))
	for _, f := range clientFeatures {
		if serverFeaturesMap[f] {
			enabled = append(enabled, f)
		}
	}

	return enabled
}

func (sm *SessionManager) getEnabledFeatures(sessionID string) []string {
	session, exists := sm.sessions[sessionID]
	if !exists {
		return []string{}
	}
	return session.EnabledFeatures
}

// Marshal/Unmarshal helpers

// MarshalHandshakeRequest serializes a handshake request to JSON
func MarshalHandshakeRequest(req *HandshakeRequest) ([]byte, error) {
	return json.Marshal(req)
}

// UnmarshalHandshakeRequest deserializes a handshake request from JSON
func UnmarshalHandshakeRequest(data []byte) (*HandshakeRequest, error) {
	var req HandshakeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal handshake request: %w", err)
	}
	return &req, nil
}

// MarshalHandshakeResponse serializes a handshake response to JSON
func MarshalHandshakeResponse(resp *HandshakeResponse) ([]byte, error) {
	return json.Marshal(resp)
}

// UnmarshalHandshakeResponse deserializes a handshake response from JSON
func UnmarshalHandshakeResponse(data []byte) (*HandshakeResponse, error) {
	var resp HandshakeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal handshake response: %w", err)
	}
	return &resp, nil
}
