//go:build ignore

/*
 * TgMusicBot - Telegram Music Bot
 * Copyright (c) 2025-2026 Ashok Shau
 *
 * Licensed under GNU GPL v3
 * See https://github.com/AshokShau/TgMusicBot
 */

package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	destHeader = "ntgcalls"
	destLib    = ""
	releaseURL = "https://api.github.com/repos/pytgcalls/ntgcalls/releases/tags/v3.0.0"
)

type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

type progressWriter struct {
	total   int64
	written int64
	lastPct int
}

func main() {
	start := time.Now()

	err := setup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nSetup completed successfully!")
	fmt.Printf("Time elapsed: %v\n", time.Since(start))
}

func setup() error {
	fmt.Printf("Looking for %s/%s static build...\n", runtime.GOOS, runtime.GOARCH)
	var release Release

	response, err := http.Get(releaseURL)
	if err != nil {
		return fmt.Errorf("failed to contact GitHub: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub returned %s", response.Status)
	}

	if err = json.NewDecoder(response.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to read release information: %w", err)
	}

	fmt.Printf("Latest release: %s\n", release.TagName)

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	switch goos {
	case "darwin":
		goos = "macos"
	case "windows":
		goos = "windows"
	}

	switch goarch {
	case "amd64":
		goarch = "x86_64"
	case "arm64":
		goarch = "arm64"
	}

	expectedName := fmt.Sprintf("ntgcalls.%s-%s-static_libs.zip", goos, goarch)
	var downloadURL string

	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, expectedName) {
			downloadURL = asset.URL
			fmt.Printf("Found: %s\n", asset.Name)
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("could not find a static build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	zipFile := "ntgcalls.zip"
	tempDir := "ntgcalls_tmp"

	fmt.Printf("Downloading: %s\n", expectedName)

	response, err = http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", response.Status)
	}

	file, err := os.Create(zipFile)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", zipFile, err)
	}

	writer := &progressWriter{
		total:   response.ContentLength,
		lastPct: -1,
	}

	_, copyErr := io.Copy(
		io.MultiWriter(file, writer),
		response.Body,
	)

	closeErr := file.Close()

	if copyErr != nil {
		os.Remove(zipFile)
		return fmt.Errorf("failed to download file: %w", copyErr)
	}

	if closeErr != nil {
		os.Remove(zipFile)
		return fmt.Errorf("failed to close downloaded file: %w", closeErr)
	}

	if writer.lastPct >= 0 {
		fmt.Println()
	}

	defer os.Remove(zipFile)

	fmt.Println("Extracting...")

	reader, err := zip.OpenReader(zipFile)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	if err = os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	for _, file := range reader.File {
		target := filepath.Join(tempDir, file.Name)
		cleanTarget := filepath.Clean(target)
		cleanTempDir := filepath.Clean(tempDir) + string(os.PathSeparator)

		if !strings.HasPrefix(cleanTarget, cleanTempDir) {
			return fmt.Errorf("invalid file path in archive: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, file.Mode()); err != nil {
				return fmt.Errorf(
					"failed to create directory %s: %w",
					file.Name,
					err,
				)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		input, err := file.Open()
		if err != nil {
			return fmt.Errorf(
				"failed to open %s from archive: %w",
				file.Name,
				err,
			)
		}

		output, err := os.OpenFile(
			cleanTarget,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
			file.Mode(),
		)
		if err != nil {
			input.Close()
			return fmt.Errorf(
				"failed to create %s: %w",
				file.Name,
				err,
			)
		}

		_, err = io.Copy(output, input)

		input.Close()
		output.Close()

		if err != nil {
			return fmt.Errorf(
				"failed to extract %s: %w",
				file.Name,
				err,
			)
		}
	}

	var copiedFiles []string

	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		filename := filepath.Base(path)

		var destination string

		if filename == "ntgcalls.h" {
			destination = filepath.Join(destHeader, filename)
		} else if strings.HasPrefix(filename, "libntgcalls.") ||
			strings.HasPrefix(filename, "ntgcalls.") {
			destination = filepath.Join(destLib, filename)
		} else {
			return nil
		}

		if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return fmt.Errorf(
				"failed to create destination directory: %w",
				err,
			)
		}

		input, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", path, err)
		}
		defer input.Close()

		output, err := os.Create(destination)
		if err != nil {
			return fmt.Errorf(
				"failed to create %s: %w",
				destination,
				err,
			)
		}

		_, err = io.Copy(output, input)

		if closeErr := output.Close(); err == nil {
			err = closeErr
		}

		if err != nil {
			return fmt.Errorf(
				"failed to copy %s: %w",
				filename,
				err,
			)
		}

		if info, err := os.Stat(path); err == nil {
			_ = os.Chmod(destination, info.Mode())
		}

		copiedFiles = append(copiedFiles, destination)

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to organize files: %w", err)
	}

	if len(copiedFiles) == 0 {
		return fmt.Errorf("no ntgcalls files were found in the archive")
	}

	fmt.Println("Files copied:")

	for _, file := range copiedFiles {
		relativePath, err := filepath.Rel(".", file)
		if err != nil {
			relativePath = file
		}

		fmt.Printf("  ✓ %s\n", relativePath)
	}

	return nil
}

func (pw *progressWriter) Write(data []byte) (int, error) {
	n := len(data)
	pw.written += int64(n)

	if pw.total <= 0 {
		return n, nil
	}

	percentage := int(
		float64(pw.written) / float64(pw.total) * 100,
	)

	if percentage != pw.lastPct && percentage%10 == 0 {
		fmt.Printf("\r Progress: %d%%", percentage)
		pw.lastPct = percentage
	}

	return n, nil
}
