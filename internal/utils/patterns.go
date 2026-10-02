/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package utils

import "regexp"

var (
	TelegramMessageRegex = regexp.MustCompile(`^https?://(?:www\.)?(?:t\.me|telegram\.me)/(?:([a-zA-Z0-9_]{4,})|c/(\d+))/(\d+)(?:\?.*)?$`)
	publicRe             = regexp.MustCompile(`^https?://(?:www\.)?(?:t\.me|telegram\.me)/([a-zA-Z0-9_]{4,})/(\d+)(?:\?.*)?$`)
	privateRe            = regexp.MustCompile(`^https?://(?:www\.)?(?:t\.me|telegram\.me)/c/(\d+)/(\d+)(?:\?.*)?$`)
)
