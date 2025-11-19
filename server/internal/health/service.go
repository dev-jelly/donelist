package health

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// Service manages health checks for all application components
type Service struct {
	checkers  []Checker
	startTime time.Time
	version   string
	logger    *zap.Logger
	mu        sync.RWMutex
}

// NewService creates a new health check service
func NewService(version string, logger *zap.Logger) *Service {
	return &Service{
		checkers:  make([]Checker, 0),
		startTime: time.Now(),
		version:   version,
		logger:    logger,
	}
}

// RegisterChecker adds a health checker to the service
func (s *Service) RegisterChecker(checker Checker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkers = append(s.checkers, checker)
}

// CheckHealth performs all health checks and returns a report
func (s *Service) CheckHealth() HealthReport {
	s.mu.RLock()
	checkers := s.checkers
	s.mu.RUnlock()

	report := HealthReport{
		Status:     StatusHealthy,
		Version:    s.version,
		Timestamp:  time.Now(),
		Uptime:     time.Since(s.startTime),
		Components: make(map[string]ComponentHealth),
	}

	// Run all health checks concurrently
	resultsChan := make(chan ComponentHealth, len(checkers))
	var wg sync.WaitGroup

	for _, checker := range checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("health check panic",
						zap.String("checker", c.Name()),
						zap.Any("panic", r),
					)
					resultsChan <- ComponentHealth{
						Name:      c.Name(),
						Status:    StatusUnhealthy,
						Error:     "health check panicked",
						Timestamp: time.Now(),
					}
				}
			}()

			result := c.Check()
			resultsChan <- result
		}(checker)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	for result := range resultsChan {
		report.Components[result.Name] = result

		// Update overall status based on component status
		if result.Status == StatusUnhealthy {
			report.Status = StatusUnhealthy
		} else if result.Status == StatusDegraded && report.Status == StatusHealthy {
			report.Status = StatusDegraded
		}
	}

	return report
}

// CheckHealthWithBuildInfo performs all health checks and returns a report with build info
func (s *Service) CheckHealthWithBuildInfo() UpdatedHealthReport {
	basicReport := s.CheckHealth()

	return UpdatedHealthReport{
		Status:     basicReport.Status,
		Version:    basicReport.Version,
		BuildInfo:  GetBuildInfo(),
		Timestamp:  basicReport.Timestamp,
		Uptime:     basicReport.Uptime,
		Components: basicReport.Components,
	}
}

// CheckHealthSimple performs a simple health check (just returns status)
func (s *Service) CheckHealthSimple() (Status, error) {
	report := s.CheckHealth()
	if report.Status != StatusHealthy {
		return report.Status, nil
	}
	return StatusHealthy, nil
}

// GetUptime returns the service uptime
func (s *Service) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

// IsHealthy returns true if all components are healthy
func (s *Service) IsHealthy() bool {
	status, _ := s.CheckHealthSimple()
	return status == StatusHealthy
}
