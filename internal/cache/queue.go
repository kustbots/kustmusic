/*
 * KustMusic - Telegram Music Bot
 * Copyright (c) 2026 KustBots
 * Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 * Licensed under GNU GPL v3
 * See https://github.com/kustbots/kustmusic
 */

package cache

import (
	"github.com/kustbots/kustmusic/internal/utils"
	"slices"
	"sync"
)

// VCData contains the state associated with a chat.
type VCData struct {
	Queue           []*utils.PlayerCache
	Autoplay        bool
	AutoplayHistory []string
}

// ChatCacher is a thread-safe per-chat music state cache.
type ChatCacher struct {
	mu        sync.RWMutex
	chatCache map[int64]*VCData
}

func newChatCacher() *ChatCacher {
	return &ChatCacher{
		chatCache: make(map[int64]*VCData),
	}
}

// getOrCreate returns the chat state, creating it when necessary.
func (c *ChatCacher) getOrCreate(chatID int64) *VCData {
	data, ok := c.chatCache[chatID]
	if !ok {
		data = &VCData{}
		c.chatCache[chatID] = data
	}
	return data
}

// AddSong appends a track to the end of the queue.
func (c *ChatCacher) AddSong(chatID int64, song *utils.PlayerCache) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := c.getOrCreate(chatID)
	data.Queue = append(data.Queue, song)

	if song != nil && song.User != utils.AutoPlay {
		data.AutoplayHistory = nil
	}

	return len(data.Queue)
}

// AddSongs appends multiple tracks to the end of the queue.
func (c *ChatCacher) AddSongs(chatID int64, songs []*utils.PlayerCache) int {
	if len(songs) == 0 {
		return c.GetQueueLength(chatID)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	data := c.getOrCreate(chatID)
	data.Queue = append(data.Queue, songs...)

	return len(data.Queue)
}

// AddSongToFront inserts a track immediately after the currently playing track.
//
// If the queue is empty, the track becomes the current track.
func (c *ChatCacher) AddSongToFront(chatID int64, song *utils.PlayerCache) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := c.getOrCreate(chatID)

	if len(data.Queue) == 0 {
		data.Queue = append(data.Queue, song)
		return 1
	}

	data.Queue = append(data.Queue, nil)
	copy(data.Queue[2:], data.Queue[1:])
	data.Queue[1] = song

	return len(data.Queue)
}

// MoveTrackToFront moves a track immediately after the currently playing track.
//
// The currently playing track at index 0 is never moved.
func (c *ChatCacher) MoveTrackToFront(chatID int64, trackID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.Queue) < 2 {
		return false
	}

	queue := data.Queue

	index := -1
	for i := 1; i < len(queue); i++ {
		if queue[i] != nil && queue[i].TrackID == trackID {
			index = i
			break
		}
	}

	if index == -1 {
		return false
	}

	track := queue[index]
	copy(queue[2:index+1], queue[1:index])

	queue[1] = track

	return true
}

// GetPlayingTrack returns the currently playing track.
func (c *ChatCacher) GetPlayingTrack(chatID int64) *utils.PlayerCache {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.Queue) == 0 {
		return nil
	}

	return data.Queue[0]
}

// GetUpcomingTrack returns the next track in the queue.
func (c *ChatCacher) GetUpcomingTrack(chatID int64) *utils.PlayerCache {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.Queue) < 2 {
		return nil
	}

	return data.Queue[1]
}

// RemoveCurrentSong removes and returns the currently playing track.
func (c *ChatCacher) RemoveCurrentSong(chatID int64) *utils.PlayerCache {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.Queue) == 0 {
		return nil
	}

	removed := data.Queue[0]

	if removed != nil && removed.Platform == utils.YouTube && removed.TrackID != "" {
		if !slices.Contains(data.AutoplayHistory, removed.TrackID) {
			data.AutoplayHistory = append(data.AutoplayHistory, removed.TrackID)
		}
	}

	queue := data.Queue
	queue[0] = nil
	data.Queue = queue[1:]

	return removed
}

