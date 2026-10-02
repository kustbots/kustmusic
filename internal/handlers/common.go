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
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/config"
	"strconv"
	"strings"
	"time"

	"github.com/kustbots/gotdbot"
)

func Args(m *gotdbot.Message) string {
	Messages := strings.Split(m.Text(), " ")
	if len(Messages) < 2 {
		return ""
	}
	return strings.TrimSpace(strings.Join(Messages[1:], " "))
}

func firstName(c *gotdbot.Client, m *gotdbot.Message) string {
	if m.SenderId == nil {
		return "Unknown"
	}

	if u, ok := m.SenderId.(*gotdbot.MessageSenderUser); ok {
		user, err := c.GetUser(u.UserId)
		if err != nil {
			return "Unknown"
		}
		return user.FirstName
	}

	if ch, ok := m.SenderId.(*gotdbot.MessageSenderChat); ok {
		chat, err := c.GetChat(ch.ChatId)
		if err != nil {
			return "Unknown"
		}
		return chat.Title
	}

	return "Unknown"
}

var replyOpts = &gotdbot.SendTextMessageOpts{
	ParseMode:             "HTML",
	DisableWebPagePreview: true,
}

func isDev(_ *gotdbot.Client, m *gotdbot.Message) bool {
	for _, dev := range config.DEVS {
		if dev == m.SenderID() {
			return true
		}
	}

	return false
}

func SenderID(sender gotdbot.MessageSender) int64 {
	switch s := sender.(type) {
	case *gotdbot.MessageSenderUser:
		return s.UserId
	case *gotdbot.MessageSenderChat:
		return s.ChatId
	default:
		return 0
	}
}

func getTargetUserID(c *gotdbot.Client, m *gotdbot.Message) (int64, error) {
	if m.ReplyToMessageID() != 0 {
		return resolveFromReply(c, m)
	}

	args := strings.Fields(Args(m))
	if len(args) == 0 {
		return 0, errors.New("no target specified: reply to a message or provide a user ID/username")
	}

	userID, err := resolveFromArg(c, args[0])
	if err != nil {
		return 0, err
	}

	if m.SenderID() == userID {
		return 0, errors.New("cannot perform action on yourself")
	}

	return userID, nil
}

func resolveFromReply(c *gotdbot.Client, m *gotdbot.Message) (int64, error) {
	replyMsg, err := m.GetRepliedMessage(c)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch replied message: %w", err)
	}

	userID := replyMsg.SenderID()
	if userID == 0 {
		return 0, errors.New("replied message has no identifiable sender")
	}

	return userID, nil
}

func resolveFromArg(c *gotdbot.Client, arg string) (int64, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		if id <= 0 {
			return 0, fmt.Errorf("invalid user ID: %d", id)
		}
		return id, nil
	}

	return resolveUsername(c, arg)
}

func resolveUsername(c *gotdbot.Client, username string) (int64, error) {
	username = strings.TrimPrefix(username, "@")
	if username == "" {
		return 0, errors.New("username cannot be empty")
	}

	chat, err := c.SearchPublicChat(username)
	if err != nil {
		return 0, fmt.Errorf("username lookup failed for %q: %w", username, err)
	}
	if chat == nil {
		return 0, fmt.Errorf("no user found for username %q", username)
	}

	return chat.Id, nil
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func getFormattedDuration(diff time.Duration) string {
	totalSeconds := int(diff.Seconds())

	months := totalSeconds / (30 * 24 * 3600)
	remaining := totalSeconds % (30 * 24 * 3600)

	weeks := remaining / (7 * 24 * 3600)
	remaining = remaining % (7 * 24 * 3600)

	days := remaining / (24 * 3600)
	remaining = remaining % (24 * 3600)

	hours := remaining / 3600
	remaining = remaining % 3600

	minutes := remaining / 60
	seconds := remaining % 60

	var parts []string

	if months > 0 {
		parts = append(parts, plural(months, "month"))
	}
	if weeks > 0 {
		parts = append(parts, plural(weeks, "week"))
	}
	if days > 0 {
		parts = append(parts, plural(days, "day"))
	}
	if hours > 0 {
		parts = append(parts, plural(hours, "hour"))
	}
	if minutes > 0 {
		parts = append(parts, plural(minutes, "minute"))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, plural(seconds, "second"))
	}

	return strings.Join(parts, " ")
}
