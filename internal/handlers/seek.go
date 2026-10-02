/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package handlers

import (
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/player"
	"github.com/kustbots/kustmusic/internal/utils"
	"strconv"

	td "github.com/kustbots/gotdbot"
)

func seekHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}
	chatID := m.ChatId

	if !cache.ChatCache.IsActive(chatID) {
		_, err := m.ReplyText(c, "The bot is not streaming in the video chat.", nil)
		return err
	}

	playingSong := cache.ChatCache.GetPlayingTrack(chatID)
	if playingSong == nil {
		_, err := m.ReplyText(c, "The bot is not streaming in the video chat.", nil)
		return err
	}

	args := Args(m)
	if args == "" {
		_, _ = m.ReplyText(c, "<b>Usage:</b> /seek duration\n<b>Example:</b> <code>/seek 15</code>", replyOpts)
		return nil
	}

	seekTime, err := strconv.Atoi(args)
	if err != nil {
		_, _ = m.ReplyText(c, "Invalid seek time provided. Please use a valid number of seconds.", nil)
		return nil
	}

	if seekTime < 10 {
		_, _ = m.ReplyText(c, "Minimum seek time is 10 seconds.", nil)
		return nil
	}

	currDur, err := player.Calls.PlayedTime(chatID)
	if err != nil {
		_, _ = m.ReplyText(c, "Failed to fetch the duration of the ongoing stream.", nil)
		return nil
	}

	toSeek := int32(currDur) + int32(seekTime)
	if toSeek >= playingSong.Duration {
		_, _ = m.ReplyText(c, fmt.Sprintf("You cannot seek beyond the track duration. Maximum allowed is %s.", utils.SecToMin(playingSong.Duration)), nil)
		return nil
	}

	if err = player.Calls.SeekStream(c, chatID, seekTime); err != nil {
		_, _ = m.ReplyText(c, utils.Notice("❌ Couldn't seek", utils.FriendlyError(err)), replyOpts)
		return nil
	}

	_, _ = m.ReplyText(c, fmt.Sprintf("<b>Stream skipped %s and started from %s seconds by</b> %s", utils.SecToMin(int32(seekTime)), utils.SecToMin(toSeek), firstName(c, m)), replyOpts)
	return nil
}
