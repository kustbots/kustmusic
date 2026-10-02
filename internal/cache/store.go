/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package cache

import (
	"slices"
	"sync"
	"time"
)

// Item holds a cached value and its expiration time.
type Item[T any] struct {
	Value      T
	Expiration time.Time
}

// Cache is a generic, thread-safe TTL cache with automatic background eviction.
type Cache[T any] struct {
	mu   sync.RWMutex
	data map[string]Item[T]
	ttl  time.Duration
}

// NewCache creates a Cache with the given default TTL and registers it
func NewCache[T any](ttl time.Duration) *Cache[T] {
	c := &Cache[T]{
		data: make(map[string]Item[T]),
		ttl:  ttl,
	}
	registerCache(c)
	return c
}

// Get returns the value for key and true if it exists and has not expired.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	item, ok := c.data[key]
	c.mu.RUnlock()

	if !ok {
		var zero T
		return zero, false
	}

	if time.Now().After(item.Expiration) {
		c.mu.Lock()

		if it, still := c.data[key]; still && time.Now().After(it.Expiration) {
			delete(c.data, key)
		}
		c.mu.Unlock()

		var zero T
		return zero, false
	}

	return item.Value, true
}

// Set stores value under key using the default TTL.
func (c *Cache[T]) Set(key string, value T) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL stores value under key with a custom TTL.
func (c *Cache[T]) SetWithTTL(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	c.data[key] = Item[T]{
		Value:      value,
		Expiration: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

// Delete removes key from the cache immediately.
func (c *Cache[T]) Delete(key string) {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
}

// Clear evicts all items at once.
func (c *Cache[T]) Clear() {
	c.mu.Lock()
	c.data = make(map[string]Item[T])
	c.mu.Unlock()
}

// Size returns the number of entries currently in the cache (including not-yet-evicted expired ones).
func (c *Cache[T]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// Close unregisters the cache from the background cleanup janitor.
func (c *Cache[T]) Close() {
	unregisterCache(c)
}

// evictExpired removes all entries whose TTL has elapsed.
func (c *Cache[T]) evictExpired() {
	now := time.Now()

	c.mu.RLock()
	var expiredKeys []string
	for key, item := range c.data {
		if now.After(item.Expiration) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	c.mu.RUnlock()

	if len(expiredKeys) == 0 {
		return
	}

	c.mu.Lock()
	for _, key := range expiredKeys {
		if item, ok := c.data[key]; ok && now.After(item.Expiration) {
			delete(c.data, key)
		}
	}
	c.mu.Unlock()
}

// cleaner is an interface for caches that can evict expired items.
type cleaner interface {
	evictExpired()
}

// janitor manages a single goroutine that cleans up multiple caches.
type janitor struct {
	mu       sync.Mutex
	caches   []cleaner
	interval time.Duration
	stop     chan struct{}
	running  bool
}

var (
	sharedJanitor   *janitor
	janitorOnce     sync.Once
	janitorInterval = time.Minute
)

func getJanitor() *janitor {
	janitorOnce.Do(func() {
		sharedJanitor = &janitor{
			interval: janitorInterval,
		}
	})
	return sharedJanitor
}

func (j *janitor) runWith(interval time.Duration, stop chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			j.mu.Lock()
			if len(j.caches) == 0 {
				j.mu.Unlock()
				continue
			}
			caches := make([]cleaner, len(j.caches))
			copy(caches, j.caches)
			j.mu.Unlock()

			for _, c := range caches {
				c.evictExpired()
			}
		case <-stop:
			return
		}
	}
}

func registerCache(c cleaner) {
	getJanitor().register(c)
}

func (j *janitor) register(c cleaner) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.caches = append(j.caches, c)
	if !j.running {
		j.interval = janitorInterval
		j.stop = make(chan struct{})
		j.running = true
		go j.runWith(j.interval, j.stop)
	}
}

func unregisterCache(c cleaner) {
	getJanitor().unregister(c)
}

func (j *janitor) unregister(c cleaner) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i, v := range j.caches {
		if v == c {
			j.caches = append(j.caches[:i], j.caches[i+1:]...)
			break
		}
	}
	if len(j.caches) == 0 && j.running {
		close(j.stop)
		j.running = false
	}
}

func (j *janitor) has(c cleaner) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return slices.Contains(j.caches, c)
}
