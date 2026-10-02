// Package playback implements the "start playing a song" state machine: take
// a link, download the complete audio with yt-dlp, check it is the whole
// track, play it into the voice chat, and confirm sound is really coming out.
package playback

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kustbots/kustmusic/internal/core/genwait"
)

// VoiceCaller is the small surface the engine needs from the voice-call
// layer. The engine never talks to Telegram itself.
type VoiceCaller interface {
	// Play starts (or seamlessly switches) playback of source, a local file
	// path, in chatID's voice chat.
	Play(ctx context.Context, chatID int64, source string) error
	// PlayedTime returns how long the current stream has actually been
	// producing audio. A caller polls it to confirm buffering finished: a
	// call that joined but is silent shows up as this staying at zero.
	PlayedTime(ctx context.Context, chatID int64) (time.Duration, error)
}

// Downloader fetches the complete audio for a link into destPath. validate,
// when not nil, can reject the finished file.
type Downloader interface {
	Download(ctx context.Context, watchURL, destPath string, validate func(string) error) error
}

// Options tunes the engine's timing; the zero value uses the defaults.
type Options struct {
	BufferConfirmMaxWait time.Duration // default 8s
	BufferPollInterval   time.Duration // default 300ms
	DownloadDir          string        // default os.TempDir()
	// MaxCacheBytes caps the downloaded audio kept on disk; 0 uses the
	// default. Past the cap the least recently played songs are deleted,
	// which stops a long-running server filling its disk.
	MaxCacheBytes int64
}

func (o Options) withDefaults() Options {
	if o.BufferConfirmMaxWait <= 0 {
		o.BufferConfirmMaxWait = 8 * time.Second
	}
	if o.BufferPollInterval <= 0 {
		o.BufferPollInterval = 300 * time.Millisecond
	}
	if o.DownloadDir == "" {
		o.DownloadDir = os.TempDir()
	}
	return o
}

// Engine ties a downloader, a per-chat generation tracker and a voice caller
// together.
type Engine struct {
	DL    Downloader
	Gen   *genwait.Tracker
	Voice VoiceCaller
	Opts  Options
	Log   *slog.Logger
	Cache *SongCache
}

func NewEngine(dl Downloader, gen *genwait.Tracker, voice VoiceCaller, opts Options) *Engine {
	if gen == nil {
		gen = genwait.NewTracker()
	}
	o := opts.withDefaults()
	return &Engine{DL: dl, Gen: gen, Voice: voice, Opts: o, Log: slog.Default(), Cache: NewSongCache(o.MaxCacheBytes)}
}

// Start plays watchURL in chatID and returns once sound is confirmed or the
// attempt definitively failed. myGen is the generation this attempt was
// scheduled under (bump it with Engine.Gen.Bump first). Start returns
// ErrStale if a newer generation superseded it, so a slow attempt can never
// fight a newer one for the same chat. expectedSeconds is the track's real
// length when known (0 when not); a download materially shorter than that is
// rejected.
func (e *Engine) Start(ctx context.Context, chatID int64, watchURL string, myGen uint64, expectedSeconds float64) error {
	if !e.Gen.Valid(chatID, myGen) {
		return ErrStale
	}

	if cachedPath, ok := e.Cache.Get(watchURL); ok {
		e.Log.Info("playing from cache", "chat_id", chatID, "path", cachedPath)
		if err := e.Voice.Play(ctx, chatID, cachedPath); err == nil && e.confirmBuffering(ctx, chatID, myGen) {
			return nil
		}
		// The cached file did not play (corrupt, or removed meanwhile), so
		// fall through to a fresh download.
		e.Log.Warn("cached file failed to play, downloading fresh", "chat_id", chatID)
	}

	// The name comes from the song, not the chat, so a repeat request from
	// any chat is a cache hit instead of another download.
	destPath := filepath.Join(e.Opts.DownloadDir, cacheFileName(watchURL, ".audio"))
	// The length check runs inside the download's retry loop, so a short file
	// is thrown away and fetched again instead of being played and cached.
	validate := func(path string) error { return e.verifyDownloadLength(path, expectedSeconds) }
	if err := e.DL.Download(ctx, watchURL, destPath, validate); err != nil {
		return fmt.Errorf("playback: download failed: %w", err)
	}
	if !e.Gen.Valid(chatID, myGen) {
		// Superseded while downloading. The file is complete and shared by
		// song, so it stays for whichever request wants it next.
		return ErrStale
	}
	if err := e.Voice.Play(ctx, chatID, destPath); err != nil {
		return fmt.Errorf("playback: play failed: %w", err)
	}
	if !e.confirmBuffering(ctx, chatID, myGen) {
		return fmt.Errorf("playback: the stream never started")
	}
	e.Cache.Set(watchURL, destPath)
	return nil
}

// Prefetch downloads watchURL into the cache ahead of time, so a later Start
// for it is a cache hit. It does nothing if the song is already cached.
func (e *Engine) Prefetch(ctx context.Context, watchURL string) error {
	if _, ok := e.Cache.Get(watchURL); ok {
		return nil
	}
	destPath := filepath.Join(e.Opts.DownloadDir, cacheFileName(watchURL, ".audio"))
	if err := e.DL.Download(ctx, watchURL, destPath, nil); err != nil {
		return err
	}
	e.Cache.Set(watchURL, destPath)
	return nil
}

// confirmBuffering polls PlayedTime until it advances past zero or the window
// elapses. It returns false on timeout or when superseded.
func (e *Engine) confirmBuffering(ctx context.Context, chatID int64, myGen uint64) bool {
	deadline := time.Now().Add(e.Opts.BufferConfirmMaxWait)
	for time.Now().Before(deadline) {
		if !e.Gen.Valid(chatID, myGen) {
			return false
		}
		elapsed, err := e.Voice.PlayedTime(ctx, chatID)
		if err == nil && elapsed > 0 {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(e.Opts.BufferPollInterval):
		}
	}
	return false
}

// ErrStale is returned when a play attempt was superseded by a newer one (a
// skip, stop or new play raced in) before it could finish.
var ErrStale = fmt.Errorf("playback: superseded by a newer attempt")
