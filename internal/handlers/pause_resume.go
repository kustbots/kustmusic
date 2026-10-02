/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package handlers

import (
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/player"
	"github.com/kustbots/kustmusic/internal/utils"
	"html"

	td "github.com/AshokShau/gotdbot"
)

func pauseHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId

	if !cache.ChatCache.IsActive(chatID) {
		_, _ = m.ReplyText(c, utils.Notice("📵 Nothing is playing", ""), nil)
		return nil
	}

	if _, err := player.Calls.Pause(chatID); err != nil {
		_, _ = m.ReplyText(c, utils.Notice("❌ Couldn't pause", utils.FriendlyError(err)), nil)
		return nil
	}

	_, err := m.ReplyText(c, utils.Notice("⏸ Paused", "By "+html.EscapeString(firstName(c, m))), &td.SendTextMessageOpts{ReplyMarkup: utils.ControlButtons("pause")})
	return err
}

func resumeHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId

	if chatID > 0 {
		_, _ = m.ReplyText(c, "This command can only be used in a supergroup.", nil)
		return nil
	}

	if !cache.ChatCache.IsActive(chatID) {
		_, _ = m.ReplyText(c, utils.Notice("📵 Nothing is playing", ""), nil)
		return nil
	}

	if _, err := player.Calls.Resume(chatID); err != nil {
		_, _ = m.ReplyText(c, utils.Notice("❌ Couldn't resume", utils.FriendlyError(err)), nil)
		return nil
	}

	_, err := m.ReplyText(c, utils.Notice("▶️ Resumed", "By "+html.EscapeString(firstName(c, m))), &td.SendTextMessageOpts{ReplyMarkup: utils.ControlButtons("resume")})
	return err
}
