package handlers

import (
	"net/http"

	"github.com/dev-jelly/donelist/internal/api/middleware"
	"github.com/dev-jelly/donelist/internal/auth"
	"github.com/dev-jelly/donelist/internal/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	hub    *websocket.Hub
	logger *zap.Logger
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(hub *websocket.Hub, logger *zap.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		hub:    hub,
		logger: logger,
	}
}

// HandleWebSocket upgrades HTTP connection to WebSocket
// GET /ws
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// Get user ID from JWT token
	userID, err := middleware.GetUserID(c)
	if err != nil {
		h.logger.Warn("Unauthorized WebSocket connection attempt", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get username from claims
	var username string
	if claims, exists := c.Get("claims"); exists {
		if jwtClaims, ok := claims.(*auth.Claims); ok {
			username = jwtClaims.Email
		}
	}
	if username == "" {
		username = userID.String() // fallback to user ID
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := websocket.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		return
	}

	// Create new client and register it
	client := websocket.NewClient(conn, h.hub, userID.String(), username, h.logger)
	h.hub.Register(client)

	// Start client goroutines
	client.Start()

	h.logger.Info("WebSocket connection established",
		zap.String("user_id", userID.String()),
		zap.String("remote_addr", c.Request.RemoteAddr),
	)
}
