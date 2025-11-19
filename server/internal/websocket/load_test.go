package websocket

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestLoadTest1000Connections tests the system with 1000+ concurrent connections
func TestLoadTest1000Connections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	numClients := 1000
	var connectedClients atomic.Int64
	var messagesReceived atomic.Int64
	var wg sync.WaitGroup

	t.Logf("Starting load test with %d concurrent connections", numClients)
	startTime := time.Now()

	// Create and register clients
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		userID := fmt.Sprintf("load-test-user-%d", i)
		username := fmt.Sprintf("user%d", i)

		client := &Client{
			Send:     make(chan []byte, 256),
			userID:   userID,
			username: username,
			hub:      hub,
			logger:   logger,
			rooms:    make(map[string]*Room),
			status:   "online",
			lastSeen: time.Now(),
		}

		clients[i] = client
		hub.Register(client)
		connectedClients.Add(1)

		// Start a goroutine to consume messages for each client
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			for range c.Send {
				messagesReceived.Add(1)
			}
		}(client)
	}

	// Wait for all registrations to complete
	time.Sleep(500 * time.Millisecond)

	registrationTime := time.Since(startTime)
	clientCount := hub.GetClientCount()

	t.Logf("Registration completed in %v", registrationTime)
	t.Logf("Connected clients: %d / %d", clientCount, numClients)

	if clientCount != numClients {
		t.Errorf("Expected %d clients, got %d", numClients, clientCount)
	}

	// Test broadcasting messages
	t.Log("Testing message broadcast...")
	broadcastStart := time.Now()

	numMessages := 100
	for i := 0; i < numMessages; i++ {
		message := NewMessage("test_message", map[string]interface{}{
			"iteration": i,
			"timestamp": time.Now().Unix(),
		})
		hub.BroadcastToAll(message)
	}

	// Wait a bit for messages to be delivered
	time.Sleep(1 * time.Second)

	broadcastTime := time.Since(broadcastStart)
	received := messagesReceived.Load()

	t.Logf("Broadcast completed in %v", broadcastTime)
	t.Logf("Messages sent: %d", numMessages)
	t.Logf("Messages received: %d", received)
	t.Logf("Average messages per client: %.2f", float64(received)/float64(numClients))
	t.Logf("Throughput: %.2f msg/sec", float64(received)/broadcastTime.Seconds())

	// Test user-specific broadcasting
	t.Log("Testing user-specific broadcast...")
	targetUser := clients[500].userID
	userMessages := 10

	for i := 0; i < userMessages; i++ {
		message := NewMessage("user_message", map[string]interface{}{
			"message": fmt.Sprintf("Message %d for specific user", i),
		})
		hub.BroadcastToUser(targetUser, message)
	}

	time.Sleep(100 * time.Millisecond)

	// Cleanup - unregister all clients
	t.Log("Cleaning up connections...")
	cleanupStart := time.Now()

	for _, client := range clients {
		hub.Unregister(client)
		// Note: hub.Unregister will close the client.Send channel
	}

	// Wait for all consumer goroutines to finish
	wg.Wait()

	cleanupTime := time.Since(cleanupStart)
	t.Logf("Cleanup completed in %v", cleanupTime)

	// Verify all clients are removed
	finalClientCount := hub.GetClientCount()
	if finalClientCount != 0 {
		t.Errorf("Expected 0 clients after cleanup, got %d", finalClientCount)
	}

	totalTime := time.Since(startTime)
	t.Logf("Total test duration: %v", totalTime)
	t.Log("Load test completed successfully")
}

