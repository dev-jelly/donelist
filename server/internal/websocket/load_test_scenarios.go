// +build integration

package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// LoadTestScenario represents a load testing scenario
type LoadTestScenario struct {
	Name        string
	Description string
	Execute     func(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error)
}

// LoadTestConfig contains configuration for load testing
type LoadTestConfig struct {
	// Server configuration
	ServerURL string
	AuthToken string

	// Load configuration
	NumClients         int
	NumMessages        int
	MessageSize        int
	MessageRate        int // messages per second per client
	ConnectionRate     int // connections per second
	TestDuration       time.Duration
	RampUpDuration     time.Duration

	// Scenario configuration
	ScenarioType       string
	RoomDistribution   string // "uniform", "pareto", "zipf"
	MessagePattern     string // "constant", "burst", "random"
	DisconnectPattern  string // "none", "random", "periodic"
	ReconnectEnabled   bool

	// Monitoring
	MetricsInterval    time.Duration
	Logger            *zap.Logger
}

// LoadTestResult contains results from a load test
type LoadTestResult struct {
	Scenario           string
	StartTime          time.Time
	EndTime            time.Time
	Duration           time.Duration

	// Connection metrics
	TotalConnections   int64
	SuccessfulConnects int64
	FailedConnects     int64
	Disconnections     int64
	Reconnections      int64

	// Message metrics
	MessagesSent       int64
	MessagesReceived   int64
	MessagesLost       int64

	// Performance metrics
	AvgConnectionTime  time.Duration
	AvgMessageLatency  time.Duration
	P95MessageLatency  time.Duration
	P99MessageLatency  time.Duration
	MaxMessageLatency  time.Duration

	// Error metrics
	TotalErrors        int64
	ConnectionErrors   int64
	MessageErrors      int64
	TimeoutErrors      int64

	// Throughput
	MessagesPerSecond  float64
	BytesPerSecond     float64
}

