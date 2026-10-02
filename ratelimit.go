package main

import (
	"sync"
	"time"
)

// Command throttle, ported from the Python bot's check_abuse
// (RATE_LIMIT_COUNT / RATE_LIMIT_WINDOW in its config). Per user, not per
// chat — one person spamming shouldn't lock out the group around them, and
// a busy group with twenty people is not abuse.
const (
	rateLimitCount  = 4
	rateLimitWindow = 6 * time.Second
)

// warnCooldown stops the "slow down" reply from becoming the spam. Someone
// holding a button down generates a burst; they need telling once, not
// twenty times.
const warnCooldown = 30 * time.Second

// commandLimiter is a sliding-window counter of recent commands per user.
type commandLimiter struct {
	mu       sync.Mutex
	history  map[int64][]time.Time
	lastWarn map[int64]time.Time
}

func newCommandLimiter() *commandLimiter {
	return &commandLimiter{
		history:  make(map[int64][]time.Time),
		lastWarn: make(map[int64]time.Time),
	}
}

// Allow reports whether userID may run a command now, and whether this is
// the right moment to tell them if not.
func (l *commandLimiter) Allow(userID int64) (allowed, warn bool) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	kept := l.history[userID][:0]
	for _, t := range l.history[userID] {
		if now.Sub(t) < rateLimitWindow {
			kept = append(kept, t)
		}
	}
	l.history[userID] = kept

	if len(kept) >= rateLimitCount {
		if now.Sub(l.lastWarn[userID]) >= warnCooldown {
			l.lastWarn[userID] = now
			return false, true
		}
		return false, false
	}

	l.history[userID] = append(kept, now)
	return true, false
}

// Sweep drops users who haven't run a command in a while, so the maps
// don't grow forever in a bot that sees thousands of users. Cheap enough
// to run on a slow timer.
func (l *commandLimiter) Sweep() {
	cutoff := time.Now().Add(-time.Hour)
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, times := range l.history {
		if len(times) == 0 || times[len(times)-1].Before(cutoff) {
			delete(l.history, id)
		}
	}
	for id, t := range l.lastWarn {
		if t.Before(cutoff) {
			delete(l.lastWarn, id)
		}
	}
}
