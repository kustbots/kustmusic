// This file replicates main-music-rx's exact visual UI (English only, per
// this rewrite's i18n scope decision): the /start home screen animation +
// caption + buttons (build_home_screen), and the now-playing player card
// (setup_player_ui) — same emoji, same "small caps"/bold-unicode styling,
// same blockquote layout, same button glyphs and callback_data, same URLs.
package main

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kustbots/kustmusic/internal/assets"
	"github.com/kustbots/kustmusic/internal/telegram"
)

const maxTitleLen = 20

// Links behind the bot's buttons. Each can be set from the environment.
// addToGroupURL is filled in at startup from the bot's own username.
var (
	communityURL    = getenv("COMMUNITY_URL", "https://t.me/kustbotschat")
	supportURL      = getenv("SUPPORT_URL", "https://github.com/kustbots/kustmusic/issues")
	supportUsername = getenv("SUPPORT_USERNAME", "")
	githubURL       = getenv("GITHUB_URL", "https://github.com/kustbots/kustmusic")
	updatesURL      = getenv("UPDATES_URL", "https://t.me/kustbots")
	addToGroupURL   = ""
)

// toBoldUnicode maps ASCII letters to their Unicode "mathematical bold"
// codepoints — matches main-music-rx's to_bold_unicode exactly, letter for
// letter.
func toBoldUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune('𝗔' + (r - 'A'))
		case r >= 'a' && r <= 'z':
			b.WriteRune('𝗮' + (r - 'a'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// actorOf renders the person who triggered an action as a clickable
// mention, or "" when nobody did (auto-advance, idle timeout).
//
// Control actions name whoever performed them so the chat can see a person
// did it. Without this, a track vanishing mid-song looked like the bot
// deciding on its own, and people assumed it had broken.
func actorOf(u *telegram.User) string {
	if u == nil {
		return ""
	}
	return mentionUser(u.ID, u.FirstName)
}

// byActor renders the " · by <user>" suffix appended to control messages,
// or "" for actions nobody triggered.
func byActor(actor string) string {
	if actor == "" {
		return ""
	}
	return "\n<i>by</i> " + actor
}

// mentionUser builds the HTML clickable mention the Python bot used for
// "requested by" (f'<a href="tg://user?id={id}">{first_name}</a>').
func mentionUser(userID int64, name string) string {
	if name == "" {
		name = "Someone"
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, userID, htmlEscape(name))
}

// queueAddedCaption replicates the Python bot's "added to queue" message
// exactly — same small-caps labels, same field order, same arrows.
func queueAddedCaption(title, readableDuration, requesterMention string, position int) string {
	return fmt.Sprintf(
		"<b>✨ ᴀᴅᴅᴇᴅ ᴛᴏ ǫᴜᴇᴜᴇ</b>\n\n"+
			"<blockquote>❍ <b>ᴛɪᴛʟє ➥</b> %s\n"+
			"❍ <b>ᴛɪϻє ➥</b> %s\n"+
			"❍ <b>ʙʏ ➥</b> %s</blockquote>\n"+
			"🔢 <b>ǫᴜєᴜє ɴᴜᴍʙєʀ:</b> <code>%d</code>",
		htmlEscape(title), htmlEscape(readableDuration), requesterMention, position,
	)
}

// queueAddedKeyboard is the Skip/Clear pair the Python bot attached to
// every "added to queue" reply — the buttons that were missing here.
func queueAddedKeyboard() telegram.InlineKeyboard {
	return telegram.InlineKeyboard{
		{
			{Text: "⏭ Skip", CallbackData: "skip", Style: telegram.StylePrimary},
			{Text: "🗑 Clear", CallbackData: "clear", Style: telegram.StyleDanger},
		},
	}
}

// sendHome posts the home screen: the bot's picture with the caption and
// buttons under it. The picture is uploaded once and re-sent by reference
// after that; if the send fails the caption still goes out as plain text.
func (a *App) sendHome(chatID int64, caption string, kb telegram.InlineKeyboard) {
	if _, err := a.tg.SendCachedPhoto(chatID, "home", assets.DP, caption, kb); err != nil {
		a.log.Warn("home picture failed, falling back to plain message", "err", err)
		_, _ = a.tg.SendMessage(chatID, caption, kb)
	}
}

// shortTitle trims title to n characters with an ellipsis.
func shortTitle(title string, n int) string {
	runes := []rune(title)
	if len(runes) <= n {
		return title
	}
	return string(runes[:n-1]) + "…"
}

// assistantMissingCard explains that the voice-chat account couldn't get
// into the group, and — crucially — names the account someone can add by
// hand to fix it immediately.
//
// Naming it is not a nicety. Telegram rate-limits an account inviting
// *itself* into a group, and once that limit is hit no amount of retrying
// helps; each /play is another invite attempt and lengthens the penalty. A
// member adding the account is not rate-limited at all, so it is the only
// action that works during a limit — and it is useless advice unless the
// message says which account. The old text said "give it a few seconds and
// hit /play again", which was the one thing guaranteed not to work.
//
// Only the account belonging to the server this chat is actually assigned
// to is named. The bot runs several, but a chat is pinned to one, and that
// is the only account whose presence in this group makes any difference —
// listing the others invites someone to add an account that will never be
// asked to play here.
//
// until/allLimited are best-effort extras: when the bot has actually
// observed a flood wait it can give a real countdown, but the card must
// still be useful when it hasn't, which is why naming the account is
// unconditional.
func assistantMissingCard(missing *assistantMissingError, until time.Time, allLimited bool) (string, telegram.InlineKeyboard) {
	// Mention() falls back to a clickable tg:// link when the account has
	// no @username, so this never renders as a bare "@".
	account := missing.Mention()

	var b strings.Builder
	b.WriteString("<b>🎧 ᴄᴏᴜʟᴅɴ'ᴛ ɢᴇᴛ sᴇᴛ ᴜᴘ</b>\n\n")
	if allLimited && !until.IsZero() {
		b.WriteString("Telegram has temporarily rate-limited my voice-chat accounts from joining new groups. " +
			"<b>The limit clears in " + humanizeWait(time.Until(until)) + "</b> — and sending /play again before " +
			"then makes the wait longer, so please don't.\n\n")
	} else {
		b.WriteString("I couldn't walk my voice-chat account into this group. " +
			"This is usually Telegram rate-limiting the account for joining groups, " +
			"and retrying /play tends to make that worse rather than better.\n\n")
	}

	b.WriteString("<b>Fastest fix — add this account to the group yourself:</b>\n")
	b.WriteString("• " + account + "\n")
	b.WriteString("\nAdding it by hand isn't rate-limited, so it works straight away. " +
		"Then tap the button and I'll carry on with your song.")

	return b.String(), telegram.InlineKeyboard{
		{{Text: "✅ ɪ'ᴠᴇ ᴀᴅᴅᴇᴅ ɪᴛ — ᴄᴀʀʀʏ ᴏɴ", CallbackData: "assistant_added", Style: telegram.StyleSuccess}},
		{{Text: "🛟 Support", URL: supportURL}},
	}
}

// humanizeWait renders a rate-limit countdown the way someone waiting
// would say it. Rounds up, because telling a group "1 minute" for 61
// seconds and having it fail again is worse than rounding the other way.
func humanizeWait(d time.Duration) string {
	if d <= 0 {
		return "a moment"
	}
	if d < time.Minute {
		secs := int(d.Round(time.Second).Seconds())
		if secs <= 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", secs)
	}
	mins := int((d + time.Minute - time.Second).Truncate(time.Minute).Minutes())
	if mins <= 1 {
		return "1 minute"
	}
	if mins < 60 {
		return fmt.Sprintf("%d minutes", mins)
	}
	hours := mins / 60
	rem := mins % 60
	if rem == 0 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, rem)
}

// oneLineTitle truncates to maxTitleLen chars, matching _one_line_title.
func oneLineTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= maxTitleLen {
		return title
	}
	return string(runes[:maxTitleLen-1]) + "…"
}

