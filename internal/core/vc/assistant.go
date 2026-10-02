// Package vc is the actual voice-call join/stream implementation, combining
// gogram (MTProto — talks to Telegram) with the ntgcalls cgo binding (talks
// to the native WebRTC engine). This is the single highest-risk file in the
// whole rewrite: it cannot be compiled in this environment (no C compiler
// available) and cannot be behaviorally verified without a real deployment
// and a human listening in a live voice chat. Every other package in this
// project has been built against real APIs and verified; this one is built
// directly against gogram's and ntgcalls' real, confirmed function
// signatures (not guessed), but its first real test is the next deploy.
package vc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"

	"github.com/kustbots/kustmusic/internal/core/vc/ntgcalls"
	_ "github.com/kustbots/kustmusic/internal/core/vc/ntgcallscompat" // links the glibc __dn_expand/__res_nquery shim libntgcalls.a needs
)

// connectWaitTimeout bounds how long connectCall waits for the
// connection-change callback to report the call actually ready for media,
// after ntg_connect's own async completion. The reference implementation
// uses 25s (AshokShau/TgMusicBot, src/vc/assistant.go) — too tight in
// practice: a fresh join's real WebRTC handshake can legitimately take
// longer than that, and 25s isn't a sign the connection has failed, just
// that it hasn't finished yet. Now that the HTTP layer sends keep-alive
// pings while waiting (see httpapi/play.go), there's no Heroku router
// deadline forcing an artificial cutoff here — this is a real safety net
// against ntgcalls never signaling at all (its promise callbacks are not
// guaranteed to fire, confirmed empirically elsewhere in this codebase),
// not a target duration. Configurable via CONNECT_WAIT_SECONDS for
// operational tuning without a redeploy.
var connectWaitTimeout = connectWaitTimeoutFromEnv()

func connectWaitTimeoutFromEnv() time.Duration {
	const defaultSeconds = 90
	if v := os.Getenv("CONNECT_WAIT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultSeconds * time.Second
}

// Assistant wraps one gogram user-account client plus one native NTgCalls
// instance, implementing everything both playapi and the bot's local
// playback mode need from a voice-call layer.
type Assistant struct {
	Client *telegram.Client
	ntg    *ntgcalls.Client

	mu          sync.Mutex
	calls       map[int64]telegram.InputGroupCall // chatID -> the call we joined, for LeaveGroupCall/participant edits
	waitConnect map[int64]chan error              // chatID -> pending connectCall waiting on onConnectionChange
	dropped     map[int64]bool                    // chatID -> its call died; the calls entry is stale, rejoin before reusing

	onEnd func(chatID int64)
}

// NewAssistant logs in with sessionString (a Pyrogram-format string session —
// see cmd/sessiontest for the verified decode path) and starts the native
// call engine.
func NewAssistant(ctx context.Context, apiID int32, apiHash, sessionString string) (*Assistant, error) {
	sess, err := DecodePyrogramSessionString(sessionString)
	if err != nil {
		return nil, fmt.Errorf("vc: decode session: %w", err)
	}

	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:         apiID,
		AppHash:       apiHash,
		StringSession: sess.Encode(),
		MemorySession: true,
	})
	if err != nil {
		return nil, fmt.Errorf("vc: create telegram client: %w", err)
	}
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("vc: connect: %w", err)
	}
	me, err := client.GetMe()
	if err != nil {
		return nil, fmt.Errorf("vc: get me: %w", err)
	}
	if me.Bot {
		return nil, fmt.Errorf("vc: session %q resolves to a bot account, need a real user account", me.Username)
	}

	ntg, err := ntgcalls.NewClient()
	if err != nil {
		return nil, fmt.Errorf("vc: init native call engine: %w", err)
	}

	a := &Assistant{
		Client:      client,
		ntg:         ntg,
		calls:       make(map[int64]telegram.InputGroupCall),
		waitConnect: make(map[int64]chan error),
		dropped:     make(map[int64]bool),
	}

	ntg.OnStreamEnd(func(chatID int64) {
		a.mu.Lock()
		cb := a.onEnd
		a.mu.Unlock()
		if cb != nil {
			cb(chatID)
		}
	})

	ntg.OnConnectionChange(func(chatID int64, state ntgcalls.ConnectionState) {
		var connErr error
		switch state {
		case ntgcalls.StateConnected:
			connErr = nil
		case ntgcalls.StateClosed, ntgcalls.StateFailed:
			connErr = fmt.Errorf("vc: connection failed")
		case ntgcalls.StateTimeout:
			connErr = fmt.Errorf("vc: connection timeout")
		default:
			return // Connecting — not a final state, keep waiting
		}

		a.mu.Lock()
		// A call that died is no longer usable, but its calls entry has to
		// stay: Stop still needs it to leave the Telegram-side call and
		// not linger as a ghost participant. Flagging it instead means
		// Play knows to rejoin rather than re-point a source at a dead
		// connection — which is what happened when the assistant was
		// removed from a group mid-stream: the native engine accepted the
		// new source without complaint and the caller was told the song
		// was playing.
		if connErr != nil {
			a.dropped[chatID] = true
		} else {
			delete(a.dropped, chatID)
		}
		waitCh := a.waitConnect[chatID]
		a.mu.Unlock()

		if waitCh == nil {
			return
		}
		select {
		case waitCh <- connErr:
		default:
		}
	})

	return a, nil
}

