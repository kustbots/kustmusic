package playback

import (
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"os"
	"sync"
	"time"
)

// defaultMaxCacheBytes caps how much downloaded audio is allowed to sit on
// the dyno at once.
//
// There was no cap at all before: every song ever played stayed on disk for
// the life of the process, so a busy server's disk grew without bound until
// something restarted it. That failure is load-shaped — it takes sustained
// traffic and a lot of distinct songs to hit — which makes it read like a
// capacity problem even though adding dynos doesn't fix it: each new dyno
// just fills up too. A restart appears to cure it because the dyno's
// filesystem is wiped, which is exactly what makes it confusing to chase.
const defaultMaxCacheBytes = 512 << 20 // 512 MiB

type cacheEntry struct {
	path     string
	size     int64
	lastUsed time.Time
}

// SongCache maps a watch URL to a completed local download's path, so a
// repeat request for the same song can skip straight to playing the
// existing file instead of re-fetching/re-downloading it. Entries are
// evicted least-recently-used once the total on disk passes maxBytes.
type SongCache struct {
	mu       sync.Mutex
	entries  map[string]*cacheEntry
	total    int64
	maxBytes int64
}

func NewSongCache(maxBytes int64) *SongCache {
	if maxBytes <= 0 {
		maxBytes = defaultMaxCacheBytes
	}
	return &SongCache{entries: make(map[string]*cacheEntry), maxBytes: maxBytes}
}

// Get returns the cached path for watchURL if it's set and the file still
// exists on disk (a dyno restart clears the filesystem but not this map),
// and marks it as just-used so eviction prefers genuinely cold entries.
func (c *SongCache) Get(watchURL string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[watchURL]
	if !ok {
		return "", false
	}
	if _, err := os.Stat(e.path); err != nil {
		// Gone from disk underneath us — drop it so the accounting doesn't
		// keep charging for bytes that aren't there any more.
		delete(c.entries, watchURL)
		c.total -= e.size
		return "", false
	}
	e.lastUsed = time.Now()
	return e.path, true
}

func (c *SongCache) Set(watchURL, path string) {
	info, err := os.Stat(path)
	if err != nil {
		return // nothing usable to record
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.entries[watchURL]; ok {
		c.total -= old.size
	}
	c.entries[watchURL] = &cacheEntry{path: path, size: info.Size(), lastUsed: time.Now()}
	c.total += info.Size()
	c.evictLocked()
}

// evictLocked deletes least-recently-used files until the cache is back
// under its size cap.
//
// Removing a file that ffmpeg is still reading is safe on Linux: unlink
// only drops the directory entry, and the inode survives until the last
// open descriptor closes, so an in-flight stream plays to its end off a
// file that no longer has a name. A later request for that song simply
// misses the cache and downloads it again.
func (c *SongCache) evictLocked() {
	for c.total > c.maxBytes && len(c.entries) > 0 {
		var oldestKey string
		var oldest *cacheEntry
		for k, e := range c.entries {
			if oldest == nil || e.lastUsed.Before(oldest.lastUsed) {
				oldestKey, oldest = k, e
			}
		}
		if oldest == nil {
			return
		}
		if err := os.Remove(oldest.path); err != nil && !os.IsNotExist(err) {
			// Couldn't reclaim it — stop rather than spin forever on a file
			// that refuses to go, and let the next Set try again.
			slog.Warn("song cache: couldn't evict file", "path", oldest.path, "err", err)
			delete(c.entries, oldestKey)
			c.total -= oldest.size
			return
		}
		delete(c.entries, oldestKey)
		c.total -= oldest.size
		slog.Info("song cache: evicted least-recently-used download",
			"path", oldest.path, "freed_bytes", oldest.size, "total_bytes", c.total)
	}
}

// Stats reports what the cache is holding, for /status. Surfacing this is
// the difference between "the fleet feels unhealthy under load" and seeing
// that a dyno is sitting on a full disk.
func (c *SongCache) Stats() (bytes int64, files int, maxBytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total, len(c.entries), c.maxBytes
}

// cacheFileName derives a stable filename from watchURL — shared by every
// chat/request for the same song, unlike the old per-chat-per-generation
// naming, which downloaded a fresh copy every time even for a song that was
// already sitting on disk from a different chat's request.
func cacheFileName(watchURL, ext string) string {
	sum := sha1.Sum([]byte(watchURL))
	return hex.EncodeToString(sum[:]) + ext
}
