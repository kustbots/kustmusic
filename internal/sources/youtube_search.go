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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/utils"
	"io"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	ytBaseURL         = "https://www.youtube.com"
	ytWatchURL        = ytBaseURL + "/watch?v="
	ytAPIKey          = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	ytClientVer       = "2.20250101.01.00"
	defaultUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
	requestRetryDelay = 200 * time.Millisecond
)

type ytClientProfile struct {
	ClientName    string
	ClientVersion string
	ClientID      string
	Origin        string
	UserAgent     string

	OsName      string
	OsVersion   string
	AndroidSDK  int
	DeviceMake  string
	DeviceModel string

	EmbedURL string
}

var ytProfiles = []ytClientProfile{
	{
		ClientName:    "WEB",
		ClientVersion: ytClientVer,
		ClientID:      "1",
		Origin:        ytBaseURL,
		UserAgent:     defaultUserAgent,
	},
	{
		ClientName:    "MWEB",
		ClientVersion: "2.20250101.01.00",
		ClientID:      "2",
		Origin:        "https://m.youtube.com",
		UserAgent:     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Mobile/15E148 Safari/604.1",
	},
	{
		ClientName:    "WEB_REMIX",
		ClientVersion: "1.20250101.01.00",
		ClientID:      "67",
		Origin:        "https://music.youtube.com",
		UserAgent:     defaultUserAgent,
	},
	{
		ClientName:    "TVHTML5",
		ClientVersion: "7.20250101.00.00",
		ClientID:      "7",
		Origin:        ytBaseURL,
		UserAgent:     "Mozilla/5.0 (SmartHub; SMART-TV; U; Linux/SmartTV) AppleWebKit/537.42 (KHTML, like Gecko) SmartTV Safari/537.42",
	},
	{
		ClientName:    "ANDROID",
		ClientVersion: "19.09.37",
		ClientID:      "3",
		OsName:        "Android",
		OsVersion:     "14",
		AndroidSDK:    34,
		DeviceMake:    "Google",
		DeviceModel:   "Pixel 8",
		UserAgent:     "com.google.android.youtube/19.09.37 (Linux; U; Android 14; en_US) gzip",
	},
	{
		ClientName:    "IOS",
		ClientVersion: "19.09.3",
		ClientID:      "5",
		OsName:        "iPhone OS",
		OsVersion:     "17.4.1.21E237",
		DeviceMake:    "Apple",
		DeviceModel:   "iPhone16,2",
		UserAgent:     "com.google.ios.youtube/19.09.3 (iPhone16,2; U; CPU iOS 17_4_1 like Mac OS X; en_US)",
	},
	{
		ClientName:    "ANDROID_MUSIC",
		ClientVersion: "7.03.51",
		ClientID:      "21",
		OsName:        "Android",
		OsVersion:     "14",
		AndroidSDK:    34,
		UserAgent:     "com.google.android.apps.youtube.music/7.03.51 (Linux; U; Android 14) gzip",
	},
	{

		ClientName:    "WEB_EMBEDDED_PLAYER",
		ClientVersion: "1.20250101.01.00",
		ClientID:      "56",
		Origin:        ytBaseURL,
		UserAgent:     defaultUserAgent,
		EmbedURL:      ytBaseURL + "/",
	},
	{
		ClientName:    "TVHTML5_SIMPLY_EMBEDDED_PLAYER",
		ClientVersion: "2.0",
		ClientID:      "85",
		Origin:        ytBaseURL,
		UserAgent:     "Mozilla/5.0 (SmartHub; SMART-TV; U; Linux/SmartTV) AppleWebKit/537.42 (KHTML, like Gecko) SmartTV Safari/537.42",
		EmbedURL:      ytBaseURL + "/",
	},
}

