/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package sources

import (
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/utils"
	"time"

	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type youTubeData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

var youtubePatterns = map[string]*regexp.Regexp{
	"youtube":   regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/.*`),
	"youtu_be":  regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtu\.be/.*`),
	"yt_music":  regexp.MustCompile(`(?i)^(?:https?://)?music\.youtube\.com/.*`),
	"yt_shorts": regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/shorts/.*`),
}

func newYouTubeData(query string) *youTubeData {
	return &youTubeData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(config.ApiUrl, "/"),
		APIKey:   config.ApiKey,
		Patterns: youtubePatterns,
	}
}

func (y *youTubeData) isValid() bool {
	if y.Query == "" {
		slog.Info("The query or patterns are empty.")
		return false
	}

	for _, pattern := range y.Patterns {
		if pattern.MatchString(y.Query) {
			return true
		}
	}
	return false
}

func (y *youTubeData) getInfo() (*utils.PlatformTracks, error) {
	if !y.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	y.Query = normalizeYouTubeURL(y.Query)
	videoID := extractVideoID(y.Query)
	playlistID := extractPlaylistID(y.Query)

	switch {
	case playlistID != "":
		if strings.HasPrefix(playlistID, "RD") {
			return GetYouTubeMixPlaylist(ctx, playlistID)
		}
		return getYouTubePlaylist(ctx, playlistID)

	case videoID != "":
		for _, query := range []string{videoID, y.Query} {
			tracks, err := searchYouTube(query, 10)
			if err != nil {
				continue
			}

			for _, track := range tracks {
				if track.Id == videoID {
					return &utils.PlatformTracks{Results: []utils.GetUrlTrack{track}}, nil
				}
			}
		}

		if title, err := getYouTubeTitleFromOEmbed(videoID); err == nil && title != "" {
			tracks, err := searchYouTube(title, 10)
			if err == nil {
				for _, track := range tracks {
					if track.Id == videoID {
						return &utils.PlatformTracks{Results: []utils.GetUrlTrack{track}}, nil
					}
				}
			}
		}

		slog.Warn("Video ID was extracted but no matching track was found in search results", "video_id", videoID)
		return getYouTubeVideo(ctx, videoID)
	}

	return nil, errors.New("no video or playlist results were found")
}

func (y *youTubeData) search() (*utils.PlatformTracks, error) {
	tracks, err := searchYouTube(y.Query, 5)
	if err != nil {
		return nil, err
	}

	if len(tracks) == 0 {
		return nil, errors.New("no video results were found")
	}

	return &utils.PlatformTracks{Results: tracks}, nil
}

func (y *youTubeData) getTrack() (*utils.TrackInfo, error) {
	if y.Query == "" {
		return nil, errors.New("the query is empty")
	}

	if !y.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	if y.ApiUrl != "" && y.APIKey != "" {
		if trackInfo, err := newApiData(y.Query).getTrack(); err == nil {
			return trackInfo, nil
		}
	}

	getInfo, err := y.getInfo()
	if err != nil {
		return nil, err
	}
	if len(getInfo.Results) == 0 {
		return nil, errors.New("no video results were found")
	}

	track := getInfo.Results[0]
	trackInfo := &utils.TrackInfo{
		Id:       track.Id,
		URL:      track.Url,
		Platform: utils.YouTube,
	}

	return trackInfo, nil
}

func (y *youTubeData) downloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	if !video && info.CdnURL != "" {
		return info.CdnURL, nil
	}

	if !video && y.ApiUrl != "" && y.APIKey != "" {
		if filePath, err := y.downloadWithApi(info.Id, video); err == nil {
			return filePath, nil
		}
	}

	return y.downloadWithYtDlp(info.Id, video)
}

// ytAttempt is one rung of the yt-dlp retry ladder.
type ytAttempt struct {
	cookie string
	client string
}

// ytAttempts lists every cookie file (random start), then cookieless fallbacks.
func (y *youTubeData) ytAttempts() []ytAttempt {
	var attempts []ytAttempt

	cookies := config.CookiesPath
	if n := len(cookies); n > 0 {
		start := 0
		if r, err := rand.Int(rand.Reader, big.NewInt(int64(n))); err == nil {
			start = int(r.Int64())
		}
		for i := range n {
			cookie := cookies[(start+i)%n]
			if _, err := os.Stat(cookie); err == nil {
				attempts = append(attempts, ytAttempt{cookie: cookie})
			}
		}
	}

	return append(attempts, ytAttempt{}, ytAttempt{client: "tv,web_safari"})
}

func (y *youTubeData) buildYtdlpParams(videoID string, video bool, attempt ytAttempt) ([]string, string) {
	outputTemplate := filepath.Join(config.DownloadsDir, "%(id)s.%(ext)s")
	cookieFile := attempt.cookie

	params := []string{
		"yt-dlp",
		"--no-warnings",
		"--quiet",
		"--geo-bypass",
		"--retries", "2",
		"--continue",
		"--no-part",
		"--concurrent-fragments", "3",
		"--socket-timeout", "10",
		"--throttled-rate", "100K",
		"--retry-sleep", "1",
		"--no-write-thumbnail",
		"--no-write-info-json",
		"--no-embed-metadata",
		"--no-embed-chapters",
		"--no-embed-subs",
		"--extractor-args", "youtube:player_js_version=actual",
		"-o", outputTemplate,
	}

	if video {
		formatSelector := "bestvideo[height<=720]+bestaudio/best[height<=720]"
		params = append(params, "-f", formatSelector, "--merge-output-format", "mp4")
	} else {
		params = append(params, "-f", "bestaudio[ext=m4a]/bestaudio")
	}

	if attempt.client != "" {
		params = append(params, "--extractor-args", "youtube:player_client="+attempt.client)
	}

	if cookieFile != "" {
		params = append(params, "--cookies", cookieFile)
	} else if config.Proxy != "" {
		params = append(params, "--proxy", config.Proxy)
	}

	videoURL := "https://www.youtube.com/watch?v=" + videoID
	params = append(params, videoURL, "--print", "after_move:filepath")

	return params, cookieFile
}

func (y *youTubeData) downloadWithYtDlp(videoID string, video bool) (string, error) {
	if videoID == "" {
		return "", errors.New("videoID is empty")
	}

	var lastErr error
	for _, attempt := range y.ytAttempts() {
		path, err := y.runYtDlp(videoID, video, attempt)
		if err == nil {
			return path, nil
		}

		lastErr = err
		slog.Warn("yt-dlp attempt failed",
			"id", videoID,
			"cookie", filepath.Base(attempt.cookie),
			"client", attempt.client,
			"error", err,
		)
	}

	return "", lastErr
}

func (y *youTubeData) runYtDlp(videoID string, video bool, attempt ytAttempt) (string, error) {
	ytdlpParams, cookieFile := y.buildYtdlpParams(videoID, video, attempt)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, ytdlpParams[0], ytdlpParams[1:]...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr := string(exitErr.Stderr)
			if cookieFile != "" && strings.Contains(stderr, "Sign in to confirm you're not a bot") {
				_ = os.Remove(cookieFile)
			}
			return "", fmt.Errorf("yt-dlp failed with exit code %d: %s", exitErr.ExitCode(), stderr)
		}

		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("yt-dlp timed out for video ID: %s", videoID)
		}

		return "", fmt.Errorf("an unexpected error occurred while downloading %s: %w", videoID, err)
	}

	downloadedPathStr := strings.TrimSpace(string(output))
	if downloadedPathStr == "" {
		return "", fmt.Errorf("no output path was returned for %s", videoID)
	}

	if _, err := os.Stat(downloadedPathStr); os.IsNotExist(err) {
		return "", fmt.Errorf("the file was not found at the reported path: %s", downloadedPathStr)
	}

	return downloadedPathStr, nil
}

func (y *youTubeData) downloadWithApi(videoID string, _ bool) (string, error) {
	videoUrl := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	api := newApiData(videoUrl)
	track, err := api.getTrack()
	if err != nil {
		return "", err
	}

	return processDownload(track)
}
