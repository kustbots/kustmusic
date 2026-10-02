// Package app runs the playback engine: the HTTP API the bot calls, the voice
// assistant, and the idle sweeper.
package app

import (
	"context"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/kustbots/kustmusic/internal/core/genwait"
	"github.com/kustbots/kustmusic/internal/core/playback"
	"github.com/kustbots/kustmusic/internal/core/vc"
	"github.com/kustbots/kustmusic/internal/playapi/config"
	"github.com/kustbots/kustmusic/internal/playapi/httpapi"
	"github.com/kustbots/kustmusic/internal/playapi/idle"
	"github.com/kustbots/kustmusic/internal/playapi/notify"
	localvc "github.com/kustbots/kustmusic/internal/playapi/vc"
)

// Start brings the playback engine up and returns. Its HTTP server listens
// on 127.0.0.1 only, for the bot in the same process. dl fetches audio.
func Start(cfg config.Config, dl playback.Downloader) {
	dyn := localvc.NewDynamic(localvc.NewStub())
	var voice localvc.Caller = dyn

	if cfg.AssistantSession == "" {
		// With no assistant session the voice layer is a stand-in, so the
		// rest of the program can still run, but nothing will play.
		slog.Warn("ASSISTANT_SESSION not set, running without a voice assistant. Nothing will play")
	}

	gen := genwait.NewTracker()
	engine := playback.NewEngine(dl, gen, voice, playback.Options{
		MaxCacheBytes: cfg.CacheMaxBytes,
	})

	idleSched := idle.NewScheduler()
	notifier := notify.New(cfg.BotWebhookURL)

	var sweeper *idle.GroupSweeper
	if cfg.IdleGroupLeave > 0 {
		sweeper = idle.NewGroupSweeper(
			cfg.IdleGroupLeave,
			func(ctx context.Context, chatID int64) error { return voice.LeaveChat(ctx, chatID) },
			func(chatID int64) bool {
				for _, id := range voice.ActiveChats() {
					if id == chatID {
						return true
					}
				}
				return false
			},
			slog.Default(),
		)
		sweeper.Protect(cfg.SweepKeepChats)
		// Persist the idle clock so a restart doesn't reset it and both
		// servers sharing an account see each other's plays. Optional: with
		// no store the sweeper keeps its old in-memory behaviour.
		if cfg.MongoURI == "" {
			slog.Warn("MONGO_URI not set — idle-group clock is in-memory only and resets on every restart")
		} else if store, err := idle.ConnectMongoActivityStore(cfg.MongoURI, cfg.MongoDB); err != nil {
			slog.Warn("couldn't reach MongoDB — idle-group clock is in-memory only", "err", err)
		} else {
			sweeper.UseStore(store)
			slog.Info("idle-group clock persisted to MongoDB", "db", cfg.MongoDB)
		}
		go sweeper.Run(context.Background(), time.Minute)
		slog.Info("idle-group sweeper enabled", "leave_after", cfg.IdleGroupLeave, "protected", len(cfg.SweepKeepChats))
	}

	if cfg.AssistantSession != "" {
		go func() {
			if d := startupDelay(cfg.SessionStartDelay); d > 0 {
				dyn.MarkNotReady("waiting out the startup delay before connecting to Telegram")
				slog.Info("holding off the Telegram connect so any previous instance can release this session first", "delay", d)
				time.Sleep(d)
			}
			connectAssistantWithRetry(cfg, dyn, sweeper)
		}()
	}

	server := httpapi.NewServer(voice, engine, gen, idleSched, sweeper, notifier, cfg.IdleLeaveTimeout, cfg.PlayRequestBudget)

	slog.Info("playback engine starting", "addr", "127.0.0.1:"+cfg.Port)
	go func() {
		log.Fatal(http.ListenAndServe("127.0.0.1:"+cfg.Port, server.Router()))
	}()
}