func (a *Assistant) OnStreamEnd(cb func(chatID int64)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onEnd = cb
}

// SetDisplayName renames the account, so groups see something that
// explains what just walked in.
//
// The account behind two of the servers was still called "LDS 1 -
// fallback" from whenever it was set up, so Telegram's join notice read
// "LDS 1 - fallback joined the group" — which tells a group nothing except
// that some stranger appeared. It's a no-op when the name already matches,
// so this is safe to call on every connect.
func (a *Assistant) SetDisplayName(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	me, err := a.Client.GetMe()
	if err != nil {
		return err
	}
	if me.FirstName == name && me.LastName == "" {
		return nil
	}
	_, err = a.Client.AccountUpdateProfile(name, "", "")
	return err
}

func (a *Assistant) AssistantIdentity(ctx context.Context) (int64, string, error) {
	me, err := a.Client.GetMe()
	if err != nil {
		return 0, "", err
	}
	return me.ID, me.Username, nil
}

func (a *Assistant) ActiveChats() []int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	chats := make([]int64, 0, len(a.calls))
	for id := range a.calls {
		chats = append(chats, id)
	}
	return chats
}

func (a *Assistant) Join(ctx context.Context, chatOrInvite string) error {
	_, err := a.Client.JoinChannel(chatOrInvite)
	return err
}

// Groups lists every group and channel this account is currently in, as
// bot-API-style chat ids (negative, -100-prefixed for supergroups) so they
// match the ids every other part of this service works with.
//
// The idle-group sweeper needs this. On its own it only knows about chats
// that have played something since the process started, which means a
// restart wiped its view and the long tail of groups the assistant joined
// weeks ago — the ones actually consuming the per-account group limit —
// were never candidates for leaving at all. Seeding from the real dialog
// list is what makes "leave groups nothing has played in" true of the
// account rather than just of this process's memory.
func (a *Assistant) Groups(ctx context.Context) ([]int64, error) {
	dialogs, err := a.Client.GetDialogs(&telegram.DialogOptions{
		Limit:   groupScanLimit,
		Context: ctx,
	})
	if err != nil {
		return nil, fmt.Errorf("vc: list dialogs: %w", err)
	}

	ids := make([]int64, 0, len(dialogs))
	for i := range dialogs {
		d := dialogs[i]
		if d.IsUser() {
			continue // DMs aren't groups and don't count against the limit
		}
		if id := d.GetChannelID(); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// groupScanLimit caps the startup dialog scan. Telegram's per-account
// group limit is in the hundreds, so this covers the whole list with room
// to spare while keeping the scan to a bounded, one-off cost at boot.
const groupScanLimit = 500

// LeaveChat makes the assistant leave chatID's group entirely (not just
// its voice call) — used to reclaim slots against Telegram's per-account
// group limit for chats that have gone quiet. Stops any live call first so
// the assistant never leaves mid-stream.
func (a *Assistant) LeaveChat(ctx context.Context, chatID int64) error {
	_ = a.Stop(ctx, chatID)
	return a.Client.LeaveChannel(chatID)
}

// SendMessage sends a plain-text DM to userID — the pub/sub mechanism
// play-api uses to notify a bot of stream-ended/left-idle events (no
// persistent connection back to the caller otherwise).
func (a *Assistant) SendMessage(ctx context.Context, userID int64, text string) error {
	_, err := a.Client.SendMessage(userID, text)
	return err
}

// connectCall runs the full join handshake for a chat we haven't joined yet.
// Order matters and is copied from the reference implementation
// (AshokShau/TgMusicBot, src/vc/assistant.go connectCall): NTgCalls needs
// SetStreamSources called between ntg_create and ntg_connect, not after —
// calling it once already connected (what this code did originally) got
// "ntg_set_stream_sources ... call rejected synchronously (code -1)" from
// the native engine on a real deploy. The native side apparently needs the
// stream source configured before it finalizes the WebRTC connection, not
// after.
//
// ntg_connect's own async completion is also NOT the real "ready for media"
// signal — see ConnectionState's comment. This function registers a wait
// channel before connecting and blocks on the connection-change callback
// reaching Connected (or a final failure/timeout) afterward, exactly like
// the reference. Skipping this originally meant the caller's own
// buffering-confirmation poll had to cover the same warmup time on top of
// its own window, reliably timing out on every fresh join.
func (a *Assistant) connectCall(chatID int64, source string) error {
	joinParams, err := a.ntg.CreateCall(chatID)
	if err != nil {
		return fmt.Errorf("vc: ntg_create: %w", err)
	}

	if err := a.ntg.SetStreamSources(chatID, source); err != nil {
		_ = a.ntg.Stop(chatID)
		return fmt.Errorf("vc: ntg_set_stream_sources: %w", err)
	}

	call, err := a.Client.GetGroupCall(chatID)
	if err != nil {
		_ = a.ntg.Stop(chatID)
		return fmt.Errorf("vc: no active group call for chat %d: %w", chatID, err)
	}

	waitCh := make(chan error, 1)
	a.mu.Lock()
	a.waitConnect[chatID] = waitCh
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.waitConnect, chatID)
		a.mu.Unlock()
	}()

	updates, err := a.Client.PhoneJoinGroupCall(&telegram.PhoneJoinGroupCallParams{
		Muted:        false,
		VideoStopped: true, // audio-only playback, never send video
		Call:         *call,
		JoinAs:       &telegram.InputPeerSelf{},
		Params:       &telegram.DataJson{Data: joinParams},
	})
	if err != nil {
		_ = a.ntg.Stop(chatID)
		return fmt.Errorf("vc: phone.joinGroupCall: %w", err)
	}

	connParams, err := extractConnectionParams(updates)
	if err != nil {
		_ = a.ntg.Stop(chatID)
		return fmt.Errorf("vc: %w", err)
	}

	if err := a.ntg.Connect(chatID, connParams); err != nil {
		_ = a.ntg.Stop(chatID)
		return fmt.Errorf("vc: ntg_connect: %w", err)
	}

	select {
	case connErr := <-waitCh:
		if connErr != nil {
			_ = a.ntg.Stop(chatID)
			return fmt.Errorf("vc: %w", connErr)
		}
	case <-time.After(connectWaitTimeout):
		_ = a.ntg.Stop(chatID)
		return fmt.Errorf("vc: connection did not become ready within %s", connectWaitTimeout)
	}

	a.mu.Lock()
	a.calls[chatID] = *call
	delete(a.dropped, chatID)
	a.mu.Unlock()
	return nil
}

