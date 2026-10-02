package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// precacher downloads a queued song in the background before anyone asks to
// play it.
//
// A cold track costs several seconds to find and download. The next song in
// a queue is known well before it is needed, usually minutes, so fetching it
// while the current track plays turns that wait into a cache hit.
//
// Requests are deduplicated: queueing the same song twice, or two chats
// queueing it at once, does one fetch, not several.
type precacher struct {
	base string
	http *http.Client
	log  *slog.Logger

	mu       sync.Mutex
	inflight map[string]bool
}

func newPrecacher(base string, log *slog.Logger) *precacher {
	return &precacher{
		base: base,
		// Generous: a cold fetch is the whole point of this, and it runs in
		// the background where nobody is waiting on it.
		http:     &http.Client{Timeout: 180 * time.Second},
		log:      log,
		inflight: make(map[string]bool),
	}
}

// Warm asks the playback engine to download watchURL into its cache in the
// background. Returns immediately; never blocks the caller.
func (p *precacher) Warm(watchURL string) {
	if p == nil || p.base == "" || watchURL == "" {
		return
	}

	p.mu.Lock()
	if p.inflight[watchURL] {
		p.mu.Unlock()
		return // already being fetched
	}
	p.inflight[watchURL] = true
	p.mu.Unlock()

	go func() {
		defer func() {
			p.mu.Lock()
			delete(p.inflight, watchURL)
			p.mu.Unlock()
			if rec := recover(); rec != nil {
				p.log.Error("panic while pre-caching", "recover", rec)
			}
		}()

		start := time.Now()
		endpoint := p.base + url.QueryEscape(watchURL)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
		if err != nil {
			return
		}
		resp, err := p.http.Do(req)
		if err != nil {
			p.log.Debug("pre-cache fetch failed", "url", watchURL, "err", err)
			return
		}
		defer resp.Body.Close()

		// The response is only a confirmation; the download itself lands in
		// the engine's cache.
		n, _ := io.Copy(io.Discard, resp.Body)
		p.log.Info("pre-cached queued song",
			"url", watchURL, "status", resp.StatusCode,
			"bytes", n, "took", time.Since(start).Round(time.Millisecond))
	}()
}
