/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package sources

import (
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/utils"
	"log/slog"

	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type apiData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

var apiPatterns = map[string]*regexp.Regexp{
	utils.Apple:      regexp.MustCompile(`(?i)^https?:\/\/music\.apple\.com\/[a-zA-Z-]+\/(?:song\/(?:[^\/]+\/)?\d+|album\/[^\/]+\/\d+(?:\?i=\d+)?|playlist\/[^\/]+\/pl\.[\w.-]+|artist\/[^\/]+\/\d+)(?:\?.*)?$`),
	utils.Spotify:    regexp.MustCompile(`(?i)^(https?://)?([a-z0-9-]+\.)*spotify\.com/(track|playlist|album|artist)/[a-zA-Z0-9]+(\?.*)?$`),
	utils.JioSaavn:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?(?:jiosaavn|saavn)\.com\/(?:s\/)?(song|album|playlist|featured)(?:\/[^\/]+)*\/([A-Za-z0-9_,-]+)(?:\/)?(?:\?.*)?$`),
	utils.Deezer:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?deezer\.com\/(?:[a-z]{2}\/)?(track|album|playlist)\/(\d+)`),
	utils.SoundCloud: regexp.MustCompile(`(?i)^(https?://)?(www\.)?soundcloud\.com/[a-zA-Z0-9_-]+/(sets/)?[a-zA-Z0-9._-]+(\?.*)?$`),
	utils.Gaana:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?gaana\.com\/(song|album|playlist|artist)\/([A-Za-z0-9\-]+)`),
	utils.Tidal:      regexp.MustCompile(`(?i)https?:\/\/(?:www\.|listen\.)?tidal\.com\/(?:browse\/)?(track|album|playlist)\/([a-zA-Z0-9-]+)(?:[\/?].*)?`),
	utils.MXPlayer:   regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?mxplayer\.in\/(?:show|movie)\/.*`),
	utils.Twitch:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.|m\.)?twitch\.tv\/(?:videos|[\w._-]+\/video)\/\d+`),
	utils.TwitchClip: regexp.MustCompile(
		`(?i)https?:\/\/(?:www\.|m\.)?(?:` +
			`twitch\.tv\/clip\/[\w-]+|` +
			`clips\.twitch\.tv\/[\w-]+|` +
			`twitch\.tv\/[\w-]+\/clip\/[\w-]+` +
			`)`,
	),
	utils.Kick:     regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/videos\/[a-fA-F0-9-]+`),
	utils.KickClip: regexp.MustCompile(`(?i)https?:\/\/(?:www\.)?kick\.com\/[\w._-]+\/clips\/[\w-]+`),
}

func newApiData(query string) *apiData {
	return &apiData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(config.ApiUrl, "/"),
		APIKey:   config.ApiKey,
		Patterns: apiPatterns,
	}
}

func (a *apiData) isValid() bool {
	if a.Query == "" || a.ApiUrl == "" || a.APIKey == "" {
		return false
	}

	for _, pattern := range a.Patterns {
		if pattern.MatchString(a.Query) {
			return true
		}
	}
	return false
}

func (a *apiData) getInfo() (*utils.PlatformTracks, error) {
	if !a.isValid() {
		return nil, errors.New("the provided URL is invalid or the platform is not supported")
	}

	fullURL := fmt.Sprintf("%s/api/get_url?%s", a.ApiUrl, url.Values{"url": {a.Query}}.Encode())
	resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey})
	if err != nil {
		return nil, fmt.Errorf("the GetInfo request failed: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching info: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode the GetInfo response: %w", err)
	}
	return &data, nil
}

func (a *apiData) search() (*utils.PlatformTracks, error) {
	if a.isValid() {
		return a.getInfo()
	}

	fullURL := fmt.Sprintf("%s/api/search?%s", a.ApiUrl, url.Values{
		"query": {a.Query},
		"limit": {"5"},
	}.Encode())

	resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey})
	if err != nil {
		return nil, fmt.Errorf("the search request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code during search: %s", resp.Status)
	}

	var data utils.PlatformTracks
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		slog.Warn("Failed to decode search response", "error", err)
		return nil, fmt.Errorf("failed to decode the search response: %w", err)
	}

	return &data, nil
}

func (a *apiData) getTrack() (*utils.TrackInfo, error) {
	fullURL := fmt.Sprintf("%s/api/track?%s", a.ApiUrl, url.Values{"url": {a.Query}}.Encode())
	resp, err := sendRequest(http.MethodGet, fullURL, nil, map[string]string{"X-API-Key": a.APIKey})
	if err != nil {
		slog.Warn("GetTrack request failed", "error", err)
		return nil, fmt.Errorf("the GetTrack request failed: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while fetching the track: %s", resp.Status)
	}

	var data utils.TrackInfo
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		slog.Warn("Failed to decode the GetTrack response", "error", err)
		return nil, fmt.Errorf("failed to decode the GetTrack response: %w", err)
	}

	return &data, nil
}

func (a *apiData) downloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	// if the track is from YouTube and video:true
	yt := newYouTubeData(a.Query)
	if info.Platform == utils.YouTube && video {
		return yt.downloadTrack(info, video)
	}

	filePath, err := processDownload(info)
	if err != nil {
		if info.Platform == utils.YouTube {
			return yt.downloadTrack(info, video)
		}
		return "", fmt.Errorf("the download process failed: %w", err)
	}

	if strings.Contains(a.ApiUrl, filePath) {
		return downloadFile(filePath, "", false)
	}

	return filePath, nil
}
