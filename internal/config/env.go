package config

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
)

func loadDevelopers() {
	value := strings.TrimSpace(os.Getenv("DEVS"))
	if value == "" {
		return
	}

	value = strings.NewReplacer(
		",", " ",
		"\n", " ",
		"\r", " ",
		"\t", " ",
	).Replace(value)

	for idStr := range strings.FieldsSeq(value) {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			slog.Warn("invalid developer ID",
				"id", idStr,
				"error", err,
			)
			continue
		}

		if !slices.Contains(DEVS, id) {
			DEVS = append(DEVS, id)
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		slog.Warn("invalid integer configuration",
			"key", key,
			"value", value,
			"default", defaultValue,
			"error", err,
		)
		return defaultValue
	}

	return result
}

func getEnvInt32(key string, defaultValue int32) int32 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	result, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		slog.Warn("invalid integer configuration",
			"key", key,
			"value", value,
			"default", defaultValue,
			"error", err,
		)
		return defaultValue
	}

	return int32(result)
}

func getEnvBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	result, err := strconv.ParseBool(value)
	if err != nil {
		slog.Warn("invalid boolean configuration",
			"key", key,
			"value", value,
			"default", defaultValue,
			"error", err,
		)
		return defaultValue
	}

	return result
}

func getSessionStrings(prefix string, max int) []string {
	sessions := make([]string, 0, max+1)

	for i := 1; i <= max; i++ {
		key := fmt.Sprintf("%s%d", prefix, i)

		if session := strings.TrimSpace(os.Getenv(key)); session != "" {
			sessions = append(sessions, session)
		}
	}

	if session := strings.TrimSpace(os.Getenv(prefix)); session != "" {
		sessions = append(sessions, session)
	}

	return sessions
}

func processCookieURLs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	var urls []string

	for url := range strings.SplitSeq(value, ",") {
		url = strings.TrimSpace(url)

		if url == "" {
			continue
		}

		if !slices.Contains(urls, url) {
			urls = append(urls, url)
		}
	}

	return urls
}