// buildHomeScreen returns the caption + keyboard build_home_screen
// produces (English only), rendered as HTML rather than the Python bot's
// Markdown. That's deliberate, not a drift: the original ran on Pyrogram,
// whose ParseMode.MARKDOWN accepts `**bold**` and `>` blockquotes as
// extensions. The plain Bot API this rewrite uses has neither in its
// legacy "Markdown" mode — it wants `*bold*` and has no blockquote at all
// — so sending the original text verbatim rendered literal `**` and `>`
// characters all over the home screen. HTML is the Bot API mode that
// actually supports everything the original layout needs, so the visual
// result matches while the markup differs. userLink is an HTML mention.
func buildHomeScreen(userLink string) (string, telegram.InlineKeyboard) {
	caption := fmt.Sprintf(
		"👋 <b>Hello,</b> %s<b>!</b>\n\n"+
			"✨ <b>%s</b> ✨\n"+
			"<i>ʏᴏᴜʀ ɢʀᴏᴜᴘ's ᴘᴇʀsᴏɴᴀʟ ᴅᴊ</i>\n\n"+
			"<blockquote>🎵 <b>Premium Audio</b>\n"+
			"│ 🚀 24x7 non-stop playback\n"+
			"│ 🔊 High-fidelity, no lag\n"+
			"│ 📀 YouTube · Spotify · Apple · Resso\n"+
			"│ 🤖 Smart auto-queue</blockquote>\n"+
			"<blockquote expandable>🛡 <b>Group Tools</b>\n"+
			"│ 🛠 Ban, kick, mute, pause, skip\n"+
			"│ ✨ Personal playlists that follow you\n"+
			"│ 🤖 Make your own copy with /clone\n"+
			"│ 🎚 Smart load balancing across servers</blockquote>\n\n"+
			"🤫 <tg-spoiler>Tip: try /play followed by any song name</tg-spoiler>\n"+
			"👇 <i>Tap a button to start listening!</i>",
		userLink, toBoldUnicode("WELCOME TO KUST MUSIC"),
	)

	kb := telegram.InlineKeyboard{
		{{Text: "➕ " + toBoldUnicode("Add Me To Your Group"), URL: addToGroupURL, Style: telegram.StyleSuccess}},
		{
			{Text: "❓ " + toBoldUnicode("Commands & Help"), CallbackData: "show_help", Style: telegram.StylePrimary},
			{Text: "💬 " + toBoldUnicode("Community"), URL: communityURL},
		},
		{
			{Text: "🛟 " + toBoldUnicode("Support"), URL: supportURL},
			{Text: "📢 " + toBoldUnicode("Updates"), URL: updatesURL},
		},
		{{Text: "⭐ " + toBoldUnicode("Open Source"), URL: githubURL}},
	}
	return caption, kb
}

