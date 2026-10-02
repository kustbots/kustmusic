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
	"strings"

	td "github.com/kustbots/gotdbot"
)

func getUrl(c *td.Client, m *td.Message, isReply bool) string {
	text := m.GetText()
	entities := m.GetEntities()

	if isReply {
		reply, err := m.GetRepliedMessage(c)
		if err == nil && reply != nil {
			text = reply.Text()
			entities = reply.GetEntities()
		}
	}

	if entities == nil || len(entities) == 0 {
		return ""
	}

	for _, entity := range entities {
		switch t := entity.Type.(type) {

		case *td.TextEntityTypeUrl:
			start := entity.Offset
			end := entity.Offset + entity.Length
			if int(end) <= len(text) {
				return text[start:end]
			}

		case *td.TextEntityTypeTextUrl:
			return t.Url
		}
	}

	return ""
}

func isValidMedia(reply *td.Message) bool {
	if reply == nil || reply.Content == nil {
		return false
	}

	switch msg := reply.Content.(type) {

	case *td.MessageAudio,
		*td.MessageVoiceNote,
		*td.MessageVideo,
		*td.MessageVideoNote:
		return true

	case *td.MessageDocument:
		if msg.Document == nil {
			return false
		}
		mime := strings.ToLower(msg.Document.MimeType)
		if strings.HasPrefix(mime, "audio/") || strings.HasPrefix(mime, "video/") {
			return true
		}

		return false
	}

	return false
}

func getFile(m *td.Message) (*td.File, string) {
	if m == nil || m.Content == nil {
		return nil, ""
	}

	switch content := m.Content.(type) {
	case *td.MessageAudio:
		return content.Audio.Audio, content.Audio.Title
	case *td.MessageVoiceNote:
		return content.VoiceNote.Voice, "voice_note.ogg"
	case *td.MessageVideo:
		return content.Video.Video, content.Video.FileName
	case *td.MessageVideoNote:
		return content.VideoNote.Video, "video_note.mp4"
	case *td.MessageDocument:
		return content.Document.Document, content.Document.FileName
	default:
		return nil, ""
	}
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
