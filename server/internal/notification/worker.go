package notification

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Worker represents a single notification worker
type Worker struct {
	id        int
	queue     *Queue
	processor *Processor
	logger    *zap.Logger
	stopChan  chan struct{}
	wg        *sync.WaitGroup
}

// WorkerPool manages a pool of notification workers
type WorkerPool struct {
	workers       []*Worker
	queue         *Queue
	processor     *Processor
	logger        *zap.Logger
	workerCount   int
	pollInterval  time.Duration
	stopChan      chan struct{}
	wg            sync.WaitGroup
	activeWorkers int32
	mu            sync.RWMutex
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(
	workerCount int,
	queue *Queue,
	processor *Processor,
	logger *zap.Logger,
) *WorkerPool {
	return &WorkerPool{
		workerCount:  workerCount,
		queue:        queue,
		processor:    processor,
		logger:       logger,
		pollInterval: 5 * time.Second,
		stopChan:     make(chan struct{}),
	}
}

// Start starts all workers in the pool
func (wp *WorkerPool) Start(ctx context.Context) error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if len(wp.workers) > 0 {
		return fmt.Errorf("worker pool already started")
	}

	wp.logger.Info("Starting worker pool",
		zap.Int("worker_count", wp.workerCount),
	)

	wp.workers = make([]*Worker, wp.workerCount)
	for i := 0; i < wp.workerCount; i++ {
		worker := &Worker{
			id:        i + 1,
			queue:     wp.queue,
			processor: wp.processor,
			logger:    wp.logger.With(zap.Int("worker_id", i+1)),
			stopChan:  wp.stopChan,
			wg:        &wp.wg,
		}
		wp.workers[i] = worker

		wp.wg.Add(1)
		go wp.runWorker(worker)
	}

	wp.logger.Info("Worker pool started",
		zap.Int("workers", len(wp.workers)),
	)

	return nil
}

// Stop gracefully stops all workers
func (wp *WorkerPool) Stop() error {
	wp.mu.Lock()
	if len(wp.workers) == 0 {
		wp.mu.Unlock()
		return fmt.Errorf("worker pool not started")
	}
	wp.mu.Unlock()

	wp.logger.Info("Stopping worker pool")

	// Signal all workers to stop
	close(wp.stopChan)

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wp.logger.Info("All workers stopped gracefully")
	case <-time.After(30 * time.Second):
		wp.logger.Warn("Worker pool stop timeout, some workers may still be running")
	}

	wp.mu.Lock()
	wp.workers = nil
	wp.mu.Unlock()

	return nil
}

// runWorker runs a single worker's processing loop
func (wp *WorkerPool) runWorker(worker *Worker) {
	defer worker.wg.Done()

	worker.logger.Info("Worker started")

	ticker := time.NewTicker(wp.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-worker.stopChan:
			worker.logger.Info("Worker stopping")
			return

		case <-ticker.C:
			wp.processJobs(worker)
		}
	}
}

// processJobs processes available jobs from the queue
func (wp *WorkerPool) processJobs(worker *Worker) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Increment active worker count
	atomic.AddInt32(&wp.activeWorkers, 1)
	defer atomic.AddInt32(&wp.activeWorkers, -1)

	// Dequeue jobs
	jobs, err := worker.queue.Dequeue(ctx, 10) // Process up to 10 jobs per tick
	if err != nil {
		worker.logger.Error("Failed to dequeue jobs", zap.Error(err))
		return
	}

	if len(jobs) == 0 {
		return
	}

	worker.logger.Debug("Processing jobs",
		zap.Int("count", len(jobs)),
	)

	// Process each job
	for _, job := range jobs {
		select {
		case <-worker.stopChan:
			// Worker is stopping, re-enqueue remaining jobs
			worker.logger.Info("Worker stopping, re-enqueueing job",
				zap.String("job_id", job.ID),
			)
			worker.queue.Enqueue(ctx, job)
			return

		default:
			wp.processJob(ctx, worker, job)
		}
	}
}

// processJob processes a single notification job
func (wp *WorkerPool) processJob(ctx context.Context, worker *Worker, job *NotificationJob) {
	startTime := time.Now()

	worker.logger.Debug("Processing notification",
		zap.String("job_id", job.ID),
		zap.String("user_id", job.UserID.String()),
		zap.String("type", job.Type),
		zap.Int("attempt", job.Attempts+1),
	)

	// Process the notification
	err := worker.processor.Process(ctx, job)

	duration := time.Since(startTime)

	if err != nil {
		worker.logger.Error("Failed to process notification",
			zap.String("job_id", job.ID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)

		// Mark as failed (will retry or move to DLQ)
		if failErr := worker.queue.Fail(ctx, job, err); failErr != nil {
			worker.logger.Error("Failed to handle job failure",
				zap.String("job_id", job.ID),
				zap.Error(failErr),
			)
		}
	} else {
		worker.logger.Info("Notification processed successfully",
			zap.String("job_id", job.ID),
			zap.String("user_id", job.UserID.String()),
			zap.String("type", job.Type),
			zap.Duration("duration", duration),
		)

		// Mark as completed
		if err := worker.queue.Complete(ctx, job.ID); err != nil {
			worker.logger.Error("Failed to mark job as complete",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
		}
	}
}

// ActiveWorkerCount returns the number of currently active workers
func (wp *WorkerPool) ActiveWorkerCount() int {
	return int(atomic.LoadInt32(&wp.activeWorkers))
}

// WorkerCount returns the total number of workers in the pool
func (wp *WorkerPool) WorkerCount() int {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	return len(wp.workers)
}
