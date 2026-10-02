// Package queue tracks each chat's pending songs. Held in memory, with
// Snapshot/Restore so the bot can write everything to MongoDB when Heroku
// signals a dyno restart and pick it back up on boot (see the bot's
// queuestate.go) — a restart mid-song otherwise dropped every queue in
// every group on the floor.
package queue

import "sync"

type Song struct {
	Title         string
	URL           string
	Duration      string // ISO-8601, e.g. "PT3M33S", as returned by yt-api's /search
	Thumbnail     string
	Query         string // what the user actually typed/said, for display
	RequesterID   int64
	RequesterName string
}

type Manager struct {
	mu    sync.Mutex
	queue map[int64][]Song // chatID -> pending songs (index 0 is currently playing)
}

func New() *Manager {
	return &Manager{queue: make(map[int64][]Song)}
}

// Push adds song to chatID's queue. Returns the position it landed at
// (1 = now playing, since callers check this to decide whether to start
// playback immediately or just report "added to queue").
func (m *Manager) Push(chatID int64, song Song) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue[chatID] = append(m.queue[chatID], song)
	return len(m.queue[chatID])
}

// Current returns the song currently playing in chatID, if any.
func (m *Manager) Current(chatID int64) (Song, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queue[chatID]
	if len(q) == 0 {
		return Song{}, false
	}
	return q[0], true
}

// Advance drops the current song and returns the next one, if any.
func (m *Manager) Advance(chatID int64) (Song, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queue[chatID]
	if len(q) == 0 {
		return Song{}, false
	}
	q = q[1:]
	m.queue[chatID] = q
	if len(q) == 0 {
		return Song{}, false
	}
	return q[0], true
}

// DropCurrent removes the song at the head of chatID's queue, but only if
// that head is still the song identified by url.
//
// This exists because a play is pushed onto the queue *before* it is
// attempted, so a setup that fails leaves the track sitting at position 0
// with nothing playing it. Every later /play in that chat then lands at
// position 2 and is announced as "added to queue" behind a phantom that
// will never finish, so the chat is wedged until someone runs /stop.
//
// The url check is what makes this safe to call from a failure path: if a
// newer request has already taken over the head, this is a no-op rather
// than something that would delete the newcomer.
func (m *Manager) DropCurrent(chatID int64, url string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queue[chatID]
	if len(q) == 0 || q[0].URL != url {
		return false
	}
	if len(q) == 1 {
		delete(m.queue, chatID)
		return true
	}
	m.queue[chatID] = q[1:]
	return true
}

// Clear empties chatID's queue entirely (used by /stop).
func (m *Manager) Clear(chatID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.queue, chatID)
}

// List returns a copy of chatID's full queue (for /queue).
func (m *Manager) List(chatID int64) []Song {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queue[chatID]
	out := make([]Song, len(q))
	copy(out, q)
	return out
}

// RemoveAt removes the song at index (0 = currently playing, per List's
// ordering) from chatID's queue and returns it. Index 0 can't be removed
// this way — use Advance to move past the currently-playing song.
func (m *Manager) RemoveAt(chatID int64, index int) (Song, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queue[chatID]
	if index <= 0 || index >= len(q) {
		return Song{}, false
	}
	removed := q[index]
	m.queue[chatID] = append(q[:index], q[index+1:]...)
	return removed, true
}

// MoveToNext moves the song at index to position 1 — right after whatever
// is currently playing (position 0) — for the queue's "▶️ Play Now"
// button. It deliberately doesn't touch position 0 itself or stop the
// current song; the caller pairs this with a normal /skip, which advances
// past the (still) currently-playing song straight into the song this just
// moved into position 1, reusing the same skip path /skip already uses.
func (m *Manager) MoveToNext(chatID int64, index int) (Song, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queue[chatID]
	if index <= 0 || index >= len(q) {
		return Song{}, false
	}
	song := q[index]
	rest := append(q[:index], q[index+1:]...)
	newQ := make([]Song, 0, len(rest)+1)
	newQ = append(newQ, rest[0])
	newQ = append(newQ, song)
	newQ = append(newQ, rest[1:]...)
	m.queue[chatID] = newQ
	return song, true
}

// Snapshot copies every non-empty queue, for persisting across a restart.
func (m *Manager) Snapshot() map[int64][]Song {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[int64][]Song, len(m.queue))
	for chatID, q := range m.queue {
		if len(q) == 0 {
			continue
		}
		songs := make([]Song, len(q))
		copy(songs, q)
		out[chatID] = songs
	}
	return out
}

// Restore installs saved queues, replacing whatever a chat currently has.
// Meant for startup, before any playback has begun.
func (m *Manager) Restore(saved map[int64][]Song) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for chatID, songs := range saved {
		if len(songs) == 0 {
			continue
		}
		m.queue[chatID] = songs
	}
}
