/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 */

package handlers

import (
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/db"
	"github.com/kustbots/kustmusic/internal/player"
	"log/slog"
	"strconv"
	"strings"

	"github.com/AshokShau/gotdbot"
)

func handleNewChat(c *gotdbot.Client, update *gotdbot.UpdateNewChat) error {
	chat := update.Chat
	if chat == nil {
		return nil
	}

	switch chat.Type.(type) {
	case *gotdbot.ChatTypeSupergroup:
		if err := db.Instance.AddChat(chat.Id); err != nil {
			c.Logger.Warnf("Failed to add chat to database: chat_id=%d error=%v", chat.Id, err)
		}
	case *gotdbot.ChatTypePrivate:
		if err := db.Instance.AddUser(chat.Id); err != nil {
			c.Logger.Warnf("Failed to add user to database: user_id=%d error=%v", chat.Id, err)
		}
	}

	return nil
}

func handleParticipant(client *gotdbot.Client, update *gotdbot.UpdateChatMember) error {
	chatID := update.ChatId
	if chatID > 0 {
		return gotdbot.EndGroups
	}

	userID := SenderID(update.NewChatMember.MemberId)
	assistant, _, err := player.Calls.GetAccount(chatID)
	if err != nil {
		client.Logger.Error("Failed to get assistant for chat", "chat_id", chatID, "error", err)
		return gotdbot.EndGroups
	}

	assistantID := assistant.App.Me().ID
	if userID != client.Me.Id && userID != assistantID {
		return gotdbot.EndGroups
	}

	chat, err := getSupergroup(client, chatID)
	if err != nil || chat == nil {
		return gotdbot.EndGroups
	}

	if chat.Usernames != nil && chat.Usernames.EditableUsername != "" {
		player.Calls.UpdateInviteLink(chatID, "https://t.me/"+chat.Usernames.EditableUsername)
	}

	go storeChatToDB(chatID)

	oldStatus := update.OldChatMember.Status
	newStatus := update.NewChatMember.Status

	if isAdmin(oldStatus) || isAdmin(newStatus) {
		cache.UpdateAdminCache(chatID, update.NewChatMember)
	}

	if userID == client.Me.Id {
		hasDeleteRights := false
		if s, ok := newStatus.(*gotdbot.ChatMemberStatusAdministrator); ok {
			if s.Rights != nil && s.Rights.CanDeleteMessages {
				hasDeleteRights = true
			}
		}

		if !hasDeleteRights && db.Instance.GetCmdDelete(chatID) {
			_ = db.Instance.SetCmdDelete(chatID, false)
			client.Logger.Info("Bot lost delete message rights; disabled cmd_delete", "chat_id", chatID)
		}
	}

	client.Logger.Debug("Member status changed",
		"user_id", userID,
		"old_status", oldStatus,
		"new_status", newStatus,
		"chat_id", chatID,
	)

	return dispatchStatusChange(client, chatID, userID, assistantID, oldStatus, newStatus, chat)
}

