// Package vc wraps the assistant's voice-call connection (gogram MTProto +
// NTgCalls cgo binding). This file defines the interface the rest of the
// service programs against; the real implementation (assistant login,
// group-call join/stream state machine, cgo NTgCalls calls) lands in
// client.go once the native library wiring is in place — that's the single
// highest-risk piece of this whole rewrite (see the project plan) and is
// deliberately isolated behind this interface so everything else (HTTP
// routes, idle-leave, the streaming-core engine) can be built and tested
// against a stub implementation first.
package vc

import (
	"context"
	"time"
)

// Caller is the real surface play-api needs from the voice-call layer. It
// satisfies streaming-core/playback.VoiceCaller plus the extra ops play-api's
// HTTP routes expose directly (pause/resume/stop/join).
type Caller interface {
	Play(ctx context.Context, chatID int64, source string) error
	PlayedTime(ctx context.Context, chatID int64) (time.Duration, error)
	Pause(ctx context.Context, chatID int64) error
	Resume(ctx context.Context, chatID int64) error
	Stop(ctx context.Context, chatID int64) error
	Join(ctx context.Context, chatOrInvite string) error
	// LeaveChat leaves the group entirely, not just its voice call — used
	// by the idle-group sweeper to reclaim Telegram's per-account group
	// limit for chats that have gone quiet.
	LeaveChat(ctx context.Context, chatID int64) error
	AssistantIdentity(ctx context.Context) (id int64, username string, err error)
	ActiveChats() []int64

	// OnStreamEnd registers a callback fired when a call's stream naturally
	// ends (no more data). Exactly one handler at a time; the real
	// implementation should call this once, at startup.
	OnStreamEnd(cb func(chatID int64))
}
