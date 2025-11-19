package websocket

import (
	"sync"
	"testing"
	"time"
)

func TestSequenceManager_NextSequence(t *testing.T) {
	sm := NewSequenceManager()

	// Test basic sequence allocation
	seq1 := sm.NextSequence("user1")
	if seq1 != 1 {
		t.Errorf("Expected first sequence to be 1, got %d", seq1)
	}

	seq2 := sm.NextSequence("user1")
	if seq2 != 2 {
		t.Errorf("Expected second sequence to be 2, got %d", seq2)
	}

	// Test different partition
	seq3 := sm.NextSequence("user2")
	if seq3 != 1 {
		t.Errorf("Expected first sequence for user2 to be 1, got %d", seq3)
	}
}

func TestSequenceManager_UpdateLastAck(t *testing.T) {
	sm := NewSequenceManager()

	// Allocate some sequences
	sm.NextSequence("user1")
	sm.NextSequence("user1")
	sm.NextSequence("user1")

	// Update last ack
	sm.UpdateLastAck("user1", 2)

	lastAck := sm.GetLastAck("user1")
	if lastAck != 2 {
		t.Errorf("Expected last ack to be 2, got %d", lastAck)
	}

	// Update with older sequence should not change
	sm.UpdateLastAck("user1", 1)
	lastAck = sm.GetLastAck("user1")
	if lastAck != 2 {
		t.Errorf("Expected last ack to remain 2, got %d", lastAck)
	}

	// Update with newer sequence
	sm.UpdateLastAck("user1", 3)
	lastAck = sm.GetLastAck("user1")
	if lastAck != 3 {
		t.Errorf("Expected last ack to be 3, got %d", lastAck)
	}
}

func TestSequenceManager_ConcurrentAccess(t *testing.T) {
	sm := NewSequenceManager()
	partitionKey := "user1"

	// Run concurrent sequence allocations
	numGoroutines := 100
	allocationsPerGoroutine := 100

	var wg sync.WaitGroup
	sequences := make([][]uint64, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			localSeqs := make([]uint64, allocationsPerGoroutine)
			for j := 0; j < allocationsPerGoroutine; j++ {
				localSeqs[j] = sm.NextSequence(partitionKey)
			}
			sequences[index] = localSeqs
		}(i)
	}

	wg.Wait()

	// Verify all sequences are unique
	seenSequences := make(map[uint64]bool)
	for _, seqs := range sequences {
		for _, seq := range seqs {
			if seenSequences[seq] {
				t.Errorf("Duplicate sequence found: %d", seq)
			}
			seenSequences[seq] = true
		}
	}

	// Verify we got exactly the expected number of sequences
	expectedCount := numGoroutines * allocationsPerGoroutine
	if len(seenSequences) != expectedCount {
		t.Errorf("Expected %d unique sequences, got %d", expectedCount, len(seenSequences))
	}

	// Verify final sequence number
	finalSeq := sm.GetCurrentSequence(partitionKey)
	if finalSeq != uint64(expectedCount) {
		t.Errorf("Expected final sequence to be %d, got %d", expectedCount, finalSeq)
	}
}

func TestOrderPreservingBuffer_InOrderMessages(t *testing.T) {
	opb := NewOrderPreservingBuffer(1, 100, 30*time.Second)

	// Send messages in order
	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	msg2 := NewMessage(MessageTypeCheckinUpdated, nil)
	msg3 := NewMessage(MessageTypeCheckinDeleted, nil)

	// First message
	ready1, err := opb.AddMessage(msg1, 1)
	if err != nil {
		t.Fatalf("Failed to add message 1: %v", err)
	}
	if len(ready1) != 1 {
		t.Errorf("Expected 1 ready message, got %d", len(ready1))
	}

	// Second message
	ready2, err := opb.AddMessage(msg2, 2)
	if err != nil {
		t.Fatalf("Failed to add message 2: %v", err)
	}
	if len(ready2) != 1 {
		t.Errorf("Expected 1 ready message, got %d", len(ready2))
	}

	// Third message
	ready3, err := opb.AddMessage(msg3, 3)
	if err != nil {
		t.Fatalf("Failed to add message 3: %v", err)
	}
	if len(ready3) != 1 {
		t.Errorf("Expected 1 ready message, got %d", len(ready3))
	}
}

