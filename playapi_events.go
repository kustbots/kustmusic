package main

import (
	"encoding/json"
	"net/http"
)

// playAPIEvent mirrors the JSON play-api POSTs to /playapi-events — see
// streaming-core... no, playapi/internal/notify's Notifier.Send. Same shape
// as what used to be DMed to the bot account as a Telegram message (never
// actually read by anything on this side, which is why auto-skip silently
// never worked): {"event": "...", "chatid": N, "bot": N}.
type playAPIEvent struct {
	Event  string `json:"event"`
	ChatID int64  `json:"chatid"`
	Bot    int64  `json:"bot"`
}

// handlePlayAPIEvent receives play-api's direct HTTP notification (see
// playapi/internal/notify) instead of the old Telegram-DM relay, and reacts
// to it — this is the actual auto-skip fix: a "stream-ended" event now
// really does advance the queue and play the next song, whereas before,
// nothing on this side was listening for the DM at all.
func (a *App) handlePlayAPIEvent(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var ev playAPIEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				a.log.Error("panic handling playapi event", "recover", rec)
			}
		}()
		switch ev.Event {
		case "stream-ended":
			a.clones.appFor(ev.ChatID, a).onStreamEnded(ev.ChatID)
		case "left-idle":
			a.clones.appFor(ev.ChatID, a).onLeftIdle(ev.ChatID)
		}
	}()
}

// onStreamEnded is the real auto-skip: the song that was playing finished
// naturally (not via /skip or /stop), so advance to whatever's next in the
// queue — or, if the queue is now empty, leave the voice chat immediately.
//
// The immediate leave lives here rather than on a play-api timer because
// the queue lives here: play-api can't tell "finished, nothing follows"
// apart from "finished, next track loading" on its own, so it used to just
// wait out a fixed idle timeout and leave late.
func (a *App) onStreamEnded(chatID int64) {
	a.progress.Stop(chatID)
	next, hasNext := a.queue.Advance(chatID)
	if !hasNext {
		a.queue.Clear(chatID)
		server := a.router.ServerFor(chatID, false)
		if err := a.router.Stop(server, chatID); err != nil {
			a.log.Warn("leave-on-empty-queue failed", "chat_id", chatID, "err", err)
		}
		// Say so, rather than just going quiet. Leaving the voice chat
		// with no message at all read as the bot dying mid-session.
		_, _ = a.tg.SendMessage(chatID, queueEndedText, queueEndedKeyboard())
		return
	}
	processing, _ := a.tg.SendMessage(chatID, processingSnowflake, nil)
	a.startPlayback(chatID, processing, next)
}

// onLeftIdle mirrors the Python bot's stream_end_handler follow-up: once
// play-api's own idle timer actually leaves the call, clear this chat's
// queue too so a stale queue doesn't silently resume on some unrelated
// future /play.
//
// The chat is only told when there was actually a session to end. play-api
// schedules its idle timer off any stream-end — including one that fires
// immediately because a song never really produced audio — so notifying
// unconditionally meant chats got a "left due to inactivity" message for a
// call they'd effectively never been in, right after a failed /play.
func (a *App) onLeftIdle(chatID int64) {
	hadSession := len(a.queue.List(chatID)) > 0
	a.queue.Clear(chatID)
	if hadSession {
		_, _ = a.tg.SendMessage(chatID, "👋 Nothing playing for a while, so I headed out. <code>/play</code> whenever you want me back.", nil)
	}
}
