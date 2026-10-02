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
	"github.com/kustbots/kustmusic/internal/utils"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func getHelpCategories() map[string]struct {
	Title   string
	Content string
	Markup  *td.ReplyMarkupInlineKeyboard
} {
	return map[string]struct {
		Title   string
		Content string
		Markup  *td.ReplyMarkupInlineKeyboard
	}{
		"help_user": {
			Title: "User Commands",
			Content: `<p>Commands available to all members of the chat.</p>

<details open>
  <summary>Playback</summary>
  <table bordered striped>
    <tr><th>Command</th><th>Description</th></tr>
    <tr><td><code>/play [song]</code></td><td>Play music from YouTube, Spotify, SoundCloud, and other supported platforms.</td></tr>
    <tr><td><code>/mix [song]</code></td><td>Create a mix playlist of related songs from YouTube.</td></tr>
    <tr><td><code>/vplay [song]</code></td><td>Play a video in the group video chat.</td></tr>
    <tr><td><code>/fplay [song]</code></td><td>Play a track immediately, skipping the current queue.</td></tr>
    <tr><td><code>/fvplay [song]</code></td><td>Play a video immediately, skipping the current queue.</td></tr>
  </table>
</details>

<details>
  <summary>General</summary>
  <table bordered striped>
    <tr><th>Command</th><th>Description</th></tr>
    <tr><td><code>/start</code></td><td>Start the bot or verify that it is online.</td></tr>
    <tr><td><code>/help</code></td><td>Open the interactive help menu.</td></tr>
    <tr><td><code>/ping</code></td><td>Display the bot's response time and system information.</td></tr>
    <tr><td><code>/privacy</code></td><td>View the bot's privacy policy.</td></tr>
    <tr><td><code>/queue</code></td><td>Display the current playback queue.</td></tr>
  </table>
</details>`,
			Markup: utils.BackHelpMenuKeyboard(),
		},
		"help_admin": {
			Title: "Admin Commands",
			Content: `<p>Commands available to chat administrators and authorized users.</p>

<details open>
  <summary>Playback Control</summary>
  <table bordered striped>
    <tr><th>Command</th><th>Description</th></tr>
    <tr><td><code>/skip</code></td><td>Skip the currently playing track.</td></tr>
    <tr><td><code>/pause</code></td><td>Pause playback.</td></tr>
    <tr><td><code>/resume</code></td><td>Resume playback.</td></tr>
    <tr><td><code>/seek [seconds]</code></td><td>Jump to a specific position in the current track.</td></tr>
    <tr><td><code>/mute</code></td><td>Mute the voice chat audio.</td></tr>
    <tr><td><code>/unmute</code></td><td>Unmute the voice chat audio.</td></tr>
  </table>
</details>

<details>
  <summary>Queue & Access</summary>
  <table bordered striped>
    <tr><th>Command</th><th>Description</th></tr>
    <tr><td><code>/remove [index]</code></td><td>Remove a track from the queue by its position.</td></tr>
    <tr><td><code>/loop [0-10]</code></td><td>Repeat the current track the specified number of times.</td></tr>
    <tr><td><code>/auth</code></td><td>Authorize a user to use administrator commands.</td></tr>
    <tr><td><code>/unauth</code></td><td>Remove a user's authorization.</td></tr>
    <tr><td><code>/authlist</code></td><td>Show all authorized users in the current chat.</td></tr>
    <tr><td><code>/join [link]</code></td><td>Join the assistant account to the group (creates link if omitted).</td></tr>
    <tr><td><code>/reload</code></td><td>Refresh administrator and invite link cache.</td></tr>
  </table>
</details>`,
			Markup: utils.BackHelpMenuKeyboard(),
		},
		"help_playlist": {
			Title: "Playlist Commands",
			Content: `<p>Create, organize, and manage your personal playlists.</p>

<details open>
  <summary>Playlist Management</summary>
  <table bordered striped>
    <tr><th>Command</th><th>Description</th></tr>
    <tr><td><code>/createplaylist</code></td><td>Create a new playlist.</td></tr>
    <tr><td><code>/deleteplaylist</code></td><td>Delete one of your playlists.</td></tr>
    <tr><td><code>/addtoplaylist</code></td><td>Add a track to a playlist.</td></tr>
    <tr><td><code>/removefromplaylist</code></td><td>Remove a track from a playlist.</td></tr>
    <tr><td><code>/playlistinfo</code></td><td>Display information about a playlist.</td></tr>
    <tr><td><code>/myplaylists</code></td><td>List all of your playlists.</td></tr>
  </table>
</details>`,
			Markup: utils.BackHelpMenuKeyboard(),
		},
		"help_autoplay": {
			Title: "Autoplay Commands",
			Content: `<p>Automatically continue playback with recommended tracks.</p>

<details open>
  <summary>Autoplay</summary>
  <table bordered striped>
    <tr><th>Command</th><th>Description</th></tr>
    <tr><td><code>/autoplay</code></td><td>Enable or disable autoplay. When enabled, recommended tracks are automatically queued when playback ends.</td></tr>
  </table>
</details>`,
			Markup: utils.BackHelpMenuKeyboard(),
		},
	}
}

