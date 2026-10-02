// Package ytdlp finds and downloads songs by running the yt-dlp program. It is
// the only place this project talks to a music source, so there is no hosted
// API to depend on or keep secret.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Config tunes the client. Every field is optional.
type Config struct {
	// Bin is the yt-dlp executable. Default "yt-dlp".
	Bin string
	// CookiesFile is a Netscape-format cookies file passed to yt-dlp. YouTube
	// often refuses downloads from server addresses without one.
	CookiesFile string
	// ExtraArgs are appended to every yt-dlp call, for example
	// "--js-runtimes", "deno" or a proxy.
	ExtraArgs []string
	// Concurrency caps simultaneous yt-dlp downloads. Default 3.
	Concurrency int
	// SearchTimeout and DownloadTimeout bound one call. Defaults 30s and 5m.
	SearchTimeout   time.Duration
	DownloadTimeout time.Duration
}

type Client struct {
	cfg Config
	sem chan struct{}
}

func New(cfg Config) *Client {
	if cfg.Bin == "" {
		cfg.Bin = "yt-dlp"
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 3
	}
	if cfg.SearchTimeout <= 0 {
		cfg.SearchTimeout = 30 * time.Second
	}
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = 5 * time.Minute
	}
	return &Client{cfg: cfg, sem: make(chan struct{}, cfg.Concurrency)}
}

// Info is what a search resolves to.
type Info struct {
	Title     string
	URL       string // canonical watch URL
	Thumbnail string
	Seconds   float64 // 0 when unknown, for example a live stream
}

type rawInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	WebpageURL string  `json:"webpage_url"`
	Thumbnail  string  `json:"thumbnail"`
	Duration   float64 `json:"duration"`
	Thumbnails []struct {
		URL string `json:"url"`
	} `json:"thumbnails"`
	Entries []rawInfo `json:"entries"`
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// baseArgs are the flags every call shares.
func (c *Client) baseArgs() []string {
	args := []string{"--no-warnings", "--no-playlist", "--ignore-config"}
	if c.cfg.CookiesFile != "" {
		args = append(args, "--cookies", c.cfg.CookiesFile)
	}
	return append(args, c.cfg.ExtraArgs...)
}

// Search resolves a song name or a link to one playable result.
func (c *Client) Search(ctx context.Context, query string) (*Info, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("ytdlp: empty query")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.SearchTimeout)
	defer cancel()

	args := append(c.baseArgs(), "--skip-download", "--dump-single-json")
	target := query
	if !isURL(query) {
		// A name becomes a one-result YouTube search. Flat mode returns the
		// id, title and length without fetching the whole page, which is
		// what keeps /play quick.
		target = "ytsearch1:" + query
		args = append(args, "--flat-playlist")
	}
	// "--" so a query that starts with "-" can never be read as a flag.
	args = append(args, "--", target)

	out, err := c.run(ctx, args)
	if err != nil {
		return nil, err
	}
	var raw rawInfo
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("ytdlp: unreadable search result")
	}
	if len(raw.Entries) > 0 {
		raw = raw.Entries[0]
	}
	info := raw.toInfo()
	if info.URL == "" {
		return nil, fmt.Errorf("ytdlp: no result for that search")
	}
	return &info, nil
}

func (r rawInfo) toInfo() Info {
	watch := r.WebpageURL
	if watch == "" && isURL(r.URL) {
		watch = r.URL
	}
	if watch == "" && r.ID != "" {
		watch = "https://www.youtube.com/watch?v=" + r.ID
	}
	thumb := r.Thumbnail
	if thumb == "" && len(r.Thumbnails) > 0 {
		thumb = r.Thumbnails[len(r.Thumbnails)-1].URL
	}
	if thumb == "" && r.ID != "" {
		thumb = "https://i.ytimg.com/vi/" + r.ID + "/hqdefault.jpg"
	}
	return Info{Title: r.Title, URL: watch, Thumbnail: thumb, Seconds: r.Duration}
}

