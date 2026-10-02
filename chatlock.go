package main

import "sync"

// chatLocks serializes playback setup per chat, so two /play commands (or
// a /play racing an auto-skip) in the same group can't both be mid-setup
// against play-api at once.
//
// Without this they could, and did: each /play bumps play-api's per-chat
// generation counter, so whichever request got there second invalidated
// the first, and the first came back "superseded by a newer play request".
// In a busy chat that produced a burst of failures for what users
// experienced as ordinary back-to-back requests — the main source of
// playback feeling random and jittery.
//
// Locking per chat (not globally) keeps unrelated groups fully parallel.
type chatLocks struct {
	mu    sync.Mutex
	locks map[int64]*sync.Mutex
}

func newChatLocks() *chatLocks {
	return &chatLocks{locks: make(map[int64]*sync.Mutex)}
}

func (c *chatLocks) get(chatID int64) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if l, ok := c.locks[chatID]; ok {
		return l
	}
	l := &sync.Mutex{}
	c.locks[chatID] = l
	return l
}

// Lock blocks until this chat's playback setup is free, then holds it.
func (c *chatLocks) Lock(chatID int64) {
	c.get(chatID).Lock()
}

func (c *chatLocks) Unlock(chatID int64) {
	c.get(chatID).Unlock()
}
