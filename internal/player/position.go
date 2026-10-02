package player

import (
	"fmt"
	"github.com/kustbots/kustmusic/ntgcalls"
)

func (c *TelegramCalls) setPlayedTimeOffset(chatID int64, offset uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeOffsets[chatID] = offset
}

func (c *TelegramCalls) getPlayedTimeOffset(chatID int64) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.timeOffsets[chatID]
}

func (c *TelegramCalls) clearPlayedTimeOffset(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.timeOffsets, chatID)
}

func (c *TelegramCalls) PlayedTime(chatId int64) (uint64, error) {
	acc, index, err := c.GetAccount(chatId)
	if err != nil {
		return 0, err
	}

	_time, err := acc.binding.Time(chatId, ntgcalls.CaptureStream)
	if err != nil {
		logger.Warn("Failed to get played time", "error", err, "index", index)
		return 0, fmt.Errorf("failed to get played time: %w", err)
	}

	return _time + c.getPlayedTimeOffset(chatId), nil
}