// minValidFileBytes is the smallest size a real audio file could be.
// Anything smaller is an error body that slipped through.
const minValidFileBytes = 1024

const downloadRetries = 3

// Download fetches the best audio for watchURL into destPath. validate, if
// not nil, can reject a finished download (for example one shorter than the
// track); a rejected file is deleted and the download retried.
func (c *Client) Download(ctx context.Context, watchURL, destPath string, validate func(string) error) error {
	if !isURL(watchURL) {
		return errors.New("ytdlp: not a link")
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < downloadRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		err := c.downloadOnce(ctx, watchURL, destPath, validate)
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, errPermanent) {
			return err
		}
	}
	return lastErr
}

func (c *Client) downloadOnce(ctx context.Context, watchURL, destPath string, validate func(string) error) error {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.DownloadTimeout)
	defer cancel()

	// A unique temp name, renamed into place at the end. destPath is derived
	// from the song alone, so two chats asking for the same track would
	// otherwise write over each other, and a stream already reading the old
	// file would turn to garbage. rename is atomic and a reader that already
	// holds the old file keeps it.
	tmp, err := os.CreateTemp(filepath.Dir(destPath), filepath.Base(destPath)+".part-*")
	if err != nil {
		return fmt.Errorf("ytdlp: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	args := append(c.baseArgs(),
		"-f", "bestaudio[ext=webm]/bestaudio/best",
		"--no-part", "--no-mtime", "--force-overwrites",
		"-o", tmpPath,
		"--", watchURL,
	)
	if _, err := c.run(ctx, args); err != nil {
		return err
	}

	fi, err := os.Stat(tmpPath)
	if err != nil || fi.Size() < minValidFileBytes {
		return fmt.Errorf("ytdlp: downloaded file is empty or too small")
	}
	// Check the real bytes, not the name: a restricted video can come back as
	// a thumbnail image or an HTML page, which would only fail later inside
	// ffmpeg with no useful reason.
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	var head [512]byte
	n, _ := io.ReadFull(f, head[:])
	f.Close()
	if ct := http.DetectContentType(head[:n]); strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "text/html") {
		return permanentf("download was %s, not audio, the video is probably restricted", ct)
	}

	if validate != nil {
		if err := validate(tmpPath); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("ytdlp: publish download: %w", err)
	}
	return nil
}

var errPermanent = errors.New("permanent")

func permanentf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errPermanent, fmt.Sprintf(format, args...))
}

// permanentMarkers are yt-dlp messages meaning another try will fail the same
// way, so retrying only makes the person wait.
var permanentMarkers = []string{
	"video unavailable", "private video", "this video is not available",
	"has been removed", "copyright", "members-only", "age-restricted",
	"confirm your age", "not made this video available", "sign in to confirm",
}

// run executes yt-dlp and returns its stdout. Failures come back as a short
// message built from yt-dlp's own last error line. The command line is never
// included, since it holds the cookies path and may end up in a chat.
func (c *Client) run(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.cfg.Bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("ytdlp: timed out")
		}
		msg := lastErrorLine(stderr.String())
		low := strings.ToLower(msg)
		if strings.Contains(low, "sign in to confirm") {
			return nil, permanentf("YouTube is asking for a login. Add cookies (see the README)")
		}
		for _, m := range permanentMarkers {
			if strings.Contains(low, m) {
				return nil, permanentf("%s", msg)
			}
		}
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return nil, errors.New("ytdlp: yt-dlp is not installed")
		}
		if msg == "" {
			msg = "yt-dlp failed"
		}
		return nil, fmt.Errorf("ytdlp: %s", msg)
	}
	return stdout.Bytes(), nil
}

func lastErrorLine(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "ERROR:") {
			l = strings.TrimSpace(strings.TrimPrefix(l, "ERROR:"))
			if len(l) > 200 {
				l = l[:200]
			}
			return l
		}
	}
	return ""
}

// Strip removes the internal "permanent: " marker so text shown to people
// reads naturally.
func Strip(err error) string {
	return strings.TrimPrefix(err.Error(), errPermanent.Error()+": ")
}
