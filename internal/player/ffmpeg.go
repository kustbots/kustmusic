/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package player

import (
	"context"
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/ntgcalls"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func getVideoDimensions(filePath string) (int, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=s=x:p=0", filePath)
	out, err := cmd.Output()
	if err != nil {
		logger.Warn("[getVideoDimensions] Failed to get video dimensions (%s): %v", filePath, err)
		return 0, 0
	}
	dimensions := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(dimensions) != 2 {
		logger.Warn("[getVideoDimensions] Invalid video dimensions(%s): %s", filePath, string(out))
		return 0, 0
	}

	width, _ := strconv.Atoi(dimensions[0])
	height, _ := strconv.Atoi(dimensions[1])
	return width, height
}

var urlRegex = regexp.MustCompile(`^https?://`)

// getMediaDescription creates a media description for ntgcalls based on the provided file path, video status, and ffmpeg parameters.
func getMediaDescription(filePath string, isVideo bool, chatId int64, ffmpegParameters string) ntgcalls.MediaDescription {
	var duration int
	track := cache.ChatCache.GetPlayingTrack(chatId)
	if track != nil {
		duration = int(track.Duration)
	}

	audioDescription := &ntgcalls.AudioDescription{
		MediaSource:  ntgcalls.MediaSourceShell,
		SampleRate:   48000,
		ChannelCount: 2,
	}

	quotedPath := fmt.Sprintf("\"%s\"", filePath)
	isURL := urlRegex.MatchString(filePath)
	isLive := isURL && duration == 0

	var audioCmd strings.Builder
	audioCmd.WriteString("ffmpeg ")
	if isLive {
		audioCmd.WriteString("-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 ")
	}

	var seekFlags, filterFlags string
	if ffmpegParameters != "" {
		if strings.Contains(ffmpegParameters, "filter:") {
			filterFlags = ffmpegParameters
		} else {
			seekFlags = ffmpegParameters
		}
	}

	if seekFlags != "" {
		audioCmd.WriteString(seekFlags + " ")
	}
	audioCmd.WriteString("-i " + quotedPath + " ")
	if filterFlags != "" {
		audioCmd.WriteString(filterFlags + " ")
	}
	audioCmd.WriteString(fmt.Sprintf(
		"-f s16le -ac %d -ar %d -v quiet pipe:1",
		audioDescription.ChannelCount,
		audioDescription.SampleRate,
	))

	audioDescription.Input = audioCmd.String()

	if !isVideo {
		return ntgcalls.MediaDescription{
			Microphone: audioDescription,
		}
	}

	originalWidth, originalHeight := getVideoDimensions(filePath)
	width := 1280
	height := 720

	if originalWidth > 0 && originalHeight > 0 {
		ratio := float64(originalWidth) / float64(originalHeight)

		newW := min(originalWidth, width)
		newH := int(float64(newW) / ratio)

		if newH > height {
			newH = height
			newW = int(float64(newH) * ratio)
		}

		if newW%2 != 0 {
			newW--
		}

		if newH%2 != 0 {
			newH--
		}

		width = newW
		height = newH
	}

	videoDescription := &ntgcalls.VideoDescription{
		MediaSource: ntgcalls.MediaSourceShell,
		Width:       int16(width),
		Height:      int16(height),
		Fps:         30,
	}

	var videoCmd strings.Builder
	videoCmd.WriteString("ffmpeg ")
	if isLive {
		videoCmd.WriteString("-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 ")
	}

	if seekFlags != "" {
		videoCmd.WriteString(seekFlags + " ")
	}
	videoCmd.WriteString("-i " + quotedPath + " ")
	if filterFlags != "" {
		videoCmd.WriteString(filterFlags + " ")
	}
	videoCmd.WriteString(fmt.Sprintf(
		"-f rawvideo -r %d -pix_fmt yuv420p -vf scale=%d:%d -v quiet pipe:1",
		videoDescription.Fps,
		videoDescription.Width,
		videoDescription.Height,
	))

	videoDescription.Input = videoCmd.String()

	return ntgcalls.MediaDescription{
		Microphone: audioDescription,
		Camera:     videoDescription,
	}
}
