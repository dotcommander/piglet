// Package loop provides a recurring-prompt scheduler for the piglet loop extension.
// It is free of SDK dependencies and fully testable in isolation.
package loop

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MinInterval is the minimum allowed scheduling interval.
// Tests may lower this to speed up timing assertions.
var MinInterval = 30 * time.Second

// Scheduler runs a prompt on a recurring interval.
// All methods are safe for concurrent use.
type Scheduler struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{} // closed when the goroutine exits
	interval time.Duration
	prompt   string
	count    int
	running  bool
}

// Start launches the scheduler. It calls onTick immediately (iteration 1),
// then again after each interval. Returns an error if:
//   - interval < MinInterval
//   - the scheduler is already running
func (s *Scheduler) Start(interval time.Duration, prompt string, onTick func(iteration int, prompt string)) error {
	if interval < MinInterval {
		return errors.New("interval must be at least " + MinInterval.String())
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return errors.New("loop already running; stop it first with /loop-stop")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.cancel = cancel
	s.done = done
	s.interval = interval
	s.prompt = prompt
	s.count = 0
	s.running = true

	go func() {
		defer close(done)
		defer func() {
			s.mu.Lock()
			s.running = false
			s.cancel = nil
			s.mu.Unlock()
		}()

		iteration := 0
		for {
			iteration++
			s.mu.Lock()
			s.count = iteration
			s.mu.Unlock()

			onTick(iteration, prompt)

			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()

	return nil
}

// Stop cancels the active loop and waits for the goroutine to exit.
// Returns true if a loop was running, false if there was nothing to stop.
func (s *Scheduler) Stop() bool {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return false
	}
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	cancel()
	<-done
	return true
}

// Status returns the current scheduler state.
func (s *Scheduler) Status() (running bool, interval time.Duration, prompt string, iterations int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.interval, s.prompt, s.count
}