// connectAssistantWithRetry connects the real assistant in the background,
// retrying with capped exponential backoff on failure instead of crashing
// the process. The HTTP server is already up (against the stub) by the time
// this runs, so a slow or repeatedly-failing connection just means play
// requests keep hitting the stub — degraded, but the dyno stays alive and
// keeps retrying, which matters for exactly the kind of transient failure
// that prompted this (AUTH_KEY_DUPLICATED from two play-api instances
// briefly sharing one Telegram account across a redeploy, which normally
// clears within a restart or two).
// watchAssistantHealth periodically re-checks that the connected assistant
// can still talk to Telegram, and flips the voice layer back to "not
// ready" the moment it can't. A session can die *after* a successful
// connect — revoked from another device, invalidated by being reused
// elsewhere (AUTH_KEY_DUPLICATED), or logged out — and without this the
// server kept advertising itself as healthy and accepting /play requests
// it could never fulfil.
func watchAssistantHealth(cfg config.Config, assistant *vc.Assistant, dyn *localvc.Dynamic) {
	const interval = 60 * time.Second
	for range time.Tick(interval) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_, _, err := assistant.AssistantIdentity(ctx)
		cancel()
		if err == nil {
			continue
		}
		slog.Error("assistant health check failed — marking this server unavailable for playback", "err", err)
		dyn.MarkNotReady("assistant session stopped working: " + err.Error())
		// Hand back to the connect loop so a recoverable failure (a
		// transient network blip, or the other end of an
		// AUTH_KEY_DUPLICATED clash going away) heals on its own instead
		// of leaving this dyno permanently useless until someone restarts
		// it. A genuinely dead session just keeps failing there, which is
		// the correct visible outcome.
		go connectAssistantWithRetry(cfg, dyn, nil)
		return
	}
}

// startupDelay spreads the fleet's Telegram reconnects out instead of
// firing them all at the same instant. The configured value is the floor;
// the added jitter is what actually breaks up a stampede, because a
// fleet-wide redeploy boots every play-api within a second of the others
// and a fixed delay would just move the whole pile-up a minute later.
func startupDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	return base + time.Duration(rand.Int63n(int64(30*time.Second)))
}

func connectAssistantWithRetry(cfg config.Config, dyn *localvc.Dynamic, sweeper *idle.GroupSweeper) {
	backoff := 3 * time.Second
	const maxBackoff = 30 * time.Second
	for {
		assistant, err := vc.NewAssistant(context.Background(), cfg.APIID, cfg.APIHash, cfg.AssistantSession)
		if err == nil {
			// The identity call is the real liveness test, not NewAssistant:
			// an expired/revoked session can connect at the transport level
			// and only fail once it actually issues an RPC
			// (AUTH_KEY_UNREGISTERED). Treating that error as merely
			// log-worthy — which this used to — marked a dead session
			// "connected" and let /play claim success on a server that
			// could never play anything.
			me, _, identErr := assistant.AssistantIdentity(context.Background())
			if identErr == nil {
				slog.Info("assistant connected", "user_id", me)
				// Give the account a name that explains itself. Telegram
				// announces it by display name when it joins a group, and
				// leftover names like "LDS 1 - fallback" told those groups
				// nothing about who had just walked in.
				if nameErr := assistant.SetDisplayName(context.Background(), cfg.AssistantName); nameErr != nil {
					slog.Warn("couldn't set the assistant's display name", "err", nameErr)
				}
				dyn.Set(assistant)
				seedSweeper(sweeper, assistant, me)
				go watchAssistantHealth(cfg, assistant, dyn)
				return
			}
			err = identErr
		}
		slog.Warn("assistant connect failed, retrying", "err", err, "retry_in", backoff)
		// Publish the failure, don't just log it. A server stuck here is
		// still answering HTTP perfectly well, so from the outside it looks
		// idle rather than broken — and "idle" is the most attractive thing
		// a least-loaded router can see. Saying so out loud is what keeps
		// new chats off a server that cannot play anything.
		dyn.MarkNotReady("assistant can't connect: " + err.Error())
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// seedSweeper tells the idle-group sweeper about every group this account
// is already in, so its clock covers the whole account rather than only
// the chats that happen to play while this process is up.
//
// Without it the sweeper was almost inert: it learned about a chat when
// that chat played something, so the groups it most needed to leave — old
// ones nobody uses any more — were invisible to it, and a restart reset
// even the little it did know. Everything seeded starts its hour now,
// which is the intended reading of "no song played in the last hour": a
// group that plays something keeps resetting its clock and is never
// touched.
//
// Recorded times are loaded first (Attach), so only groups with no record
// at all start a fresh clock; a group quiet for two days before a restart
// is still two days quiet after it.
func seedSweeper(sweeper *idle.GroupSweeper, assistant *vc.Assistant, accountID int64) {
	if sweeper == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sweeper.Attach(ctx, accountID)

	groups, err := assistant.Groups(ctx)
	if err != nil {
		slog.Warn("couldn't list groups to seed the idle sweeper", "err", err)
		return
	}
	for _, chatID := range groups {
		sweeper.TouchIfUnknown(chatID)
	}
	slog.Info("idle sweeper seeded from the assistant's group list", "groups", len(groups))
}
