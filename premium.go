package main

import (
	"context"
	"sync"
	"time"

	"github.com/kustbots/kustmusic/internal/store"
)

// premiumCache mirrors main-music-rx's in-memory premium_users dict,
// refreshed from Mongo periodically so a lookup on the hot /play path never
// waits on a database round-trip.
type premiumCache struct {
	mu    sync.RWMutex
	users map[int64]bool
}

func newPremiumCache() *premiumCache {
	return &premiumCache{users: make(map[int64]bool)}
}

func (c *premiumCache) isPremium(userID int64) bool {
	if userID == 0 {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.users[userID]
}

func (c *premiumCache) refresh(db *store.Store) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	users, err := db.LoadPremiumUsers(ctx)
	if err != nil {
		return
	}
	c.mu.Lock()
	c.users = users
	c.mu.Unlock()
}

func (c *premiumCache) add(userID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users[userID] = true
}
