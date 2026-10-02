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
	"github.com/kustbots/kustmusic/internal/db"
	"github.com/kustbots/kustmusic/internal/player"
	"github.com/kustbots/kustmusic/internal/utils"
	"html"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func playCallbackHandler(c *td.Client, cb *td.UpdateNewCallbackQuery) error {
	data := cb.DataString()
	if !adminModeCB(c, cb) {
		return td.EndGroups
	}

	chatID := cb.ChatId
	user, err := c.GetUser(cb.SenderUserId)
	if err != nil {
		user = &td.User{FirstName: "Unknown", Id: cb.SenderUserId}
	}

	if !cache.ChatCache.IsActive(chatID) {
		text := "There is no active playback."
		_ = cb.Answer(c, 0, false, text, "")
		_, _ = cb.EditMessageText(c, text, &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons(""), ParseMode: "HTML", DisableWebPagePreview: true})
		return nil
	}

	currentTrack := cache.ChatCache.GetPlayingTrack(chatID)
	if currentTrack == nil {
		_ = cb.Answer(c, 0, false, "There is no active playback.", "")
		_, _ = cb.EditMessageText(c, "There is no active playback.", &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons(""), ParseMode: "HTML", DisableWebPagePreview: true})
		return nil
	}

	by := html.EscapeString(user.FirstName)
	card := func(header, footer, mode string) {
		_, _ = cb.EditMessageText(c, utils.PlayerCard(header, currentTrack, footer), utils.CardOpts(currentTrack, utils.ControlButtons(mode)))
	}

	switch {
	case strings.Contains(data, "play_skip"):
		if err := player.Calls.PlayNext(c, chatID); err != nil {
			_ = cb.Answer(c, 0, false, "Unable to skip the current track.", "")
			_, _ = cb.EditMessageText(c, "Unable to skip the current track.", &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons(""), ParseMode: "HTML", DisableWebPagePreview: true})
			return nil
		}
		_ = cb.Answer(c, 0, false, "Track skipped.", "")
		_ = c.DeleteMessages(chatID, []int64{cb.MessageId}, &td.DeleteMessagesOpts{Revoke: true})
		return nil

	case strings.Contains(data, "play_stop"):
		if err := player.Calls.Stop(chatID, false); err != nil {
			_ = cb.Answer(c, 0, false, "Unable to stop playback.", "")
			_, _ = cb.EditMessageText(c, "Unable to stop playback.", &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons(""), ParseMode: "HTML", DisableWebPagePreview: true})
			return nil
		}

		msg := utils.Notice("⏹ Playback stopped", "Stopped by "+by)
		_ = cb.Answer(c, 0, false, "Playback stopped.", "")
		_, err := cb.EditMessageText(c, msg, &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons(""), ParseMode: "HTML", DisableWebPagePreview: true})
		return err

	case strings.Contains(data, "play_pause"):
		if _, err = player.Calls.Pause(chatID); err != nil {
			_ = cb.Answer(c, 0, false, "Unable to pause playback.", "")
			_, _ = cb.EditMessageText(c, "Unable to pause playback.", &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons(""), ParseMode: "HTML", DisableWebPagePreview: true})
			return nil
		}
		_ = cb.Answer(c, 0, false, "Playback paused.", "")
		card(utils.HeaderPaused, "<i>Paused by "+by+"</i>", "pause")
		return nil

	case strings.Contains(data, "play_resume"):
		if _, err := player.Calls.Resume(chatID); err != nil {
			_ = cb.Answer(c, 0, false, "Unable to resume playback.", "")
			_, _ = cb.EditMessageText(c, "Unable to resume playback.", &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons("pause"), ParseMode: "HTML", DisableWebPagePreview: true})
			return nil
		}
		_ = cb.Answer(c, 0, false, "Playback resumed.", "")
		card(utils.HeaderPlaying, "<i>Resumed by "+by+"</i>", "resume")
		return nil

	case strings.Contains(data, "play_mute"):
		if _, err := player.Calls.Mute(chatID); err != nil {
			_ = cb.Answer(c, 0, false, "Unable to mute playback.", "")
			_, _ = cb.EditMessageText(c, "Unable to mute playback.", &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons("mute"), ParseMode: "HTML", DisableWebPagePreview: true})
			return nil
		}
		_ = cb.Answer(c, 0, false, "Playback muted.", "")
		card(utils.HeaderMuted, "<i>Muted by "+by+"</i>", "mute")
		return nil

	case strings.Contains(data, "play_unmute"):
		if _, err := player.Calls.Unmute(chatID); err != nil {
			_ = cb.Answer(c, 0, false, "Unable to unmute playback.", "")
			_, _ = cb.EditMessageText(c, "Unable to unmute playback.", &td.EditTextMessageOpts{ReplyMarkup: utils.ControlButtons("unmute"), ParseMode: "HTML"})
			return nil
		}
		_ = cb.Answer(c, 0, false, "Playback unmuted.", "")
		card(utils.HeaderPlaying, "<i>Unmuted by "+by+"</i>", "unmute")
		return nil

	case strings.Contains(data, "play_add_to_list"):
		playlists, err := db.Instance.GetUserPlaylists(cb.SenderUserId)
		if err != nil {
			_ = cb.Answer(c, 0, false, "Unable to fetch playlists.", "")
			return nil
		}

		var playlistID string
		if len(playlists) == 0 {
			playlistID, err = db.Instance.CreatePlaylist("My Playlist", cb.SenderUserId)
			if err != nil {
				_ = cb.Answer(c, 0, false, "Unable to create playlist.", "")
				return nil
			}
		} else {
			playlistID = playlists[0].ID
		}

		song := db.Song{
			URL:      currentTrack.URL,
			Name:     currentTrack.Name,
			TrackID:  currentTrack.TrackID,
			Duration: currentTrack.Duration,
			Platform: currentTrack.Platform,
		}

		err = db.Instance.AddSongToPlaylist(playlistID, song)
		if err != nil {
			_ = cb.Answer(c, 0, false, "Unable to add track to playlist.", "")
			return nil
		}

		playlist, err := db.Instance.GetPlaylist(playlistID)
		if err != nil {
			_ = cb.Answer(c, 0, false, "Playlist not found.", "")
			return nil
		}

		_ = cb.Answer(c, 0, false, fmt.Sprintf("Track \"%s\" added to playlist \"%s\".", song.Name, playlist.Name), "")
		return nil

	case strings.HasPrefix(data, "play_now_"):
		trackID := strings.TrimPrefix(data, "play_now_")
		if ok := cache.ChatCache.MoveTrackToFront(chatID, trackID); !ok {
			_ = cb.Answer(c, 0, false, "Track not found in queue.", "")
			return nil
		}

		_ = cb.Answer(c, 0, false, "Playing now.", "")
		if err = player.Calls.PlayNext(c, chatID); err != nil {
			_ = cb.Answer(c, 0, false, "Unable to play the track.", "")
			return nil
		}

		_ = c.DeleteMessages(chatID, []int64{cb.MessageId}, &td.DeleteMessagesOpts{Revoke: true})
		return nil
	}

	card(utils.HeaderPlaying, "", "resume")
	return nil
}

func vcPlayHandler(c *td.Client, cb *td.UpdateNewCallbackQuery) error {
	data := cb.DataString()

	if strings.Contains(data, "vcplay_close") {
		_ = cb.Answer(c, 0, false, "Closing panel.", "")
		_ = c.DeleteMessages(cb.ChatId, []int64{cb.MessageId}, &td.DeleteMessagesOpts{Revoke: true})
		return nil
	}

	c.Logger.Info("Received vcplay callback", "data", data)
	return nil
}