// LoadTestClient represents a single client in a load test
type LoadTestClient struct {
	ID          string
	conn        *websocket.Conn
	config      *LoadTestConfig
	metrics     *ClientMetrics
	logger      *zap.Logger
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// ClientMetrics tracks metrics for a single client
type ClientMetrics struct {
	ConnectTime       time.Duration
	MessagesSent      int64
	MessagesReceived  int64
	Errors            int64
	Latencies         []time.Duration
	mu                sync.Mutex
}

// LoadTestRunner runs load test scenarios
type LoadTestRunner struct {
	config    *LoadTestConfig
	scenarios map[string]*LoadTestScenario
	logger    *zap.Logger
}

// NewLoadTestRunner creates a new load test runner
func NewLoadTestRunner(config *LoadTestConfig) *LoadTestRunner {
	if config.Logger == nil {
		config.Logger, _ = zap.NewDevelopment()
	}

	runner := &LoadTestRunner{
		config:    config,
		logger:    config.Logger,
		scenarios: make(map[string]*LoadTestScenario),
	}

	// Register default scenarios
	runner.registerDefaultScenarios()

	return runner
}

// registerDefaultScenarios registers built-in test scenarios
func (r *LoadTestRunner) registerDefaultScenarios() {
	r.scenarios["basic"] = &LoadTestScenario{
		Name:        "Basic Load Test",
		Description: "Simple connection and message exchange",
		Execute:     r.basicLoadTest,
	}

	r.scenarios["burst"] = &LoadTestScenario{
		Name:        "Burst Load Test",
		Description: "Sudden burst of connections and messages",
		Execute:     r.burstLoadTest,
	}

	r.scenarios["sustained"] = &LoadTestScenario{
		Name:        "Sustained Load Test",
		Description: "Long-running sustained load",
		Execute:     r.sustainedLoadTest,
	}

	r.scenarios["reconnect"] = &LoadTestScenario{
		Name:        "Reconnection Test",
		Description: "Test reconnection behavior under load",
		Execute:     r.reconnectLoadTest,
	}

	r.scenarios["room_stress"] = &LoadTestScenario{
		Name:        "Room Stress Test",
		Description: "Stress test room-based message distribution",
		Execute:     r.roomStressTest,
	}

	r.scenarios["sequence_ordering"] = &LoadTestScenario{
		Name:        "Sequence Ordering Test",
		Description: "Verify message ordering under load",
		Execute:     r.sequenceOrderingTest,
	}

	r.scenarios["conflict_resolution"] = &LoadTestScenario{
		Name:        "Conflict Resolution Test",
		Description: "Test conflict resolution under concurrent updates",
		Execute:     r.conflictResolutionTest,
	}

	r.scenarios["network_chaos"] = &LoadTestScenario{
		Name:        "Network Chaos Test",
		Description: "Simulate network issues (delays, drops, reordering)",
		Execute:     r.networkChaosTest,
	}
}

// RunScenario runs a specific load test scenario
func (r *LoadTestRunner) RunScenario(scenarioName string) (*LoadTestResult, error) {
	scenario, exists := r.scenarios[scenarioName]
	if !exists {
		return nil, fmt.Errorf("scenario not found: %s", scenarioName)
	}

	r.logger.Info("Starting load test scenario",
		zap.String("scenario", scenarioName),
		zap.String("description", scenario.Description),
		zap.Int("num_clients", r.config.NumClients),
		zap.Duration("duration", r.config.TestDuration),
	)

	ctx, cancel := context.WithTimeout(context.Background(), r.config.TestDuration)
	defer cancel()

	result, err := scenario.Execute(ctx, r.config)
	if err != nil {
		return nil, fmt.Errorf("scenario execution failed: %w", err)
	}

	r.logger.Info("Load test scenario completed",
		zap.String("scenario", scenarioName),
		zap.Duration("duration", result.Duration),
		zap.Int64("messages_sent", result.MessagesSent),
		zap.Int64("messages_received", result.MessagesReceived),
		zap.Float64("messages_per_second", result.MessagesPerSecond),
	)

	return result, nil
}

// basicLoadTest implements a basic load test scenario
func (r *LoadTestRunner) basicLoadTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "basic",
		StartTime: time.Now(),
	}

	clients := make([]*LoadTestClient, config.NumClients)
	var wg sync.WaitGroup

	// Create and connect clients
	for i := 0; i < config.NumClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			client := r.createClient(fmt.Sprintf("client_%d", idx))
			if err := client.Connect(); err != nil {
				atomic.AddInt64(&result.FailedConnects, 1)
				r.logger.Error("Failed to connect client",
					zap.String("client_id", client.ID),
					zap.Error(err),
				)
				return
			}

			atomic.AddInt64(&result.SuccessfulConnects, 1)
			clients[idx] = client

			// Start message loop
			client.StartMessaging(ctx, config.MessageRate)
		}(i)

		// Rate limit connections
		if config.ConnectionRate > 0 {
			time.Sleep(time.Second / time.Duration(config.ConnectionRate))
		}
	}

	// Wait for test duration
	select {
	case <-ctx.Done():
	case <-time.After(config.TestDuration):
	}

	// Stop all clients
	for _, client := range clients {
		if client != nil {
			client.Stop()
		}
	}

	wg.Wait()

	// Collect metrics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalConnections = int64(config.NumClients)

	for _, client := range clients {
		if client != nil {
			result.MessagesSent += client.metrics.MessagesSent
			result.MessagesReceived += client.metrics.MessagesReceived
			result.TotalErrors += client.metrics.Errors
		}
	}

	result.MessagesPerSecond = float64(result.MessagesSent) / result.Duration.Seconds()

	return result, nil
}

