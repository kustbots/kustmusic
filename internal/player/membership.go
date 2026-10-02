/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package player

import (
	"context"
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/config"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	td "github.com/AshokShau/gotdbot"
	tg "github.com/amarnathcjd/gogram/telegram"
)

const inviteLinkName = "KustBots"
const maxLeaveRetries = 2
const autoLeaveInterval = 18 * time.Hour

func handleFlood(err error) bool {
	wait := tg.GetFloodWait(err)
	if wait <= 0 {
		return false
	}

	if wait > 5 {
		logger.Warn("Flood wait too long, skipping sleep", "seconds", wait)
		return false
	}

	logger.Warn("Flood wait detected, sleeping", "seconds", wait)
	time.Sleep(time.Duration(wait+1) * time.Second)
	return true
}

func (c *TelegramCalls) UpdateMembership(chatId, userId int64, status td.ChatMemberStatus) {
	cacheKey := fmt.Sprintf("%d:%d", chatId, userId)
	c.statusCache.Set(cacheKey, status)
	logger.Infof("Updated membership status for user %d in chat %d: %T", userId, chatId, status)
}

func (c *TelegramCalls) UpdateInviteLink(chatId int64, link string) {
	cacheKey := strconv.FormatInt(chatId, 10)
	if link == "" {
		c.inviteCache.Delete(cacheKey)
		return
	}

	c.inviteCache.Set(cacheKey, link)
}

func (c *TelegramCalls) ClearChatCache(chatId int64) {
	c.inviteCache.Delete(strconv.FormatInt(chatId, 10))
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, acc := range c.accounts {
		if acc != nil && acc.App != nil && acc.App.Me() != nil {
			c.statusCache.Delete(fmt.Sprintf("%d:%d", chatId, acc.App.Me().ID))
		}
	}
}

func (c *TelegramCalls) LeaveAll() (int, error) {
	var totalLeft atomic.Int64
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	c.mu.RLock()
	var accounts []*AssistantAccount
	accounts = append(accounts, c.accounts...)
	c.mu.RUnlock()

	for _, acc := range accounts {
		wg.Add(1)
		go func(a *AssistantAccount) {
			defer wg.Done()
			count, err := c.leaveAssistantDialogs(a)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}
			totalLeft.Add(int64(count))
		}(acc)
	}

	wg.Wait()
	return int(totalLeft.Load()), firstErr
}

func (c *TelegramCalls) LeaveAllForAccount(index int) (int, error) {
	c.mu.RLock()
	if index < 0 || index >= len(c.accounts) {
		c.mu.RUnlock()
		return 0, fmt.Errorf("no assistant account was found for index %d", index)
	}
	acc := c.accounts[index]
	c.mu.RUnlock()
	return c.leaveAssistantDialogs(acc)
}

func (c *TelegramCalls) leaveAssistantDialogs(acc *AssistantAccount) (int, error) {
	userBot := acc.App
	var totalLeft int
	dialogs, err := userBot.GetDialogs(&tg.DialogOptions{
		Limit:            -1,
		SleepThresholdMs: 20,
	})
	if err != nil {
		return 0, fmt.Errorf("account %s: failed to get dialogs: %w",
			userBot.Me().FirstName, err)
	}

	logger.Info("found dialogs",
		"user", userBot.Me().FirstName,
		"count", len(dialogs),
	)

	for _, d := range dialogs {
		var chatID int64
		switch p := d.Peer.(type) {
		case *tg.PeerChannel:
			chatID = p.ChannelID
		case *tg.PeerChat:
			chatID = p.ChatID
		default:
			continue
		}

		if chatID == 0 {
			continue
		}

		if cache.ChatCache.IsActive(chatID) {
			continue
		}

		retries := 0
		for {
			if cache.ChatCache.IsActive(chatID) {
				break
			}

			err = userBot.LeaveChannel(chatID)
			if err == nil {
				totalLeft++
				break
			}
			if strings.Contains(err.Error(), "USER_NOT_PARTICIPANT") ||
				strings.Contains(err.Error(), "CHANNEL_PRIVATE") {
				break
			}
			wait := tg.GetFloodWait(err)
			if wait > 0 {
				retries++
				if retries > maxLeaveRetries {
					logger.Warn("leave: giving up after repeated flood waits",
						"user", userBot.Me().FirstName,
						"chat_id", chatID,
						"retries", retries,
					)
					break
				}
				logger.Warn("flood wait",
					"user", userBot.Me().FirstName,
					"chat_id", chatID,
					"seconds", wait,
				)
				time.Sleep(time.Duration(wait+20) * time.Second)
				continue
			}
			logger.Warn("leave failed",
				"user", userBot.Me().FirstName,
				"chat_id", chatID,
				"error", err,
			)
			break
		}

		time.Sleep(1 * time.Second)
	}
	return totalLeft, nil
}

