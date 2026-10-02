package main

import (
	"fmt"
	"time"

	"github.com/kustbots/kustmusic/internal/telegram"
)

// Playback controls are admin-only. Anyone can queue a song; only admins
// can take one away.
//
// Without this, any member could pause, skip, or stop the music mid-song
// for everyone else in the group — which in a busy chat is less a feature
// than a way to annoy people, and there's no way to tell from the outside
// whether the bot broke or someone just pressed a button. /play stays open
// to everyone: adding to the queue doesn't take anything from anybody.
const controlDeniedText = "🔒 Only admins can control playback. You can still queue songs with /play."

// adminCacheTTL is how long an admin lookup is trusted.
//
// Every control press was costing a getChatMember, and those were measured
// at ~1.4s each against a busy Telegram — so tapping pause twice cost three
// seconds of API calls and made the bot feel dead. Admin rights change
// rarely; a few minutes of staleness is a fine trade for not calling
// Telegram on every button.
const adminCacheTTL = 3 * time.Minute

type adminVerdict struct {
	isAdmin bool
	expires time.Time
}

// mayControlPlayback reports whether user is allowed to pause/resume/skip/
// stop in chatID.
//
// Private chats are always allowed — there are no admins in a DM, and the
// only person who could be interrupted is the one pressing the button.
func (a *App) mayControlPlayback(chatID int64, user *telegram.User) bool {
	if chatID >= 0 {
		return true
	}
	if user == nil {
		return false
	}

	key := fmt.Sprintf("%d:%d", chatID, user.ID)
	a.adminMu.Lock()
	v, cached := a.adminCache[key]
	a.adminMu.Unlock()
	if cached && time.Now().Before(v.expires) {
		return v.isAdmin
	}

	ok, err := a.tg.IsAdmin(chatID, user.ID)
	if err != nil {
		// Don't lock a group out of its own controls because one
		// getChatMember call failed. Log it and let the action through —
		// the failure mode of guessing "not admin" here is worse than the
		// occasional unauthorised skip.
		a.log.Warn("couldn't check admin status for a playback control",
			"chat_id", chatID, "user_id", user.ID, "err", err)
		// Cache the fail-open answer briefly too. Without it, a spell of
		// Telegram timeouts meant every press retried and waited out the
		// full client timeout again.
		a.rememberAdmin(key, true, unknownTTL)
		return true
	}
	a.rememberAdmin(key, ok, adminCacheTTL)
	return ok
}

func (a *App) rememberAdmin(key string, isAdmin bool, ttl time.Duration) {
	a.adminMu.Lock()
	a.adminCache[key] = adminVerdict{isAdmin: isAdmin, expires: time.Now().Add(ttl)}
	a.adminMu.Unlock()
}

// requireControlAdminCmd gates a typed command, replying in the chat when
// the sender isn't allowed.
func (a *App) requireControlAdminCmd(m *telegram.Message) bool {
	if a.mayControlPlayback(m.Chat.ID, m.From) {
		return true
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, controlDeniedText, nil)
	return false
}

// requireControlAdminCallback gates a button press. The refusal goes back
// as the callback's own answer — a toast only the person who tapped sees —
// rather than a message in the chat, so a stray tap doesn't spam everyone.
func (a *App) requireControlAdminCallback(cq *telegram.CallbackQuery) bool {
	if a.mayControlPlayback(cq.Message.Chat.ID, &cq.From) {
		return true
	}
	_ = a.tg.AnswerCallbackQuery(cq.ID, "Only admins can control playback — but /play is all yours.")
	return false
}