// burstLoadTest implements a burst load test scenario
func (r *LoadTestRunner) burstLoadTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "burst",
		StartTime: time.Now(),
	}

	// Create all clients simultaneously
	clients := make([]*LoadTestClient, config.NumClients)
	var connectWg sync.WaitGroup

	connectWg.Add(config.NumClients)
	for i := 0; i < config.NumClients; i++ {
		go func(idx int) {
			defer connectWg.Done()

			client := r.createClient(fmt.Sprintf("burst_client_%d", idx))
			if err := client.Connect(); err != nil {
				atomic.AddInt64(&result.FailedConnects, 1)
				return
			}

			atomic.AddInt64(&result.SuccessfulConnects, 1)
			clients[idx] = client
		}(i)
	}

	connectWg.Wait()

	// Send burst of messages
	var messageWg sync.WaitGroup
	for _, client := range clients {
		if client == nil {
			continue
		}

		messageWg.Add(1)
		go func(c *LoadTestClient) {
			defer messageWg.Done()

			// Send messages as fast as possible
			for i := 0; i < config.NumMessages; i++ {
				select {
				case <-ctx.Done():
					return
				default:
					msg := r.generateMessage(c.ID, i)
					if err := c.SendMessage(msg); err != nil {
						atomic.AddInt64(&result.MessageErrors, 1)
					} else {
						atomic.AddInt64(&result.MessagesSent, 1)
					}
				}
			}
		}(client)
	}

	messageWg.Wait()

	// Cleanup
	for _, client := range clients {
		if client != nil {
			client.Stop()
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalConnections = int64(config.NumClients)
	result.MessagesPerSecond = float64(result.MessagesSent) / result.Duration.Seconds()

	return result, nil
}

// sustainedLoadTest implements a long-running sustained load test
func (r *LoadTestRunner) sustainedLoadTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "sustained",
		StartTime: time.Now(),
	}

	// Ramp up connections gradually
	clients := make([]*LoadTestClient, 0, config.NumClients)
	rampUpRate := config.NumClients / int(config.RampUpDuration.Seconds())
	if rampUpRate < 1 {
		rampUpRate = 1
	}

	var wg sync.WaitGroup
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	clientsCreated := 0
	metricsCollector := r.startMetricsCollection(ctx, &clients, result)

	for {
		select {
		case <-ctx.Done():
			goto cleanup
		case <-ticker.C:
			// Create batch of clients
			batchSize := rampUpRate
			if clientsCreated+batchSize > config.NumClients {
				batchSize = config.NumClients - clientsCreated
			}

			for i := 0; i < batchSize; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					client := r.createClient(fmt.Sprintf("sustained_client_%d", clientsCreated+idx))
					if err := client.Connect(); err != nil {
						atomic.AddInt64(&result.FailedConnects, 1)
						return
					}

					atomic.AddInt64(&result.SuccessfulConnects, 1)
					clients = append(clients, client)

					// Start sustained messaging
					client.StartMessaging(ctx, config.MessageRate)
				}(i)
			}

			clientsCreated += batchSize

			if clientsCreated >= config.NumClients {
				break
			}
		}
	}

	// Maintain sustained load
	select {
	case <-ctx.Done():
	case <-time.After(config.TestDuration):
	}

cleanup:
	// Stop metrics collection
	close(metricsCollector)

	// Stop all clients
	for _, client := range clients {
		if client != nil {
			client.Stop()
		}
	}

	wg.Wait()

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalConnections = int64(len(clients))

	return result, nil
}

// reconnectLoadTest tests reconnection behavior
func (r *LoadTestRunner) reconnectLoadTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "reconnect",
		StartTime: time.Now(),
	}

	clients := make([]*LoadTestClient, config.NumClients)

	// Connect all clients
	for i := 0; i < config.NumClients; i++ {
		client := r.createClient(fmt.Sprintf("reconnect_client_%d", i))
		if err := client.Connect(); err != nil {
			atomic.AddInt64(&result.FailedConnects, 1)
			continue
		}
		atomic.AddInt64(&result.SuccessfulConnects, 1)
		clients[i] = client
	}

	// Simulate disconnections and reconnections
	disconnectInterval := config.TestDuration / 5 // Disconnect 5 times during test
	ticker := time.NewTicker(disconnectInterval)
	defer ticker.Stop()

	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			goto cleanup
		case <-ticker.C:
			// Disconnect random subset of clients
			numToDisconnect := rand.Intn(config.NumClients/2) + 1

			for i := 0; i < numToDisconnect; i++ {
				idx := rand.Intn(config.NumClients)
				if clients[idx] != nil {
					wg.Add(1)
					go func(client *LoadTestClient) {
						defer wg.Done()

						// Disconnect
						client.Disconnect()
						atomic.AddInt64(&result.Disconnections, 1)

						// Wait and reconnect
						time.Sleep(time.Duration(rand.Intn(5)) * time.Second)

						if err := client.Reconnect(); err != nil {
							atomic.AddInt64(&result.ConnectionErrors, 1)
						} else {
							atomic.AddInt64(&result.Reconnections, 1)
						}
					}(clients[idx])
				}
			}
		}
	}

cleanup:
	// Stop all clients
	for _, client := range clients {
		if client != nil {
			client.Stop()
		}
	}

	wg.Wait()

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalConnections = int64(config.NumClients)

	return result, nil
}

