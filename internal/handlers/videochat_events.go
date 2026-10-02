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
	"github.com/kustbots/kustmusic/internal/utils"

	"fmt"
	"time"

	td "github.com/kustbots/gotdbot"
)

func handleVoiceChatMessage(c *td.Client, update *td.UpdateNewMessage) error {
	m := update.Message
	chatID := m.ChatId

	if m.IsGroup() {
		text := fmt.Sprintf(
			"This chat (%d) is not a supergroup yet.\n<b>⚠️ Please convert this chat to a supergroup and add me as admin.</b>\n\nIf you don't know how to convert, use this guide:\n🔗 https://te.legra.ph/How-to-Convert-a-Group-to-a-Supergroup-01-02\n\nIf you have any questions, join our support group:",
			chatID,
		)

		_, _ = c.SendTextMessage(chatID, text, &td.SendTextMessageOpts{
			ReplyMarkup:           utils.AddMeMarkup(c.Me.Usernames.EditableUsername),
			DisableWebPagePreview: true,
			ParseMode:             "HTML",
		})

		time.Sleep(1 * time.Second)
		_ = c.LeaveChat(chatID)
		return nil
	}

	go storeChatToDB(chatID)

	if m.Content == nil {
		return nil
	}
	var message string
	switch m.Content.(type) {
	case *td.MessageVideoChatStarted:
		cache.ChatCache.ClearChat(chatID)
		message = "🎙️ Video chat started!\nUse /play <song name> to play music."
	case *td.MessageVideoChatEnded:
		cache.ChatCache.ClearChat(chatID)
		message = "🎧 Video chat ended!\nAll queues cleared."
	default:
		return nil
	}

	_, _ = c.SendTextMessage(chatID, message, nil)
	return td.EndGroups
}
