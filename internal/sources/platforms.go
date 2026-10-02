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
)

type service interface {
	// isValid determines if the service can handle the given query.
	isValid() bool
	// getInfo retrieves metadata for a track or playlist.
	getInfo() (*utils.PlatformTracks, error)
	// search queries the service for a track.
	search() (*utils.PlatformTracks, error)
	// getTrack fetches detailed information for a single track.
	getTrack() (*utils.TrackInfo, error)
	// downloadTrack handles the download of a track.
	downloadTrack(trackInfo *utils.TrackInfo, video bool) (string, error)
}

// DlWrapper provides a unified interface for music service interactions.
type DlWrapper struct{ service service }

func NewDlWrapper(query string) *DlWrapper {
	yt := newYouTubeData(query)
	api := newApiData(query)
	direct := newDirectLink(query)

	var chosen service
	if yt.isValid() {
		chosen = yt
	} else if api.isValid() {
		chosen = api
	} else if direct.isValid() {
		chosen = direct
	} else {
		switch config.DefaultService {
		case "spotify":
			chosen = api
		default:
			chosen = yt
		}
	}

	return &DlWrapper{
		service: chosen,
	}
}

// IsValid checks if the underlying service can handle the query.
func (d *DlWrapper) IsValid() bool {
	return d.service != nil && d.service.isValid()
}

// GetInfo retrieves metadata by delegating the call to the wrapped service.
func (d *DlWrapper) GetInfo() (*utils.PlatformTracks, error) {
	return d.service.getInfo()
}

// Search performs a search by delegating the call to the wrapped service.
func (d *DlWrapper) Search() (*utils.PlatformTracks, error) {
	return d.service.search()
}

// GetTrack retrieves detailed track information by delegating the call to the wrapped service.
func (d *DlWrapper) GetTrack() (*utils.TrackInfo, error) {
	return d.service.getTrack()
}

// DownloadTrack downloads a track by delegating the call to the wrapped service.
// It returns the file path of the downloaded track or an error if the download fails.
func (d *DlWrapper) DownloadTrack(info *utils.TrackInfo, video bool) (string, error) {
	return d.service.downloadTrack(info, video)
}