// roomStressTest tests room-based message distribution
func (r *LoadTestRunner) roomStressTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "room_stress",
		StartTime: time.Now(),
	}

	// Create rooms with different sizes
	numRooms := 10
	roomSizes := r.generateRoomDistribution(config.NumClients, numRooms, config.RoomDistribution)

	var wg sync.WaitGroup
	clientIdx := 0

	for roomID, roomSize := range roomSizes {
		for i := 0; i < roomSize; i++ {
			wg.Add(1)
			go func(room int, idx int) {
				defer wg.Done()

				client := r.createClient(fmt.Sprintf("room_%d_client_%d", room, idx))
				client.RoomID = fmt.Sprintf("room_%d", room)

				if err := client.Connect(); err != nil {
					atomic.AddInt64(&result.FailedConnects, 1)
					return
				}

				atomic.AddInt64(&result.SuccessfulConnects, 1)

				// Join room and start messaging
				if err := client.JoinRoom(client.RoomID); err != nil {
					atomic.AddInt64(&result.ConnectionErrors, 1)
					return
				}

				// Start room-specific messaging
				client.StartRoomMessaging(ctx, config.MessageRate)
			}(roomID, i)

			clientIdx++
		}
	}

	// Wait for test duration
	select {
	case <-ctx.Done():
	case <-time.After(config.TestDuration):
	}

	wg.Wait()

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalConnections = int64(config.NumClients)

	return result, nil
}

// sequenceOrderingTest verifies message ordering
func (r *LoadTestRunner) sequenceOrderingTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "sequence_ordering",
		StartTime: time.Now(),
	}

	// Create sender and receiver clients
	numSenders := config.NumClients / 2
	numReceivers := config.NumClients - numSenders

	senders := make([]*LoadTestClient, numSenders)
	receivers := make([]*LoadTestClient, numReceivers)
	sequenceTrackers := make(map[string]*SequenceTracker)

	// Connect senders
	for i := 0; i < numSenders; i++ {
		client := r.createClient(fmt.Sprintf("sender_%d", i))
		if err := client.Connect(); err != nil {
			continue
		}
		senders[i] = client
		sequenceTrackers[client.ID] = NewSequenceTracker()
	}

	// Connect receivers
	for i := 0; i < numReceivers; i++ {
		client := r.createClient(fmt.Sprintf("receiver_%d", i))
		if err := client.Connect(); err != nil {
			continue
		}
		receivers[i] = client
	}

	// Send sequenced messages
	var wg sync.WaitGroup
	for _, sender := range senders {
		if sender == nil {
			continue
		}

		wg.Add(1)
		go func(s *LoadTestClient, tracker *SequenceTracker) {
			defer wg.Done()

			for seq := 0; seq < config.NumMessages; seq++ {
				select {
				case <-ctx.Done():
					return
				default:
					msg := r.generateSequencedMessage(s.ID, seq)
					if err := s.SendMessage(msg); err != nil {
						atomic.AddInt64(&result.MessageErrors, 1)
					} else {
						tracker.RecordSent(seq)
						atomic.AddInt64(&result.MessagesSent, 1)
					}

					// Rate limiting
					if config.MessageRate > 0 {
						time.Sleep(time.Second / time.Duration(config.MessageRate))
					}
				}
			}
		}(sender, sequenceTrackers[sender.ID])
	}

	wg.Wait()

	// Verify sequence ordering
	for _, tracker := range sequenceTrackers {
		if gaps := tracker.GetGaps(); len(gaps) > 0 {
			atomic.AddInt64(&result.MessagesLost, int64(len(gaps)))
			r.logger.Warn("Sequence gaps detected",
				zap.Ints("gaps", gaps),
			)
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// conflictResolutionTest tests conflict resolution
func (r *LoadTestRunner) conflictResolutionTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "conflict_resolution",
		StartTime: time.Now(),
	}

	// Create clients that will update the same data
	clients := make([]*LoadTestClient, config.NumClients)
	dataItems := 10 // Number of data items to update concurrently

	for i := 0; i < config.NumClients; i++ {
		client := r.createClient(fmt.Sprintf("conflict_client_%d", i))
		if err := client.Connect(); err != nil {
			continue
		}
		clients[i] = client
	}

	// Generate concurrent updates
	var wg sync.WaitGroup
	for _, client := range clients {
		if client == nil {
			continue
		}

		wg.Add(1)
		go func(c *LoadTestClient) {
			defer wg.Done()

			for i := 0; i < config.NumMessages; i++ {
				select {
				case <-ctx.Done():
					return
				default:
					// Update random data item
					itemID := rand.Intn(dataItems)
					msg := r.generateUpdateMessage(c.ID, itemID, i)

					if err := c.SendMessage(msg); err != nil {
						atomic.AddInt64(&result.MessageErrors, 1)
					} else {
						atomic.AddInt64(&result.MessagesSent, 1)
					}

					// Small random delay to increase conflict probability
					time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
				}
			}
		}(client)
	}

	wg.Wait()

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalConnections = int64(config.NumClients)

	return result, nil
}