func dispatchStatusChange(client *gotdbot.Client, chatID, userID, assistantID int64, oldStatus, newStatus gotdbot.ChatMemberStatus, chat *gotdbot.Supergroup) error {
	wasLeft := isStatus[*gotdbot.ChatMemberStatusLeft](oldStatus)
	nowLeft := isStatus[*gotdbot.ChatMemberStatusLeft](newStatus)
	wasMember := isStatus[*gotdbot.ChatMemberStatusMember](oldStatus)
	nowMember := isStatus[*gotdbot.ChatMemberStatusMember](newStatus)
	wasAdmin := isStatus[*gotdbot.ChatMemberStatusAdministrator](oldStatus)
	nowAdmin := isStatus[*gotdbot.ChatMemberStatusAdministrator](newStatus)
	nowBanned := isStatus[*gotdbot.ChatMemberStatusBanned](newStatus)
	wasBanned := isStatus[*gotdbot.ChatMemberStatusBanned](oldStatus)
	wasRestricted := isStatus[*gotdbot.ChatMemberStatusRestricted](oldStatus)
	nowRestricted := isStatus[*gotdbot.ChatMemberStatusRestricted](newStatus)

	switch {
	case wasLeft && (nowMember || nowAdmin || nowRestricted):
		return onJoin(client, chatID, userID, assistantID, chat)

	case (wasMember || wasAdmin || wasRestricted) && nowLeft:
		return onLeave(client, chatID, userID, assistantID)

	case nowBanned:
		return onBan(client, chatID, userID, assistantID)

	case wasBanned && nowLeft:
		slog.Info("User unbanned from chat", "user_id", userID, "chat_id", chatID)
		player.Calls.UpdateMembership(chatID, userID, &gotdbot.ChatMemberStatusLeft{})
		return nil

	case nowRestricted || wasRestricted:
		return onRestriction(client, chatID, userID, assistantID, oldStatus, newStatus)

	default:
		return onPromotionOrDemotion(client, chatID, userID, wasAdmin, nowAdmin, chat)
	}
}

func onRestriction(client *gotdbot.Client, chatID, userID, assistantID int64, oldStatus, newStatus gotdbot.ChatMemberStatus) error {
	_, wasRestricted := oldStatus.(*gotdbot.ChatMemberStatusRestricted)
	newRestricted, nowRestricted := newStatus.(*gotdbot.ChatMemberStatusRestricted)

	switch {
	case wasRestricted && !nowRestricted:
		client.Logger.Info("User restriction lifted", "user_id", userID, "chat_id", chatID)
		player.Calls.UpdateMembership(chatID, userID, newStatus)

	case !wasRestricted && nowRestricted:
		client.Logger.Info("User restricted in chat", "user_id", userID, "chat_id", chatID)
		player.Calls.UpdateMembership(chatID, userID, newStatus)

		if userID == assistantID {
			msg := fmt.Sprintf(
				"⚠️ My assistant has been restricted in this chat.\n\n"+
					"If this was a mistake, please unrestrict <code>%d</code>.",
				assistantID,
			)
			if _, err := client.SendTextMessage(chatID, msg, &gotdbot.SendTextMessageOpts{
				ParseMode: "HTML",
			}); err != nil {
				return err
			}
		}

	default:
		client.Logger.Info("User permissions updated while restricted",
			"user_id", userID,
			"chat_id", chatID,
			"can_send_basic_messages", newRestricted.Permissions.CanSendBasicMessages,
		)
		player.Calls.UpdateMembership(chatID, userID, newStatus)
	}

	return nil
}

func onJoin(client *gotdbot.Client, chatID, userID, assistantID int64, chat *gotdbot.Supergroup) error {
	client.Logger.Info("User joined chat", "user_id", userID, "chat_id", chatID)
	if userID == client.Me.Id {
		client.Logger.Info("Bot joined chat", "chat_id", chatID)
		sendJoinLog(client, chatID, chat)
	}

	player.Calls.UpdateMembership(chatID, userID, &gotdbot.ChatMemberStatusMember{})
	return nil
}

func onLeave(client *gotdbot.Client, chatID, userID, assistantID int64) error {
	client.Logger.Info("User left chat", "user_id", userID, "chat_id", chatID)
	if userID == assistantID {
		cache.ChatCache.ClearChat(chatID)
	}

	player.Calls.UpdateMembership(chatID, userID, &gotdbot.ChatMemberStatusLeft{})
	if userID == client.Me.Id {
		if err := player.Calls.Stop(chatID, false); err != nil {
			client.Logger.Error("Failed to stop VC after leave", "error", err)
		}
	}

	return nil
}

