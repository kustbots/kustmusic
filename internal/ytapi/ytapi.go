// Package ytapi resolves a /play query (a song name or a link) to a watch
// URL and its metadata, using yt-dlp.
package ytapi

import (
	"context"
	"fmt"
	"time"

	"github.com/kustbots/kustmusic/internal/core/ytdlp"
)

type Client struct {
	dl *ytdlp.Client
}

func New(dl *ytdlp.Client) *Client { return &Client{dl: dl} }

type SearchResult struct {
	Title     string
	URL       string
	Duration  string // ISO-8601, for example PT3M33S ("" when unknown)
	Thumbnail string
}

// Resolve looks query up and returns its metadata. A link goes through the
// same call as a name, so the player card gets a real title and length
// instead of showing the raw link with no progress bar.
func (c *Client) Resolve(query string) (*SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	info, err := c.dl.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ytapi: %s", ytdlp.Strip(err))
	}
	return &SearchResult{
		Title:     info.Title,
		URL:       info.URL,
		Duration:  isoDuration(info.Seconds),
		Thumbnail: info.Thumbnail,
	}, nil
}

func isoDuration(seconds float64) string {
	s := int(seconds)
	if s <= 0 {
		return ""
	}
	h, m, sec := s/3600, (s%3600)/60, s%60
	out := "PT"
	if h > 0 {
		out += fmt.Sprintf("%dH", h)
	}
	if m > 0 || h > 0 {
		out += fmt.Sprintf("%dM", m)
	}
	return out + fmt.Sprintf("%dS", sec)
}