// networkChaosTest simulates network issues
func (r *LoadTestRunner) networkChaosTest(ctx context.Context, config *LoadTestConfig) (*LoadTestResult, error) {
	result := &LoadTestResult{
		Scenario:  "network_chaos",
		StartTime: time.Now(),
	}

	// Create clients with chaos proxy
	clients := make([]*ChaosClient, config.NumClients)

	for i := 0; i < config.NumClients; i++ {
		client := r.createChaosClient(fmt.Sprintf("chaos_client_%d", i))

		// Configure chaos parameters
		client.SetLatency(50*time.Millisecond, 200*time.Millisecond)
		client.SetPacketLoss(0.05) // 5% packet loss
		client.SetReordering(0.1)  // 10% message reordering

		if err := client.Connect(); err != nil {
			atomic.AddInt64(&result.FailedConnects, 1)
			continue
		}

		atomic.AddInt64(&result.SuccessfulConnects, 1)
		clients[i] = client
	}

	// Run test with network chaos
	var wg sync.WaitGroup
	for _, client := range clients {
		if client == nil {
			continue
		}

		wg.Add(1)
		go func(c *ChaosClient) {
			defer wg.Done()

			// Periodically change chaos parameters
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						// Randomly adjust chaos parameters
						c.SetLatency(
							time.Duration(rand.Intn(100))*time.Millisecond,
							time.Duration(rand.Intn(500))*time.Millisecond,
						)
						c.SetPacketLoss(rand.Float64() * 0.1) // 0-10% loss
					}
				}
			}()

			// Send messages under chaotic conditions
			c.StartMessaging(ctx, config.MessageRate)
		}(client)
	}

	// Wait for test duration
	select {
	case <-ctx.Done():
	case <-time.After(config.TestDuration):
	}

	wg.Wait()

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.TotalConnections = int64(config.NumClients)

	return result, nil
}

// Helper methods

func (r *LoadTestRunner) createClient(id string) *LoadTestClient {
	return &LoadTestClient{
		ID:      id,
		config:  r.config,
		metrics: &ClientMetrics{},
		logger:  r.logger,
		stopCh:  make(chan struct{}),
	}
}

func (r *LoadTestRunner) createChaosClient(id string) *ChaosClient {
	// ChaosClient would be a wrapper around LoadTestClient with network chaos simulation
	return nil // Implementation would go here
}

func (r *LoadTestRunner) generateMessage(clientID string, sequence int) *Message {
	return &Message{
		Type: "test",
		Data: map[string]interface{}{
			"client_id": clientID,
			"sequence":  sequence,
			"timestamp": time.Now().UnixNano(),
			"payload":   r.generatePayload(r.config.MessageSize),
		},
	}
}

func (r *LoadTestRunner) generateSequencedMessage(clientID string, sequence int) *Message {
	return &Message{
		Type: "sequenced",
		Data: map[string]interface{}{
			"client_id": clientID,
			"sequence":  sequence,
			"timestamp": time.Now().UnixNano(),
		},
	}
}

func (r *LoadTestRunner) generateUpdateMessage(clientID string, itemID int, version int) *Message {
	return &Message{
		Type: "update",
		Data: map[string]interface{}{
			"client_id": clientID,
			"item_id":   itemID,
			"version":   version,
			"value":     rand.Intn(1000),
			"timestamp": time.Now().UnixNano(),
		},
	}
}

func (r *LoadTestRunner) generatePayload(size int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, size)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func (r *LoadTestRunner) generateRoomDistribution(totalClients, numRooms int, distribution string) []int {
	sizes := make([]int, numRooms)

	switch distribution {
	case "uniform":
		baseSize := totalClients / numRooms
		remainder := totalClients % numRooms
		for i := range sizes {
			sizes[i] = baseSize
			if i < remainder {
				sizes[i]++
			}
		}

	case "pareto":
		// 80-20 rule: 20% of rooms have 80% of clients
		largeRooms := numRooms / 5
		if largeRooms < 1 {
			largeRooms = 1
		}
		largeClients := (totalClients * 8) / 10
		smallClients := totalClients - largeClients

		for i := 0; i < largeRooms; i++ {
			sizes[i] = largeClients / largeRooms
		}
		for i := largeRooms; i < numRooms; i++ {
			sizes[i] = smallClients / (numRooms - largeRooms)
		}

	default: // zipf or fallback
		// Simple approximation of Zipf distribution
		total := 0.0
		for i := 1; i <= numRooms; i++ {
			total += 1.0 / float64(i)
		}

		allocated := 0
		for i := 0; i < numRooms-1; i++ {
			sizes[i] = int(float64(totalClients) / float64(i+1) / total)
			allocated += sizes[i]
		}
		sizes[numRooms-1] = totalClients - allocated // Remainder
	}

	return sizes
}

