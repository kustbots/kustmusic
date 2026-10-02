package vc

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Dynamic wraps a Caller whose underlying implementation can be swapped at
// runtime — lets the HTTP server start immediately against a stub while the
// real assistant connects (with retries) in the background, instead of
// blocking startup or crashing the whole process on a connection failure.
// That crash used to be real: log.Fatalf on a single failed vc.NewAssistant
// call took the whole dyno down, including on transient, self-recovering
// failures — e.g. two play-api instances briefly sharing one Telegram
// account across a redeploy (AUTH_KEY_DUPLICATED), which is expected to
// clear on its own once one side's connection settles.
type Dynamic struct {
	mu            sync.RWMutex
	impl          Caller
	onStreamEndCB func(chatID int64)
	ready         bool
	// reason explains, in words, why this server currently can't stream.
	// It exists so an unhealthy server can say "I'm broken, don't route to
	// me" *and* say why — a bare ready=false tells whoever is picking
	// servers to skip this one, but tells whoever is debugging the fleet
	// nothing about whether it's a dead session, a revoked account, or
	// simply a process that has only just booted.
	reason string
}

func NewDynamic(initial Caller) *Dynamic {
	return &Dynamic{impl: initial, reason: "assistant has not connected yet"}
}

// Set swaps in the real assistant, re-registering any previously-registered
// OnStreamEnd callback so it isn't silently dropped (it was registered
// against whatever implementation was current at the time — the stub,
// before the real assistant connected). Marks this Dynamic ready: only
// after this does /play have something that can genuinely stream audio.
func (d *Dynamic) Set(impl Caller) {
	d.mu.Lock()
	d.impl = impl
	d.ready = true
	d.reason = ""
	cb := d.onStreamEndCB
	d.mu.Unlock()
	if cb != nil {
		impl.OnStreamEnd(cb)
	}
}

// Ready reports whether a real assistant is connected, as opposed to the
// in-memory stub this starts out holding.
//
// This matters because the stub cheerfully reports success: its Play
// returns nil and its PlayedTime returns a growing duration, so the whole
// buffering-confirmation chain passes and /play answers "Playing media"
// while absolutely nothing reaches the voice chat. That's exactly what a
// dead/expired session looked like in production — the bot announced a
// song that was never playing. Callers check this and fail loudly instead.
func (d *Dynamic) Ready() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ready
}

// MarkNotReady drops back to "no real assistant" — used when the connected
// assistant turns out to be dead (e.g. its session was invalidated
// elsewhere) so /play stops claiming success until it reconnects. reason is
// carried through to the fleet's health probe so the failure is visible
// where routing decisions are actually made, rather than only in this
// dyno's logs.
func (d *Dynamic) MarkNotReady(reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ready = false
	if reason != "" {
		d.reason = reason
	}
}

// NotReadyReason returns why this server can't stream right now, or "" when
// it can. Surfaced over /active_calls so the bot — and anyone reading the
// fleet's health at a glance — sees the actual cause (a duplicated session,
// a revoked account) instead of an unexplained gap in capacity.
func (d *Dynamic) NotReadyReason() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.ready {
		return ""
	}
	if d.reason == "" {
		return "assistant not connected"
	}
	return d.reason
}

func (d *Dynamic) current() Caller {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.impl
}

func (d *Dynamic) Play(ctx context.Context, chatID int64, source string) error {
	return d.current().Play(ctx, chatID, source)
}

func (d *Dynamic) PlayedTime(ctx context.Context, chatID int64) (time.Duration, error) {
	return d.current().PlayedTime(ctx, chatID)
}

func (d *Dynamic) Pause(ctx context.Context, chatID int64) error {
	return d.current().Pause(ctx, chatID)
}

func (d *Dynamic) Resume(ctx context.Context, chatID int64) error {
	return d.current().Resume(ctx, chatID)
}

func (d *Dynamic) Stop(ctx context.Context, chatID int64) error {
	return d.current().Stop(ctx, chatID)
}

func (d *Dynamic) Join(ctx context.Context, chatOrInvite string) error {
	return d.current().Join(ctx, chatOrInvite)
}

func (d *Dynamic) LeaveChat(ctx context.Context, chatID int64) error {
	return d.current().LeaveChat(ctx, chatID)
}

func (d *Dynamic) AssistantIdentity(ctx context.Context) (int64, string, error) {
	return d.current().AssistantIdentity(ctx)
}

func (d *Dynamic) ActiveChats() []int64 {
	return d.current().ActiveChats()
}

func (d *Dynamic) OnStreamEnd(cb func(chatID int64)) {
	d.mu.Lock()
	d.onStreamEndCB = cb
	impl := d.impl
	d.mu.Unlock()
	impl.OnStreamEnd(cb)
}

// SendMessage forwards to the current implementation if it supports sending
// (the real assistant does; the stub does not, so this is a safe no-op
// until the real connection lands).
func (d *Dynamic) SendMessage(ctx context.Context, userID int64, text string) error {
	type sender interface {
		SendMessage(ctx context.Context, userID int64, text string) error
	}
	if s, ok := d.current().(sender); ok {
		return s.SendMessage(ctx, userID, text)
	}
	slog.Debug("SendMessage: no real assistant connected yet, dropping", "user_id", userID)
	return nil
}
