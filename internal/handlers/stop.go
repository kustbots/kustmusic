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
	"html"

	td "github.com/kustbots/gotdbot"
)

func stopHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId
	if !cache.ChatCache.IsActive(chatID) {
		_, _ = m.ReplyText(c, "<b>📵 Nothing is playing</b>", nil)
		return nil
	}

	_ = player.Calls.Stop(chatID, false)
	_, _ = m.ReplyText(c, fmt.Sprintf("<b>⏹ Stream ended</b>\n<blockquote>By %s</blockquote>", html.EscapeString(firstName(c, m))), replyOpts)
	return nil
}