var (
	labelDurationRe = regexp.MustCompile(`(\d+)\s*(hours?|minutes?|seconds?)`)
	videoIDRe1      = regexp.MustCompile(`(?i)(?:youtube\.com/(?:watch\?v=|embed/|shorts/|live/)|youtu\.be/)([A-Za-z0-9_-]{11})`)
	videoIDRe2      = regexp.MustCompile(`(?:v=|\/)([0-9A-Za-z_-]{11})`)
	playlistIDRe1   = regexp.MustCompile(`(?i)(?:youtube\.com|music\.youtube\.com).*(?:\?|&)list=([A-Za-z0-9_-]+)`)
	playlistIDRe2   = regexp.MustCompile(`list=([0-9A-Za-z_-]+)`)
)

func setYTHeadersWithProfile(req *http.Request, prof ytClientProfile) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", prof.UserAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("X-YouTube-Client-Name", prof.ClientID)
	req.Header.Set("X-YouTube-Client-Version", prof.ClientVersion)

	if prof.Origin != "" {
		req.Header.Set("Origin", prof.Origin)
		req.Header.Set("Referer", prof.Origin+"/")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-Dest", "empty")
	}
}

func ytContextWithProfile(prof ytClientProfile) map[string]any {
	clientCtx := map[string]any{
		"clientName":    prof.ClientName,
		"clientVersion": prof.ClientVersion,
		"hl":            "en",
		"gl":            "US",
	}
	if prof.OsName != "" {
		clientCtx["osName"] = prof.OsName
	}
	if prof.OsVersion != "" {
		clientCtx["osVersion"] = prof.OsVersion
	}
	if prof.AndroidSDK > 0 {
		clientCtx["androidSdkVersion"] = prof.AndroidSDK
	}
	if prof.DeviceMake != "" {
		clientCtx["deviceMake"] = prof.DeviceMake
	}
	if prof.DeviceModel != "" {
		clientCtx["deviceModel"] = prof.DeviceModel
	}

	innerContext := map[string]any{"client": clientCtx}
	if prof.EmbedURL != "" {
		innerContext["thirdParty"] = map[string]any{"embedUrl": prof.EmbedURL}
	}

	return map[string]any{"context": innerContext}
}

func formatYTErr(action string, statusCode int, statusText string, body []byte) error {
	bodyStr := string(body)
	if statusCode == http.StatusForbidden || statusCode == http.StatusTooManyRequests ||
		strings.Contains(strings.ToLower(bodyStr), "<html") || strings.Contains(bodyStr, "automated queries") {
		return fmt.Errorf("%s failed: status=%d %s (rate limited by YouTube / automated query block)", action, statusCode, statusText)
	}

	if len(bodyStr) > 100 {
		bodyStr = bodyStr[:100] + "..."
	}
	return fmt.Errorf("%s failed: status=%d %s body=%q", action, statusCode, statusText, bodyStr)
}

func ytRequest(ctx context.Context, path string, extraFields map[string]any, accept func(map[string]any) bool) (map[string]any, error) {
	var lastErr error

	for i, prof := range ytProfiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(requestRetryDelay):
			}
		}

		payload := ytContextWithProfile(prof)
		maps.Copy(payload, extraFields)

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}

		endpoint := ytBaseURL + path + "?key=" + ytAPIKey
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		setYTHeadersWithProfile(req, prof)

		res, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s (%s): %w", path, prof.ClientName, err)
			continue
		}

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			snippet, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
			_ = res.Body.Close()
			lastErr = formatYTErr(fmt.Sprintf("youtube %s (%s)", path, prof.ClientName), res.StatusCode, res.Status, snippet)
			continue
		}

		var out map[string]any
		err = json.NewDecoder(io.LimitReader(res.Body, 10*1024*1024)).Decode(&out)
		_ = res.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("decode response (%s): %w", prof.ClientName, err)
			continue
		}

		if accept != nil && !accept(out) {
			lastErr = fmt.Errorf("youtube %s (%s): response rejected (empty/invalid)", path, prof.ClientName)
			continue
		}

		return out, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("youtube %s failed: no client profiles succeeded", path)
	}
	return nil, lastErr
}