func helpCallbackHandler(c *td.Client, cb *td.UpdateNewCallbackQuery) error {
	data := cb.DataString()

	user, err := c.GetUser(cb.SenderUserId)
	if err != nil {
		user = &td.User{FirstName: "User", Id: cb.SenderUserId}
	}

	helpCategories := getHelpCategories()

	if strings.Contains(data, "help_all") {
		_ = cb.Answer(c, 0, false, "Opening help menu...", "")
		response := fmt.Sprintf(
			"<h3>📖 Commands</h3>\n"+
				"<p>Hey %s, here is everything <b>%s</b> can do.</p>\n\n"+
				"<p><i>Pick a category below to see its commands.</i></p>",
			user.FirstName,
			c.Me.FirstName,
		)

		richMessage := &td.InputRichMessage{
			Source: &td.RichMessageSourceHtml{
				Text: response,
			},
		}
		_, _ = c.EditMessageText(cb.ChatId, &td.InputMessageRichMessage{Message: richMessage}, cb.MessageId, &td.EditMessageTextOpts{ReplyMarkup: utils.HelpMenuKeyboard()})
		return nil
	}

	if strings.Contains(data, "help_back") {
		_ = cb.Answer(c, 0, false, "Returning to main menu...", "")

		opts := &td.EditMessageTextOpts{ReplyMarkup: utils.AddMeMarkup(c.Me.Usernames.EditableUsername)}
		if _, err := c.EditMessageText(cb.ChatId, &td.InputMessageRichMessage{Message: welcomeMessage(user.FirstName, c.Me.FirstName, true)}, cb.MessageId, opts); err != nil {
			_, _ = c.EditMessageText(cb.ChatId, &td.InputMessageRichMessage{Message: welcomeMessage(user.FirstName, c.Me.FirstName, false)}, cb.MessageId, opts)
		}
		return nil
	}

	if category, ok := helpCategories[data]; ok {
		_ = cb.Answer(c, 0, false, category.Title, "")
		response := fmt.Sprintf("<h3>%s</h3>\n\n%s\n\n<i>Use the buttons below to go back.</i>", category.Title, category.Content)
		richMessage := &td.InputRichMessage{
			Source: &td.RichMessageSourceHtml{
				Text: response,
			},
		}
		_, _ = c.EditMessageText(cb.ChatId, &td.InputMessageRichMessage{Message: richMessage}, cb.MessageId, &td.EditMessageTextOpts{ReplyMarkup: category.Markup})
		return nil
	}

	_ = cb.Answer(c, 0, true, "Unknown help category.", "")
	return nil
}