// TestLoadTestRoomBroadcast tests room-based broadcasting under load
func TestLoadTestRoomBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	roomManager := hub.GetRoomManager()
	numRooms := 10
	clientsPerRoom := 100
	totalClients := numRooms * clientsPerRoom

	t.Logf("Starting room broadcast test with %d rooms, %d clients per room", numRooms, clientsPerRoom)
	startTime := time.Now()

	// Create rooms
	rooms := make([]*Room, numRooms)
	for i := 0; i < numRooms; i++ {
		roomID := fmt.Sprintf("load-test-room-%d", i)
		room, err := roomManager.CreateRoom(roomID, fmt.Sprintf("Test Room %d", i), "system")
		if err != nil {
			t.Fatalf("Failed to create room: %v", err)
		}
		rooms[i] = room
	}

	// Create clients and join rooms
	var wg sync.WaitGroup
	var messagesReceived atomic.Int64

	for i := 0; i < totalClients; i++ {
		roomIdx := i % numRooms
		userID := fmt.Sprintf("load-test-user-%d", i)
		username := fmt.Sprintf("user%d", i)

		client := &Client{
			Send:     make(chan []byte, 256),
			userID:   userID,
			username: username,
			hub:      hub,
			logger:   logger,
			rooms:    make(map[string]*Room),
			status:   "online",
			lastSeen: time.Now(),
		}

		hub.Register(client)

		// Join the room
		go func() {
			rooms[roomIdx].Join <- client
		}()

		// Start message consumer
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			for range c.Send {
				messagesReceived.Add(1)
			}
		}(client)
	}

	// Wait for setup
	time.Sleep(1 * time.Second)

	setupTime := time.Since(startTime)
	t.Logf("Setup completed in %v", setupTime)

	// Broadcast messages to each room
	t.Log("Broadcasting messages to rooms...")
	broadcastStart := time.Now()

	messagesPerRoom := 50
	for _, room := range rooms {
		for i := 0; i < messagesPerRoom; i++ {
			message := Message{
				Type:      "test_broadcast",
				RoomID:    room.ID,
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"iteration": i,
				},
			}

			if data, err := message.Marshal(); err == nil {
				room.Broadcast <- data
			}
		}
	}

	// Wait for message delivery
	time.Sleep(2 * time.Second)

	broadcastTime := time.Since(broadcastStart)
	received := messagesReceived.Load()

	t.Logf("Broadcast completed in %v", broadcastTime)
	t.Logf("Total messages sent: %d", numRooms*messagesPerRoom)
	t.Logf("Total messages received: %d", received)
	t.Logf("Throughput: %.2f msg/sec", float64(received)/broadcastTime.Seconds())

	// Expected: each room has clientsPerRoom clients, each should receive messagesPerRoom messages
	expectedMessages := int64(numRooms * clientsPerRoom * messagesPerRoom)
	receivedRatio := float64(received) / float64(expectedMessages) * 100

	t.Logf("Expected total deliveries: %d", expectedMessages)
	t.Logf("Delivery ratio: %.2f%%", receivedRatio)

	if receivedRatio < 95.0 {
		t.Logf("Warning: Low delivery ratio (%.2f%%), expected > 95%%", receivedRatio)
	}

	// Cleanup
	t.Log("Cleaning up...")
	for _, room := range rooms {
		roomManager.DeleteRoom(room.ID)
	}

	totalTime := time.Since(startTime)
	t.Logf("Total test duration: %v", totalTime)
	t.Log("Room broadcast load test completed successfully")
}

// BenchmarkMessageBroadcast benchmarks message broadcasting performance
func BenchmarkMessageBroadcast(b *testing.B) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	// Create 100 mock clients
	numClients := 100
	for i := 0; i < numClients; i++ {
		client := &Client{
			Send:     make(chan []byte, 256),
			userID:   fmt.Sprintf("bench-user-%d", i),
			username: fmt.Sprintf("user%d", i),
			hub:      hub,
			logger:   logger,
			rooms:    make(map[string]*Room),
		}

		hub.Register(client)

		// Consume messages
		go func(c *Client) {
			for range c.Send {
				// Discard messages
			}
		}(client)
	}

	// Wait for setup
	time.Sleep(100 * time.Millisecond)

	message := NewMessage("bench_message", map[string]interface{}{
		"data": "benchmark test message",
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hub.BroadcastToAll(message)
	}
}

// BenchmarkRoomBroadcast benchmarks room-based broadcasting
func BenchmarkRoomBroadcast(b *testing.B) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	roomManager := hub.GetRoomManager()
	room, _ := roomManager.CreateRoom("bench-room", "Bench Room", "system")

	// Create 50 clients in the room
	numClients := 50
	for i := 0; i < numClients; i++ {
		client := &Client{
			Send:     make(chan []byte, 256),
			userID:   fmt.Sprintf("bench-user-%d", i),
			username: fmt.Sprintf("user%d", i),
			hub:      hub,
			logger:   logger,
			rooms:    make(map[string]*Room),
		}

		hub.Register(client)
		room.Join <- client

		// Consume messages
		go func(c *Client) {
			for range c.Send {
				// Discard messages
			}
		}(client)
	}

	// Wait for setup
	time.Sleep(100 * time.Millisecond)

	message := Message{
		Type:      "bench_message",
		RoomID:    room.ID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"data": "benchmark test message",
		},
	}

	data, _ := message.Marshal()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		room.Broadcast <- data
	}
}

// TestConcurrentRoomOperations tests concurrent room join/leave operations
func TestConcurrentRoomOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent operations test in short mode")
	}

	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	roomManager := hub.GetRoomManager()
	room, _ := roomManager.CreateRoom("concurrent-test-room", "Test Room", "system")

	numOperations := 500
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Logf("Starting %d concurrent room operations", numOperations)

	// Perform concurrent joins and leaves
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			default:
			}

			client := &Client{
				Send:     make(chan []byte, 256),
				userID:   fmt.Sprintf("concurrent-user-%d", idx),
				username: fmt.Sprintf("user%d", idx),
				hub:      hub,
				logger:   logger,
				rooms:    make(map[string]*Room),
			}

			hub.Register(client)

			// Join room
			room.Join <- client
			time.Sleep(10 * time.Millisecond)

			// Leave room
			room.Leave <- client
			time.Sleep(10 * time.Millisecond)

			hub.Unregister(client)
		}(i)
	}

	wg.Wait()

	// Verify room is empty
	clientCount := room.GetClientCount()
	if clientCount != 0 {
		t.Errorf("Expected 0 clients in room after operations, got %d", clientCount)
	}

	t.Log("Concurrent operations test completed successfully")
}