func (c *TelegramCalls) startAutoLeave(ctx context.Context, bot *td.Client) {
	if !config.AutoLeave {
		return
	}
	go func() {
		logger.Info("AutoLeave enabled, starting background task",
			"interval", autoLeaveInterval)
		ticker := time.NewTicker(autoLeaveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logger.Info("AutoLeave: background task stopped")
				return
			case <-ticker.C:
				c.runAutoLeave(bot)
			}
		}
	}()
}

func (c *TelegramCalls) runAutoLeave(bot *td.Client) {
	logger.Info("AutoLeave: leaving inactive chats")
	leftCount, err := c.LeaveAll()
	if err != nil {
		logger.Error("AutoLeave: failed to leave chats", "error", err)
		return
	}

	logger.Info("AutoLeave: completed", "leftCount", leftCount)
	if leftCount > 0 && config.LoggerId != 0 {
		msg := fmt.Sprintf("AutoLeave: Assistant left %d inactive chats", leftCount)
		if _, err = bot.SendTextMessage(config.LoggerId, msg, nil); err != nil {
			logger.Error("AutoLeave: failed to send log message", "error", err)
		}
	}
}

// JoinAssistant ensures the assistant joins the specified chat, optionally using a custom invite link.
func (c *TelegramCalls) JoinAssistant(bot *td.Client, chatID int64, customLink ...string) (*AssistantAccount, error) {
	acc, index, err := c.GetAccount(chatID)
	if err != nil {
		return nil, err
	}

	link := ""
	if len(customLink) > 0 {
		link = strings.TrimSpace(customLink[0])
	}

	if link != "" {
		if err := c.joinChatWithInvite(bot, chatID, acc, index, link); err != nil {
			return nil, err
		}
		return acc, nil
	}

	if err := c.ensureAssistantMember(bot, chatID, acc, index); err != nil {
		return nil, err
	}
	return acc, nil
}

func isActiveMember(status td.ChatMemberStatus) bool {
	switch status.(type) {
	case *td.ChatMemberStatusMember, td.ChatMemberStatusMember,
		*td.ChatMemberStatusAdministrator, td.ChatMemberStatusAdministrator,
		*td.ChatMemberStatusCreator, td.ChatMemberStatusCreator:
		return true
	}
	return false
}

func isLeftStatus(status td.ChatMemberStatus) bool {
	switch status.(type) {
	case *td.ChatMemberStatusLeft, td.ChatMemberStatusLeft:
		return true
	}
	return false
}

func bannedOrRestricted(status td.ChatMemberStatus) (isBanned, isRestricted bool) {
	switch status.(type) {
	case *td.ChatMemberStatusBanned, td.ChatMemberStatusBanned:
		return true, false
	case *td.ChatMemberStatusRestricted, td.ChatMemberStatusRestricted:
		return false, true
	}
	return false, false
}

func (c *TelegramCalls) ensureAssistantMember(bot *td.Client, chatID int64, acc *AssistantAccount, index int) error {
	status, err := c.checkAssistantStatus(bot, chatID, acc, index)
	if err != nil {
		return fmt.Errorf("(client%d): failed to check user status: %w", index, err)
	}

	logger.Infof("assistant-%d status in chat %d: %T", index, chatID, status)

	switch {
	case isActiveMember(status):
		return nil

	case isLeftStatus(status):
		logger.Info("assistant is not in chat, joining", "chat_id", chatID, "index", index)
		return c.joinChatWithInvite(bot, chatID, acc, index, "")

	default:
		if isBanned, isRestricted := bannedOrRestricted(status); isBanned || isRestricted {
			logger.Info("assistant is banned or restricted, attempting recovery",
				"chat_id", chatID, "banned", isBanned, "restricted", isRestricted, "index", index)
			return c.recoverBannedAssistant(bot, chatID, acc, index, isBanned, isRestricted)
		}

		logger.Warn("unknown assistant status, attempting to join", "status", status, "index", index)
		return c.joinChatWithInvite(bot, chatID, acc, index, "")
	}
}

