package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/kustbots/kustmusic/internal/telegram"
)

// broadcastState is a direct port of main-music-rx's _broadcast_state: one
// broadcast runs at a time, sent with a small delay between each chat to
// stay under Telegram's flood limits, checkpointed periodically so a crash
// loses at most a handful of chats' progress.
type broadcastState struct {
	mu         sync.Mutex
	active     bool
	remaining  []int64
	kind       string // "text" or "copy"
	fromChatID int64
	messageID  int
	text       string
	total      int
	sent       int
	failed     int
	startedBy  int64
	cancelCh   chan struct{}
}

func newBroadcastState() *broadcastState {
	return &broadcastState{}
}

func (a *App) cmdBroadcast(m *telegram.Message, args string) {
	if m.From == nil || m.From.ID != a.ownerID {
		return
	}
	b := a.broadcast
	b.mu.Lock()
	if b.active {
		remaining := len(b.remaining)
		b.mu.Unlock()
		_, _ = a.tg.SendMessage(m.Chat.ID, fmt.Sprintf("⚠️ A broadcast is already running (%d remaining). Use /bcancel to stop it first.", remaining), nil)
		return
	}
	b.mu.Unlock()

	var kind, text string
	var fromChatID int64
	var messageID int
	args = strings.TrimSpace(args)
	if m.ReplyToMessage != nil {
		kind = "copy"
		fromChatID = m.Chat.ID
		messageID = m.ReplyToMessage.MessageID
	} else if args != "" {
		kind = "text"
		text = args
	} else {
		_, _ = a.tg.SendMessage(m.Chat.ID, "⚠️ Usage: reply to a message with /broadcast, or /broadcast <text>", nil)
		return
	}

	if a.db == nil {
		_, _ = a.tg.SendMessage(m.Chat.ID, "💾 No database connection, so no broadcast. Try again shortly.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	chatIDs, err := a.db.ListBroadcastChats(ctx)
	cancel()
	if err != nil {
		a.log.Error("broadcast chat list failed", "err", err)
		_, _ = a.tg.SendMessage(m.Chat.ID, "💾 Couldn't load the chat list. Check the logs for what went wrong.", nil)
		return
	}
	if len(chatIDs) == 0 {
		_, _ = a.tg.SendMessage(m.Chat.ID, "📭 No chats on file to broadcast to yet.", nil)
		return
	}

	b.mu.Lock()
	b.active = true
	b.remaining = chatIDs
	b.kind = kind
	b.fromChatID = fromChatID
	b.messageID = messageID
	b.text = text
	b.total = len(chatIDs)
	b.sent = 0
	b.failed = 0
	b.startedBy = m.From.ID
	b.cancelCh = make(chan struct{})
	b.mu.Unlock()

	go a.broadcastWorker()
	_, _ = a.tg.SendMessage(m.Chat.ID, fmt.Sprintf("📣 Broadcast started — %d chats queued.", len(chatIDs)), nil)
}

func (a *App) broadcastWorker() {
	b := a.broadcast
	for {
		b.mu.Lock()
		if len(b.remaining) == 0 {
			b.mu.Unlock()
			break
		}
		select {
		case <-b.cancelCh:
			b.mu.Unlock()
			goto done
		default:
		}
		chatID := b.remaining[0]
		kind, fromChatID, messageID, text := b.kind, b.fromChatID, b.messageID, b.text
		b.mu.Unlock()

		var err error
		if kind == "copy" {
			err = a.tg.ForwardMessage(chatID, fromChatID, messageID)
		} else {
			_, err = a.tg.SendMessage(chatID, text, nil)
		}

		b.mu.Lock()
		if err != nil {
			b.failed++
			if a.db != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = a.db.RemoveBroadcastChat(ctx, chatID)
				cancel()
			}
		} else {
			b.sent++
		}
		b.remaining = b.remaining[1:]
		checkpoint := (b.sent+b.failed)%25 == 0
		state := bson.M{
			"active": b.active, "remaining": b.remaining, "kind": b.kind,
			"total": b.total, "sent": b.sent, "failed": b.failed, "started_by": b.startedBy,
		}
		b.mu.Unlock()

		if checkpoint && a.db != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = a.db.PersistBroadcastState(ctx, state)
			cancel()
		}

		time.Sleep(350 * time.Millisecond)
	}
done:
	b.mu.Lock()
	b.active = false
	sent, total, failed, startedBy := b.sent, b.total, b.failed, b.startedBy
	b.mu.Unlock()

	if a.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.db.ClearBroadcastState(ctx)
		cancel()
	}
	_, _ = a.tg.SendMessage(startedBy, fmt.Sprintf("✅ Broadcast finished.\nSent: %d/%d\nFailed (removed from list): %d", sent, total, failed), nil)
}

func (a *App) cmdBroadcastStatus(m *telegram.Message) {
	if m.From == nil || m.From.ID != a.ownerID {
		return
	}
	b := a.broadcast
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		_, _ = a.tg.SendMessage(m.Chat.ID, "ℹ️ No broadcast currently running.", nil)
		return
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, fmt.Sprintf("📣 <b>Broadcast in progress</b>\nSent: %d | Failed: %d | Remaining: %d | Total: %d",
		b.sent, b.failed, len(b.remaining), b.total), nil)
}

func (a *App) cmdBroadcastCancel(m *telegram.Message) {
	if m.From == nil || m.From.ID != a.ownerID {
		return
	}
	b := a.broadcast
	b.mu.Lock()
	if !b.active {
		b.mu.Unlock()
		_, _ = a.tg.SendMessage(m.Chat.ID, "ℹ️ No broadcast currently running.", nil)
		return
	}
	close(b.cancelCh)
	b.mu.Unlock()
	_, _ = a.tg.SendMessage(m.Chat.ID, "🛑 Broadcast cancelled.", nil)
}
