/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package utils

type PlayerCache struct {
	URL       string `json:"url"`
	Name      string `json:"name"`
	Loop      int    `json:"loop"`
	User      string `json:"user"`
	FilePath  string `json:"file_path"`
	Thumbnail string `json:"thumbnail"`
	TrackID   string `json:"track_id"`
	Duration  int32  `json:"duration"`
	Channel   string `json:"channel"`
	Views     string `json:"views"`
	IsVideo   bool   `json:"is_video"`
	Platform  string `json:"platform"`
}

type TrackInfo struct {
	Id       string `json:"id"`
	URL      string `json:"url"`
	CdnURL   string `json:"cdnurl"`
	Key      string `json:"key"`
	Platform string `json:"platform"`
}

type GetUrlTrack struct {
	Title     string `json:"title"`
	Id        string `json:"id"`
	Url       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	Duration  int32  `json:"duration"`
	Channel   string `json:"channel"`
	Views     string `json:"views"`
	Platform  string `json:"platform"`
}

type PlatformTracks struct {
	Results []GetUrlTrack `json:"results"`
}

const (
	Telegram   = "telegram"
	YouTube    = "youtube"
	Spotify    = "spotify"
	JioSaavn   = "jiosaavn"
	Apple      = "apple_music"
	SoundCloud = "soundcloud"
	Deezer     = "Deezer"
	Gaana      = "Gaana"
	DirectLink = "direct_link"
	Tidal      = "tidal"
	MXPlayer   = "mxplayer"
	Twitch     = "twitch"
	TwitchClip = "twitch_clip"
	Kick       = "kick"
	KickClip   = "kick_clip"
)

const (
	Admins   = "admins"
	Everyone = "everyone"
	AutoPlay = "Autoplay"
)

type FFProbeFormat struct {
	Format struct {
		Duration string `json:"duration"`
		Tags     struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"format"`
}