// randomTip mirrors random_tip: a tip line ~40% of the time, else "". Only
// includes tips for features this rewrite actually has — the original's
// /couple and clone-bot tips are dropped since those features aren't
// ported (couples intentionally removed; cloning was never in scope).
func randomTip() string {
	if rand.Float64() > 0.4 {
		return ""
	}
	tips := []string{
		"💡 Tip: use /playlist to manage your saved songs — replay them anytime, no re-searching.",
		"💡 Tip: hit ✨ while a song's playing to save it to your personal /playlist.",
		"💡 Tip: admins can /skip, /pause, /resume, or /stop anytime, no need to wait.",
		"💡 Tip: queueing up several songs at once cuts processing time a lot — try a playlist link.",
		"💡 Tip: I automatically pick whichever server has the least load right now.",
	}
	if supportUsername != "" {
		tips = append(tips, "💡 Tip: stuck or something broke? Contact support at "+supportUsername+".")
	}
	return tips[rand.Intn(len(tips))]
}

// The Python bot's two-stage progress feedback, replicated exactly: a bare
// snowflake goes out the instant the command lands (message.reply("❄️")),
// then that same message is edited into the fuller status text once the
// search resolves and playback is actually being set up
// (send_processing_message), and finally replaced by the player card.
const processingSnowflake = "❄️"

const processingStatus = "<blockquote><b>✨ Hold on…</b>\n" +
	"Your track is getting tuned, polished, and sent to the stage! 🥀\n" +
	"💕 <i>Streaming will start in just a moment…</i></blockquote>"

const processingStatusPremium = "<blockquote>✨ <b>ᴘʀᴇᴍɪᴜᴍ ᴅᴇᴛᴇᴄᴛᴇᴅ:</b> <b>ꜱᴘᴇᴇᴅ 𝟻x! 🚀</b>\n" +
	"<i>ᴘʟᴇᴀꜱᴇ ᴡᴀɪᴛ ᴀ ꜰᴇᴡ ꜱᴇᴄᴏɴᴅꜱ…</i></blockquote>"

