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
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/db"
	"github.com/kustbots/kustmusic/internal/player"
	"github.com/kustbots/kustmusic/internal/sources"
	"github.com/kustbots/kustmusic/internal/utils"
	"html"
	"slices"
	"strings"

	td "github.com/kustbots/gotdbot"
)

// videoDisabledNotice is shown when /vplay is used while video playback is turned off.
var videoDisabledNotice = utils.Notice("🎥 Video playback is off", "Video streaming is disabled right now to keep audio smooth for everyone. Use /play for music.")

// playHandler handles the /play command.
func playHandler(c *td.Client, m *td.Message) error {
	if !playMode(c, m) {
		return td.EndGroups
	}

	return handlePlay(c, m, false, false)
}

// vPlayHandler handles the /vplay command.
func vPlayHandler(c *td.Client, m *td.Message) error {
	if !playMode(c, m) {
		return td.EndGroups
	}

	if !config.EnableVideoPlayback {
		_, _ = m.ReplyText(c, videoDisabledNotice, nil)
		return td.EndGroups
	}
	return handlePlay(c, m, true, false)
}

// fPlayHandler handles the /fplay command.
func fPlayHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	return handlePlay(c, m, false, true)
}

// fVPlayHandler handles the /fvplay command.
func fVPlayHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	if !config.EnableVideoPlayback {
		_, _ = m.ReplyText(c, videoDisabledNotice, nil)
		return td.EndGroups
	}
	return handlePlay(c, m, true, true)
}

func handlePlay(c *td.Client, m *td.Message, isVideo bool, force bool) error {
	chatID := m.ChatId

	if queueLen := cache.ChatCache.GetQueueLength(chatID); queueLen > 10 {
		_, _ = m.ReplyText(c, utils.Notice("📛 Queue is full", "Maximum 10 tracks. Use /end to clear it."), nil)
		return td.EndGroups
	}

	isReply := m.ReplyToMessageID() != 0
	args := Args(m)
	url := getUrl(c, m, isReply)

	rMsg := m
	var err error
	if isReply && args == "" && url == "" {
		r, err := m.GetRepliedMessage(c)
		if err == nil && r != nil {
			args = r.Text()
		}
	}

	input := coalesce(url, args)

	if strings.HasPrefix(input, "tgpl_") {
		playlist, err := db.Instance.GetPlaylist(input)
		if err != nil {
			_, err = m.ReplyText(c, utils.Notice("❌ Playlist not found", ""), nil)
			return err
		}

		tracks := db.ConvertSongsToTracks(playlist.Songs)
		if len(tracks) == 0 {
			_, err = m.ReplyText(c, utils.Notice("❌ Playlist is empty", ""), nil)
			return err
		}

		updater, err := m.ReplyText(c, utils.Notice("🔎 Loading playlist…", "Collecting the tracks, one moment."), nil)
		if err != nil {
			c.Logger.Warn("failed to send message", "error", err)
			return td.EndGroups
		}

		return handleMultipleTracks(c, m, updater, tracks, chatID, isVideo, force)
	}

	if match := utils.TelegramMessageRegex.FindStringSubmatch(input); match != nil {
		rMsg, err = utils.GetMessage(c, input)
		if err != nil {
			c.Logger.Warn("failed to parse message", "error", err.Error())
			_, err = m.ReplyText(c, "Invalid Telegram link.", nil)
			return err
		}
	} else if isReply {
		rMsg, err = m.GetRepliedMessage(c)
		if err != nil {
			_, err = m.ReplyText(c, "Invalid reply message.", nil)
			return err
		}
	}

	if isValid := isValidMedia(rMsg); isValid {
		isReply = true
	}

	if url == "" && args == "" && (!isReply || !isValidMedia(rMsg)) {
		_, _ = m.ReplyText(c, "<b>🎧 PLAY A SONG</b>\n"+utils.Divider+"\n<code>/play song name or link</code>\n\n<b>Supported platforms</b><blockquote>🎬 YouTube\n🟢 Spotify\n🎶 JioSaavn\n🍎 Apple Music</blockquote>", &td.SendTextMessageOpts{ReplyMarkup: utils.SupportKeyboard(), ParseMode: "HTML"})
		return td.EndGroups
	}

	updater, err := m.ReplyText(c, utils.Notice("🔎 Searching…", "Finding your track and getting it ready."), nil)
	if err != nil {
		c.Logger.Warn("failed to send message", "error", err)
		return td.EndGroups
	}

	if isReply && isValidMedia(rMsg) {
		return handleMedia(c, m, updater, rMsg, chatID, isVideo, force)
	}

	wrapper := sources.NewDlWrapper(input)
	if url != "" {
		if !wrapper.IsValid() {
			_, _ = updater.EditText(c, "<b>❌ Unsupported link</b>\n\n<b>Supported platforms</b><blockquote>🎬 YouTube\n🟢 Spotify\n🎶 JioSaavn\n🍎 Apple Music</blockquote>", &td.EditTextMessageOpts{ReplyMarkup: utils.SupportKeyboard(), ParseMode: "HTML"})
			return td.EndGroups
		}

		trackInfo, err := wrapper.GetInfo()
		if err != nil {
			_, _ = updater.EditText(c, utils.Notice("❌ Couldn't fetch track info", utils.FriendlyError(err)), nil)
			return td.EndGroups
		}

		if trackInfo.Results == nil || len(trackInfo.Results) == 0 {
			_, _ = updater.EditText(c, utils.Notice("😕 No tracks found", ""), nil)
			return td.EndGroups
		}

		return handleUrl(c, m, updater, trackInfo, chatID, isVideo, force)
	}

	return handleTextSearch(c, m, updater, wrapper, chatID, isVideo, force)
}

