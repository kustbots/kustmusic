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
	"github.com/kustbots/kustmusic/internal/db"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func requireAdmin(c *td.Client, m *td.Message) bool {
	if m.IsPrivate() {
		return false
	}

	status, err := cache.GetUserAdmin(c, m.ChatId, m.SenderID(), false)
	if err != nil {
		c.Logger.Warn("failed to get user admin status", "error", err)
		_, _ = m.ReplyText(c, "Unable to verify administrator status.", nil)
		return false
	}

	switch status.Status.(type) {
	case *td.ChatMemberStatusCreator, *td.ChatMemberStatusAdministrator:
		return true
	default:
		_, _ = m.ReplyText(c, "You must be an administrator to use this command.", nil)
		return false
	}
}

func authListHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	if m.IsPrivate() {
		return td.EndGroups
	}

	authUsers := db.Instance.GetAuthUsers(m.ChatId)
	if len(authUsers) == 0 {
		_, err := m.ReplyText(c, "No authorized users found.", nil)
		return err
	}

	var text strings.Builder
	text.WriteString("<b>Authorized Users</b>\n\n")

	for _, userID := range authUsers {
		fmt.Fprintf(
			&text,
			"• <a href=\"tg://user?id=%d\">%d</a>\n",
			userID,
			userID,
		)
	}

	_, err := m.ReplyText(c, text.String(), replyOpts)
	return err
}

func addAuthHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	if !requireAdmin(c, m) {
		return td.EndGroups
	}

	userID, err := getTargetUserID(c, m)
	if err != nil {
		_, replyErr := m.ReplyText(c, err.Error(), nil)
		if replyErr != nil {
			c.Logger.Warn("failed to send target user error", "error", replyErr)
		}
		return td.EndGroups
	}

	chatID := m.ChatId

	if db.Instance.IsAuthUser(chatID, userID) {
		_, err = m.ReplyText(c, "This user is already authorized.", nil)
		return err
	}

	if err = db.Instance.AddAuthUser(chatID, userID); err != nil {
		c.Logger.Error("failed to add authorized user", "chat_id", chatID, "user_id", userID, "error", err)
		_, replyErr := m.ReplyText(c, "Failed to authorize the user.", nil)
		if replyErr != nil {
			return replyErr
		}

		return td.EndGroups
	}

	_, err = m.ReplyText(
		c,
		fmt.Sprintf("User %d has been authorized.", userID),
		nil,
	)
	return err
}

func removeAuthHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	if !requireAdmin(c, m) {
		return td.EndGroups
	}

	userID, err := getTargetUserID(c, m)
	if err != nil {
		_, replyErr := m.ReplyText(c, err.Error(), nil)
		if replyErr != nil {
			c.Logger.Warn("failed to send target user error", "error", replyErr)
		}
		return td.EndGroups
	}

	chatID := m.ChatId

	if !db.Instance.IsAuthUser(chatID, userID) {
		_, err = m.ReplyText(c, "This user is not authorized.", nil)
		return err
	}

	if err = db.Instance.RemoveAuthUser(chatID, userID); err != nil {
		c.Logger.Error("failed to remove authorized user", "chat_id", chatID, "user_id", userID, "error", err)

		_, replyErr := m.ReplyText(c, "Failed to remove authorized user.", nil)
		if replyErr != nil {
			return replyErr
		}

		return td.EndGroups
	}

	_, err = m.ReplyText(c, fmt.Sprintf("User %d has been removed from the authorized list.", userID), nil)
	return err
}
