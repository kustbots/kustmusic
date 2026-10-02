/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func GetMessage(client *td.Client, url string) (*td.Message, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("url must not be empty")
	}

	link, err := buildMessageLink(url)
	if err != nil {
		return nil, err
	}

	return resolveMessage(client, link)
}

func buildMessageLink(url string) (string, error) {
	if m := publicRe.FindStringSubmatch(url); m != nil {
		msgID, err := strconv.Atoi(m[2])
		if err != nil {
			return "", fmt.Errorf("invalid message ID in URL: %w", err)
		}
		return fmt.Sprintf("https://t.me/%s/%d", m[1], msgID), nil
	}

	if m := privateRe.FindStringSubmatch(url); m != nil {
		chatID, err1 := strconv.ParseInt(m[1], 10, 64)
		msgID, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			return "", fmt.Errorf("invalid chat/message ID in private URL: %v %v", err1, err2)
		}
		return fmt.Sprintf("https://t.me/c/%d/%d", chatID, msgID), nil
	}

	return "", errors.New("invalid Telegram message URL")
}

func resolveMessage(client *td.Client, link string) (*td.Message, error) {
	info, err := client.GetMessageLinkInfo(link)
	if err != nil {
		slog.Info("failed to get message link info", "link", link, "error", err)
		return nil, fmt.Errorf("get message link info: %w", err)
	}

	if info.Message != nil {
		return info.Message, nil
	}
	return nil, fmt.Errorf("failed to get message link info")
}
