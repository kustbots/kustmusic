package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kustbots/kustmusic/internal/admin"
	"github.com/kustbots/kustmusic/internal/telegram"
)

// requireGroupAdmin checks the sender is an admin of m.Chat (group/supergroup
// only, matching main-music-rx's is_user_admin — private chats always
// fail this check there too). Replies with the same error message on
// failure and returns false.
func (a *App) requireGroupAdmin(m *telegram.Message) bool {
	if m.Chat.Type != "supergroup" && m.Chat.Type != "group" {
		return false
	}
	if m.From == nil {
		return false
	}
	ok, err := a.tg.IsAdmin(m.Chat.ID, m.From.ID)
	if err != nil || !ok {
		_, _ = a.tg.SendMessage(m.Chat.ID, "🔒 Admins only, sorry.", nil)
		return false
	}
	return true
}

func mentionHTML(userID int64) string {
	return fmt.Sprintf(`<a href="tg://user?id=%d">%d</a>`, userID, userID)
}

func (a *App) cmdBan(m *telegram.Message, args string) {
	if !a.requireGroupAdmin(m) {
		return
	}
	targetID, errMsg := admin.ExtractTargetUser(a.tg, m, args)
	if errMsg != "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, errMsg, nil)
		return
	}
	if err := a.tg.BanChatMember(m.Chat.ID, targetID); err != nil {
		a.log.Warn("ban failed", "chat_id", m.Chat.ID, "target", targetID, "err", err)
		_, _ = a.tg.SendMessage(m.Chat.ID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, "🔨 "+mentionHTML(targetID)+" is banned. Door's locked.", nil)
}

func (a *App) cmdUnban(m *telegram.Message, args string) {
	if !a.requireGroupAdmin(m) {
		return
	}
	targetID, errMsg := admin.ExtractTargetUser(a.tg, m, args)
	if errMsg != "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, errMsg, nil)
		return
	}
	if err := a.tg.UnbanChatMember(m.Chat.ID, targetID); err != nil {
		a.log.Warn("unban failed", "chat_id", m.Chat.ID, "target", targetID, "err", err)
		_, _ = a.tg.SendMessage(m.Chat.ID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, "🕊 "+mentionHTML(targetID)+" is unbanned — welcome back.", nil)
}

func (a *App) cmdKick(m *telegram.Message, args string) {
	if !a.requireGroupAdmin(m) {
		return
	}
	targetID, errMsg := admin.ExtractTargetUser(a.tg, m, args)
	if errMsg != "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, errMsg, nil)
		return
	}
	if err := a.tg.BanChatMember(m.Chat.ID, targetID); err != nil {
		a.log.Warn("kick failed", "chat_id", m.Chat.ID, "target", targetID, "err", err)
		_, _ = a.tg.SendMessage(m.Chat.ID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_ = a.tg.UnbanChatMember(m.Chat.ID, targetID) // immediately unban so they can rejoin — matches Python's kick_handler
	_, _ = a.tg.SendMessage(m.Chat.ID, "👢 "+mentionHTML(targetID)+" has been shown the door. They can rejoin.", nil)
}

func mutedPermissions() telegram.ChatPermissions {
	return telegram.ChatPermissions{}
}

func fullPermissions() telegram.ChatPermissions {
	return telegram.ChatPermissions{
		CanSendMessages:       true,
		CanSendMediaMessages:  true,
		CanSendOtherMessages:  true,
		CanAddWebPagePreviews: true,
	}
}

func (a *App) cmdMute(m *telegram.Message, args string) {
	if !a.requireGroupAdmin(m) {
		return
	}
	targetID, errMsg := admin.ExtractTargetUser(a.tg, m, args)
	if errMsg != "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, errMsg, nil)
		return
	}
	if err := a.tg.RestrictChatMember(m.Chat.ID, targetID, mutedPermissions(), 0); err != nil {
		a.log.Warn("mute failed", "chat_id", m.Chat.ID, "target", targetID, "err", err)
		_, _ = a.tg.SendMessage(m.Chat.ID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, "🔇 "+mentionHTML(targetID)+" is muted. Peace and quiet.", nil)
}

func (a *App) cmdUnmute(m *telegram.Message, args string) {
	if !a.requireGroupAdmin(m) {
		return
	}
	targetID, errMsg := admin.ExtractTargetUser(a.tg, m, args)
	if errMsg != "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, errMsg, nil)
		return
	}
	if err := a.tg.RestrictChatMember(m.Chat.ID, targetID, fullPermissions(), 0); err != nil {
		a.log.Warn("unmute failed", "chat_id", m.Chat.ID, "target", targetID, "err", err)
		_, _ = a.tg.SendMessage(m.Chat.ID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, "🔊 "+mentionHTML(targetID)+" can talk again.", nil)
}

func (a *App) cmdTmute(m *telegram.Message, args string) {
	if !a.requireGroupAdmin(m) {
		return
	}
	parts := strings.Fields(args)
	if len(parts) < 2 {
		_, _ = a.tg.SendMessage(m.Chat.ID, "Usage: /tmute <user> <minutes>\nExample: /tmute @john 15", nil)
		return
	}
	minutes, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		_, _ = a.tg.SendMessage(m.Chat.ID, "⏱ That's not a number of minutes. Try <code>/tmute @user 10</code>.", nil)
		return
	}
	targetID, errMsg := admin.ExtractTargetUser(a.tg, m, args)
	if errMsg != "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, errMsg, nil)
		return
	}
	until := time.Now().Add(time.Duration(minutes) * time.Minute).Unix()
	if err := a.tg.RestrictChatMember(m.Chat.ID, targetID, mutedPermissions(), until); err != nil {
		a.log.Warn("mute failed", "chat_id", m.Chat.ID, "target", targetID, "err", err)
		_, _ = a.tg.SendMessage(m.Chat.ID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, fmt.Sprintf("⏱ %s is muted for %d minutes. See you then.", mentionHTML(targetID), minutes), nil)
}

// cmdPrime is owner-only (main-music-rx hardcodes a specific admin ID for
// this — not the same as OWNER_ID — kept as its own check here too).
const primeAdminID = 7618467489

func (a *App) cmdPrime(m *telegram.Message, args string) {
	if m.From == nil || m.From.ID != primeAdminID {
		return
	}
	args = strings.TrimSpace(args)
	if args == "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, "⚠️ Usage: /prime <user_id or @username>", nil)
		return
	}
	target := strings.TrimPrefix(strings.Fields(args)[0], "@")
	var targetID int64
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		targetID = id
	} else {
		chat, err := a.tg.GetChatByUsername(target)
		if err != nil {
			_, _ = a.tg.SendMessage(m.Chat.ID, "🤔 Can't work out who you mean. Reply to them, or give me their @username.", nil)
			return
		}
		targetID = chat.ID
	}

	a.premium.add(targetID)
	if a.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.db.SavePremiumUser(ctx, targetID, m.From.ID); err != nil {
			a.log.Warn("failed to persist premium user", "err", err)
		}
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, fmt.Sprintf("✅ Added user <code>%d</code> to premium list.", targetID), nil)
}
