package websocket

import (
	"fmt"
	"sync"
	"time"
)

// SequenceManager manages monotonically increasing sequences per partition (user/room)
type SequenceManager struct {
	// Per-partition sequence counters (key: userID or roomID)
	sequences map[string]*PartitionSequence
	mu        sync.RWMutex
}

// PartitionSequence tracks sequence state for a single partition
type PartitionSequence struct {
	// Current sequence number
	current uint64
	// Last acknowledged sequence by clients
	lastAcked uint64
	mu        sync.Mutex
}

// NewSequenceManager creates a new sequence manager
func NewSequenceManager() *SequenceManager {
	return &SequenceManager{
		sequences: make(map[string]*PartitionSequence),
	}
}

// NextSequence allocates the next sequence number for a partition
func (sm *SequenceManager) NextSequence(partitionKey string) uint64 {
	sm.mu.Lock()
	partition, exists := sm.sequences[partitionKey]
	if !exists {
		partition = &PartitionSequence{
			current:   0,
			lastAcked: 0,
		}
		sm.sequences[partitionKey] = partition
	}
	sm.mu.Unlock()

	partition.mu.Lock()
	defer partition.mu.Unlock()

	partition.current++
	return partition.current
}

// UpdateLastAck updates the last acknowledged sequence for a partition
func (sm *SequenceManager) UpdateLastAck(partitionKey string, seq uint64) {
	sm.mu.Lock()
	partition, exists := sm.sequences[partitionKey]
	if !exists {
		partition = &PartitionSequence{
			current:   0,
			lastAcked: 0,
		}
		sm.sequences[partitionKey] = partition
	}
	sm.mu.Unlock()

	partition.mu.Lock()
	defer partition.mu.Unlock()

	if seq > partition.lastAcked {
		partition.lastAcked = seq
	}
}

// GetLastAck returns the last acknowledged sequence for a partition
func (sm *SequenceManager) GetLastAck(partitionKey string) uint64 {
	sm.mu.RLock()
	partition, exists := sm.sequences[partitionKey]
	sm.mu.RUnlock()

	if !exists {
		return 0
	}

	partition.mu.Lock()
	defer partition.mu.Unlock()
	return partition.lastAcked
}

// GetCurrentSequence returns the current sequence number for a partition
func (sm *SequenceManager) GetCurrentSequence(partitionKey string) uint64 {
	sm.mu.RLock()
	partition, exists := sm.sequences[partitionKey]
	sm.mu.RUnlock()

	if !exists {
		return 0
	}

	partition.mu.Lock()
	defer partition.mu.Unlock()
	return partition.current
}

// OrderPreservingBuffer manages out-of-order message reordering
type OrderPreservingBuffer struct {
	// Buffer for out-of-order messages (key: sequence number)
	buffer map[uint64]*SequencedMessage
	// Expected next sequence number
	nextExpected uint64
	// Maximum buffer size before dropping messages
	maxBufferSize int
	// Message expiration time
	expirationTime time.Duration
	mu             sync.RWMutex
}

// SequencedMessage wraps a message with sequence and timing information
type SequencedMessage struct {
	Message    *Message
	Sequence   uint64
	ReceivedAt time.Time
}

// NewOrderPreservingBuffer creates a new order-preserving buffer
func NewOrderPreservingBuffer(initialSeq uint64, maxBufferSize int, expirationTime time.Duration) *OrderPreservingBuffer {
	if maxBufferSize == 0 {
		maxBufferSize = 1000 // Default: 1000 messages
	}
	if expirationTime == 0 {
		expirationTime = 30 * time.Second // Default: 30 seconds
	}

	return &OrderPreservingBuffer{
		buffer:         make(map[uint64]*SequencedMessage),
		nextExpected:   initialSeq,
		maxBufferSize:  maxBufferSize,
		expirationTime: expirationTime,
	}
}

