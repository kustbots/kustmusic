package player

import (
	"context"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/sources"
	"github.com/kustbots/kustmusic/internal/utils"
	"math/rand"
	"slices"
	"time"

	td "github.com/AshokShau/gotdbot"
)

func (c *TelegramCalls) handleAutoplay(bot *td.Client, chatID int64, lastTrackID string) error {
	history := cache.ChatCache.GetAutoplayHistory(chatID)
	if len(history) >= int(config.AutoPlayLimit) {
		cache.ChatCache.ClearAutoplayHistory(chatID)
		return c.handleNoSong(bot, chatID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tracks, err := sources.GetYouTubeMixPlaylist(ctx, "RD"+lastTrackID)
	if err != nil || tracks == nil || len(tracks.Results) == 0 {
		return c.handleNoSong(bot, chatID)
	}

	var candidates []utils.GetUrlTrack
	for _, track := range tracks.Results {
		if track.Id == lastTrackID || slices.Contains(history, track.Id) {
			continue
		}

		candidates = append(candidates, track)
	}

	if len(candidates) == 0 {
		return c.handleNoSong(bot, chatID)
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	nextTrack := candidates[0]
	cache.ChatCache.AddAutoplayHistory(chatID, lastTrackID, nextTrack.Id)

	saveCache := &utils.PlayerCache{
		URL:       nextTrack.Url,
		Name:      nextTrack.Title,
		User:      utils.AutoPlay,
		Thumbnail: nextTrack.Thumbnail,
		TrackID:   nextTrack.Id,
		Duration:  nextTrack.Duration,
		Channel:   nextTrack.Channel,
		Views:     nextTrack.Views,
		Platform:  utils.YouTube,
	}

	cache.ChatCache.AddSong(chatID, saveCache)
	return c.playTrack(bot, chatID, saveCache)
}
