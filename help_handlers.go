package main

import (
	"github.com/kustbots/kustmusic/internal/telegram"
)

// The help menu, ported from the Python bot's show_help / help_music /
// help_admin / help_util callbacks. The original's fourth category was
// "❤️ Couple Suggestion"; couples aren't part of this rewrite (dropped
// deliberately, see main.go's package comment), so that slot documents
// /playlist instead — which this bot does have and the original never gave
// its own help page.
func helpMenuKeyboard() telegram.InlineKeyboard {
	return telegram.InlineKeyboard{
		{
			{Text: "🎵 Music Controls", CallbackData: "help_music", Style: telegram.StylePrimary},
			{Text: "🛡️ Admin Tools", CallbackData: "help_admin", Style: telegram.StylePrimary},
		},
		{
			{Text: "✨ Playlist", CallbackData: "help_playlist", Style: telegram.StylePrimary},
			{Text: "🔍 Utility", CallbackData: "help_util", Style: telegram.StylePrimary},
		},
		{
			{Text: "🏠 Home", CallbackData: "go_back", Style: telegram.StyleSuccess},
		},
	}
}

func helpBackKeyboard() telegram.InlineKeyboard {
	return telegram.InlineKeyboard{
		{{Text: "🔙 Back", CallbackData: "show_help", Style: telegram.StyleSuccess}},
	}
}

// supportKeyboard is the Python bot's get_support_markup — attached to
// error messages so a user who hits a failure has somewhere to go.
func supportKeyboard() telegram.InlineKeyboard {
	return telegram.InlineKeyboard{
		{
			{Text: "💬 Community", URL: communityURL},
			{Text: "🛟 Support", URL: supportURL},
		},
		{{Text: "⭐ Open Source", URL: githubURL}},
	}
}

const helpMenuText = "📜 <b>Choose a category to explore commands:</b>"

// helpCmd is one command and what it does, in its own quote block so a page
// reads as a stack of cards rather than a wall of text. cmd and desc are
// already HTML-safe.
func helpCmd(cmd, desc string) string {
	return "<blockquote>➜ <code>" + cmd + "</code>\n• " + desc + "</blockquote>"
}

var helpMusicText = "🎵 <b>ᴍᴜsɪᴄ &amp; ᴘʟᴀʏʙᴀᴄᴋ</b>\n" +
	helpCmd("/play &lt;song name or URL&gt;", "Play a song (YouTube/Spotify/Resso/Apple Music/SoundCloud).") +
	helpCmd("/queue", "Show the queue, with per-song play-now and remove buttons.") +
	helpCmd("/skip", "Skip the currently playing song.") +
	helpCmd("/pause", "Pause the current stream.") +
	helpCmd("/resume", "Resume a paused stream.") +
	helpCmd("/stop</code> or <code>/end", "Stop playback and clear the queue. <i>(Admins only)</i>")

var helpAdminText = "🛡️ <b>ᴀᴅᴍɪɴ &amp; ᴍᴏᴅᴇʀᴀᴛɪᴏɴ</b> <i>(admins only)</i>\n" +
	helpCmd("/mute @user", "Mute a user indefinitely.") +
	helpCmd("/unmute @user", "Unmute a previously muted user.") +
	helpCmd("/tmute @user &lt;minutes&gt;", "Temporarily mute for a set duration.") +
	helpCmd("/kick @user", "Kick (ban + unban) a user immediately.") +
	helpCmd("/ban @user", "Ban a user.") +
	helpCmd("/unban @user", "Unban a previously banned user.")

var helpPlaylistText = "✨ <b>ᴘʟᴀʏʟɪsᴛ</b>\n" +
	helpCmd("/playlist", "View your saved songs — tap any one to play or remove it.") +
	"<blockquote>➜ <b>✨ Add to Playlist</b>\n• Tap this on the player card while a song is playing to save it.\n" +
	"• Your playlist is personal and follows you across every group.</blockquote>"

var helpUtilText = "🔍 <b>ᴜᴛɪʟɪᴛʏ &amp; ᴇxᴛʀᴀs</b>\n" +
	helpCmd("/ping", "Check the bot's response time and uptime.") +
	helpCmd("/clear", "Clear the entire queue. <i>(Admins only)</i>") +
	helpCmd("/clone &lt;bot token&gt;", "Run your own copy of this bot. Send it to me in private.") +
	helpCmd("/start", "Show the home screen.")

func (a *App) cbShowHelp(cq *telegram.CallbackQuery) {
	a.editCard(cq, helpMenuText, helpMenuKeyboard())
}

func (a *App) cbHelpPage(cq *telegram.CallbackQuery, text string) {
	a.editCard(cq, text, helpBackKeyboard())
}

// editCard rewrites the message a button was tapped on, whether it's a
// plain text message or a media card.
//
// This is why "Commands & Help" did nothing. The home screen is sent as an
// animation, and Bot API refuses editMessageText on a message carrying
// media — it wants editMessageCaption. Every tap failed silently, because
// the error was discarded. The Python bot never had the problem: it talked
// MTProto, where a single edit call handles both cases, so the port
// inherited a call that simply doesn't exist over HTTP.
//
// Falls back to replacing the message outright if neither edit works, so a
// tap always produces something rather than nothing.
func (a *App) editCard(cq *telegram.CallbackQuery, text string, kb telegram.InlineKeyboard) {
	chatID, messageID := cq.Message.Chat.ID, cq.Message.MessageID

	var err error
	if cq.Message.Caption != "" {
		err = a.tg.EditMessageCaption(chatID, messageID, text, kb)
	} else {
		err = a.tg.EditMessageText(chatID, messageID, text, kb)
	}
	if err == nil {
		return
	}

	// Caption/text detection can still be wrong — a media message with no
	// caption at all looks like a text message here — so try the other
	// method before giving up on editing.
	if cq.Message.Caption != "" {
		err = a.tg.EditMessageText(chatID, messageID, text, kb)
	} else {
		err = a.tg.EditMessageCaption(chatID, messageID, text, kb)
	}
	if err == nil {
		return
	}

	a.log.Warn("couldn't edit message for help navigation, sending a new one", "chat_id", chatID, "err", err)
	_ = a.tg.DeleteMessage(chatID, messageID)
	_, _ = a.tg.SendMessage(chatID, text, kb)
}

// cbGoBack returns to the home screen. The original re-rendered it in
// place; this sends a fresh one because the home screen is an animation
// (sendAnimation) and Telegram can't edit a text message into one.
func (a *App) cbGoBack(cq *telegram.CallbackQuery) {
	name := "Music Lover"
	if cq.From.FirstName != "" {
		name = cq.From.FirstName
	}
	userLink := mentionUser(cq.From.ID, name)
	caption, kb := buildHomeScreen(userLink)
	_ = a.tg.DeleteMessage(cq.Message.Chat.ID, cq.Message.MessageID)
	a.sendHome(cq.Message.Chat.ID, caption, kb)
}