// AddMessage adds a message to the buffer and returns any messages ready for delivery
func (opb *OrderPreservingBuffer) AddMessage(msg *Message, seq uint64) ([]*Message, error) {
	opb.mu.Lock()
	defer opb.mu.Unlock()

	now := time.Now()

	// Check if message is duplicate (already processed)
	if seq < opb.nextExpected {
		return nil, fmt.Errorf("duplicate message: seq %d < expected %d", seq, opb.nextExpected)
	}

	// If message is the next expected, process it immediately
	if seq == opb.nextExpected {
		result := []*Message{msg}
		opb.nextExpected++

		// Check if any buffered messages are now ready
		for {
			if bufferedMsg, exists := opb.buffer[opb.nextExpected]; exists {
				// Check if message has expired
				if now.Sub(bufferedMsg.ReceivedAt) > opb.expirationTime {
					// Remove expired message
					delete(opb.buffer, opb.nextExpected)
					opb.nextExpected++
					continue
				}

				result = append(result, bufferedMsg.Message)
				delete(opb.buffer, opb.nextExpected)
				opb.nextExpected++
			} else {
				break
			}
		}

		// Cleanup expired messages in buffer
		opb.cleanupExpired(now)

		return result, nil
	}

	// Message is out of order, buffer it
	// Check buffer size limit
	if len(opb.buffer) >= opb.maxBufferSize {
		return nil, fmt.Errorf("buffer full: cannot buffer seq %d", seq)
	}

	// Check if already buffered (duplicate)
	if _, exists := opb.buffer[seq]; exists {
		return nil, fmt.Errorf("duplicate buffered message: seq %d", seq)
	}

	// Add to buffer
	opb.buffer[seq] = &SequencedMessage{
		Message:    msg,
		Sequence:   seq,
		ReceivedAt: now,
	}

	// Cleanup expired messages
	opb.cleanupExpired(now)

	return nil, nil // No messages ready yet
}

// cleanupExpired removes expired messages from the buffer
// Must be called with lock held
func (opb *OrderPreservingBuffer) cleanupExpired(now time.Time) {
	for seq, msg := range opb.buffer {
		if now.Sub(msg.ReceivedAt) > opb.expirationTime {
			delete(opb.buffer, seq)
		}
	}
}

// GetMissingSequences returns a list of missing sequence numbers (gaps)
func (opb *OrderPreservingBuffer) GetMissingSequences(currentSeq uint64) []uint64 {
	opb.mu.RLock()
	defer opb.mu.RUnlock()

	var missing []uint64
	for seq := opb.nextExpected; seq < currentSeq; seq++ {
		if _, exists := opb.buffer[seq]; !exists {
			missing = append(missing, seq)
		}
	}

	return missing
}

// GetBufferStats returns statistics about the buffer
func (opb *OrderPreservingBuffer) GetBufferStats() map[string]interface{} {
	opb.mu.RLock()
	defer opb.mu.RUnlock()

	return map[string]interface{}{
		"next_expected":   opb.nextExpected,
		"buffered_count":  len(opb.buffer),
		"max_buffer_size": opb.maxBufferSize,
		"expiration_time": opb.expirationTime.Seconds(),
	}
}

// Reset resets the buffer to a new starting sequence
func (opb *OrderPreservingBuffer) Reset(initialSeq uint64) {
	opb.mu.Lock()
	defer opb.mu.Unlock()

	opb.buffer = make(map[uint64]*SequencedMessage)
	opb.nextExpected = initialSeq
}

// ClientSequenceTracker tracks sequence state for a client
type ClientSequenceTracker struct {
	// User or room partition key
	partitionKey string
	// Order-preserving buffer
	buffer *OrderPreservingBuffer
	// Sequence manager reference
	seqManager *SequenceManager
	mu         sync.RWMutex
}

// NewClientSequenceTracker creates a new client sequence tracker
func NewClientSequenceTracker(partitionKey string, seqManager *SequenceManager) *ClientSequenceTracker {
	// Get the current sequence for this partition
	currentSeq := seqManager.GetCurrentSequence(partitionKey)

	// Start from 1 if no sequences have been allocated yet, otherwise currentSeq + 1
	initialSeq := currentSeq + 1
	if currentSeq == 0 {
		initialSeq = 1
	}

	return &ClientSequenceTracker{
		partitionKey: partitionKey,
		buffer:       NewOrderPreservingBuffer(initialSeq, 1000, 30*time.Second),
		seqManager:   seqManager,
	}
}

// ProcessMessage processes an incoming message with sequence number
func (cst *ClientSequenceTracker) ProcessMessage(msg *Message, seq uint64) ([]*Message, error) {
	return cst.buffer.AddMessage(msg, seq)
}

// AckSequence acknowledges receipt of a sequence number
func (cst *ClientSequenceTracker) AckSequence(seq uint64) {
	cst.seqManager.UpdateLastAck(cst.partitionKey, seq)
}

// RequestRetransmission requests retransmission of missing sequences
func (cst *ClientSequenceTracker) RequestRetransmission(highestSeen uint64) []uint64 {
	return cst.buffer.GetMissingSequences(highestSeen)
}

// GetStats returns tracking statistics
func (cst *ClientSequenceTracker) GetStats() map[string]interface{} {
	stats := cst.buffer.GetBufferStats()
	stats["partition_key"] = cst.partitionKey
	stats["last_acked"] = cst.seqManager.GetLastAck(cst.partitionKey)
	return stats
}
