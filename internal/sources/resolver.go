/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package sources

import (
	"fmt"
	"github.com/kustbots/kustmusic/internal/utils"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

// processDownload processes a track download based on platform metadata.
func processDownload(track *utils.TrackInfo) (string, error) {
	if track.CdnURL == "" {
		return "", errMissingCDNURL
	}

	if track.Key != "" && strings.EqualFold(track.Platform, "spotify") {
		return processSpotify(track)
	}

	return track.CdnURL, nil
}

func DlCachedTrack(cached *utils.PlayerCache, bot *td.Client) (string, error) {
	if cached.Platform == utils.DirectLink {
		return cached.URL, nil
	}

	if cached.Platform == utils.Telegram {
		return downloadTelegramFile(cached, bot)
	}

	dlBot := bot
	if DlBot != nil {
		dlBot = DlBot
	}

	return downloadViaWrapper(cached, dlBot)
}

func downloadViaWrapper(cached *utils.PlayerCache, dlBot *td.Client) (string, error) {
	wrapper := NewDlWrapper(cached.URL)
	if !wrapper.IsValid() {
		return "", fmt.Errorf("invalid cached URL: %s", cached.URL)
	}

	track, err := wrapper.GetTrack()
	if err != nil {
		return "", fmt.Errorf("get track info: %w", err)
	}

	path, err := wrapper.DownloadTrack(track, cached.IsVideo)
	if err != nil {
		return "", err
	}

	if utils.TelegramMessageRegex.MatchString(path) {
		return downloadFromTelegramMessage(dlBot, path)
	}

	return path, nil
}

func downloadTelegramFile(cached *utils.PlayerCache, bot *td.Client) (string, error) {
	file, err := bot.GetRemoteFile(cached.TrackID, nil)
	if err != nil {
		return "", err
	}

	d, err := file.Download(bot, 0, 0, 1, &td.DownloadFileOpts{Synchronous: true})
	if err != nil {
		return "", err
	}

	return d.Local.Path, nil
}

func downloadFromTelegramMessage(bot *td.Client, msgURL string) (string, error) {
	msg, err := utils.GetMessage(bot, msgURL)
	if err != nil {
		return "", fmt.Errorf("get telegram message: %w", err)
	}

	file, err := msg.Download(bot, 1, 0, 0, true)
	if err != nil {
		return "", err
	}

	if file == nil || file.Local == nil {
		return "", fmt.Errorf("failed to download file from Telegram message")
	}

	return file.Local.Path, nil
}
