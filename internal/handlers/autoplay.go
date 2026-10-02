/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package handlers

import (
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"

	td "github.com/AshokShau/gotdbot"
)

func autoplayHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId

	if cache.ChatCache.GetPlayingTrack(chatID) == nil {
		_, err := m.ReplyText(c, "Bot is not streaming in the video chat.", nil)
		return err
	}

	state := cache.ChatCache.GetAutoplay(chatID)
	text := "<b>Autoplay Control</b>\n\nWhen autoplay is enabled, the bot will automatically play recommended songs from YouTube when the queue is empty."
	button := autoplayButton(state)

	_, err := m.ReplyText(c, text, &td.SendTextMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: button,
	})
	return err
}

func autoplayCallbackHandler(c *td.Client, cb *td.UpdateNewCallbackQuery) error {
	if !adminModeCB(c, cb) {
		return nil
	}

	chatID := cb.ChatId
	if cache.ChatCache.GetPlayingTrack(chatID) == nil {
		_ = cb.Answer(c, 0, true, "Bot is not streaming in the video chat.", "")
		return nil
	}

	state := cache.ChatCache.GetAutoplay(chatID)
	newState := !state
	cache.ChatCache.SetAutoplay(chatID, newState)

	text := "<b>Autoplay Control</b>\n\nWhen autoplay is enabled, the bot will automatically play recommended songs from YouTube when the queue is empty."
	button := autoplayButton(newState)

	_, err := cb.EditMessageText(c, text, &td.EditTextMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: button,
	})
	if err != nil {
		c.Logger.Warn("Failed to edit autoplay message", "error", err)
	}

	var status string
	if newState {
		status = "enabled"
	} else {
		status = "disabled"
	}
	_ = cb.Answer(c, 0, false, fmt.Sprintf("Autoplay has been %s.", status), "")

	return nil
}

func autoplayButton(state bool) *td.ReplyMarkupInlineKeyboard {
	var text string
	if state {
		text = "Autoplay: ON | ✅"
	} else {
		text = "Autoplay: OFF | ❌"
	}

	return &td.ReplyMarkupInlineKeyboard{
		Rows: [][]td.InlineKeyboardButton{
			{
				{
					Text: text,
					Type: &td.InlineKeyboardButtonTypeCallback{
						Data: []byte("autoplay_toggle"),
					},
				},
			},
		},
	}
}
