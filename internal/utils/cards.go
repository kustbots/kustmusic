package utils

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kustbots/gotdbot"
)

// Divider separates a card header from its body.
const Divider = "━━━━━━━━━━━━━━━━━━"

// Card headers shared by every player message.
const (
	HeaderPlaying = "🎧 NOW PLAYING"
	HeaderPaused  = "⏸ PAUSED"
	HeaderMuted   = "🔇 MUTED"
	HeaderQueued  = "📥 ADDED TO QUEUE"
)

var thumbHTTPClient = &http.Client{Timeout: 2 * time.Second}

// ThumbURL returns the best artwork URL for a track, or "" when it has none.
func ThumbURL(t *PlayerCache) string {
	if t == nil {
		return ""
	}

	if t.Platform == YouTube && t.TrackID != "" {
		base := "https://i.ytimg.com/vi/" + t.TrackID + "/"
		for _, name := range []string{"maxresdefault.jpg", "hq720.jpg"} {
			resp, err := thumbHTTPClient.Head(base + name)
			if err != nil {
				break
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base + name
			}
		}
		return base + "mqdefault.jpg"
	}

	if strings.HasPrefix(t.Thumbnail, "http") {
		return t.Thumbnail
	}

	return ""
}

// PlayerCard builds the text of a player card. Footer is appended as-is and must already be HTML-safe.
func PlayerCard(header string, t *PlayerCache, footer string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<b>%s</b>\n%s\n", header, Divider)

	name := html.EscapeString(t.Name)
	if t.URL != "" {
		fmt.Fprintf(&sb, "🎵 <b><a href='%s'>%s</a></b>\n\n", html.EscapeString(t.URL), name)
	} else {
		fmt.Fprintf(&sb, "🎵 <b>%s</b>\n\n", name)
	}

	sb.WriteString("<blockquote>")
	fmt.Fprintf(&sb, "⏱ <b>Duration:</b> %s", SecToMin(t.Duration))
	if t.Channel != "" {
		fmt.Fprintf(&sb, "\n🎙 <b>Artist:</b> %s", html.EscapeString(t.Channel))
	}
	fmt.Fprintf(&sb, "\n👤 <b>Requested by:</b> %s", html.EscapeString(t.User))
	sb.WriteString("</blockquote>")

	if footer != "" {
		sb.WriteString("\n" + footer)
	}

	return sb.String()
}

// CardOpts returns edit options that show the track artwork above the card text.
func CardOpts(t *PlayerCache, markup gotdbot.ReplyMarkup) *gotdbot.EditTextMessageOpts {
	opts := &gotdbot.EditTextMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	}

	if thumb := ThumbURL(t); thumb != "" {
		opts.Url = thumb
		opts.ForceLargeMedia = true
		opts.ShowAboveText = true
	} else {
		opts.DisableWebPagePreview = true
	}

	return opts
}

// FriendlyError logs the raw error and returns a short, HTML-safe reason that is safe to show in chat.
func FriendlyError(err error) string {
	if err == nil {
		return "Something went wrong. Please try again."
	}

	slog.Warn("user-facing error", "error", err)

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "sign in"), strings.Contains(msg, "not a bot"),
		strings.Contains(msg, "429"), strings.Contains(msg, "too many requests"):
		return "The music source is busy right now. Try again in a moment."
	case strings.Contains(msg, "age-restricted"), strings.Contains(msg, "confirm your age"):
		return "This track is age-restricted and can't be played."
	case strings.Contains(msg, "unavailable"), strings.Contains(msg, "private video"),
		strings.Contains(msg, "removed"), strings.Contains(msg, "not available"):
		return "This track isn't available. Try a different one."
	case strings.Contains(msg, "timed out"), strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline"):
		return "That took too long to load. Please try again."
	default:
		return "Something went wrong. Please try again."
	}
}

// PlaybackError keeps the bot's own formatted messages and hides everything else.
func PlaybackError(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "<b>") {
		return msg
	}

	// Assistant and invite-link problems carry instructions the group admin needs to see.
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "assistant") || strings.Contains(lower, "invite link") || strings.Contains(lower, "limiting join") {
		slog.Warn("user-facing error", "error", err)

		msg = strings.TrimPrefix(msg, "playback failed after trying all assistants: ")
		msg = strings.TrimPrefix(msg, "playback failed: ")

		var kept []string
		for _, part := range strings.Split(msg, "\n\n") {
			if !strings.HasPrefix(part, "Reason:") {
				kept = append(kept, part)
			}
		}

		return "<b>⚠️ The assistant can't join this chat</b>\n\n" + strings.Join(kept, "\n\n")
	}

	return Notice("❌ Couldn't start playback", FriendlyError(err))
}

// Notice builds a short one-card status message. Body must already be HTML-safe.
func Notice(title, body string) string {
	if body == "" {
		return "<b>" + title + "</b>"
	}
	return fmt.Sprintf("<b>%s</b>\n<blockquote>%s</blockquote>", title, body)
}