// handleMedia handles playing media from a message.
func handleMedia(c *td.Client, m *td.Message, updater *td.Message, dlMsg *td.Message, chatId int64, isVideo bool, force bool) error {
	file, fileName := getFile(dlMsg)
	if file == nil {
		_, err := updater.EditText(c, utils.Notice("❌ No playable media in that message", ""), nil)
		return err
	}

	if file.Size > config.MaxFileSize {
		_, err := updater.EditText(c, utils.Notice("❌ File too large", fmt.Sprintf("Maximum size is %d MB.", config.MaxFileSize/(1024*1024))), nil)
		if err != nil {
			c.Logger.Warn("Edit message failed", "error", err)
		}
		return nil
	}

	fileId := dlMsg.RemoteFileID()
	if _track := cache.ChatCache.GetTrackIfExists(chatId, fileId); _track != nil {
		_, err := updater.EditText(c, utils.Notice("♻️ Already in the queue", "That track is playing or queued."), nil)
		return err
	}

	dur := dlMsg.RemoteDuration()
	link, err := dlMsg.GetLink(c)
	if err != nil {
		c.Logger.Warn("Failed to get file link", "error", err)
		link.Link = ""
	}

	saveCache := utils.PlayerCache{
		URL: link.Link, Name: fileName, User: firstName(c, m), TrackID: fileId,
		Duration: dur, IsVideo: isVideo, Platform: utils.Telegram,
	}

	if queued, err := enqueueTrack(c, updater, chatId, &saveCache, force); queued {
		return err
	}

	file, err = dlMsg.Download(c, 1, 0, 0, true)
	if err != nil {
		cache.ChatCache.RemoveCurrentSong(chatId)
		_, err = updater.EditText(c, utils.Notice("❌ Couldn't load this track", utils.FriendlyError(err)), nil)
		return err
	}

	filePath := file.Local.Path
	if dur == 0 {
		dur = utils.GetMediaDuration(filePath)
		saveCache.Duration = dur
	}

	saveCache.FilePath = filePath

	if err = player.Calls.PlayMedia(c, chatId, saveCache.FilePath, saveCache.IsVideo); err != nil {
		cache.ChatCache.RemoveCurrentSong(chatId)
		_, err = updater.EditText(c, utils.PlaybackError(err), &td.EditTextMessageOpts{ParseMode: "HTML", DisableWebPagePreview: true})
		return err
	}

	return sendStartedStreaming(c, updater, &saveCache)
}

