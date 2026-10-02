/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package player

import (
	"context"
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/db"
	"github.com/kustbots/kustmusic/internal/sources"
	"github.com/kustbots/kustmusic/internal/utils"
	"html"
	"log/slog"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

// errorKind classifies a Telegram group call error for retry strategy.
type errorKind int

const (
	errFatal     errorKind = iota // return immediately with a user-facing message
	errRetryOnce                  // retry the same assistant once (e.g. participants race)
	errRotate                     // try a different assistant (flood/frozen/channels)
	errUnknown                    // log and return as-is
)

func classifyError(err error) errorKind {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "is closed"),
		strings.Contains(msg, "GROUPCALL_FORBIDDEN"):
		return errFatal
	case strings.Contains(msg, "GROUPCALL_INVALID"):
		return errFatal
	case strings.Contains(msg, "GROUPCALL_ADD_PARTICIPANTS_FAILED"),
		strings.Contains(msg, "Timeout while fetching data"),
		strings.Contains(msg, "INTERDC_X_CALL_ERROR"):
		return errRetryOnce
	case strings.Contains(msg, "CHANNELS_TOO_MUCH"),
		strings.Contains(msg, "FROZEN_METHOD_INVALID"),
		strings.Contains(msg, "FLOOD_WAIT_X"),
		strings.Contains(msg, "limiting join attempts"),
		strings.Contains(msg, "USER_DEACTIVATED"):
		return errRotate
	default:
		return errUnknown
	}
}

func fatalMessage(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "is closed") || strings.Contains(msg, "GROUPCALL_FORBIDDEN") {
		return errors.New("<b>📵 No active video chat</b>\n<blockquote>Start a video chat in this group, then try again.</blockquote>")
	}

	if strings.Contains(msg, "GROUPCALL_INVALID") {
		return errors.New("<b>GROUPCALL_INVALID:</b> start a video chat and try again.\n\nIf the problem persists, please report it to the developer.")
	}
	return err
}

func (c *TelegramCalls) playMediaWithAssistant(bot *td.Client, chatID int64, filePath string, video bool, ffmpegParameters string, call *AssistantAccount, index int) error {
	if chatID > 0 {
		return errors.New("private calls are not supported for media playback")
	}

	if err := c.ensureAssistantMember(bot, chatID, call, index); err != nil {
		cache.ChatCache.ClearChat(chatID)
		return err
	}

	logger.Debug("Playing media in chat", "id", chatID, "path", filePath, "index", index)

	if ffmpegParameters == "" {
		c.clearPlayedTimeOffset(chatID)
	}

	mediaDesc := getMediaDescription(filePath, video, chatID, ffmpegParameters)
	if err := c.startCallStream(context.Background(), call, chatID, mediaDesc); err != nil {
		cache.ChatCache.ClearChat(chatID)
		return err
	}

	if db.Instance.GetLoggerStatus() {
		go sendLogger(bot, chatID, cache.ChatCache.GetPlayingTrack(chatID))
	}

	return nil
}

func (c *TelegramCalls) PlayMedia(bot *td.Client, chatID int64, filePath string, video bool, ffmpegParameters ...string) error {
	ffParam := ""
	if len(ffmpegParameters) > 0 {
		ffParam = ffmpegParameters[0]
	}

	call, index, err := c.GetAccount(chatID)
	if err != nil {
		return err
	}

	err = c.playMediaWithAssistant(bot, chatID, filePath, video, ffParam, call, index)
	if err == nil {
		_ = db.Instance.SetAssistant(chatID, index)
		return nil
	}

	switch classifyError(err) {
	case errFatal:
		return fatalMessage(err)

	case errUnknown:
		logger.Error("Failed to play the media", "error", err, "index", index, "chatID", chatID)
		return fmt.Errorf("playback failed: %w", err)

	case errRetryOnce:
		err = c.playMediaWithAssistant(bot, chatID, filePath, video, ffParam, call, index)
		if err == nil {
			_ = db.Instance.SetAssistant(chatID, index)
			return nil
		}
		if classifyError(err) != errRotate {
			return fmt.Errorf("playback failed: %w", err)
		}
		fallthrough // GROUPCALL_ADD_PARTICIPANTS_FAILED can escalate to rotation

	case errRotate:
		c.evictAssistant(chatID, index, err)
	}

	return c.rotateAndPlay(bot, chatID, filePath, video, ffParam, map[int]bool{index: true}, err)
}

