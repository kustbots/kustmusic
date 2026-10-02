package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/kustbots/kustmusic/internal/queue"
	"github.com/kustbots/kustmusic/internal/store"
	"github.com/kustbots/kustmusic/internal/telegram"
)

const playlistPageSize = 10

func songFromPlaylistEntry(e *store.PlaylistEntry) queue.Song {
	return queue.Song{Title: e.SongTitle, URL: e.URL, Duration: e.Duration}
}

func (a *App) cmdPlaylist(m *telegram.Message) {
	if m.From == nil {
		return
	}
	a.sendPlaylistPage(m.Chat.ID, 0, m.From.ID)
}

func (a *App) cbAddToPlaylist(cq *telegram.CallbackQuery) {
	if a.db == nil {
		return
	}
	chatID := cq.Message.Chat.ID
	current, ok := a.queue.Current(chatID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	added, err := a.db.AddToPlaylist(ctx, store.PlaylistEntry{
		ChatID: chatID, UserID: cq.From.ID,
		SongTitle: current.Title, URL: current.URL, Duration: current.Duration,
	})
	if err != nil {
		a.log.Warn("add to playlist failed", "err", err)
		return
	}
	if !added {
		return // already saved — matches Python's silent-ish "already in playlist" case, surfaced via answerCallbackQuery text instead of a new message
	}
}

func (a *App) sendPlaylistPage(chatID int64, page int, userID int64) {
	if a.db == nil {
		_, _ = a.tg.SendMessage(chatID, "💾 Can't reach the database right now, so playlists are off. Try again in a bit.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entries, err := a.db.ListPlaylist(ctx, userID)
	if err != nil {
		a.log.Warn("playlist load failed", "user_id", userID, "err", err)
		_, _ = a.tg.SendMessage(chatID, "💾 Couldn't pull up your playlist just now. Give it another go?", nil)
		return
	}
	if len(entries) == 0 {
		_, _ = a.tg.SendMessage(chatID, "📭 Your playlist's empty. Hit <b>✨ Add to Playlist</b> on any player card to start one.", nil)
		return
	}

	start := page * playlistPageSize
	if start >= len(entries) {
		start = 0
		page = 0
	}
	end := start + playlistPageSize
	if end > len(entries) {
		end = len(entries)
	}

	var kb telegram.InlineKeyboard
	for i, e := range entries[start:end] {
		kb = append(kb, []telegram.InlineButton{
			{Text: fmt.Sprintf("%d. %s", start+i+1, e.SongTitle), CallbackData: "playlist_detail|" + e.ID.Hex()},
		})
	}
	var nav []telegram.InlineButton
	if page > 0 {
		nav = append(nav, telegram.InlineButton{Text: "⬅️ Prev", CallbackData: fmt.Sprintf("playlist_page|%d", page-1)})
	}
	if end < len(entries) {
		nav = append(nav, telegram.InlineButton{Text: "Next ➡️", CallbackData: fmt.Sprintf("playlist_page|%d", page+1)})
	}
	if len(nav) > 0 {
		kb = append(kb, nav)
	}
	_, _ = a.tg.SendMessage(chatID, "🎶 <b>Your Playlist</b>", kb)
}

func (a *App) cbPlaylistPage(cq *telegram.CallbackQuery) {
	parts := strings.SplitN(cq.Data, "|", 2)
	page := 0
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &page)
	}
	a.sendPlaylistPage(cq.Message.Chat.ID, page, cq.From.ID)
}

func (a *App) cbPlaylistDetail(cq *telegram.CallbackQuery) {
	if a.db == nil {
		return
	}
	parts := strings.SplitN(cq.Data, "|", 2)
	if len(parts) != 2 {
		return
	}
	id, err := primitive.ObjectIDFromHex(parts[1])
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	entry, err := a.db.GetPlaylistEntry(ctx, id)
	if err != nil {
		_, _ = a.tg.SendMessage(cq.Message.Chat.ID, "🤷 That song's not in your playlist any more.", nil)
		return
	}
	text := fmt.Sprintf("<b>Title:</b> %s\n<b>Duration:</b> %s\n<b>URL:</b> %s",
		htmlEscape(entry.SongTitle), htmlEscape(entry.Duration), htmlEscape(entry.URL))
	kb := telegram.InlineKeyboard{
		{
			{Text: "▶️ Play This Song", CallbackData: "play_song|" + parts[1]},
			{Text: "🗑 Remove", CallbackData: "remove_from_playlist|" + parts[1]},
		},
		{{Text: "⬅️ Back to Playlist", CallbackData: "playlist_page|0"}},
	}
	_, _ = a.tg.SendMessage(cq.Message.Chat.ID, text, kb)
}

func (a *App) cbPlaySong(cq *telegram.CallbackQuery) {
	if a.db == nil {
		return
	}
	parts := strings.SplitN(cq.Data, "|", 2)
	if len(parts) != 2 {
		return
	}
	id, err := primitive.ObjectIDFromHex(parts[1])
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	entry, err := a.db.GetPlaylistEntry(ctx, id)
	cancel()
	if err != nil {
		_, _ = a.tg.SendMessage(cq.Message.Chat.ID, "🤷 That song's not in your playlist any more.", nil)
		return
	}

	chatID := cq.Message.Chat.ID
	song := songFromPlaylistEntry(entry)
	song.RequesterID = cq.From.ID
	song.RequesterName = cq.From.FirstName
	position := a.queue.Push(chatID, song)
	if position > 1 {
		a.precache.Warm(song.URL) // same reason as /play — see cmdPlay
		_, _ = a.tg.SendMessage(chatID, fmt.Sprintf("➕ Added to queue (#%d): <b>%s</b>", position, htmlEscape(song.Title)), nil)
		return
	}
	processing, _ := a.tg.SendMessage(chatID, fmt.Sprintf("▶️ Playing <b>%s</b> from your playlist...", htmlEscape(song.Title)), nil)
	a.startPlayback(chatID, processing, song)
}

func (a *App) cbRemoveFromPlaylist(cq *telegram.CallbackQuery) {
	if a.db == nil {
		return
	}
	parts := strings.SplitN(cq.Data, "|", 2)
	if len(parts) != 2 {
		return
	}
	id, err := primitive.ObjectIDFromHex(parts[1])
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := a.db.RemoveFromPlaylist(ctx, id); err != nil {
		_, _ = a.tg.SendMessage(cq.Message.Chat.ID, "🤷 That one wouldn't budge. Try again?", nil)
		return
	}
	a.sendPlaylistPage(cq.Message.Chat.ID, 0, cq.From.ID)
}