func (r *LoadTestRunner) startMetricsCollection(ctx context.Context, clients *[]*LoadTestClient, result *LoadTestResult) chan struct{} {
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(r.config.MetricsInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				// Collect metrics from all clients
				var totalSent, totalReceived, totalErrors int64

				for _, client := range *clients {
					if client != nil {
						totalSent += client.metrics.MessagesSent
						totalReceived += client.metrics.MessagesReceived
						totalErrors += client.metrics.Errors
					}
				}

				r.logger.Info("Metrics snapshot",
					zap.Int64("messages_sent", totalSent),
					zap.Int64("messages_received", totalReceived),
					zap.Int64("errors", totalErrors),
				)
			}
		}
	}()

	return done
}

// LoadTestClient methods

func (c *LoadTestClient) Connect() error {
	u, err := url.Parse(c.config.ServerURL)
	if err != nil {
		return err
	}

	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}

	header := make(map[string][]string)
	if c.config.AuthToken != "" {
		header["Authorization"] = []string{"Bearer " + c.config.AuthToken}
	}

	start := time.Now()
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return err
	}

	c.conn = conn
	c.metrics.ConnectTime = time.Since(start)

	return nil
}

func (c *LoadTestClient) Disconnect() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *LoadTestClient) Reconnect() error {
	c.Disconnect()
	time.Sleep(time.Second) // Brief pause before reconnecting
	return c.Connect()
}

func (c *LoadTestClient) SendMessage(msg *Message) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	err = c.conn.WriteMessage(websocket.TextMessage, data)
	if err == nil {
		atomic.AddInt64(&c.metrics.MessagesSent, 1)
	}

	return err
}

func (c *LoadTestClient) StartMessaging(ctx context.Context, rate int) {
	if rate <= 0 {
		return
	}

	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	sequence := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			msg := &Message{
				Type: "test",
				Data: map[string]interface{}{
					"client_id": c.ID,
					"sequence":  sequence,
					"timestamp": time.Now().UnixNano(),
				},
			}

			if err := c.SendMessage(msg); err != nil {
				atomic.AddInt64(&c.metrics.Errors, 1)
			}

			sequence++
		}
	}
}

func (c *LoadTestClient) Stop() {
	close(c.stopCh)
	c.Disconnect()
}

// Additional helper types

type SequenceTracker struct {
	sent     map[int]bool
	received map[int]bool
	mu       sync.Mutex
}

func NewSequenceTracker() *SequenceTracker {
	return &SequenceTracker{
		sent:     make(map[int]bool),
		received: make(map[int]bool),
	}
}

func (st *SequenceTracker) RecordSent(seq int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sent[seq] = true
}

func (st *SequenceTracker) RecordReceived(seq int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.received[seq] = true
}

func (st *SequenceTracker) GetGaps() []int {
	st.mu.Lock()
	defer st.mu.Unlock()

	var gaps []int
	for seq := range st.sent {
		if !st.received[seq] {
			gaps = append(gaps, seq)
		}
	}
	return gaps
}

// Placeholder for additional client types
type LoadTestClient struct {
	ID      string
	RoomID  string
	conn    *websocket.Conn
	config  *LoadTestConfig
	metrics *ClientMetrics
	logger  *zap.Logger
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func (c *LoadTestClient) JoinRoom(roomID string) error {
	// Implementation would send room join message
	return nil
}

func (c *LoadTestClient) StartRoomMessaging(ctx context.Context, rate int) {
	// Implementation would send room-specific messages
}

type ChaosClient struct {
	*LoadTestClient
	latencyMin   time.Duration
	latencyMax   time.Duration
	packetLoss   float64
	reorderProb  float64
}

func (c *ChaosClient) SetLatency(min, max time.Duration) {
	c.latencyMin = min
	c.latencyMax = max
}

func (c *ChaosClient) SetPacketLoss(prob float64) {
	c.packetLoss = prob
}

func (c *ChaosClient) SetReordering(prob float64) {
	c.reorderProb = prob
}