func (c *TelegramCalls) evictAssistant(chatID int64, index int, err error) {
	_ = db.Instance.RemoveAssistant(chatID)
	if strings.Contains(err.Error(), "CHANNELS_TOO_MUCH") {
		go func() { _, _ = c.LeaveAllForAccount(index) }()
	}
}

func (c *TelegramCalls) rotateAndPlay(bot *td.Client, chatID int64, filePath string, video bool, ffmpegParameters string, tried map[int]bool, lastErr error) error {
	for {
		call, nextIndex, err := c.nextUntried(tried)
		if err != nil {
			logger.Error("Playback failed after full rotation", "error", lastErr, "chatID", chatID)
			return fmt.Errorf("playback failed after trying all assistants: %w", lastErr)
		}
		tried[nextIndex] = true

		err = c.playMediaWithAssistant(bot, chatID, filePath, video, ffmpegParameters, call, nextIndex)
		if err == nil {
			_ = db.Instance.SetAssistant(chatID, nextIndex)
			return nil
		}
		lastErr = err

		switch classifyError(err) {
		case errRetryOnce:
			err = c.playMediaWithAssistant(bot, chatID, filePath, video, ffmpegParameters, call, nextIndex)
			if err == nil {
				_ = db.Instance.SetAssistant(chatID, nextIndex)
				return nil
			}
			lastErr = err
			if classifyError(err) == errRotate {
				c.evictAssistant(chatID, nextIndex, err)
				continue
			}
			return fmt.Errorf("playback failed: %w", lastErr)

		case errRotate:
			c.evictAssistant(chatID, nextIndex, err)
			continue

		default:
			// errFatal or errUnknown — stop rotating.
			return fmt.Errorf("playback failed: %w", lastErr)
		}
	}
}

func (c *TelegramCalls) prepareTrackDownload(bot *td.Client, song *utils.PlayerCache, reply *td.Message) error {
	if song.FilePath != "" {
		return nil
	}

	dlPath, err := sources.DlCachedTrack(song, bot)
	song.FilePath = dlPath
	if err != nil || song.FilePath == "" {
		_, _ = reply.EditText(bot, utils.Notice("⚠️ Download failed", "Skipping to the next track…"), nil)
		return err
	}

	return nil
}

func (c *TelegramCalls) PlayNext(bot *td.Client, chatID int64) error {
	loop := cache.ChatCache.GetLoopCount(chatID)
	if loop > 0 {
		cache.ChatCache.SetLoopCount(chatID, loop-1)
		if currentsSong := cache.ChatCache.GetPlayingTrack(chatID); currentsSong != nil {
			return c.playTrack(bot, chatID, currentsSong)
		}
	}

	cache.ChatCache.RemoveCurrentSong(chatID)
	if nextSong := cache.ChatCache.GetPlayingTrack(chatID); nextSong != nil {
		return c.playTrack(bot, chatID, nextSong)
	}

	lastTrackID := cache.ChatCache.GetLastAutoplayTrackID(chatID)
	if lastTrackID != "" && cache.ChatCache.GetAutoplay(chatID) {
		return c.handleAutoplay(bot, chatID, lastTrackID)
	}

	return c.handleNoSong(bot, chatID)
}

func (c *TelegramCalls) handleNoSong(bot *td.Client, chatID int64) error {
	_ = c.Stop(chatID, false)
	_, _ = bot.SendTextMessage(chatID, utils.Notice("✅ Queue finished", "Add more songs with /play."), nil)
	return nil
}

func (c *TelegramCalls) playTrack(bot *td.Client, chatID int64, song *utils.PlayerCache) error {
	reply, err := bot.SendTextMessage(chatID, utils.Notice("📥 Loading next track…", html.EscapeString(song.Name)), nil)
	if err != nil {
		slog.Info("[playTrack] Failed to send message", "error", err)
		return err
	}

	if err = c.prepareTrackDownload(bot, song, reply); err != nil {
		return c.PlayNext(bot, chatID)
	}

	if err = c.PlayMedia(bot, chatID, song.FilePath, song.IsVideo); err != nil {
		_, _ = reply.EditText(bot, utils.PlaybackError(err), &td.EditTextMessageOpts{ParseMode: "HTML", DisableWebPagePreview: true})
		return nil
	}

	if song.Duration == 0 {
		song.Duration = utils.GetMediaDuration(song.FilePath)
	}

	text := utils.PlayerCard(utils.HeaderPlaying, song, "")

	_, err = reply.EditText(bot, text, utils.CardOpts(song, utils.ControlButtons("play")))

	if err != nil {
		slog.Info("[playTrack] Failed to edit message", "error", err)
		return nil
	}

	return nil
}