// extractConnectionParams scans a phone.joinGroupCall response for the
// UpdateGroupCallConnection carrying Telegram's WebRTC answer params.
func extractConnectionParams(updates telegram.Updates) (string, error) {
	var list []telegram.Update
	switch v := updates.(type) {
	case *telegram.UpdatesObj:
		list = v.Updates
	case *telegram.UpdateShort:
		list = []telegram.Update{v.Update}
	default:
		return "", fmt.Errorf("unexpected updates type %T", updates)
	}
	for _, u := range list {
		if conn, ok := u.(*telegram.UpdateGroupCallConnection); ok && conn.Params != nil {
			return conn.Params.Data, nil
		}
	}
	return "", fmt.Errorf("no UpdateGroupCallConnection in phone.joinGroupCall response")
}

// Play starts (or, if this chat's call is already joined, seamlessly
// switches) audio playback. The two paths differ in SetStreamSources
// timing: already-joined just re-sets the source on the live call, but a
// fresh join must set it as part of connectCall's specific handshake order
// (see connectCall's comment).
func (a *Assistant) Play(ctx context.Context, chatID int64, source string) error {
	a.mu.Lock()
	_, already := a.calls[chatID]
	dead := a.dropped[chatID]
	a.mu.Unlock()
	if already && !dead {
		return a.ntg.SetStreamSources(chatID, source)
	}
	if dead {
		// Tear the dead call down before rejoining, so the native engine
		// and the Telegram side both start from a clean state.
		_ = a.Stop(ctx, chatID)
	}
	return a.connectCall(chatID, source)
}

func (a *Assistant) PlayedTime(ctx context.Context, chatID int64) (time.Duration, error) {
	ms, err := a.ntg.Time(chatID)
	if err != nil {
		return 0, err
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func (a *Assistant) Pause(ctx context.Context, chatID int64) error  { return a.ntg.Pause(chatID) }
func (a *Assistant) Resume(ctx context.Context, chatID int64) error { return a.ntg.Resume(chatID) }

func (a *Assistant) Stop(ctx context.Context, chatID int64) error {
	if err := a.ntg.Stop(chatID); err != nil {
		// still try to leave the Telegram-side call even if the native
		// engine stop failed — better to leave a silent/broken call than
		// stay joined forever.
		_ = err
	}
	a.mu.Lock()
	call, ok := a.calls[chatID]
	delete(a.calls, chatID)
	delete(a.dropped, chatID)
	a.mu.Unlock()
	if !ok {
		return nil
	}
	_, err := a.Client.PhoneLeaveGroupCall(call, 0)
	return err
}
