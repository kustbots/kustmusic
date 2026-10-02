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
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"sort"
	"strconv"
	"strings"

	td "github.com/kustbots/gotdbot"
)

func removeHandler(c *td.Client, m *td.Message) error {
	if !adminMode(c, m) {
		return td.EndGroups
	}

	chatID := m.ChatId

	if !cache.ChatCache.IsActive(chatID) {
		_, _ = m.ReplyText(c, "The bot is not streaming in the video chat.", nil)
		return nil
	}

	queue := cache.ChatCache.GetQueue(chatID)
	if len(queue) == 0 {
		_, _ = m.ReplyText(c, "The queue is currently empty.", nil)
		return nil
	}

	args := Args(m)
	if args == "" {
		_, _ = m.ReplyText(c, "<b>Usage:</b> <code>/remove [track number or range]</code>\n\nExamples:\n- <code>/remove 1</code> (removes track #1)\n- <code>/remove 1-5</code> (removes tracks 1 to 5)\n- <code>/remove 1,3,5</code> (removes tracks 1, 3, and 5)", replyOpts)
		return nil
	}

	var tracksToRemove []int
	for part := range strings.SplitSeq(args, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				continue
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err1 == nil && err2 == nil {
				if start > end {
					start, end = end, start
				}

				if start < 1 {
					start = 1
				}
				if end >= len(queue) {
					end = len(queue)
				}
				for i := start; i <= end; i++ {
					tracksToRemove = append(tracksToRemove, i)
				}
			}
		} else {
			if val, err := strconv.Atoi(part); err == nil {
				tracksToRemove = append(tracksToRemove, val)
			}
		}
	}

	if len(tracksToRemove) == 0 {
		_, _ = m.ReplyText(c, "Please provide a valid track number or range.", nil)
		return nil
	}

	uniqueTracks := make(map[int]bool)
	var sortedTracks []int
	for _, t := range tracksToRemove {
		if t > 0 && t <= len(queue) && !uniqueTracks[t] {
			uniqueTracks[t] = true
			sortedTracks = append(sortedTracks, t)
		}
	}

	if len(sortedTracks) == 0 {
		_, _ = m.ReplyText(c, "No valid tracks to remove.", nil)
		return nil
	}

	sort.Slice(sortedTracks, func(i, j int) bool {
		return sortedTracks[i] > sortedTracks[j]
	})

	for _, t := range sortedTracks {
		cache.ChatCache.RemoveTrack(chatID, t)
	}

	var err error
	if len(sortedTracks) == 1 {
		_, err = m.ReplyText(c, fmt.Sprintf("Track #%d has been removed by %s.", sortedTracks[0], firstName(c, m)), replyOpts)
	} else {
		_, err = m.ReplyText(c, fmt.Sprintf("%d tracks have been removed by %s.", len(sortedTracks), firstName(c, m)), replyOpts)
	}

	return err
}
