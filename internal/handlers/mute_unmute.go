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
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/player"
	"github.com/kustbots/kustmusic/internal/utils"
	"html"

	td "github.com/kustbots/gotdbot"
)

func muteHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	if args := Args(m); args != "" {
		return td.EndGroups
	}

	chatID := m.ChatId
	if !cache.ChatCache.IsActive(chatID) {
		_, err := m.ReplyText(c, utils.Notice("📵 Nothing is playing", ""), nil)
		return err
	}

	if _, err := player.Calls.Mute(chatID); err != nil {
		_, err = m.ReplyText(c, utils.Notice("❌ Couldn't mute", utils.FriendlyError(err)), nil)
		return err
	}

	_, err := m.ReplyText(c, utils.Notice("🔇 Muted", "By "+html.EscapeString(firstName(c, m))), &td.SendTextMessageOpts{ReplyMarkup: utils.ControlButtons("mute")})
	return err
}

func unmuteHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	if args := Args(m); args != "" {
		return td.EndGroups
	}

	chatID := m.ChatId
	if !cache.ChatCache.IsActive(chatID) {
		_, err := m.ReplyText(c, utils.Notice("📵 Nothing is playing", ""), nil)
		return err
	}

	if _, err := player.Calls.Unmute(chatID); err != nil {
		_, err = m.ReplyText(c, utils.Notice("❌ Couldn't unmute", utils.FriendlyError(err)), nil)
		return err
	}

	_, err := m.ReplyText(c, utils.Notice("🔊 Unmuted", "By "+html.EscapeString(firstName(c, m))), &td.SendTextMessageOpts{ReplyMarkup: utils.ControlButtons("unmute")})
	return err
}
