/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	td "github.com/kustbots/gotdbot"
)

func runShellCommand(cmd string, timeout time.Duration) (string, string, int) {
	var shell string
	var args []string

	if runtime.GOOS == "windows" {
		shell = "cmd"
		args = []string{"/C", cmd}
	} else {
		shell = "bash"
		args = []string{"-c", cmd}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := exec.CommandContext(ctx, shell, args...)

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Sprintf("Command timed out after %v seconds", timeout.Seconds()), -1
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), exitCode
}

func shellRunner(c *td.Client, m *td.Message) error {
	args := strings.TrimSpace(Args(m))
	if args == "" {
		_, _ = m.ReplyText(c, "Usage: /sh cmd", nil)
		return td.EndGroups
	}

	msg, err := m.ReplyText(c, "Running...", nil)
	if err != nil {
		return td.EndGroups
	}

	commands := strings.Split(args, "\n")
	var outputParts []string

	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}

		stdout, stderr, code := runShellCommand(cmd, 300*time.Second)
		part := fmt.Sprintf("<b>Command:</b> <code>%s</code>\n", cmd)
		if stdout != "" {
			part += fmt.Sprintf("<b>Output:</b>\n<pre>%s</pre>\n", stdout)
		}

		if stderr != "" {
			part += fmt.Sprintf("<b>Error:</b>\n<pre>%s</pre>\n", stderr)
		}
		part += fmt.Sprintf("<b>Exit Code:</b> <code>%d</code>\n", code)
		outputParts = append(outputParts, part)
	}

	finalOutput := strings.Join(outputParts, "\n")
	if strings.TrimSpace(finalOutput) == "" {
		finalOutput = "<b>📭 No output was returned</b>"
	}

	if len(finalOutput) <= 3500 {
		_, _ = msg.EditText(c, finalOutput, &td.EditTextMessageOpts{ParseMode: "HTML"})
		return td.EndGroups
	}

	file := filepath.Join(config.DownloadsDir, fmt.Sprintf("%d.txt", time.Now().UnixNano()))
	if err := os.WriteFile(file, []byte(finalOutput), 0644); err != nil {
		_, _ = msg.EditText(c, fmt.Sprintf("Failed to write output: %v", err), nil)
		return td.EndGroups
	}
	defer os.Remove(file)

	_, err = msg.EditMedia(c, td.InputMessageDocument{
		Document: &td.InputDocument{Document: td.InputFileLocal{Path: file}},
	}, nil)

	if err != nil {
		_, _ = msg.EditText(c, "Error: "+err.Error(), nil)
		return td.EndGroups
	}

	return td.EndGroups
}

func shellCommand(c *td.Client, m *td.Message) error {
	if !isDev(c, m) {
		return td.EndGroups
	}

	return shellRunner(c, m)
}