// handleTextSearch handles a text search for a song.
func handleTextSearch(c *td.Client, m *td.Message, updater *td.Message, wrapper *sources.DlWrapper, chatId int64, isVideo bool, force bool) error {
	searchResult, err := wrapper.Search()
	if err != nil {
		_, err = updater.EditText(c, utils.Notice("❌ Search failed", utils.FriendlyError(err)), nil)
		return err
	}

	if searchResult.Results == nil || len(searchResult.Results) == 0 {
		_, err = updater.EditText(c, utils.Notice("😕 No results", "Try a different name or paste a link."), nil)
		return err
	}

	song := searchResult.Results[0]
	if _track := cache.ChatCache.GetTrackIfExists(chatId, song.Id); _track != nil {
		_, err := updater.EditText(c, utils.Notice("♻️ Already in the queue", "That track is playing or queued."), nil)
		return err
	}

	return handleSingleTrack(c, m, updater, song, "", chatId, isVideo, force)
}

// handleUrl handles a URL search for a song.
func handleUrl(c *td.Client, m *td.Message, updater *td.Message, trackInfo *utils.PlatformTracks, chatId int64, isVideo bool, force bool) error {
	if len(trackInfo.Results) == 1 {
		track := trackInfo.Results[0]
		if _track := cache.ChatCache.GetTrackIfExists(chatId, track.Id); _track != nil {
			_, err := updater.EditText(c, utils.Notice("♻️ Already in the queue", "That track is playing or queued."), nil)
			return err
		}
		return handleSingleTrack(c, m, updater, track, "", chatId, isVideo, force)
	}

	return handleMultipleTracks(c, m, updater, trackInfo.Results, chatId, isVideo, force)
}

// handleSingleTrack handles a single track.
func handleSingleTrack(c *td.Client, m *td.Message, updater *td.Message, song utils.GetUrlTrack, filePath string, chatId int64, isVideo bool, force bool) error {
	if song.Duration > config.SongDurationLimit {
		_, err := updater.EditText(c, utils.Notice("⏳ Track too long", fmt.Sprintf("Maximum duration is %d minutes.", config.SongDurationLimit/60)), nil)
		return err
	}

	saveCache := utils.PlayerCache{
		URL: song.Url, Name: song.Title, User: firstName(c, m), FilePath: filePath,
		Thumbnail: song.Thumbnail, TrackID: song.Id, Duration: song.Duration, Channel: song.Channel, Views: song.Views,
		IsVideo: isVideo, Platform: song.Platform,
	}

	if queued, err := enqueueTrack(c, updater, chatId, &saveCache, force); queued {
		return err
	}

	if saveCache.FilePath == "" {
		dlResult, err := sources.DlCachedTrack(&saveCache, c)
		if err != nil {
			cache.ChatCache.RemoveCurrentSong(chatId)
			_, err = updater.EditText(c, utils.Notice("❌ Couldn't load this track", utils.FriendlyError(err)), nil)
			return err
		}

		saveCache.FilePath = dlResult
	}

	if err := player.Calls.PlayMedia(c, chatId, saveCache.FilePath, saveCache.IsVideo); err != nil {
		cache.ChatCache.RemoveCurrentSong(chatId)
		_, err = updater.EditText(c, utils.PlaybackError(err), &td.EditTextMessageOpts{ParseMode: "HTML", DisableWebPagePreview: true})
		return err
	}

	if err := sendStartedStreaming(c, updater, &saveCache); err != nil {
		c.Logger.Warn("Edit message failed", "error", err)
		return err
	}

	return nil
}