// Shown when the last song finishes and the assistant leaves — otherwise
// the bot just went silent, which reads as it having crashed.
const queueEndedText = "<blockquote><b>✅ ǫᴜᴇᴜᴇ ᴇɴᴅᴇᴅ</b></blockquote>\n\n" +
	"That was the last track — I've left the voice chat.\n\n" +
	"🎵 Start again any time with <code>/play &lt;song name&gt;</code>"

func queueEndedKeyboard() telegram.InlineKeyboard {
	return telegram.InlineKeyboard{
		{{Text: "✨ ᴏᴘᴇɴ ᴘʟᴀʏʟɪsᴛ ✨", CallbackData: "playlist_page|0", Style: telegram.StyleSuccess}},
		{
			{Text: "💬 Community", URL: communityURL},
			{Text: "🛟 Support", URL: supportURL},
		},
	}
}

// playerCaption replicates setup_player_ui's base_caption exactly (English,
// "API Playback" mode — this bot has no local-playback mode).
func playerCaption(title, requesterMention, displayServer, modeText string) string {
	caption := "<blockquote><b>🎧 ᴋᴜsᴛ ✘ ᴍᴜsɪᴄ</b> ⏤͟͞● <i>ɴᴏᴡ sᴛʀєᴀᴍɪɴɢ</i></blockquote>\n" +
		"🎶 <b>" + htmlEscape(shortTitle(title, 42)) + "</b>\n\n" +
		"<blockquote>❍ <b>ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> " + requesterMention + "\n" +
		"❍ <b>ʟᴅs sᴇʀᴠᴇʀ:</b> " + htmlEscape(displayServer) + "\n" +
		"❍ <b>ᴍᴏᴅᴇ:</b> " + htmlEscape(modeText) +
		"</blockquote>"

	if tip := randomTip(); tip != "" {
		caption += "\n\n<i>" + htmlEscape(tip) + "</i>"
	}
	return caption
}

// progressBarStyled replicates get_progress_bar_styled exactly.
func progressBarStyled(elapsed, total float64) string {
	const barLength = 14
	if total <= 0 {
		return "Progress: N/A"
	}
	fraction := elapsed / total
	if fraction > 1 {
		fraction = 1
	}
	markerIndex := int(fraction * barLength)
	if markerIndex >= barLength {
		markerIndex = barLength - 1
	}
	bar := strings.Repeat("━", markerIndex) + "❄️" + strings.Repeat("─", barLength-markerIndex-1)
	return fmt.Sprintf("%s %s %s", formatTime(elapsed), bar, formatTime(total))
}

var iso8601Pattern = regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)

// iso8601ToSeconds parses yt-api's ISO-8601 duration strings ("PT3M33S")
// into seconds — matches main-music-rx's iso8601_to_seconds. Returns 0 for
// anything it can't parse (e.g. a live stream with no duration), same as
// the Python version treating that as "no progress bar total".
func iso8601ToSeconds(d string) float64 {
	m := iso8601Pattern.FindStringSubmatch(d)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	return float64(h*3600 + min*60 + s)
}

func formatTime(seconds float64) string {
	secs := int(seconds)
	m, s := secs/60, secs%60
	h, m := m/60, m%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// playerKeyboard replicates setup_player_ui's base_keyboard exactly: the
// same glyph-only control row, the progress-bar button, and the playlist
// button, all with the same callback_data.
func playerKeyboard(totalDurationSeconds float64) telegram.InlineKeyboard {
	return playerKeyboardAt(0, totalDurationSeconds)
}

// playerKeyboardAt is the same keyboard with the progress bar advanced to
// elapsed, so the first render and every later redraw share one layout.
func playerKeyboardAt(elapsed, total float64) telegram.InlineKeyboard {
	return telegram.InlineKeyboard{
		{
			{Text: "▷", CallbackData: "pause"},
			{Text: "II", CallbackData: "resume"},
			{Text: "‣‣I", CallbackData: "skip", Style: telegram.StylePrimary},
			{Text: "▢", CallbackData: "stop", Style: telegram.StyleDanger},
		},
		{{Text: progressBarStyled(elapsed, total), CallbackData: "progress"}},
		{{Text: "✨ ᴀᴅᴅ тσ ρℓαυℓιѕт ✨", CallbackData: "add_to_playlist", Style: telegram.StyleSuccess}},
	}
}