// RemoveTrack removes the track at index.
//
// The queue order of all remaining tracks is preserved.
func (c *ChatCacher) RemoveTrack(chatID int64, index int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, ok := c.chatCache[chatID]
	if !ok || index < 0 || index >= len(data.Queue) {
		return false
	}

	queue := data.Queue

	copy(queue[index:], queue[index+1:])
	queue[len(queue)-1] = nil

	data.Queue = queue[:len(queue)-1]

	return true
}

// IsActive reports whether the chat currently has a track.
func (c *ChatCacher) IsActive(chatID int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	return ok && len(data.Queue) > 0
}

// GetAutoplay returns the autoplay state for the chat.
func (c *ChatCacher) GetAutoplay(chatID int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	return ok && data.Autoplay
}

// SetAutoplay updates the autoplay state for the chat.
func (c *ChatCacher) SetAutoplay(chatID int64, state bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := c.getOrCreate(chatID)
	data.Autoplay = state

	if !state {
		data.AutoplayHistory = nil
	}
}

// GetAutoplayHistory returns a copy of the autoplay history for the chat.
//
// The returned slice preserves the order in which tracks were added.
func (c *ChatCacher) GetAutoplayHistory(chatID int64) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.AutoplayHistory) == 0 {
		return nil
	}

	history := make([]string, len(data.AutoplayHistory))
	copy(history, data.AutoplayHistory)
	return history
}

// AddAutoplayHistory adds track IDs to the autoplay history for the chat.
//
// Duplicate IDs are ignored while insertion order is preserved.
func (c *ChatCacher) AddAutoplayHistory(chatID int64, trackIDs ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := c.getOrCreate(chatID)

	for _, id := range trackIDs {
		if id == "" || slices.Contains(data.AutoplayHistory, id) {
			continue
		}

		data.AutoplayHistory = append(data.AutoplayHistory, id)
	}
}

// ClearAutoplayHistory clears the autoplay history for the chat.
func (c *ChatCacher) ClearAutoplayHistory(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, ok := c.chatCache[chatID]
	if ok {
		data.AutoplayHistory = nil
	}
}

// GetLastAutoplayTrackID returns the ID of the most recently added
func (c *ChatCacher) GetLastAutoplayTrackID(chatID int64) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.AutoplayHistory) == 0 {
		return ""
	}

	return data.AutoplayHistory[len(data.AutoplayHistory)-1]
}

// ClearChat removes all state associated with a chat.
func (c *ChatCacher) ClearChat(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.chatCache, chatID)
}

// GetQueueLength returns the number of queued tracks.
func (c *ChatCacher) GetQueueLength(chatID int64) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok {
		return 0
	}

	return len(data.Queue)
}

// GetLoopCount returns the loop count of the currently playing track.
func (c *ChatCacher) GetLoopCount(chatID int64) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.Queue) == 0 || data.Queue[0] == nil {
		return 0
	}

	return data.Queue[0].Loop
}

// SetLoopCount updates the loop count of the currently playing track.
func (c *ChatCacher) SetLoopCount(chatID int64, loop int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.Queue) == 0 || data.Queue[0] == nil {
		return false
	}

	data.Queue[0].Loop = loop
	return true
}

// GetQueue returns a shallow copy of the chat's queue.
func (c *ChatCacher) GetQueue(chatID int64) []*utils.PlayerCache {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok || len(data.Queue) == 0 {
		return nil
	}

	queue := make([]*utils.PlayerCache, len(data.Queue))
	copy(queue, data.Queue)

	return queue
}

// GetActiveChats returns all chat IDs with at least one queued track.
func (c *ChatCacher) GetActiveChats() []int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	active := make([]int64, 0)
	for chatID, data := range c.chatCache {
		if len(data.Queue) > 0 {
			active = append(active, chatID)
		}
	}

	return active
}

// GetTrackIfExists returns a queued track by its ID.
func (c *ChatCacher) GetTrackIfExists(chatID int64, trackID string) *utils.PlayerCache {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.chatCache[chatID]
	if !ok {
		return nil
	}

	for _, track := range data.Queue {
		if track != nil && track.TrackID == trackID {
			return track
		}
	}

	return nil
}

var ChatCache = newChatCacher()
