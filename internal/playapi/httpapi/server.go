// Package httpapi is play-api's HTTP surface — a direct route-for-route port
// of the Python Quart service (see the project plan for the full contract).
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kustbots/kustmusic/internal/core/genwait"
	"github.com/kustbots/kustmusic/internal/core/playback"
	"github.com/kustbots/kustmusic/internal/playapi/idle"
	"github.com/kustbots/kustmusic/internal/playapi/notify"
	"github.com/kustbots/kustmusic/internal/playapi/vc"
)

type Server struct {
	Voice        vc.Caller
	Engine       *playback.Engine
	Gen          *genwait.Tracker
	Idle         *idle.Scheduler
	Sweeper      *idle.GroupSweeper
	Notify       *notify.Notifier
	Log          *slog.Logger
	IdleTimeout  time.Duration
	PlayBudget   time.Duration
	StartedAt    time.Time
	activeStream *activeStreamMap
	identity     identityCache
	// joins throttles /join. It lives on the server rather than being a
	// package-level value because it is per-assistant state, and one
	// process is one assistant.
	joins *joinLimiter
}

func NewServer(voice vc.Caller, engine *playback.Engine, gen *genwait.Tracker, idleSched *idle.Scheduler, sweeper *idle.GroupSweeper, notifier *notify.Notifier, idleTimeout, playBudget time.Duration) *Server {
	s := &Server{
		Voice:        voice,
		Engine:       engine,
		Gen:          gen,
		Idle:         idleSched,
		Sweeper:      sweeper,
		Notify:       notifier,
		Log:          slog.Default(),
		IdleTimeout:  idleTimeout,
		PlayBudget:   playBudget,
		StartedAt:    time.Now(),
		activeStream: newActiveStreamMap(),
		joins:        &joinLimiter{},
	}

	// Wire natural stream-end -> tell the bot, and nothing else.
	//
	// No idle timer here any more: the bot owns the queue, so it's the only
	// side that knows whether a finished song has a successor. It reacts to
	// this event by either starting the next track or calling /stop, which
	// leaves the call immediately. The old behavior — scheduling a
	// timed leave here — meant the assistant lingered in an empty voice
	// chat for IdleTimeout after the last song, and fired a spurious
	// "left due to inactivity" even for streams that had already failed.
	voice.OnStreamEnd(func(chatID int64) {
		botID := s.activeStream.get(chatID)
		if botID != 0 {
			_ = s.Notify.Send(context.Background(), botID, map[string]any{
				"event": "stream-ended", "chatid": chatID, "bot": botID,
			})
		}
	})

	return s
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer) // a panic in one handler can't take down the process
	r.Use(middleware.Logger)

	r.Get("/play", s.handlePlay)
	r.Get("/stop", s.handleStop)
	r.Get("/pause", s.handlePause)
	r.Get("/resume", s.handleResume)
	r.Get("/join", s.handleJoin)
	r.Get("/cache", s.handleCache)
	r.Get("/active_calls", s.handleActiveCalls)
	r.Get("/assistant", s.handleAssistant)
	r.Get("/status", s.handleStatus)
	r.Get("/restart", s.handleRestart)

	return r
}
