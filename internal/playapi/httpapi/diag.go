package httpapi

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"
)

func (s *Server) handleActiveCalls(w http.ResponseWriter, r *http.Request) {
	streams := s.activeStream.all()
	// "ready" is what lets the bot's load poller tell a genuinely idle
	// server apart from one whose assistant session is dead. Both host zero
	// calls, and a least-loaded picker reading only the count sees the dead
	// one as the emptiest — so the most broken server in the fleet became
	// the default destination for every new chat.
	//
	// Still a 200 with the count intact: this endpoint is also the fleet's
	// liveness check, and the server *is* answering. The distinction being
	// reported here is "can it stream", not "is the process up".
	ready := true
	if rc, ok := s.Voice.(interface{ Ready() bool }); ok {
		ready = rc.Ready()
	}
	body := map[string]any{
		"active_calls": streams,
		"count":        len(streams),
		"ready":        ready,
	}
	// When it can't stream, say why in the same breath. The bot only needs
	// ready=false to route around this server, but a human looking at the
	// fleet needs to know whether that's a duplicated session, a revoked
	// account, or just a dyno that booted twenty seconds ago.
	if !ready {
		reason := "assistant not connected"
		if rr, ok := s.Voice.(interface{ NotReadyReason() string }); ok {
			if r := rr.NotReadyReason(); r != "" {
				reason = r
			}
		}
		body["reason"] = reason
	}
	writeJSON(w, http.StatusOK, body)
}

// handleAssistant reports which Telegram account this server streams
// through. The bot calls it to learn the assistant's user id, which it
// needs to check whether that account is actually present in a group
// before claiming a song is playing there.
//
// A server with no connected assistant answers 503 rather than a partial
// identity: it cannot play anything, and the bot should route the chat to
// a different server instead of checking membership for an account that
// isn't going to stream regardless.
func (s *Server) handleAssistant(w http.ResponseWriter, r *http.Request) {
	if rc, ok := s.Voice.(interface{ Ready() bool }); ok && !rc.Ready() {
		s.InvalidateIdentity()
		writeError(w, http.StatusServiceUnavailable,
			"assistant not connected on this server - its session is missing, expired, or still reconnecting")
		return
	}

	id, username, err := s.Identity(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       id,
		"username": username,
		"mention":  assistantMention(id, username),
	})
}

// assistantMention renders the assistant in a form a user can act on:
// its @username when it has one, and an inline user link when it doesn't
// (accounts without a public username can still be opened by id).
func assistantMention(id int64, username string) string {
	if username != "" {
		return "@" + username
	}
	return "tg://user?id=" + strconv.FormatInt(id, 10)
}

// handleStatus reports basic process health. Go's own runtime stats replace
// the Python service's raw /proc reads — same intent (cheap, no external
// dependency), idiomatic for the language instead of a subprocess shell-out.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	status := map[string]any{
		"goroutines":     runtime.NumGoroutine(),
		"cpu_cores":      runtime.NumCPU(),
		"heap_alloc_mb":  float64(mem.HeapAlloc) / 1024 / 1024,
		"heap_sys_mb":    float64(mem.HeapSys) / 1024 / 1024,
		"uptime_seconds": time.Since(s.StartedAt).Seconds(),
		"active_calls":   len(s.activeStream.all()),
	}

	// Disk, not just heap. The Go heap here is a couple of megabytes whether
	// this server is idle or streaming, because the audio never passes
	// through it — so heap numbers look reassuring while the real
	// load-dependent resource, downloaded audio on disk, goes unreported.
	// That is the one that used to grow without limit.
	if s.Engine != nil && s.Engine.Cache != nil {
		used, files, max := s.Engine.Cache.Stats()
		status["cache_used_mb"] = float64(used) / 1024 / 1024
		status["cache_max_mb"] = float64(max) / 1024 / 1024
		status["cache_files"] = files
		if max > 0 {
			status["cache_used_pct"] = float64(used) / float64(max) * 100
		}
	}

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"message": "Restarting application..."})
	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(1)
	}()
}
