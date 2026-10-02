package main

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/kustbots/kustmusic/internal/telegram"
)

// liveProbeTimeout bounds each server probe in /live. Every server is
// probed concurrently, so this is roughly the whole command's worst case —
// short enough that one wedged server cannot leave the owner staring at a
// silent chat.
const liveProbeTimeout = 8 * time.Second

// cmdLive reports voice-chat load across the playback pool: how many streams
// each server is actually running, and how many chats the router has placed
// on it. Owner-only, because it exposes the server topology.
func (a *App) cmdLive(m *telegram.Message) {
	if m.From == nil || m.From.ID != a.ownerID {
		return
	}

	msg, err := a.tg.SendMessage(m.Chat.ID, "📡 Checking playback servers…", nil)
	if err != nil {
		return
	}

	stats := a.router.Snapshot(liveProbeTimeout)
	if len(stats) == 0 {
		_ = a.tg.EditMessageText(m.Chat.ID, msg.MessageID, "⚠️ No playback servers configured.", nil)
		return
	}

	var b strings.Builder
	b.WriteString("📡 <b>Playback servers</b>\n")

	totalLive, totalAssigned, online, degraded := 0, 0, 0, 0
	var section bool
	for _, s := range stats {
		if s.Video && !section {
			b.WriteString("\n<b>— video pool —</b>\n")
			section = true
		}
		totalLive += s.Live
		totalAssigned += s.Assigned

		label := fmt.Sprintf("#%d", s.Index)
		switch {
		case !s.Online:
			degraded++
			b.WriteString(fmt.Sprintf("\n❌ %s <code>%s</code>\n     %s\n",
				label, html.EscapeString(shortHost(s.URL)), html.EscapeString(s.Note)))
		case s.Assistant == "":
			// Up, but with no usable assistant it cannot stream anything —
			// worth calling out separately from a server that is simply idle.
			degraded++
			online++
			b.WriteString(fmt.Sprintf("\n⚠️ %s <code>%s</code>\n     live %d · assigned %d · no assistant (%s)\n",
				label, html.EscapeString(shortHost(s.URL)), s.Live, s.Assigned,
				html.EscapeString(s.Note)))
		default:
			online++
			b.WriteString(fmt.Sprintf("\n✅ %s <code>%s</code>\n     live %d · assigned %d · @%s\n",
				label, html.EscapeString(shortHost(s.URL)), s.Live, s.Assigned,
				html.EscapeString(s.Assistant)))
		}
	}

	b.WriteString(fmt.Sprintf(
		"\n━━━━━━━━━━━━━━\n🔊 <b>Live VCs:</b> %d\n📌 <b>Assigned chats:</b> %d\n🖥 <b>Servers:</b> %d up / %d total",
		totalLive, totalAssigned, online, len(stats)))
	if degraded > 0 {
		b.WriteString(fmt.Sprintf("\n⚠️ <b>Degraded:</b> %d", degraded))
	}

	_ = a.tg.EditMessageText(m.Chat.ID, msg.MessageID, b.String(), nil)
}

// shortHost trims a server base URL down to its hostname so the report stays
// readable on a phone.
func shortHost(raw string) string {
	s := strings.TrimSuffix(raw, "/")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSuffix(s, ".herokuapp.com")
}