func onBan(client *gotdbot.Client, chatID, userID, assistantID int64) error {
	client.Logger.Debug("User banned from chat", "user_id", userID, "chat_id", chatID)
	player.Calls.UpdateMembership(chatID, userID, &gotdbot.ChatMemberStatusBanned{})

	if userID == assistantID {
		cache.ChatCache.ClearChat(chatID)
		msg := fmt.Sprintf(
			"🚫 My assistant has been banned from this chat.\n\n"+
				"If this was a mistake, please unban <code>%d</code>.",
			assistantID,
		)
		if _, err := client.SendTextMessage(chatID, msg, &gotdbot.SendTextMessageOpts{
			ParseMode: "HTML",
		}); err != nil {
			return err
		}
	}

	if userID == client.Me.Id {
		if err := player.Calls.Stop(chatID, true); err != nil {
			client.Logger.Error("Failed to stop VC after ban", "error", err)
		}
	}

	return nil
}

func onPromotionOrDemotion(
	client *gotdbot.Client,
	chatID, userID int64,
	wasAdmin, nowAdmin bool,
	chat *gotdbot.Supergroup,
) error {
	switch {
	case !wasAdmin && nowAdmin:
		client.Logger.Info("User promoted in chat", "user_id", userID, "chat_id", chatID)
		player.Calls.UpdateMembership(chatID, userID, &gotdbot.ChatMemberStatusAdministrator{})

	case wasAdmin && !nowAdmin:
		client.Logger.Info("User demoted in chat", "user_id", userID, "chat_id", chatID)
		player.Calls.UpdateMembership(chatID, userID, &gotdbot.ChatMemberStatusMember{})

	default:
		client.Logger.Info("onPromotionOrDemotion", "user_id", userID, "chat_id", chatID)
	}

	return nil
}

func getSupergroup(client *gotdbot.Client, chatID int64) (*gotdbot.Supergroup, error) {
	s := strings.TrimPrefix(strconv.FormatInt(chatID, 10), "-100")
	id, _ := strconv.ParseInt(s, 10, 64)

	chat, err := client.GetSupergroup(id)
	if err != nil {
		if strings.Contains(err.Error(), "Invalid supergroup identifier") {
			_ = client.LeaveChat(chatID)
			return nil, nil
		}

		client.Logger.Error("Failed to fetch supergroup", "chat_id", chatID, "error", err)
		return nil, err
	}

	if chat.IsDirectMessagesGroup {
		_ = client.LeaveChat(chatID)
		return nil, nil
	}

	return chat, nil
}

func sendJoinLog(client *gotdbot.Client, chatID int64, _ *gotdbot.Supergroup) {
	text := fmt.Sprintf("<b>🤖 Bot Joined a New Chat</b>\n📌 <b>Chat ID:</b> <code>%d</code>", chatID)
	if _, err := client.SendTextMessage(config.LoggerId, text, &gotdbot.SendTextMessageOpts{
		ParseMode: "HTML",
	}); err != nil {
		client.Logger.Warn("Failed to send join log", "error", err)
	}
}

func storeChatToDB(chatID int64) {
	slog.Debug("Storing chat reference", "chat_id", chatID)
	switch {
	case chatID > 0:
		if err := db.Instance.AddUser(chatID); err != nil {
			slog.Error("Failed to store user reference", "chat_id", chatID, "error", err)
		}

	case chatID < 0:
		if err := db.Instance.AddChat(chatID); err != nil {
			slog.Error("Failed to add chat to database", "chat_id", chatID, "error", err)
		}

	default:
		slog.Warn("Invalid chat ID", "chat_id", chatID)
	}
}

func isAdmin(status gotdbot.ChatMemberStatus) bool {
	switch status.(type) {
	case *gotdbot.ChatMemberStatusAdministrator, *gotdbot.ChatMemberStatusCreator:
		return true
	default:
		return false
	}
}

func isStatus[T gotdbot.ChatMemberStatus](status gotdbot.ChatMemberStatus) bool {
	_, ok := status.(T)
	return ok
}