func TestOrderPreservingBuffer_OutOfOrderMessages(t *testing.T) {
	opb := NewOrderPreservingBuffer(1, 100, 30*time.Second)

	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	msg2 := NewMessage(MessageTypeCheckinUpdated, nil)
	msg3 := NewMessage(MessageTypeCheckinDeleted, nil)

	// Send message 1
	ready1, err := opb.AddMessage(msg1, 1)
	if err != nil {
		t.Fatalf("Failed to add message 1: %v", err)
	}
	if len(ready1) != 1 {
		t.Errorf("Expected 1 ready message, got %d", len(ready1))
	}

	// Send message 3 (out of order)
	ready3, err := opb.AddMessage(msg3, 3)
	if err != nil {
		t.Fatalf("Failed to add message 3: %v", err)
	}
	if len(ready3) != 0 {
		t.Errorf("Expected 0 ready messages for out-of-order message, got %d", len(ready3))
	}

	// Verify message 3 is buffered
	stats := opb.GetBufferStats()
	if stats["buffered_count"].(int) != 1 {
		t.Errorf("Expected 1 buffered message, got %d", stats["buffered_count"])
	}

	// Send message 2 (fills the gap)
	ready2, err := opb.AddMessage(msg2, 2)
	if err != nil {
		t.Fatalf("Failed to add message 2: %v", err)
	}
	// Should return both message 2 and message 3
	if len(ready2) != 2 {
		t.Errorf("Expected 2 ready messages, got %d", len(ready2))
	}

	// Verify buffer is empty
	stats = opb.GetBufferStats()
	if stats["buffered_count"].(int) != 0 {
		t.Errorf("Expected 0 buffered messages, got %d", stats["buffered_count"])
	}
}

func TestOrderPreservingBuffer_DuplicateMessages(t *testing.T) {
	opb := NewOrderPreservingBuffer(1, 100, 30*time.Second)

	msg1 := NewMessage(MessageTypeCheckinCreated, nil)

	// Send message 1
	_, err := opb.AddMessage(msg1, 1)
	if err != nil {
		t.Fatalf("Failed to add message 1: %v", err)
	}

	// Try to send message 1 again (duplicate)
	_, err = opb.AddMessage(msg1, 1)
	if err == nil {
		t.Error("Expected error for duplicate message, got nil")
	}
}

func TestOrderPreservingBuffer_BufferSizeLimit(t *testing.T) {
	maxBufferSize := 10
	opb := NewOrderPreservingBuffer(1, maxBufferSize, 30*time.Second)

	// Process first message
	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	_, err := opb.AddMessage(msg1, 1)
	if err != nil {
		t.Fatalf("Failed to add message 1: %v", err)
	}

	// Fill buffer with out-of-order messages
	for i := 2; i < maxBufferSize+2; i++ {
		msg := NewMessage(MessageTypeCheckinUpdated, nil)
		_, err := opb.AddMessage(msg, uint64(i+1))
		if err != nil {
			t.Fatalf("Failed to add message %d: %v", i+1, err)
		}
	}

	// Try to add one more message (should fail - buffer full)
	msgOverflow := NewMessage(MessageTypeCheckinDeleted, nil)
	_, err = opb.AddMessage(msgOverflow, uint64(maxBufferSize+3))
	if err == nil {
		t.Error("Expected error when buffer is full, got nil")
	}
}

func TestOrderPreservingBuffer_MessageExpiration(t *testing.T) {
	expirationTime := 100 * time.Millisecond
	opb := NewOrderPreservingBuffer(1, 100, expirationTime)

	// Process first message
	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	_, err := opb.AddMessage(msg1, 1)
	if err != nil {
		t.Fatalf("Failed to add message 1: %v", err)
	}

	// Add out-of-order message 3
	msg3 := NewMessage(MessageTypeCheckinDeleted, nil)
	_, err = opb.AddMessage(msg3, 3)
	if err != nil {
		t.Fatalf("Failed to add message 3: %v", err)
	}

	// Verify message 3 is buffered
	stats := opb.GetBufferStats()
	if stats["buffered_count"].(int) != 1 {
		t.Errorf("Expected 1 buffered message, got %d", stats["buffered_count"])
	}

	// Wait for expiration
	time.Sleep(expirationTime + 50*time.Millisecond)

	// Try to add message 2 (should process but message 3 should have expired)
	msg2 := NewMessage(MessageTypeCheckinUpdated, nil)
	ready, err := opb.AddMessage(msg2, 2)
	if err != nil {
		t.Fatalf("Failed to add message 2: %v", err)
	}

	// Should only return message 2 (message 3 expired)
	if len(ready) != 1 {
		t.Errorf("Expected 1 ready message (expired message should be removed), got %d", len(ready))
	}
}

func TestOrderPreservingBuffer_GetMissingSequences(t *testing.T) {
	opb := NewOrderPreservingBuffer(1, 100, 30*time.Second)

	// Process messages with gaps
	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	_, err := opb.AddMessage(msg1, 1)
	if err != nil {
		t.Fatalf("Failed to add message 1: %v", err)
	}

	msg3 := NewMessage(MessageTypeCheckinDeleted, nil)
	_, err = opb.AddMessage(msg3, 3)
	if err != nil {
		t.Fatalf("Failed to add message 3: %v", err)
	}

	msg5 := NewMessage(MessageTypeCheckinUpdated, nil)
	_, err = opb.AddMessage(msg5, 5)
	if err != nil {
		t.Fatalf("Failed to add message 5: %v", err)
	}

	// Check missing sequences up to sequence 6
	missing := opb.GetMissingSequences(6)

	expectedMissing := []uint64{2, 4}
	if len(missing) != len(expectedMissing) {
		t.Errorf("Expected %d missing sequences, got %d", len(expectedMissing), len(missing))
	}

	for i, seq := range missing {
		if seq != expectedMissing[i] {
			t.Errorf("Expected missing sequence %d, got %d", expectedMissing[i], seq)
		}
	}
}

