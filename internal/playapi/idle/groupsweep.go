package idle

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// GroupSweeper leaves groups the assistant hasn't played anything in for a
// while, reclaiming slots against Telegram's per-account group limit (the
// thing that eventually produces CHANNELS_TOO_MUCH and blocks joining any
// new chat at all).
//
// This only leaves groups that have been quiet for the full window and are
// not currently playing. A chat the assistant leaves this way has to be
// re-joined before it can play again — the bot handles that automatically
// on the next /play (see the bot's ensureAssistantInChat), so a quiet group
// coming back to life re-invites the assistant rather than silently
// failing.
//
// With an ActivityStore attached, the clock is shared and persistent: it
// survives restarts, and plays on any server backed by the same account
// count. Without one it behaves exactly as it always did, in memory only.
type GroupSweeper struct {
	mu     sync.Mutex
	last   map[int64]time.Time
	keep   map[int64]bool // chats that must never be swept
	window time.Duration
	leave  func(ctx context.Context, chatID int64) error
	active func(chatID int64) bool
	log    *slog.Logger

	store   ActivityStore
	account int64 // assistant user id; 0 until Attach
}

// storeTimeout bounds each round trip to the store. Writes happen off the
// request path, but a hung database must not pile up goroutines.
const storeTimeout = 5 * time.Second

func NewGroupSweeper(window time.Duration, leave func(ctx context.Context, chatID int64) error, active func(chatID int64) bool, log *slog.Logger) *GroupSweeper {
	return &GroupSweeper{
		last:   make(map[int64]time.Time),
		keep:   make(map[int64]bool),
		window: window,
		leave:  leave,
		active: active,
		log:    log,
	}
}

// UseStore gives the sweeper somewhere persistent to keep its clock. Call
// before the assistant connects; the store isn't read until Attach.
func (g *GroupSweeper) UseStore(store ActivityStore) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.store = store
}

// Attach binds the sweeper to the assistant account it is sweeping for and
// loads that account's recorded times, keeping whichever is fresher where
// memory and the store disagree.
//
// It must run before seeding. Seeding starts the clock for every group not
// already known, and it is the store that makes a group "known" across a
// restart — seed first and every group would look freshly active again,
// which is precisely the reset this exists to stop.
func (g *GroupSweeper) Attach(ctx context.Context, accountID int64) {
	g.mu.Lock()
	g.account = accountID
	store := g.store
	g.mu.Unlock()

	if store == nil || accountID == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()
	recorded, err := store.Load(ctx, accountID)
	if err != nil {
		g.log.Warn("couldn't load idle-group times; this boot starts every clock fresh", "err", err)
		return
	}

	g.mu.Lock()
	for chatID, at := range recorded {
		if cur, ok := g.last[chatID]; !ok || at.After(cur) {
			g.last[chatID] = at
		}
	}
	g.mu.Unlock()
	g.log.Info("idle-group times restored from store", "groups", len(recorded))
}

// storeFor returns the store and account to write to, or nil when there is
// nothing to persist yet.
func (g *GroupSweeper) storeFor() (ActivityStore, int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.store == nil || g.account == 0 {
		return nil, 0
	}
	return g.store, g.account
}

// persist runs one store write off the caller's goroutine. Losing a write
// is not dangerous — the effect is at worst that a group looks idler than
// it is, and the pre-leave re-check reads the store again anyway.
func (g *GroupSweeper) persist(what string, chatID int64, write func(ctx context.Context, store ActivityStore, account int64) error) {
	store, account := g.storeFor()
	if store == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
		defer cancel()
		if err := write(ctx, store, account); err != nil {
			g.log.Warn("idle-group store write failed", "op", what, "chat_id", chatID, "err", err)
		}
	}()
}

// Touch records that chatID just played something, resetting its clock.
func (g *GroupSweeper) Touch(chatID int64) {
	now := time.Now()
	g.mu.Lock()
	g.last[chatID] = now
	g.mu.Unlock()

	g.persist("touch", chatID, func(ctx context.Context, s ActivityStore, account int64) error {
		return s.Touch(ctx, account, chatID, now)
	})
}

