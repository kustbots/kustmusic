package vc

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Stub is an in-memory fake satisfying Caller, so the rest of the service
// (HTTP routes, idle-leave, streaming-core wiring) can be built, compiled,
// and exercised with real HTTP requests before the real gogram+ntgcalls
// implementation is wired in. Not for production use.
type Stub struct {
	mu      sync.Mutex
	playing map[int64]time.Time // chatID -> when Play() was called
	onEnd   func(chatID int64)
}

func NewStub() *Stub {
	return &Stub{playing: make(map[int64]time.Time)}
}

func (s *Stub) Play(ctx context.Context, chatID int64, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing[chatID] = time.Now()
	return nil
}

func (s *Stub) PlayedTime(ctx context.Context, chatID int64) (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	start, ok := s.playing[chatID]
	if !ok {
		return 0, fmt.Errorf("vc/stub: chat %d not playing", chatID)
	}
	return time.Since(start), nil
}

func (s *Stub) Pause(ctx context.Context, chatID int64) error  { return nil }
func (s *Stub) Resume(ctx context.Context, chatID int64) error { return nil }

func (s *Stub) Stop(ctx context.Context, chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.playing, chatID)
	return nil
}

func (s *Stub) Join(ctx context.Context, chatOrInvite string) error { return nil }

func (s *Stub) LeaveChat(ctx context.Context, chatID int64) error {
	return s.Stop(ctx, chatID)
}

func (s *Stub) AssistantIdentity(ctx context.Context) (int64, string, error) {
	return 0, "stub-assistant", nil
}

func (s *Stub) ActiveChats() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	chats := make([]int64, 0, len(s.playing))
	for id := range s.playing {
		chats = append(chats, id)
	}
	return chats
}

func (s *Stub) OnStreamEnd(cb func(chatID int64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEnd = cb
}
