package httpapi

import (
	"context"
	"sync"
	"time"
)

// identityTTL is how long a resolved assistant identity stays cached.
//
// The account behind a given server never changes without a redeploy, so
// this could be cached forever — but a bounded TTL means a server that
// reconnects with a different session eventually reports the truth without
// needing a restart.
const identityTTL = 30 * time.Minute

// identityCache memoizes the assistant's own user id and username.
//
// Resolving it means an MTProto GetMe round-trip, and the bot now asks for
// it before every play (it needs the id to check whether that assistant is
// actually a member of the group). Doing a network round-trip on each of
// those would add latency to the one path that matters most, so it is
// resolved once and reused.
type identityCache struct {
	mu       sync.Mutex
	id       int64
	username string
	fetched  time.Time
}

// Identity returns the connected assistant's id and username, resolving it
// at most once per identityTTL. Errors are never cached — a failure here
// usually means the assistant is still connecting, and the next request
// should try again rather than inherit a stale failure.
func (s *Server) Identity(ctx context.Context) (int64, string, error) {
	s.identity.mu.Lock()
	defer s.identity.mu.Unlock()

	if s.identity.id != 0 && time.Since(s.identity.fetched) < identityTTL {
		return s.identity.id, s.identity.username, nil
	}

	id, username, err := s.Voice.AssistantIdentity(ctx)
	if err != nil {
		return 0, "", err
	}
	s.identity.id, s.identity.username, s.identity.fetched = id, username, time.Now()
	return id, username, nil
}

// InvalidateIdentity drops the cached identity — called when the assistant
// is known to have changed or gone away, so the next lookup re-resolves.
func (s *Server) InvalidateIdentity() {
	s.identity.mu.Lock()
	defer s.identity.mu.Unlock()
	s.identity.id, s.identity.username, s.identity.fetched = 0, "", time.Time{}
}
