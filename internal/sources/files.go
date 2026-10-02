/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package sources

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	downloadTimeout        = 40 * time.Second
	defaultDownloadDirPerm = 0755
)

var (
	errMissingCDNURL = errors.New("missing cdn url")
)

var (
	sanitizeRegex = regexp.MustCompile(`[<>:"/\\|?*]`)
	filenameRegex = regexp.MustCompile(`filename\*?=(?:UTF-8'')?([^;]+)`)
)

func sanitizeFilename(fileName string) string {
	fileName = strings.ReplaceAll(fileName, "/", "")
	fileName = strings.ReplaceAll(fileName, "\\", "")
	fileName = sanitizeRegex.ReplaceAllString(fileName, "")
	fileName = strings.TrimSpace(fileName)
	return fileName
}

func extractFilename(contentDisp string) string {
	if contentDisp == "" {
		return ""
	}
	matches := filenameRegex.FindStringSubmatch(contentDisp)
	if len(matches) > 1 {
		decoded, err := url.QueryUnescape(matches[1])
		if err == nil {
			return decoded
		}
	}
	return ""
}
