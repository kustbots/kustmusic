/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package player

import (
	"context"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/ntgcalls"
	"sync"
	"time"

	td "github.com/kustbots/gotdbot"
	l "github.com/kustbots/gotdbot/logger"
)

var logger *l.Logger

const connectWaitTimeout = 20 * time.Second

type TelegramCalls struct {
	mu                 sync.RWMutex
	accounts           []*AssistantAccount
	statusCache        *cache.Cache[td.ChatMemberStatus]
	inviteCache        *cache.Cache[string]
	streamEndCallbacks []func(chatID int64, t ntgcalls.StreamType, d ntgcalls.StreamDevice)
	timeOffsets        map[int64]uint64
}

var (
	instance *TelegramCalls
	once     sync.Once
)

func getCalls() *TelegramCalls {
	once.Do(func() {
		instance = &TelegramCalls{
			accounts:    make([]*AssistantAccount, 0),
			statusCache: cache.NewCache[td.ChatMemberStatus](2 * time.Hour),
			inviteCache: cache.NewCache[string](2 * time.Hour),
			timeOffsets: make(map[int64]uint64),
		}
	})
	return instance
}

var Calls = getCalls()

func (c *TelegramCalls) OnStreamEnd(callback func(chatID int64, t ntgcalls.StreamType, d ntgcalls.StreamDevice)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streamEndCallbacks = append(c.streamEndCallbacks, callback)
}

func (c *TelegramCalls) RegisterHandlers(client *td.Client) {
	logger = client.Logger
	c.startAutoLeave(context.Background(), client)

	c.mu.RLock()
	accounts := c.accounts
	c.mu.RUnlock()

	c.OnStreamEnd(func(chatID int64, t ntgcalls.StreamType, d ntgcalls.StreamDevice) {
		logger.Info("[OnStreamEnd] Stream ended", "chat_id", chatID, "type", t, "device", d)
		if t == ntgcalls.VideoStream {
			return
		}

		if err := c.PlayNext(client, chatID); err != nil {
			logger.Warn("[OnStreamEnd] Failed to play the song", "error", err)
		}
	})

	for _, acc := range accounts {
		if _, err := acc.App.SendMessage(client.Me.Usernames.EditableUsername, "/start"); err != nil {
			acc.App.Log.Warnf("failed to start bot: %v", err)
		}

		if config.LoggerId == 0 {
			continue
		}

		if _, err := acc.App.SendMessage(config.LoggerId, "Userbot started."); err != nil {
			acc.App.Log.Warnf("Failed to send message: (%d) %v", config.LoggerId, err)
		}
	}
}
