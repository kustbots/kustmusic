// Package idle schedules/cancels the "leave the call if nothing plays for a
// while" timer per chat — direct port of the Python service's
// schedule_idle_leave/cancel_idle_leave pair.
package idle

import (
	"context"
	"sync"
	"time"
)

type Scheduler struct {
	mu    sync.Mutex
	tasks map[int64]context.CancelFunc
}

func NewScheduler() *Scheduler {
	return &Scheduler{tasks: make(map[int64]context.CancelFunc)}
}

// Cancel stops any pending idle-leave timer for chatID (no-op if none).
func (s *Scheduler) Cancel(chatID int64) {
	s.mu.Lock()
	cancel, ok := s.tasks[chatID]
	if ok {
		delete(s.tasks, chatID)
	}
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// Schedule replaces any existing timer for chatID with a new one; onFire
// runs after `after` unless Cancel is called first.
func (s *Scheduler) Schedule(chatID int64, after time.Duration, onFire func()) {
	s.Cancel(chatID)

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.tasks[chatID] = cancel
	s.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(after):
		}
		s.mu.Lock()
		delete(s.tasks, chatID)
		s.mu.Unlock()
		onFire()
	}()
}