// TouchIfUnknown starts chatID's clock only if it isn't already being
// tracked. Used to seed the sweeper from the assistant's real group list
// at startup without overwriting the fresher timestamp of a chat that has
// already played something this session.
func (g *GroupSweeper) TouchIfUnknown(chatID int64) {
	now := time.Now()
	g.mu.Lock()
	_, known := g.last[chatID]
	if !known {
		g.last[chatID] = now
	}
	g.mu.Unlock()
	if known {
		return
	}

	g.persist("seed", chatID, func(ctx context.Context, s ActivityStore, account int64) error {
		return s.TouchIfMissing(ctx, account, chatID, now)
	})
}

// Protect marks chats the sweeper must never leave, whatever their idle
// time — an escape hatch for a log or support chat the assistant is
// expected to stay in permanently. Empty by default, so this changes
// nothing unless it's configured.
func (g *GroupSweeper) Protect(chatIDs []int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, id := range chatIDs {
		g.keep[id] = true
	}
}

// Forget drops chatID's tracking entirely (it already left, or never
// really joined).
func (g *GroupSweeper) Forget(chatID int64) {
	g.mu.Lock()
	delete(g.last, chatID)
	g.mu.Unlock()

	g.persist("forget", chatID, func(ctx context.Context, s ActivityStore, account int64) error {
		return s.Forget(ctx, account, chatID)
	})
}

// forgetLocal drops chatID from memory only — for when the store already
// says it's gone, so there is nothing to delete there.
func (g *GroupSweeper) forgetLocal(chatID int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.last, chatID)
}

// recheck consults the store before a leave. It returns false when the
// leave must not happen: another server on the same account played more
// recently than this one knew, the store can't be read (a doubtful leave
// is worse than a late one, since every leave costs a rate-limited rejoin),
// or the record is gone because that other server already left.
func (g *GroupSweeper) recheck(ctx context.Context, chatID int64, now time.Time) bool {
	store, account := g.storeFor()
	if store == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()

	at, found, err := store.LastActive(ctx, account, chatID)
	if err != nil {
		g.log.Warn("couldn't confirm idle time, not leaving this cycle", "chat_id", chatID, "err", err)
		return false
	}
	if !found {
		g.forgetLocal(chatID)
		return false
	}
	if now.Sub(at) < g.window {
		g.mu.Lock()
		if cur, ok := g.last[chatID]; !ok || at.After(cur) {
			g.last[chatID] = at
		}
		g.mu.Unlock()
		return false
	}
	return true
}

// SweepOnce leaves every tracked chat that's been quiet longer than the
// window and isn't currently in a call.
func (g *GroupSweeper) SweepOnce(ctx context.Context) {
	now := time.Now()
	g.mu.Lock()
	var stale []int64
	for chatID, last := range g.last {
		if g.keep[chatID] {
			continue
		}
		if now.Sub(last) >= g.window {
			stale = append(stale, chatID)
		}
	}
	g.mu.Unlock()

	for _, chatID := range stale {
		// Re-check liveness outside the lock: a chat that started playing
		// again between the scan and here must not be yanked out of a
		// live call.
		if g.active != nil && g.active(chatID) {
			g.Touch(chatID)
			continue
		}
		if !g.recheck(ctx, chatID, now) {
			continue
		}
		if err := g.leave(ctx, chatID); err != nil {
			g.log.Warn("idle group leave failed", "chat_id", chatID, "err", err)
			// Still forget it — a chat we can't leave (already removed,
			// deleted, banned) shouldn't be retried every cycle forever.
		} else {
			g.log.Info("left idle group", "chat_id", chatID, "quiet_for", g.window)
		}
		g.Forget(chatID)
	}
}

// Run sweeps on interval until ctx is cancelled.
func (g *GroupSweeper) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.SweepOnce(ctx)
		}
	}
}
