package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kustbots/kustmusic/internal/core/playback"
)

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// pingGraceWindow is how long handlePlay waits for a normal, fast result
// before switching to keep-alive pings. Once a request crosses this
// threshold it commits to a 200 status line (see the comment above the
// pinging branch below for why), so this stays generous enough that the
// common case — CDN resolves and confirms buffering — finishes within it
// and keeps the exact status-code contract callers already rely on.
const pingGraceWindow = 20 * time.Second

const pingInterval = 8 * time.Second

// handlePlay is the core endpoint: downloads the song, plays it and confirms
// sound is coming out. A fresh join's real WebRTC handshake can legitimately
// run well past a hosting router's ~30s deadline (see CONNECT_WAIT_SECONDS in
// internal/core/vc/assistant.go), so once a
// request runs longer than pingGraceWindow this starts sending single-byte
// keep-alive pings to keep the connection alive from the router's
// perspective, matching the Python service's original approach to the same
// problem but adapted for how far this port pushes the connect-wait.
func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	chatIDStr := r.URL.Query().Get("chatid")
	videoURL := r.URL.Query().Get("url")
	apiParam := r.URL.Query().Get("api") // accepted for backward compatibility, no longer changes behavior
	if apiParam == "" {
		apiParam = "1"
	}
	botIDStr := r.URL.Query().Get("bot")

	if chatIDStr == "" || videoURL == "" {
		writeError(w, http.StatusBadRequest, "Missing chatid or url parameter")
		return
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid chatid parameter")
		return
	}
	botID, _ := strconv.ParseInt(botIDStr, 10, 64) // best-effort; 0 means "no bot to notify"

	// How long the track really runs, per the caller. Optional and
	// best-effort: an older bot doesn't send it and gets 0, which simply
	// skips the length check rather than failing the request — the two
	// services have to be able to deploy independently. When present it is
	// what lets the engine tell a complete download from one that was cut
	// short, which nothing at the HTTP layer can see for a chunked response.
	expectedSeconds, _ := strconv.ParseFloat(r.URL.Query().Get("duration"), 64)

	// Refuse to pretend. Without a real assistant connected the voice layer
	// is the in-memory stub, which reports every play as a success — so a
	// server whose session had expired kept answering "Playing media" for
	// songs nobody could hear. Fail loudly instead, so the bot surfaces a
	// real error and the caller can route elsewhere.
	if rc, ok := s.Voice.(interface{ Ready() bool }); ok && !rc.Ready() {
		writeError(w, http.StatusServiceUnavailable,
			"assistant not connected on this server — its session is missing, expired, or still reconnecting")
		return
	}

	s.Idle.Cancel(chatID)
	if s.Sweeper != nil {
		s.Sweeper.Touch(chatID) // resets this chat's idle-group clock
	}
	myGen := s.Gen.Bump(chatID)
	s.activeStream.set(chatID, botID)

	ctx, cancel := context.WithTimeout(r.Context(), s.PlayBudget)
	defer cancel()

	resultCh := make(chan error, 1)
	go func() {
		resultCh <- s.Engine.Start(ctx, chatID, videoURL, myGen, expectedSeconds)
	}()

	successBody := map[string]any{
		"message":      "Playing media",
		"chatid":       chatIDStr,
		"url":          videoURL,
		"api_selected": apiParam,
		"bot":          botIDStr,
	}

	select {
	case err = <-resultCh:
		// Finished within the grace window — nothing has been written to
		// the response yet, so this keeps the exact same status-code
		// contract callers already rely on (a real 200/409/500, not a body
		// field to check).
		if err != nil {
			s.activeStream.pop(chatID)
			if errors.Is(err, playback.ErrStale) {
				writeError(w, http.StatusConflict, "superseded by a newer play request")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, successBody)
		return

	case <-time.After(pingGraceWindow):
		// Running long. From here on the response body is going to contain
		// ping bytes before we know the outcome, and HTTP requires the
		// status line before any body bytes — so this commits to 200 now
		// and reports the real outcome in the JSON body's "error" field
		// instead of the status code for whatever remains of this request.
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)
	if canFlush {
		flusher.Flush()
	}

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case err = <-resultCh:
			if err != nil {
				s.activeStream.pop(chatID)
				msg := err.Error()
				if errors.Is(err, playback.ErrStale) {
					msg = "superseded by a newer play request"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
				return
			}
			_ = json.NewEncoder(w).Encode(successBody)
			return
		case <-ticker.C:
			_, _ = w.Write([]byte(" "))
			if canFlush {
				flusher.Flush()
			}
		}
	}
}