func (c *TelegramCalls) recoverBannedAssistant(
	bot *td.Client,
	chatID int64,
	acc *AssistantAccount,
	index int,
	isBanned,
	isRestricted bool,
) error {
	assistantID := acc.App.Me().ID

	member, err := cache.GetUserAdmin(bot, chatID, bot.Me.Id, false)
	if err != nil {
		if strings.Contains(err.Error(), "is not an administrator in chat") {
			return fmt.Errorf(
				"bot is not an admin in this chat, cannot recover assistant (%d)",
				assistantID,
			)
		}
		return fmt.Errorf("failed to check bot permissions: %w", err)
	}

	admin, ok := member.Status.(*td.ChatMemberStatusAdministrator)
	if !ok || admin.Rights == nil || !admin.Rights.CanRestrictMembers {
		return fmt.Errorf(
			"the assistant (ID: <code>%d</code>) is banned or restricted and the bot lacks the "+
				"'Ban Users' permission (CanRestrictMembers) to unban it",
			assistantID,
		)
	}

	err = bot.SetChatMemberStatus(
		chatID,
		td.MessageSenderUser{UserId: assistantID},
		&td.ChatMemberStatusMember{},
	)

	if err != nil &&
		(!isBanned || !strings.Contains(err.Error(), "Bots can't add new chat members")) {
		if isBanned {
			return fmt.Errorf("failed to unban assistant: %w", err)
		}
		return fmt.Errorf("failed to lift assistant restrictions: %w", err)
	}

	if err = c.joinChatWithInvite(bot, chatID, acc, index, ""); err != nil {
		return err
	}

	c.UpdateMembership(chatID, assistantID, &td.ChatMemberStatusMember{})
	return nil
}

func (c *TelegramCalls) checkAssistantStatus(bot *td.Client, chatID int64, acc *AssistantAccount, index int) (td.ChatMemberStatus, error) {
	userID := acc.App.Me().ID
	cacheKey := fmt.Sprintf("%d:%d", chatID, userID)
	if cached, ok := c.statusCache.Get(cacheKey); ok {
		return cached, nil
	}

	member, err := bot.GetChatMember(chatID, td.MessageSenderUser{UserId: userID})
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "USER_NOT_PARTICIPANT") {
			c.UpdateMembership(chatID, userID, &td.ChatMemberStatusLeft{})
			return &td.ChatMemberStatusLeft{}, nil
		}

		return nil, fmt.Errorf("GetChatMember (client %d) chat=%d user=%d: %w", index, chatID, userID, err)
	}

	c.UpdateMembership(chatID, userID, member.Status)
	return member.Status, nil
}

func (c *TelegramCalls) joinChatWithInvite(bot *td.Client, chatID int64, acc *AssistantAccount, index int, customLink string) error {
	ub := acc.App
	cacheKey := strconv.FormatInt(chatID, 10)

	link := customLink
	var err error
	if link == "" {
		link, err = c.resolveInviteLink(bot, chatID, cacheKey)
		if err != nil {
			return err
		}
	}

	logger.Info("joining via invite link", "chat_id", chatID, "index", index, "link", link)

	_, err = ub.JoinChannel(link)
	if err != nil {
		if customLink == "" && (strings.Contains(err.Error(), "INVITE_HASH_EXPIRED") || strings.Contains(err.Error(), "INVITE_HASH_INVALID")) {
			logger.Warn("invite link expired or invalid, clearing cache and retrying with fresh link", "chat_id", chatID, "index", index)
			c.UpdateInviteLink(chatID, "")
			freshLink, createErr := c.resolveInviteLink(bot, chatID, cacheKey)
			if createErr == nil && freshLink != "" {
				_, retryErr := ub.JoinChannel(freshLink)
				if retryErr == nil {
					c.UpdateMembership(chatID, ub.Me().ID, &td.ChatMemberStatusMember{})
					return nil
				}
				err = retryErr
			}
		}

		return c.handleJoinError(bot, chatID, ub.Me().ID, index, err)
	}

	c.UpdateMembership(chatID, ub.Me().ID, &td.ChatMemberStatusMember{})
	if customLink != "" {
		c.UpdateInviteLink(chatID, customLink)
	}
	return nil
}

