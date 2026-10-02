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
	"github.com/kustbots/kustmusic/internal/player"
	"github.com/kustbots/kustmusic/internal/utils"
	"html"
	"math"
	"strconv"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func queueHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId

	chat, err := c.GetChat(chatID)
	if err != nil {
		_, _ = m.ReplyText(c, "Error fetching chat information.", nil)
		return nil
	}

	queue := cache.ChatCache.GetQueue(chatID)
	if len(queue) == 0 {
		_, _ = m.ReplyText(c, "<b>📭 The queue is empty</b>", nil)
		return nil
	}

	if !cache.ChatCache.IsActive(chatID) {
		_, _ = m.ReplyText(c, "<b>📵 Nothing is playing</b>", nil)
		return nil
	}

	current := queue[0]
	playedTime, _ := player.Calls.PlayedTime(chatID)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<b>📀 QUEUE · %s</b>\n%s\n", html.EscapeString(chat.Title), utils.Divider))

	b.WriteString("<b>🎧 Now playing</b>\n<blockquote>")
	b.WriteString(fmt.Sprintf("🎵 <b>%s</b>\n", html.EscapeString(truncate(current.Name, 45))))
	b.WriteString(fmt.Sprintf("👤 <b>By:</b> %s\n", html.EscapeString(current.User)))
	b.WriteString(fmt.Sprintf("⏱ <b>Duration:</b> %s\n", utils.SecToMin(current.Duration)))
	b.WriteString("🔁 <b>Loop:</b> ")
	if current.Loop > 0 {
		b.WriteString("On\n")
	} else {
		b.WriteString("Off\n")
	}
	b.WriteString("▶️ <b>Progress:</b> ")
	if playedTime > 0 && playedTime < math.MaxInt {
		b.WriteString(utils.SecToMin(int32(playedTime)))
	} else {
		b.WriteString("0:00")
	}
	b.WriteString("</blockquote>\n")

	if len(queue) > 1 {
		b.WriteString(fmt.Sprintf("\n<b>⏭ Up next (%d)</b>\n", len(queue)-1))

		for i, song := range queue[1:] {
			if i >= 14 {
				break
			}
			b.WriteString(strconv.Itoa(i + 1))
			b.WriteString(". ")
			b.WriteString(html.EscapeString(truncate(song.Name, 45)))
			b.WriteString(" · <code>")
			b.WriteString(utils.SecToMin(song.Duration))
			b.WriteString("</code>\n")
		}

		if len(queue) > 15 {
			b.WriteString(fmt.Sprintf("...and %d more tracks\n", len(queue)-15))
		}
	}

	b.WriteString(fmt.Sprintf("\n📊 <b>Total:</b> %d tracks", len(queue)))

	text := b.String()
	if len(text) > 4096 {
		var sb strings.Builder
		progress := "0:00"
		if playedTime > 0 && playedTime < math.MaxInt {
			progress = utils.SecToMin(int32(int(playedTime)))
		}
		sb.WriteString(fmt.Sprintf(
			"<b>Queue for %s</b>\n\n<b>Now Playing:</b>\n• <code>%s</code>\n• %s/%s min\n\n<b>Total:</b> %d tracks",
			chat.Title,
			truncate(current.Name, 45),
			progress,
			utils.SecToMin(current.Duration),
			len(queue),
		))
		text = sb.String()
	}

	_, err = m.ReplyText(c, text, replyOpts)
	return err
}
