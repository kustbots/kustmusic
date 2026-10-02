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
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/utils"
	"html"
	"runtime"
	"time"

	td "github.com/AshokShau/gotdbot"
)

func pingHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	start := time.Now()

	msg, err := m.ReplyText(c, "🏓 Pinging…", nil)
	if err != nil {
		return err
	}

	latency := time.Since(start).Milliseconds()
	uptime := getFormattedDuration(time.Since(startTime))

	response := fmt.Sprintf(
		"<b>🏓 PONG</b>\n"+utils.Divider+"\n<blockquote>"+
			"⚡ <b>Latency:</b> <code>%d ms</code>\n"+
			"⏳ <b>Uptime:</b> <code>%s</code>\n"+
			"🧵 <b>Routines:</b> <code>%d</code></blockquote>",
		latency, uptime, runtime.NumGoroutine(),
	)

	_, err = msg.EditText(c, response, &td.EditTextMessageOpts{ParseMode: "HTML"})
	return err
}

// welcomeMessage builds the private start screen. The carded layout spaces the sections out with a
// table and a collapsible block; the plain layout is the fallback if Telegram rejects it.
func welcomeMessage(userName, botName string, carded bool) *td.InputRichMessage {
	head := fmt.Sprintf(
		"<img src=\"%s\"/>\n"+
			"<h3>Hey %s 👋</h3>\n"+
			"<p><b>%s</b> plays music and video in your group voice chats — fast, clean, no lag.</p>\n",
		config.StartImg,
		html.EscapeString(userName),
		html.EscapeString(botName),
	)

	body := "<p>🎧 <b>Crystal-clear audio</b> with a live player card</p>\n" +
		"<p>📀 <b>Queues, playlists, mixes</b> and autoplay</p>\n" +
		"<p>🌐 <b>YouTube, Spotify, Apple Music, SoundCloud</b> and more</p>\n" +
		"<p><i>Add me to a group, start a video chat and send</i> <code>/play song name</code></p>"

	if carded {
		body = "<table bordered striped>" +
			"<tr><th>✨</th><th>What you get</th></tr>" +
			"<tr><td>🎧</td><td><b>Crystal-clear audio</b> with a live player card</td></tr>" +
			"<tr><td>📀</td><td><b>Queues, playlists, mixes</b> and autoplay</td></tr>" +
			"<tr><td>🌐</td><td><b>YouTube, Spotify, Apple Music, SoundCloud</b> and more</td></tr>" +
			"</table>\n" +
			"<details open>\n" +
			"  <summary>🚀 Get started</summary>\n" +
			"  <p>Add me to a group, start a video chat, then send <code>/play song name</code></p>\n" +
			"</details>"
	}

	return &td.InputRichMessage{
		Source: &td.RichMessageSourceHtml{
			Text: head + body,
		},
	}
}

func startHandler(c *td.Client, m *td.Message) error {
	chatID := m.ChatId
	go storeChatToDB(chatID)

	deleteCmd(c, m)

	if m.IsPrivate() {
		opts := &td.SendTextMessageOpts{
			ReplyMarkup: utils.AddMeMarkup(c.Me.Usernames.EditableUsername),
		}

		_, err := m.ReplyRichMessage(c, welcomeMessage(firstName(c, m), c.Me.FirstName, true), opts)
		if err != nil {
			c.Logger.Warn("welcome card failed, sending the plain layout", "error", err)
			_, err = m.ReplyRichMessage(c, welcomeMessage(firstName(c, m), c.Me.FirstName, false), opts)
		}

		return err
	}

	uptime := getFormattedDuration(time.Since(startTime))
	htmlText := fmt.Sprintf(
		"<h3>🎧 %s is online</h3>\n"+
			"<p>⏳ <b>Uptime:</b> <code>%s</code></p>\n"+
			"<p><i>Start a video chat and send</i> <code>/play song name</code> <i>to get the music going.</i></p>\n\n"+
			"<p><tg-button type=\"url\" url=\"%s\">Updates</tg-button> <tg-button type=\"url\" url=\"%s\">Support</tg-button> <tg-button type=\"url\" url=\"%s\">Community</tg-button></p>",
		c.Me.FirstName,
		uptime,
		config.SupportChannel,
		config.SupportGroup,
		config.Community,
	)

	richMessage := &td.InputRichMessage{
		Source: &td.RichMessageSourceHtml{
			Text: htmlText,
		},
	}

	_, err := m.ReplyRichMessage(c, richMessage, nil)
	return err
}
