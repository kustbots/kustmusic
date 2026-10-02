/*
 * KustMusic - Telegram Music Bot
 * Copyright (c) 2026 KustBots
 * Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 * Licensed under GNU GPL v3
 * See https://github.com/kustbots/kustmusic
 */

package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "github.com/joho/godotenv/autoload"
)

const (
	defaultDBName         = "KustMusic"
	defaultAPIURL         = ""
	defaultService        = "youtube"
	defaultDownloadsDir   = "downloads"
	defaultSupportGroup   = "https://t.me/kustbhai"
	defaultSupportChannel = "https://t.me/kustbots"
	defaultCommunity      = "https://t.me/kustbotschat"
	localCookiesDir       = "cookies"
	defaultStartImage     = "https://raw.githubusercontent.com/kustbots/kustmusic/main/assets/start.jpg"
	defaultMaxFileSize    = int64(500 * 1024 * 1024)
	defaultSongDuration   = int32(3600)
	defaultSessionType    = "pyrogram"
	defaultMaxSessions    = 10
	defaultAutoPlay       = int32(10)
	defaultAutoLeave      = false
	defaultVideoPlayback  = true
)

var (
	ApiId          = getEnvInt32("API_ID", 0)
	ApiHash        = os.Getenv("API_HASH")
	Token          = os.Getenv("TOKEN")
	DlBotToken     = os.Getenv("DL_BOT_TOKEN")
	SessionStrings = getSessionStrings("STRING", defaultMaxSessions)
	SessionType    = getEnv("SESSION_TYPE", defaultSessionType)

	MongoUri = os.Getenv("MONGO_URI")
	DbName   = getEnv("DB_NAME", defaultDBName)

	ApiUrl = getEnv("API_URL", defaultAPIURL)
	ApiKey = os.Getenv("API_KEY")

	OwnerId  = getEnvInt64("OWNER_ID", 0)
	LoggerId = getEnvInt64("LOGGER_ID", 0)

	Proxy             = os.Getenv("PROXY")
	DefaultService    = strings.ToLower(getEnv("DEFAULT_SERVICE", defaultService))
	AutoPlayLimit     = getEnvInt32("AUTO_PLAY_LIMIT", defaultAutoPlay)
	MaxFileSize       = getEnvInt64("MAX_FILE_SIZE", defaultMaxFileSize)
	SongDurationLimit = getEnvInt32("SONG_DURATION_LIMIT", defaultSongDuration)
	DownloadsDir      = getEnv("DOWNLOADS_DIR", defaultDownloadsDir)

	SupportGroup   = getEnv("SUPPORT_GROUP", defaultSupportGroup)
	SupportChannel = getEnv("SUPPORT_CHANNEL", defaultSupportChannel)
	Community      = getEnv("COMMUNITY", defaultCommunity)
	StartImg       = getEnv("START_IMG", defaultStartImage)

	AutoLeave           = getEnvBool("AUTO_LEAVE", defaultAutoLeave)
	EnableVideoPlayback = getEnvBool("ENABLE_VPLAY", defaultVideoPlayback)

	DEVS        []int64
	CookiesPath []string
	cookiesUrl  = processCookieURLs(os.Getenv("COOKIES_URL"))
)

func LoadEnv() error {
	loadDevelopers()

	if OwnerId != 0 && !slices.Contains(DEVS, OwnerId) {
		DEVS = append(DEVS, OwnerId)
	}

	if err := validate(); err != nil {
		return fmt.Errorf("config is invalid: %w", err)
	}

	if err := os.MkdirAll(DownloadsDir, 0o755); err != nil {
		return errors.New("failed to create downloads directory")
	}

	if files, err := filepath.Glob(filepath.Join(localCookiesDir, "*.txt")); err == nil {
		CookiesPath = append(CookiesPath, files...)
	}

	if len(cookiesUrl) == 0 {
		return nil
	}

	if err := os.MkdirAll(cookiesDr, 0o750); err != nil {
		return errors.New("failed to create cookies directory")
	}

	go saveAllCookies(cookiesUrl)
	return nil
}

func validate() error {
	required := []struct {
		name  string
		valid bool
	}{
		{"API_ID", ApiId > 0},
		{"API_HASH", ApiHash != ""},
		{"TOKEN", Token != ""},
		{"MONGO_URI", MongoUri != ""},
		{"OWNER_ID", OwnerId > 0},
	}

	var missing []string

	for _, field := range required {
		if !field.valid {
			missing = append(missing, field.name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"missing required configuration: %s",
			strings.Join(missing, ", "),
		)
	}

	if len(SessionStrings) == 0 {
		return fmt.Errorf(
			"at least one session string is required (STRING or STRING1-%d)",
			defaultMaxSessions,
		)
	}

	if MaxFileSize <= 0 {
		return fmt.Errorf("MAX_FILE_SIZE must be greater than 0")
	}

	if SongDurationLimit <= 0 {
		return fmt.Errorf("SONG_DURATION_LIMIT must be greater than 0")
	}

	if !isValidService(DefaultService) {
		slog.Warn("invalid default service, falling back to default",
			"service", DefaultService,
			"default", defaultService,
		)

		DefaultService = defaultService
	}

	if AutoPlayLimit < 0 {
		AutoPlayLimit = 5
	}

	return nil
}

func isValidService(service string) bool {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "youtube", "spotify":
		return true
	default:
		return false
	}
}