func ytPost(ctx context.Context, path string, extraFields map[string]any) (map[string]any, error) {
	return ytRequest(ctx, path, extraFields, nil)
}

func searchYouTube(query string, limit int) ([]utils.GetUrlTrack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var tracks []utils.GetUrlTrack
	_, err := ytRequest(ctx, "/youtubei/v1/search", map[string]any{"query": query}, func(data map[string]any) bool {
		var found []utils.GetUrlTrack
		parseResults(data, &found, limit)
		if len(found) == 0 {
			return false
		}
		tracks = found
		return true
	})

	if len(tracks) > 0 {
		return tracks, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, errors.New("youtube search returned no results across client profiles")
}

func parseResults(node any, tracks *[]utils.GetUrlTrack, limit int) {
	stack := []any{node}
	for len(stack) > 0 && len(*tracks) < limit {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch v := curr.(type) {
		case []any:
			for _, v0 := range slices.Backward(v) {
				stack = append(stack, v0)
			}

		case map[string]any:
			if vr, ok := dig(v, "videoRenderer").(map[string]any); ok {
				if isLiveNow(vr) {
					continue
				}
				id := safeString(vr["videoId"])
				title := safeString(dig(vr, "title", "runs", 0, "text"))
				durationText := safeString(dig(vr, "lengthText", "simpleText"))
				if id == "" || title == "" || durationText == "" {
					continue
				}
				*tracks = append(*tracks, utils.GetUrlTrack{
					Id:        id,
					Url:       ytWatchURL + id,
					Title:     title,
					Thumbnail: safeString(dig(vr, "thumbnail", "thumbnails", 0, "url")),
					Duration:  parseDuration(durationText),
					Views:     safeString(dig(vr, "viewCountText", "simpleText")),
					Channel:   safeString(dig(vr, "ownerText", "runs", 0, "text")),
					Platform:  utils.YouTube,
				})
				continue
			}

			for _, child := range v {
				stack = append(stack, child)
			}
		}
	}
}

func isLiveNow(vr map[string]any) bool {
	badges, ok := vr["badges"].([]any)
	if !ok {
		return false
	}
	for _, badge := range badges {
		meta, ok := dig(badge, "metadataBadgeRenderer").(map[string]any)
		if !ok {
			continue
		}
		if safeString(meta["style"]) == "BADGE_STYLE_TYPE_LIVE_NOW" {
			return true
		}
	}
	return false
}

func getYouTubeTitleFromOEmbed(videoID string) (string, error) {
	apiURL := fmt.Sprintf("https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=%s&format=json", videoID)

	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("oEmbed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oEmbed returned status code: %d", resp.StatusCode)
	}

	var data struct {
		Title string `json:"title"`
	}

	if err = json.NewDecoder(io.LimitReader(resp.Body, 1*1024*1024)).Decode(&data); err != nil {
		return "", fmt.Errorf("failed to decode oEmbed response: %w", err)
	}

	if data.Title == "" {
		return "", errors.New("oEmbed response contained empty title")
	}

	return data.Title, nil
}

func getYouTubeVideo(ctx context.Context, videoID string) (*utils.PlatformTracks, error) {
	resp, err := ytRequest(ctx, "/youtubei/v1/player", map[string]any{"videoId": videoID}, func(data map[string]any) bool {
		return digStr(data, "videoDetails", "videoId") != ""
	})
	if err != nil {
		return nil, err
	}

	video := mapPlayerToTrack(resp)
	if video.Id == "" {
		return nil, errors.New("video not found")
	}
	return &utils.PlatformTracks{Results: []utils.GetUrlTrack{video}}, nil
}

func getYouTubePlaylist(ctx context.Context, playlistID string) (*utils.PlatformTracks, error) {
	resp, err := ytPost(ctx, "/youtubei/v1/browse", map[string]any{"browseId": "VL" + playlistID})
	if err != nil {
		return nil, err
	}

	videos := extractPlaylistVideos(resp)
	return buildTrackList(videos, mapYTVideo), nil
}