func TestOrderPreservingBuffer_Reset(t *testing.T) {
	opb := NewOrderPreservingBuffer(1, 100, 30*time.Second)

	// Add some messages
	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	_, _ = opb.AddMessage(msg1, 1)

	msg3 := NewMessage(MessageTypeCheckinDeleted, nil)
	_, _ = opb.AddMessage(msg3, 3)

	// Verify buffer has messages
	stats := opb.GetBufferStats()
	if stats["buffered_count"].(int) == 0 {
		t.Error("Expected buffered messages before reset")
	}

	// Reset buffer
	opb.Reset(10)

	// Verify buffer is empty
	stats = opb.GetBufferStats()
	if stats["buffered_count"].(int) != 0 {
		t.Errorf("Expected 0 buffered messages after reset, got %d", stats["buffered_count"])
	}

	if stats["next_expected"].(uint64) != 10 {
		t.Errorf("Expected next expected to be 10, got %d", stats["next_expected"])
	}
}

func TestClientSequenceTracker_ProcessMessage(t *testing.T) {
	sm := NewSequenceManager()
	tracker := NewClientSequenceTracker("user1", sm)

	// Process in-order messages (starting from seq 1)
	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	ready1, err := tracker.ProcessMessage(msg1, 1)
	if err != nil {
		t.Fatalf("Failed to process message 1: %v", err)
	}
	if len(ready1) != 1 {
		t.Errorf("Expected 1 ready message, got %d", len(ready1))
	}

	// Process out-of-order message
	msg3 := NewMessage(MessageTypeCheckinDeleted, nil)
	ready3, err := tracker.ProcessMessage(msg3, 3)
	if err != nil {
		t.Fatalf("Failed to process message 3: %v", err)
	}
	if len(ready3) != 0 {
		t.Errorf("Expected 0 ready messages for out-of-order, got %d", len(ready3))
	}

	// Fill the gap
	msg2 := NewMessage(MessageTypeCheckinUpdated, nil)
	ready2, err := tracker.ProcessMessage(msg2, 2)
	if err != nil {
		t.Fatalf("Failed to process message 2: %v", err)
	}
	if len(ready2) != 2 {
		t.Errorf("Expected 2 ready messages, got %d", len(ready2))
	}
}

func TestClientSequenceTracker_AckSequence(t *testing.T) {
	sm := NewSequenceManager()
	tracker := NewClientSequenceTracker("user1", sm)

	// Ack some sequences
	tracker.AckSequence(5)
	tracker.AckSequence(10)

	lastAck := sm.GetLastAck("user1")
	if lastAck != 10 {
		t.Errorf("Expected last ack to be 10, got %d", lastAck)
	}
}

func TestClientSequenceTracker_RequestRetransmission(t *testing.T) {
	sm := NewSequenceManager()
	tracker := NewClientSequenceTracker("user1", sm)

	// Process messages with gaps
	msg1 := NewMessage(MessageTypeCheckinCreated, nil)
	_, _ = tracker.ProcessMessage(msg1, 1)

	msg3 := NewMessage(MessageTypeCheckinDeleted, nil)
	_, _ = tracker.ProcessMessage(msg3, 3)

	msg5 := NewMessage(MessageTypeCheckinUpdated, nil)
	_, _ = tracker.ProcessMessage(msg5, 5)

	// Request retransmission
	missing := tracker.RequestRetransmission(6)

	expectedMissing := []uint64{2, 4}
	if len(missing) != len(expectedMissing) {
		t.Errorf("Expected %d missing sequences, got %d", len(expectedMissing), len(missing))
	}
}

func BenchmarkSequenceManager_NextSequence(b *testing.B) {
	sm := NewSequenceManager()
	partitionKey := "user1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm.NextSequence(partitionKey)
	}
}

func BenchmarkOrderPreservingBuffer_InOrder(b *testing.B) {
	opb := NewOrderPreservingBuffer(1, 10000, 30*time.Second)
	msg := NewMessage(MessageTypeCheckinCreated, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = opb.AddMessage(msg, uint64(i+1))
	}
}

func BenchmarkOrderPreservingBuffer_OutOfOrder(b *testing.B) {
	opb := NewOrderPreservingBuffer(1, 10000, 30*time.Second)
	msg := NewMessage(MessageTypeCheckinCreated, nil)

	// Process first message
	_, _ = opb.AddMessage(msg, 1)

	b.ResetTimer()
	// Add out-of-order messages (skip sequence 2)
	for i := 0; i < b.N; i++ {
		_, _ = opb.AddMessage(msg, uint64(i+3))
	}
}