func (c *TelegramCalls) resolveInviteLink(bot *td.Client, chatID int64, cacheKey string) (string, error) {
	if cached, ok := c.inviteCache.Get(cacheKey); ok && cached != "" {
		return cached, nil
	}

	chatLink, err := bot.CreateChatInviteLink(
		chatID, 0, 0, inviteLinkName,
		&td.CreateChatInviteLinkOpts{CreatesJoinRequest: false},
	)

	if err != nil {
		return "", fmt.Errorf("create invite link for chat %d: %w", chatID, err)
	}

	link := chatLink.InviteLink
	if link == "" {
		return "", errors.New("telegram returned an empty invite link")
	}

	c.UpdateInviteLink(chatID, link)
	return link, nil
}

func (c *TelegramCalls) handleJoinError(bot *td.Client, chatID, userID int64, index int, err error) error {
	if wait := tg.GetFloodWait(err); wait > 0 {
		return fmt.Errorf(
			"Telegram is temporarily limiting join attempts (wait ~%ds).\n\n"+
				"Please wait a few minutes and try again.", wait,
		)
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "INVITE_REQUEST_SENT"):
		time.Sleep(time.Second)

		if approveErr := bot.ProcessChatJoinRequest(
			chatID,
			userID,
			&td.ProcessChatJoinRequestOpts{Approve: true},
		); approveErr != nil {
			slog.Warn(
				"failed to approve join request",
				"error", approveErr,
				"index", index,
			)

			return fmt.Errorf(
				"The assistant has sent a join request but I couldn't approve it automatically.\n\n" +
					"Please make sure the bot is an admin with permission to approve join requests, then try again",
			)
		}

		return nil

	case strings.Contains(errMsg, "USER_ALREADY_PARTICIPANT"):
		c.UpdateMembership(chatID, userID, &td.ChatMemberStatusMember{})
		return nil

	case strings.Contains(errMsg, "INVITE_HASH_EXPIRED"):
		cached, _ := c.inviteCache.Get(strconv.FormatInt(chatID, 10))

		logger.Warn(
			"invite link expired",
			"chat_id", chatID,
			"index", index,
			"cached_link", cached,
		)

		c.inviteCache.Delete(strconv.FormatInt(chatID, 10))
		c.UpdateMembership(chatID, userID, &td.ChatMemberStatusLeft{})

		return errors.New(
			"The assistant couldn't join because the group's invite link is no longer valid.\n\n" +
				"This usually happens when an admin revokes or resets the invite link.\n\n" +
				"Please run /reload and then try your command again.",
		)

	case strings.Contains(errMsg, "CHANNEL_PRIVATE"):
		c.inviteCache.Delete(strconv.FormatInt(chatID, 10))
		c.UpdateMembership(chatID, userID, &td.ChatMemberStatusLeft{})

		return fmt.Errorf(
			"The assistant (ID: <code>%d</code>) cannot access this group.\n\n"+
				"It may have been banned, removed, or the group privacy settings are preventing access.\n\n"+
				"Please unban the assistant (ID: <code>%d</code>) and try again.",
			userID, userID,
		)

	case strings.Contains(errMsg, "USER_BANNED_IN_CHANNEL"):
		return fmt.Errorf(
			"The assistant (ID: <code>%d</code>) is banned from this group.\n\n"+
				"Please unban the assistant (ID: <code>%d</code>) and try again.",
			userID, userID,
		)

	case strings.Contains(errMsg, "INVITE_HASH_INVALID"):
		return errors.New(
			"The invite link is invalid.\n\n" +
				"Please run /reload so the bot can generate a new invite link.",
		)
	}

	logger.Warn(
		"unhandled JoinChannel error",
		"error", err,
		"index", index,
	)

	return fmt.Errorf(
		"The assistant (ID: <code>%d</code>) couldn't join the group.\n\n"+
			"Reason: %v\n\n"+
			"If the issue continues, try:\n"+
			"• Running /reload\n"+
			"• Checking that the bot is an admin\n"+
			"• Making sure the assistant (ID: <code>%d</code>) is not banned",
		userID, err, userID,
	)
}
