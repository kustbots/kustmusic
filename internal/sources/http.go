/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package sources

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/config"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 30 * time.Second
	defaultConnectTimeout = 15 * time.Second
	maxRetries            = 2
	initialBackoff        = 1 * time.Second
)

var client = &http.Client{
	Timeout: defaultRequestTimeout,
	Transport: &http.Transport{
		TLSHandshakeTimeout: defaultConnectTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},

		ResponseHeaderTimeout: defaultRequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   defaultConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 2 {
			return fmt.Errorf("too many redirects (%d)", len(via))
		}
		return nil
	},
}

func sendRequest(method, fullURL string, body io.Reader, headers map[string]string) (*http.Response, error) {
	baseReq, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create base request: %w", err)
	}

	baseReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	baseReq.Header.Set("Accept", "*/*")

	for k, v := range headers {
		baseReq.Header.Set(k, v)
	}

	var resp *http.Response
	var reqErr error
	backoff := initialBackoff

	for attempt := range maxRetries {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
		req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
		if err != nil {
			cancel()
			reqErr = err
			break
		}
		req.Header = baseReq.Header.Clone()

		resp, reqErr = client.Do(req)
		if reqErr == nil {
			if resp.StatusCode < 500 {
				resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
				return resp, nil
			}
			cancel()
			if err = resp.Body.Close(); err != nil {
				slog.Info("failed to close response body", "error", err)
			}
			reqErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		} else {
			cancel()
			if isTemporaryError(reqErr) {
				slog.Info("Temporary error on", "attempt", attempt+1, "maxRetries", maxRetries, "error", reqErr)
				continue
			}
			break // Do not retry on permanent errors
		}
	}

	if reqErr == nil {
		reqErr = fmt.Errorf("request failed after %d attempts", maxRetries)
	}

	return nil, fmt.Errorf("request failed: %s", reqErr.Error())
}

func isTemporaryError(err error) bool {
	if netErr, ok := errors.AsType[net.Error](err); ok {
		return netErr.Timeout()
	}
	return false
}

func generateUniqueName(ext string) string {
	n, _ := rand.Int(rand.Reader, big.NewInt(99999))
	return fmt.Sprintf("%d_%05d%s", time.Now().UnixNano(), n.Int64(), ext)
}

func determineFilename(urlStr, contentDisp string) string {
	if filename := extractFilename(contentDisp); filename != "" {
		return filepath.Join(config.DownloadsDir, sanitizeFilename(filename))
	}

	if parsedURL, err := url.Parse(urlStr); err == nil {
		filename := path.Base(parsedURL.Path)
		if filename != "" && filename != "/" && !strings.Contains(filename, "?") {
			return filepath.Join(config.DownloadsDir, sanitizeFilename(filename))
		}
	}

	return filepath.Join(config.DownloadsDir, generateUniqueName(".tmp"))
}

func writeToFile(filename string, data io.Reader) error {
	out, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create the file: %w", err)
	}
	defer func(out *os.File) {
		_ = out.Close()
	}(out)

	if _, err := io.Copy(out, data); err != nil {
		return fmt.Errorf("failed to write to the file: %w", err)
	}

	return nil
}

func downloadFile(urlStr, fileName string, overwrite bool) (string, error) {
	if urlStr == "" {
		return "", errors.New("an empty URL was provided")
	}

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create the request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("the request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code received: %d", resp.StatusCode)
	}

	if fileName == "" {
		fileName = determineFilename(urlStr, resp.Header.Get("Content-Disposition"))
	}

	if !overwrite {
		if _, err := os.Stat(fileName); err == nil {
			return fileName, nil // File already exists, no need to download again.
		}
	}

	if err = os.MkdirAll(filepath.Dir(fileName), defaultDownloadDirPerm); err != nil {
		return "", fmt.Errorf("failed to create the directory: %w", err)
	}

	tempPath := fileName + ".part"
	if err = writeToFile(tempPath, resp.Body); err != nil {
		return "", err
	}

	if err = os.Rename(tempPath, fileName); err != nil {
		return "", fmt.Errorf("failed to rename the temporary file: %w", err)
	}

	return fileName, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
