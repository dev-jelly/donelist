package notification

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// HealthCheck represents the health status of the notification system
type HealthCheck struct {
	scheduler *Scheduler
	queue     *Queue
	logger    *zap.Logger
}

// NewHealthCheck creates a new health check instance
func NewHealthCheck(scheduler *Scheduler, queue *Queue, logger *zap.Logger) *HealthCheck {
	return &HealthCheck{
		scheduler: scheduler,
		queue:     queue,
		logger:    logger,
	}
}

// Check performs a comprehensive health check
func (h *HealthCheck) Check(ctx context.Context) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Check scheduler status
	schedulerStatus, err := h.checkScheduler(ctx)
	if err != nil {
		h.logger.Error("Scheduler health check failed", zap.Error(err))
		result["scheduler"] = map[string]interface{}{
			"healthy": false,
			"error":   err.Error(),
		}
	} else {
		result["scheduler"] = schedulerStatus
	}

	// Check queue status
	queueStatus, err := h.checkQueue(ctx)
	if err != nil {
		h.logger.Error("Queue health check failed", zap.Error(err))
		result["queue"] = map[string]interface{}{
			"healthy": false,
			"error":   err.Error(),
		}
	} else {
		result["queue"] = queueStatus
	}

	// Overall health status
	schedulerHealthy := schedulerStatus != nil && schedulerStatus["healthy"] == true
	queueHealthy := queueStatus != nil

	result["healthy"] = schedulerHealthy && queueHealthy
	result["timestamp"] = time.Now().UTC()

	return result, nil
}

// checkScheduler checks the scheduler's health
func (h *HealthCheck) checkScheduler(ctx context.Context) (map[string]interface{}, error) {
	if h.scheduler == nil {
		return nil, fmt.Errorf("scheduler not initialized")
	}

	status, err := h.scheduler.GetHealthStatus(ctx)
	if err != nil {
		return nil, err
	}

	return status, nil
}

// checkQueue checks the queue's health
func (h *HealthCheck) checkQueue(ctx context.Context) (map[string]interface{}, error) {
	if h.queue == nil {
		return nil, fmt.Errorf("queue not initialized")
	}

	stats, err := h.queue.GetQueueStats(ctx)
	if err != nil {
		return nil, err
	}

	// Determine health based on queue metrics
	healthy := true
	warnings := []string{}

	// Check for high DLQ count
	if dlqCount, ok := stats["dlq_count"].(int64); ok && dlqCount > 100 {
		warnings = append(warnings, fmt.Sprintf("high DLQ count: %d", dlqCount))
		healthy = false
	}

	// Check for large backlog
	if readyCount, ok := stats["ready_count"].(int64); ok && readyCount > 1000 {
		warnings = append(warnings, fmt.Sprintf("large backlog: %d", readyCount))
	}

	result := map[string]interface{}{
		"healthy": healthy,
		"stats":   stats,
	}

	if len(warnings) > 0 {
		result["warnings"] = warnings
	}

	return result, nil
}

// IsHealthy returns a simple boolean indicating if the system is healthy
func (h *HealthCheck) IsHealthy(ctx context.Context) bool {
	result, err := h.Check(ctx)
	if err != nil {
		return false
	}

	healthy, ok := result["healthy"].(bool)
	return ok && healthy
}
