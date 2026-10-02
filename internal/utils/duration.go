/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func GetMediaDuration(input string) int32 {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	args := []string{
		"-v", "error",
		"-print_format", "json",
		"-show_entries", "format=duration",
		input,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		slog.Info("ffprobe timeout exceeded for", "arg1", input)
		return 0
	}

	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		slog.Info("ffprobe failed", "arg1", msg)
		return 0
	}

	var out ffprobeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		slog.Info("ffprobe failed", "error", err)
		return 0
	}

	if out.Format.Duration == "" {
		slog.Info("ffprobe succeeded but duration not found")
		return 0
	}

	dur, err := strconv.ParseFloat(out.Format.Duration, 64)
	if err != nil {
		slog.Info("ffprobe failed", "error", err)
		return 0
	}

	return int32(dur + 0.5)
}

func SecToMin(seconds int32) string {
	if seconds <= 0 {
		return "0:00"
	}

	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60

	if d > 0 {
		return fmt.Sprintf("%dd %02d:%02d:%02d", d, h, m, s)
	}

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}

	return fmt.Sprintf("%d:%02d", m, s)
}
