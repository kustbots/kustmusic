// Package config loads the playback engine's settings from the environment.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIID            int32
	APIHash          string
	AssistantSession string
	// AssistantName is the display name given to the assistant account.
	// Empty leaves the account's name alone.
	AssistantName string

	// YTDLPPath is the yt-dlp executable. YTDLPArgs are extra flags for every
	// call. CookiesFile is a Netscape cookies file; the app writes one from
	// the COOKIES variable when that is set.
	YTDLPPath   string
	YTDLPArgs   []string
	CookiesFile string

	// BotWebhookURL is where stream-ended and left-idle events are posted.
	// The app points it at the bot running in the same process.
	BotWebhookURL string

	IdleLeaveTimeout  time.Duration
	IdleGroupLeave    time.Duration
	SweepKeepChats    []int64
	PlayRequestBudget time.Duration
	SessionStartDelay time.Duration
	CacheMaxBytes     int64

	// MongoURI is optional. With it, the idle-group clock survives restarts.
	MongoURI string
	MongoDB  string

	// Port is the playback engine's own HTTP port. It listens on 127.0.0.1
	// only, because the bot in the same process is its only client.
	Port string
}

// Load reads the environment. API_ID and API_HASH come from
// https://my.telegram.org and have no defaults: they identify whoever runs the
// bot, so they must be yours.
func Load() Config {
	return Config{
		APIID:            int32(mustAtoi(os.Getenv("API_ID"))),
		APIHash:          os.Getenv("API_HASH"),
		AssistantSession: firstNonEmpty(os.Getenv("ASSISTANT_SESSION"), os.Getenv("STRING_SESSION")),
		AssistantName:    os.Getenv("ASSISTANT_NAME"),

		YTDLPPath:   getenv("YTDLP_PATH", "yt-dlp"),
		YTDLPArgs:   strings.Fields(os.Getenv("YTDLP_ARGS")),
		CookiesFile: os.Getenv("COOKIES_FILE"),

		// How long a chat can sit idle in a voice chat after a song ends
		// before the assistant leaves it.
		IdleLeaveTimeout: time.Duration(mustAtoi(getenv("IDLE_LEAVE_TIMEOUT_SECONDS", "180"))) * time.Second,
		// How long a chat can go without playing anything before the
		// assistant leaves the group entirely, freeing a slot against
		// Telegram's per-account group limit. 0 disables it. Rejoining is
		// rate-limited by Telegram, so the default is generous: 72 hours.
		IdleGroupLeave: time.Duration(mustAtoi(getenv("IDLE_GROUP_LEAVE_SECONDS", "259200"))) * time.Second,
		// Chat ids the sweeper must never leave, comma-separated.
		SweepKeepChats: parseChatIDs(os.Getenv("SWEEP_KEEP_CHATS")),
		// The ceiling on one play request, including a fresh join's WebRTC
		// handshake.
		PlayRequestBudget: time.Duration(mustAtoi(getenv("PLAY_REQUEST_BUDGET_SECONDS", "150"))) * time.Second,
		// How long to wait after boot before touching Telegram. A server that
		// reconnects the instant it boots can race the instance it replaced,
		// which Telegram may still consider connected, and two connections on
		// one auth key from two addresses kills the session. 0 skips the wait.
		SessionStartDelay: time.Duration(mustAtoi(getenv("SESSION_START_DELAY_SECONDS", "0"))) * time.Second,
		// Ceiling on downloaded audio kept on disk, in MiB.
		CacheMaxBytes: int64(mustAtoi(getenv("CACHE_MAX_MB", "512"))) << 20,

		MongoURI: os.Getenv("MONGO_URI"),
		MongoDB:  getenv("MONGO_DB", "kustmusic"),
		Port:     getenv("PLAYER_PORT", "8081"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// parseChatIDs reads a comma-separated list of chat ids, skipping anything
// that is not one rather than failing the whole config.
func parseChatIDs(csv string) []int64 {
	var out []int64
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}
