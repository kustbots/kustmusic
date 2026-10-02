package httpapi

import "sync"

// activeStreamMap tracks which bot account requested each chat's current
// stream, so notifications (stream-ended, left-idle) route back to the right
// caller — direct port of the Python service's `active_streams` dict.
type activeStreamMap struct {
	mu   sync.Mutex
	data map[int64]int64 // chatID -> botID
}

func newActiveStreamMap() *activeStreamMap {
	return &activeStreamMap{data: make(map[int64]int64)}
}

func (m *activeStreamMap) set(chatID, botID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[chatID] = botID
}

func (m *activeStreamMap) get(chatID int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[chatID]
}

func (m *activeStreamMap) pop(chatID int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	botID := m.data[chatID]
	delete(m.data, chatID)
	return botID
}

func (m *activeStreamMap) all() map[int64]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[int64]int64, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	return out
}
