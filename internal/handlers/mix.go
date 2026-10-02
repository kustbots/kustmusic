/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package handlers

import (
	"context"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/sources"
	"github.com/kustbots/kustmusic/internal/utils"
	"time"

	td "github.com/AshokShau/gotdbot"
)

// mixHandler handles the /mix command.
func mixHandler(c *td.Client, m *td.Message) error {
	if !playMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId

	if queueLen := cache.ChatCache.GetQueueLength(chatID); queueLen >= 10 {
		_, _ = m.ReplyText(c, utils.Notice("📛 Queue is full", "Maximum 10 tracks. Use /end to clear it."), nil)
		return td.EndGroups
	}

	isReply := m.ReplyToMessageID() != 0
	args := Args(m)
	url := getUrl(c, m, isReply)

	if isReply && args == "" && url == "" {
		r, err := m.GetRepliedMessage(c)
		if err == nil && r != nil {
			args = r.Text()
		}
	}

	input := coalesce(url, args)
	var seedTrackID string

	if input == "" {
		playing := cache.ChatCache.GetPlayingTrack(chatID)
		if playing == nil {
			_, _ = m.ReplyText(c, "<b>Usage:</b>\n/mix [song or URL]\n\nOr run /mix while a song is currently playing to create a mix based on it.", &td.SendTextMessageOpts{ParseMode: "HTML"})
			return td.EndGroups
		}
		seedTrackID = playing.TrackID
	}

	updater, err := m.ReplyText(c, utils.Notice("🔀 Building your mix…", "Picking related tracks."), nil)
	if err != nil {
		c.Logger.Warn("failed to send message", "error", err)
		return td.EndGroups
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limit := int(config.AutoPlayLimit)
	tracks, err := sources.GetYouTubeMix(ctx, input, seedTrackID, limit)
	if err != nil {
		_, err = updater.EditText(c, utils.Notice("❌ Couldn't build the mix", utils.FriendlyError(err)), nil)
		return err
	}

	if len(tracks) == 0 {
		_, err = updater.EditText(c, "No tracks found for mix.", nil)
		return err
	}

	var filtered []utils.GetUrlTrack
	for _, track := range tracks {
		if _track := cache.ChatCache.GetTrackIfExists(chatID, track.Id); _track == nil {
			filtered = append(filtered, track)
		}
	}

	if len(filtered) == 0 {
		_, err = updater.EditText(c, "All mix tracks are already in the queue or playing.", nil)
		return err
	}

	return handleMultipleTracks(c, m, updater, filtered, chatID, false, false)
}