// handleMultipleTracks handles multiple tracks.
func handleMultipleTracks(c *td.Client, m *td.Message, updater *td.Message, tracks []utils.GetUrlTrack, chatId int64, isVideo bool, force bool) error {
	if len(tracks) == 0 {
		_, err := updater.EditText(c, utils.Notice("😕 No tracks found", ""), nil)
		return err
	}

	queueHeader := "<b>" + utils.HeaderQueued + "</b>\n" + utils.Divider + "\n<blockquote expandable>"
	var tracksToAdd []*utils.PlayerCache
	var skippedTracks []string

	shouldPlayFirst := false
	var firstTrack *utils.PlayerCache

	for _, track := range tracks {
		if track.Duration > config.SongDurationLimit {
			skippedTracks = append(skippedTracks, track.Title)
			continue
		}

		saveCache := &utils.PlayerCache{
			Name: track.Title, TrackID: track.Id, Duration: track.Duration,
			Thumbnail: track.Thumbnail, User: firstName(c, m), Platform: track.Platform,
			IsVideo: isVideo, URL: track.Url, Channel: track.Channel, Views: track.Views,
		}
		tracksToAdd = append(tracksToAdd, saveCache)
	}

	if len(tracksToAdd) == 0 {
		if len(skippedTracks) > 0 {
			_, err := updater.EditText(c, fmt.Sprintf("All tracks were skipped (max duration %d min).", config.SongDurationLimit/60), nil)
			return err
		}
		_, err := updater.EditText(c, "No valid tracks found.", nil)
		return err
	}

	var qLenAfter int
	var startLen int

	if force {
		qLenAfter = 0
		for _, t := range slices.Backward(tracksToAdd) {
			qLenAfter = cache.ChatCache.AddSongToFront(chatId, t)
		}
		startLen = qLenAfter - len(tracksToAdd)
		if startLen > 0 {
			_ = player.Calls.PlayNext(c, chatId)
			_ = c.DeleteMessages(chatId, []int64{updater.Id}, &td.DeleteMessagesOpts{Revoke: true})
			return nil
		}
	} else {
		qLenAfter = cache.ChatCache.AddSongs(chatId, tracksToAdd)
		startLen = qLenAfter - len(tracksToAdd)
	}

	if startLen == 0 {
		shouldPlayFirst = true
		firstTrack = tracksToAdd[0]
		firstTrack.Loop = 1
	}

	var sb strings.Builder
	sb.WriteString(queueHeader)

	var totalDuration int32
	for i, track := range tracksToAdd {
		currentQLen := startLen + i + 1
		escTrackName := html.EscapeString(track.Name)
		fmt.Fprintf(&sb, "<b>%d.</b> %s · <code>%s</code>\n",
			currentQLen, escTrackName, utils.SecToMin(track.Duration))
		totalDuration += track.Duration
	}

	sb.WriteString("</blockquote>")
	escRequester := html.EscapeString(firstName(c, m))
	queueSummary := fmt.Sprintf(
		"\n📀 <b>In queue:</b> %d\n⏱ <b>Total time:</b> %s\n👤 <b>Requested by:</b> %s",
		qLenAfter, utils.SecToMin(totalDuration), escRequester,
	)

	sb.WriteString(queueSummary)
	if len(skippedTracks) > 0 {
		fmt.Fprintf(&sb, "\n\n<b>Skipped %d tracks</b> (exceeded duration limit).", len(skippedTracks))
	}

	fullMessage := sb.String()

	if len(fullMessage) > 4096 {
		fullMessage = queueSummary
	}

	if shouldPlayFirst && firstTrack != nil {
		_ = player.Calls.PlayNext(c, chatId)
	}

	_, err := updater.EditText(c, fullMessage, &td.EditTextMessageOpts{
		ParseMode:             "HTML",
		ReplyMarkup:           utils.QueueMarkup(tracksToAdd[0].TrackID),
		DisableWebPagePreview: true,
	})

	return err
}

func enqueueTrack(c *td.Client, updater *td.Message, chatId int64, saveCache *utils.PlayerCache, force bool) (bool, error) {
	var qLen int
	if force {
		qLen = cache.ChatCache.AddSongToFront(chatId, saveCache)
	} else {
		qLen = cache.ChatCache.AddSong(chatId, saveCache)
	}

	if qLen > 1 {
		if force {
			_ = player.Calls.PlayNext(c, chatId)
			_ = c.DeleteMessages(chatId, []int64{updater.Id}, &td.DeleteMessagesOpts{Revoke: true})
			return true, nil
		}
		queueInfo := utils.PlayerCard(fmt.Sprintf("%s · #%d", utils.HeaderQueued, qLen-1), saveCache, "")

		_, err := updater.EditText(c, queueInfo, utils.CardOpts(saveCache, utils.QueueMarkup(saveCache.TrackID)))
		return true, err
	}

	return false, nil
}

func sendStartedStreaming(c *td.Client, updater *td.Message, saveCache *utils.PlayerCache) error {
	nowPlaying := utils.PlayerCard(utils.HeaderPlaying, saveCache, "")

	_, err := updater.EditText(c, nowPlaying, utils.CardOpts(saveCache, utils.ControlButtons("play")))

	return err
}
