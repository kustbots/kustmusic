package main

import (
	"context"
	"time"

	"github.com/kustbots/kustmusic/internal/queue"
	"github.com/kustbots/kustmusic/internal/store"
	"github.com/kustbots/kustmusic/internal/telegram"
)

// Heroku sends SIGTERM and then waits — 30 seconds on the common dyno
// types — before killing the process. Writing a few hundred queues to
// Mongo takes well under a second, so this budget is generous; it exists
// so a hung database can't hold the shutdown open until Heroku kills it
// mid-write.
const shutdownSaveTimeout = 15 * time.Second

// saveQueues writes every live queue to MongoDB. Called when Heroku
// signals a restart.
//
// Deliberately unconditional about ordering: it snapshots first, then
// writes, so a chat that finishes a song during the save isn't lost to a
// half-written state.
func (a *App) saveQueues() {
	if a.db == nil {
		return
	}
	snapshot := a.queue.Snapshot()
	if len(snapshot) == 0 {
		// Still clear the backup — leaving yesterday's queues behind would
		// resurrect them on the next boot.
		ctx, cancel := context.WithTimeout(context.Background(), shutdownSaveTimeout)
		defer cancel()
		_ = a.db.ClearQueues(ctx)
		return
	}

	saved := make([]store.SavedQueue, 0, len(snapshot))
	for chatID, songs := range snapshot {
		entry := store.SavedQueue{ChatID: chatID, Songs: make([]store.SavedSong, 0, len(songs))}
		for _, s := range songs {
			entry.Songs = append(entry.Songs, store.SavedSong{
				Title:         s.Title,
				URL:           s.URL,
				Duration:      s.Duration,
				Thumbnail:     s.Thumbnail,
				Query:         s.Query,
				RequesterID:   s.RequesterID,
				RequesterName: s.RequesterName,
			})
		}
		saved = append(saved, entry)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownSaveTimeout)
	defer cancel()
	if err := a.db.SaveQueues(ctx, saved); err != nil {
		a.log.Error("couldn't save queues before shutdown", "err", err)
		return
	}
	a.log.Info("saved queues before shutdown", "chats", len(saved))
}

// restoreQueues reloads the queues saved by the previous run and picks
// playback back up where it left off.
//
// The backup is cleared immediately after loading, not after playback
// succeeds: if resuming is what's crashing the bot, a backup that survives
// would make it crash again on every boot. Losing one queue beats a
// restart loop.
func (a *App) restoreQueues() {
	if a.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	saved, err := a.db.LoadQueues(ctx)
	if err != nil {
		a.log.Warn("couldn't load saved queues", "err", err)
		return
	}
	if len(saved) == 0 {
		return
	}
	if err := a.db.ClearQueues(ctx); err != nil {
		a.log.Warn("couldn't clear the queue backup after loading", "err", err)
	}

	restored := make(map[int64][]queue.Song, len(saved))
	for _, entry := range saved {
		songs := make([]queue.Song, 0, len(entry.Songs))
		for _, s := range entry.Songs {
			songs = append(songs, queue.Song{
				Title:         s.Title,
				URL:           s.URL,
				Duration:      s.Duration,
				Thumbnail:     s.Thumbnail,
				Query:         s.Query,
				RequesterID:   s.RequesterID,
				RequesterName: s.RequesterName,
			})
		}
		if len(songs) > 0 {
			restored[entry.ChatID] = songs
		}
	}
	if len(restored) == 0 {
		return
	}
	a.queue.Restore(restored)
	a.log.Info("restored queues from the previous run", "chats", len(restored))

	// Resume in parallel, but bounded. Serially, the last group in a busy
	// fleet would wait minutes for its music back — a fresh voice-chat
	// join can take most of a minute on its own. All at once, though, a
	// restart during peak hours would fire every queued chat at the
	// play-api fleet simultaneously and turn a routine deploy into a
	// self-inflicted load spike.
	go func() {
		slots := make(chan struct{}, maxConcurrentResumes)
		for chatID, songs := range restored {
			slots <- struct{}{}
			go func(chatID int64, song queue.Song) {
				defer func() { <-slots }()
				a.resumeChat(chatID, song)
			}(chatID, songs[0])
		}
	}()
}

// maxConcurrentResumes caps how many chats are brought back at once after
// a restart. Five keeps every play-api server busy without any of them
// queueing, and keeps the burst of "back online" messages inside
// Telegram's rate limits.
const maxConcurrentResumes = 5

// resumeChat tells one chat its music is coming back, then restarts it.
func (a *App) resumeChat(chatID int64, song queue.Song) {
	defer func() {
		if rec := recover(); rec != nil {
			a.log.Error("panic resuming a chat after restart", "chat_id", chatID, "recover", rec)
		}
	}()

	notice, err := a.tg.SendMessage(chatID,
		"🔄 <b>ʙᴀᴄᴋ ᴏɴʟɪɴᴇ</b>\n\nHad to restart — picking your queue back up where we left off.", nil)
	if err != nil {
		if telegram.IsTransportError(err) {
			// The network was unreachable, which says nothing at all about
			// this chat. Keep the queue and resume anyway — treating this
			// as "chat is gone" once wiped seven groups' queues during a
			// spell of dial timeouts, for chats that were all fine.
			a.log.Warn("couldn't announce resume, network problem — resuming anyway",
				"chat_id", chatID, "err", err)
			notice = nil
		} else {
			// Telegram itself said no: kicked, deleted, blocked. That is
			// about this chat, and the queue really is dead.
			a.log.Info("skipping resume, chat is gone", "chat_id", chatID, "err", err)
			a.queue.Clear(chatID)
			return
		}
	}
	a.startPlayback(chatID, notice, song)
}