func GetYouTubeMixPlaylist(ctx context.Context, playlistID string) (*utils.PlatformTracks, error) {
	resp, err := ytPost(ctx, "/youtubei/v1/next", map[string]any{"playlistId": playlistID})
	if err != nil {
		return nil, err
	}

	videos := extractMixPlaylistVideos(resp)
	return buildTrackList(videos, mapMixVideo), nil
}

func GetYouTubeMix(ctx context.Context, query string, seedTrackID string, limit int) ([]utils.GetUrlTrack, error) {
	startTrackID := seedTrackID
	if startTrackID == "" && query != "" {
		tracks, err := searchYouTube(query, 1)
		if err != nil {
			return nil, err
		}
		if len(tracks) == 0 {
			return nil, errors.New("no tracks found for query")
		}
		startTrackID = tracks[0].Id
	}

	if startTrackID == "" {
		return nil, errors.New("no query or seed track ID provided")
	}

	playlistID := "RD" + startTrackID
	mix, err := GetYouTubeMixPlaylist(ctx, playlistID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var uniqueTracks []utils.GetUrlTrack

	for _, track := range mix.Results {
		if track.Id == "" || seen[track.Id] {
			continue
		}
		seen[track.Id] = true
		uniqueTracks = append(uniqueTracks, track)
		if limit > 0 && len(uniqueTracks) >= limit {
			break
		}
	}

	return uniqueTracks, nil
}

func buildTrackList(videos []map[string]any, mapper func(map[string]any) utils.GetUrlTrack) *utils.PlatformTracks {
	out := make([]utils.GetUrlTrack, 0, len(videos))
	for _, v := range videos {
		if t := mapper(v); t.Id != "" {
			out = append(out, t)
		}
	}
	return &utils.PlatformTracks{Results: out}
}

func mapYTVideo(v map[string]any) utils.GetUrlTrack {
	id := digStr(v, "videoId")
	return utils.GetUrlTrack{
		Id:        id,
		Title:     digStr(v, "title", "runs", 0, "text"),
		Url:       ytWatchURL + id,
		Thumbnail: lastThumbURL(digArray(v, "thumbnail", "thumbnails")),
		Channel:   digStr(v, "shortBylineText", "runs", 0, "text"),
		Duration:  parseYTDuration(v),
		Views:     digStr(v, "viewCountText", "simpleText"),
		Platform:  utils.YouTube,
	}
}

func mapMixVideo(v map[string]any) utils.GetUrlTrack {
	id := digStr(v, "videoId")
	return utils.GetUrlTrack{
		Id:        id,
		Title:     digStr(v, "title", "simpleText"),
		Url:       ytWatchURL + id,
		Thumbnail: lastThumbURL(digArray(v, "thumbnail", "thumbnails")),
		Channel:   digStr(v, "shortBylineText", "runs", 0, "text"),
		Duration:  parseYTDuration(v),
		Platform:  utils.YouTube,
	}
}

func mapPlayerToTrack(src map[string]any) utils.GetUrlTrack {
	id := digStr(src, "videoDetails", "videoId")
	return utils.GetUrlTrack{
		Id:        id,
		Title:     digStr(src, "videoDetails", "title"),
		Url:       ytWatchURL + id,
		Thumbnail: lastThumbURL(digArray(src, "videoDetails", "thumbnail", "thumbnails")),
		Channel:   digStr(src, "videoDetails", "author"),
		Duration:  atoi(digStr(src, "videoDetails", "lengthSeconds")),
		Views:     digStr(src, "videoDetails", "viewCount"),
		Platform:  utils.YouTube,
	}
}

func extractPlaylistVideos(src map[string]any) []map[string]any {
	contents := digArray(src,
		"contents",
		"twoColumnBrowseResultsRenderer",
		"tabs", 0,
		"tabRenderer",
		"content",
		"sectionListRenderer",
		"contents", 0,
		"itemSectionRenderer",
		"contents", 0,
		"playlistVideoListRenderer",
		"contents",
	)
	var out []map[string]any
	for _, c := range contents {
		if v, ok := c["playlistVideoRenderer"].(map[string]any); ok {
			out = append(out, v)
		}
	}
	return out
}

func extractMixPlaylistVideos(src map[string]any) []map[string]any {
	contents := digArray(src,
		"contents",
		"twoColumnWatchNextResults",
		"playlist", "playlist", "contents",
	)
	var out []map[string]any
	for _, c := range contents {
		if v, ok := c["playlistPanelVideoRenderer"].(map[string]any); ok {
			out = append(out, v)
		}
	}
	return out
}

func lastThumbURL(thumbs []map[string]any) string {
	if len(thumbs) == 0 {
		return ""
	}
	t, _ := thumbs[len(thumbs)-1]["url"].(string)
	return t
}

func normalizeYouTubeURL(rawURL string) string {
	var id string
	switch {
	case strings.Contains(rawURL, "youtu.be/"):
		id = extractSegment(rawURL, "youtu.be/")
	case strings.Contains(rawURL, "youtube.com/shorts/"):
		id = extractSegment(rawURL, "youtube.com/shorts/")
	default:
		return rawURL
	}
	return ytWatchURL + id
}

func extractSegment(u, sep string) string {
	parts := strings.SplitN(u, sep, 2)
	if len(parts) < 2 {
		return u
	}
	after := parts[1]
	after = strings.SplitN(after, "?", 2)[0]
	after = strings.SplitN(after, "#", 2)[0]
	return after
}

func extractVideoID(u string) string {
	if m := videoIDRe1.FindStringSubmatch(u); len(m) > 1 {
		return m[1]
	}
	if m := videoIDRe2.FindStringSubmatch(u); len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractPlaylistID(u string) string {
	if m := playlistIDRe1.FindStringSubmatch(u); len(m) > 1 {
		return m[1]
	}
	if m := playlistIDRe2.FindStringSubmatch(u); len(m) > 1 {
		return m[1]
	}
	return ""
}

func parseYTDuration(v map[string]any) int32 {
	if txt := digStr(v, "lengthText", "simpleText"); txt != "" {
		return parseTimeToSeconds(txt)
	}
	if label := digStr(v, "lengthText", "accessibility", "accessibilityData", "label"); label != "" {
		return parseLabelDuration(label)
	}
	return 0
}

func parseDuration(s string) int32 {
	parts := strings.Split(s, ":")
	var total, mul int32 = 0, 1
	for _, part := range slices.Backward(parts) {
		total += atoi(part) * mul
		mul *= 60
	}
	return total
}

func parseTimeToSeconds(s string) int32 {
	parts := strings.Split(s, ":")
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return int32(total)
}

func parseLabelDuration(s string) int32 {
	total := 0
	for _, m := range labelDurationRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		switch {
		case strings.HasPrefix(m[2], "hour"):
			total += n * 3600
		case strings.HasPrefix(m[2], "minute"):
			total += n * 60
		default:
			total += n
		}
	}
	return int32(total)
}

func dig(v any, path ...any) any {
	cur := v
	for _, p := range path {
		if cur == nil {
			return nil
		}
		switch k := p.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			cur = m[k]
		case int:
			a, ok := cur.([]any)
			if !ok || k < 0 || k >= len(a) {
				return nil
			}
			cur = a[k]
		}
	}
	return cur
}

func digStr(src any, path ...any) string {
	s, _ := dig(src, path...).(string)
	return s
}

func digArray(src any, path ...any) []map[string]any {
	arr, ok := dig(src, path...).([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func safeString(v any) string {
	s, _ := v.(string)
	return s
}

func atoi(s string) int32 {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return int32(n)
}
