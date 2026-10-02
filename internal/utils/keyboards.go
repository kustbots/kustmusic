/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package utils

import (
	"fmt"
	"github.com/kustbots/kustmusic/internal/config"

	"github.com/AshokShau/gotdbot"
)

func cb(text, data string, style gotdbot.ButtonStyle) gotdbot.InlineKeyboardButton {
	return gotdbot.InlineKeyboardButton{
		Text: text,
		Type: &gotdbot.InlineKeyboardButtonTypeCallback{
			Data: []byte(data),
		},
		Style: style,
	}
}

func url(text, link string, style gotdbot.ButtonStyle) gotdbot.InlineKeyboardButton {
	return gotdbot.InlineKeyboardButton{
		Text: text,
		Type: &gotdbot.InlineKeyboardButtonTypeUrl{
			Url: link,
		},
		Style: style,
	}
}

var CloseBtn = cb("✖ Close", "vcplay_close", gotdbot.ButtonStyleDanger{})
var HomeBtn = cb("🏠 Home", "help_back", gotdbot.ButtonStylePrimary{})
var HelpBtn = cb("📖 Commands", "help_all", gotdbot.ButtonStyleDefault{})
var UserBtn = cb("👤 Users", "help_user", gotdbot.ButtonStyleDefault{})
var AdminBtn = cb("🛡 Admins", "help_admin", gotdbot.ButtonStyleDefault{})
var PlaylistBtn = cb("🎼 Playlist", "help_playlist", gotdbot.ButtonStyleDefault{})
var AutoplayBtn = cb("♾ Autoplay", "help_autoplay", gotdbot.ButtonStyleDefault{})

var CommunityBtn = url("👥 Community", config.Community, gotdbot.ButtonStylePrimary{})
var channelBtn = url("📢 Updates", config.SupportChannel, gotdbot.ButtonStyleDefault{})
var groupBtn = url("💬 Support", config.SupportGroup, gotdbot.ButtonStyleDefault{})

func SupportKeyboard() *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{channelBtn, groupBtn},
			{CloseBtn},
		},
	}
}

func SupportBtn() *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{channelBtn, groupBtn},
		},
	}
}

func SettingsKeyboard(playMode, adminMode string, cmdDelete bool, language string, autoplay bool) *gotdbot.ReplyMarkupInlineKeyboard {
	playText := "Everyone"
	if playMode == Admins {
		playText = "Admins"
	}

	deleteText := "False"
	if cmdDelete {
		deleteText = "True"
	}

	adminText := "Everyone"
	if adminMode == Admins {
		adminText = "Admins"
	}

	langText := "English"
	if language != "en" && language != "" {
		langText = language
	}

	autoplayText := "Disabled"
	if autoplay {
		autoplayText = "Enabled"
	}

	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{
				cb("Play Mode ➜", "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(playText, "settings_play", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb("Command Delete ➜", "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(deleteText, "settings_delete", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb("Admin Mode ➜", "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(adminText, "settings_admin", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb("Autoplay ➜", "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(autoplayText, "settings_autoplay", gotdbot.ButtonStyleDefault{}),
			},
			{
				cb("Language ➜", "settings_main", gotdbot.ButtonStyleDefault{}),
				cb(langText, "settings_lang", gotdbot.ButtonStyleDefault{}),
			},
			{CloseBtn},
		},
	}
}

func HelpMenuKeyboard() *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{UserBtn, AdminBtn},
			{PlaylistBtn, AutoplayBtn},
			{HomeBtn, CloseBtn},
		},
	}
}

func BackHelpMenuKeyboard() *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{HelpBtn, HomeBtn},
			{CloseBtn, CommunityBtn},
		},
	}
}

func ControlButtons(mode string) *gotdbot.ReplyMarkupInlineKeyboard {
	skipBtn := cb("⏭ Skip", "play_skip", gotdbot.ButtonStyleDefault{})
	stopBtn := cb("⏹ Stop", "play_stop", gotdbot.ButtonStyleDanger{})
	pauseBtn := cb("⏸ Pause", "play_pause", gotdbot.ButtonStylePrimary{})
	resumeBtn := cb("▶️ Resume", "play_resume", gotdbot.ButtonStyleSuccess{})
	muteBtn := cb("🔇 Mute", "play_mute", gotdbot.ButtonStyleDefault{})
	unmuteBtn := cb("🔊 Unmute", "play_unmute", gotdbot.ButtonStyleSuccess{})
	addToPlaylistBtn := cb("➕ Save to playlist", "play_add_to_list", gotdbot.ButtonStylePrimary{})

	switch mode {

	case "play":
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{pauseBtn, skipBtn, stopBtn},
				{addToPlaylistBtn, CloseBtn},
			},
		}

	case "pause":
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{resumeBtn, skipBtn, stopBtn},
				{CloseBtn},
			},
		}

	case "resume":
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{skipBtn, stopBtn, pauseBtn},
				{CloseBtn},
			},
		}

	case "mute":
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{unmuteBtn, skipBtn, stopBtn},
				{CloseBtn},
			},
		}

	case "unmute":
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{skipBtn, stopBtn, muteBtn},
				{CloseBtn},
			},
		}

	default:
		return &gotdbot.ReplyMarkupInlineKeyboard{
			Rows: [][]gotdbot.InlineKeyboardButton{
				{CloseBtn},
			},
		}
	}
}

func AddMeMarkup(username string) *gotdbot.ReplyMarkupInlineKeyboard {

	addMeBtn := url(
		"➕ Add me to your group",
		fmt.Sprintf("https://t.me/%s?startgroup=true", username),
		gotdbot.ButtonStylePrimary{},
	)

	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{addMeBtn},
			{HelpBtn},
			{channelBtn, groupBtn},
			{CommunityBtn},
		},
	}
}

func PlayNowButton(trackID string) gotdbot.InlineKeyboardButton {
	return cb("▶️ Play now", fmt.Sprintf("play_now_%s", trackID), gotdbot.ButtonStyleDanger{})
}

func QueueMarkup(trackID string) *gotdbot.ReplyMarkupInlineKeyboard {
	return &gotdbot.ReplyMarkupInlineKeyboard{
		Rows: [][]gotdbot.InlineKeyboardButton{
			{PlayNowButton(trackID), CloseBtn},
		},
	}
}
