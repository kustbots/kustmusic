/*
 * TgMusicBot - Telegram Music Bot
 * Copyright (c) 2025-2026 Ashok Shau
 *
 * Licensed under GNU GPL v3
 * See https://github.com/AshokShau/TgMusicBot
 */

package config

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	cookiesDr      = "src/cookies"
	maxCookieSize  = 10 << 20 // 10 MiB
	requestTimeout = 30 * time.Second
)

var cookieHTTPClient = &http.Client{
	Timeout: requestTimeout,
}

// fetchContent downloads content from Pastebin, Batbin, or a direct URL.
func fetchContent(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	downloadURL, err := resolveRawURL(rawURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request for %q: %w", downloadURL, err)
	}

	req.Header.Set("User-Agent", "KustMusic/1.0")

	resp, err := cookieHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request %q: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"unexpected HTTP status %s for %q",
			resp.Status,
			downloadURL,
		)
	}

	reader := io.LimitReader(resp.Body, maxCookieSize+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read response from %q: %w", downloadURL, err)
	}

	if len(body) > maxCookieSize {
		return "", fmt.Errorf(
			"response from %q exceeds maximum size of %d bytes",
			downloadURL,
			maxCookieSize,
		)
	}

	return string(body), nil
}

func resolveRawURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.Trim(parsed.Path, "/")

	switch host {
	case "pastebin.com", "www.pastebin.com":
		if path == "" {
			return "", fmt.Errorf("invalid Pastebin URL %q", rawURL)
		}

		id, _, _ := strings.Cut(path, "/")
		return "https://pastebin.com/raw/" + id, nil

	case "batbin.me", "www.batbin.me":
		if path == "" {
			return "", fmt.Errorf("invalid Batbin URL %q", rawURL)
		}

		id, _, _ := strings.Cut(path, "/")
		return "https://batbin.me/raw/" + id, nil

	default:
		return rawURL, nil
	}
}

func saveContent(sourceURL, content string) (string, error) {
	filename := cookieFilename(sourceURL)

	if err := os.MkdirAll(cookiesDr, 0o750); err != nil {
		return "", fmt.Errorf("create cookies directory: %w", err)
	}

	filePath := filepath.Join(cookiesDr, filename)

	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write cookie file %q: %w", filePath, err)
	}

	return filePath, nil
}

func cookieFilename(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		if path := strings.Trim(parsed.Path, "/"); path != "" {
			name := filepath.Base(path)
			name = sanitizeFilename(name)

			if name != "" && name != "." && name != ".." {
				return name + ".txt"
			}
		}

		if parsed.Hostname() != "" {
			return sanitizeFilename(parsed.Hostname()) + ".txt"
		}
	}

	return "cookie.txt"
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)

	var builder strings.Builder
	builder.Grow(len(name))

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}

	name = strings.Trim(builder.String(), "._")

	if name == "" {
		return "cookie"
	}

	return name
}

func saveAllCookies(urls []string) {
	if len(urls) == 0 {
		return
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}

		wg.Add(1)

		go func(rawURL string) {
			defer wg.Done()

			content, err := fetchContent(rawURL)
			if err != nil {
				slog.Warn("failed to fetch cookies", "url", rawURL, "error", err)
				return
			}

			path, err := saveContent(rawURL, content)
			if err != nil {
				slog.Warn("failed to save cookies", "url", rawURL, "error", err)
				return
			}

			mu.Lock()
			CookiesPath = append(CookiesPath, path)
			mu.Unlock()

			slog.Debug("cookies downloaded", "url", rawURL, "path", path)
		}(rawURL)
	}

	wg.Wait()

	slog.Info("cookie downloads completed", "requested", len(urls), "saved", len(CookiesPath))
}
