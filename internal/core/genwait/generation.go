// Package genwait provides the per-chat "generation" counter both services
// use to guard against stale background work (a backup-download-and-switch
// that finishes after a newer /play or /stop has already superseded it).
// Bumped on every play-start and every stop; a background task snapshots the
// generation at start and checks it hasn't changed before acting.
package genwait

import "sync"

type Tracker struct {
	mu  sync.Mutex
	gen map[int64]uint64
}

func NewTracker() *Tracker {
	return &Tracker{gen: make(map[int64]uint64)}
}

// Bump increments and returns the new generation for chatID. Call this at
// the start of every play attempt and on every stop/skip.
func (t *Tracker) Bump(chatID int64) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gen[chatID]++
	return t.gen[chatID]
}

// Current returns the live generation for chatID without mutating it.
func (t *Tracker) Current(chatID int64) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.gen[chatID]
}

// Valid reports whether myGen is still the live generation for chatID — call
// this before a background task is allowed to act (e.g. switch playback
// source). A false result means a newer play/stop already superseded it.
func (t *Tracker) Valid(chatID int64, myGen uint64) bool {
	return t.Current(chatID) == myGen
}
