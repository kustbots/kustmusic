/*
 * TgMusicBot - Telegram Music Bot
 * Copyright (c) 2025-2026 Ashok Shau
 *
 * Licensed under GNU GPL v3
 * See https://github.com/AshokShau/TgMusicBot
 */

package cache

import (
	"fmt"
	"time"

	td "github.com/AshokShau/gotdbot"
)

var AdminCache = NewCache[[]*td.ChatMember](time.Hour)
var allCreatorRights = td.ChatAdministratorRights{
	CanChangeInfo:           true,
	CanDeleteMessages:       true,
	CanDeleteStories:        true,
	CanEditMessages:         true,
	CanEditStories:          true,
	CanInviteUsers:          true,
	CanManageChat:           true,
	CanManageDirectMessages: true,
	CanManageTags:           true,
	CanManageTopics:         true,
	CanManageVideoChats:     true,
	CanPinMessages:          true,
	CanPostMessages:         true,
	CanPostStories:          true,
	CanPromoteMembers:       true,
	CanRestrictMembers:      true,
	IsAnonymous:             false,
}

func adminCacheKey(chatID int64) string {
	return fmt.Sprintf("admins:%d", chatID)
}

func memberID(member *td.ChatMember) (int64, bool) {
	if member == nil {
		return 0, false
	}

	switch sender := member.MemberId.(type) {
	case *td.MessageSenderUser:
		return sender.UserId, true
	case *td.MessageSenderChat:
		return sender.ChatId, true
	default:
		return 0, false
	}
}

func isAdministrator(member *td.ChatMember) bool {
	if member == nil {
		return false
	}

	switch member.Status.(type) {
	case *td.ChatMemberStatusAdministrator,
		*td.ChatMemberStatusCreator:
		return true
	default:
		return false
	}
}

// GetChatAdminIDs returns the IDs of all cached administrators.
//
// An error is returned when the administrator list is not cached.
func GetChatAdminIDs(chatID int64) ([]int64, error) {
	admins, ok := AdminCache.Get(adminCacheKey(chatID))
	if !ok {
		return nil, fmt.Errorf("admins for chat %d are not cached", chatID)
	}

	ids := make([]int64, 0, len(admins))

	for _, admin := range admins {
		if id, ok := memberID(admin); ok {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// GetAdmins returns the administrator list for chatID.
//
// Unless forceReload is true, the cached list is returned when available.
func GetAdmins(client *td.Client, chatID int64, forceReload bool) ([]*td.ChatMember, error) {
	key := adminCacheKey(chatID)

	if !forceReload {
		if admins, ok := AdminCache.Get(key); ok {
			return admins, nil
		}
	}

	res, err := client.SearchChatMembers(
		chatID,
		0,
		"",
		&td.SearchChatMembersOpts{
			Filter: td.ChatMembersFilterAdministrators{},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("fetch admins for chat %d: %w", chatID, err)
	}

	admins := make([]*td.ChatMember, len(res.Members))
	for i := range res.Members {
		admins[i] = &res.Members[i]
	}

	AdminCache.Set(key, admins)

	return admins, nil
}

// GetUserAdmin returns the administrator record for userID.
//
// The lookup uses the cached administrator list unless forceReload is true.
func GetUserAdmin(
	client *td.Client,
	chatID, userID int64,
	forceReload bool,
) (*td.ChatMember, error) {
	admins, err := GetAdmins(client, chatID, forceReload)
	if err != nil {
		return nil, err
	}

	for _, admin := range admins {
		if id, ok := memberID(admin); ok && id == userID {
			return admin, nil
		}
	}

	return nil, fmt.Errorf(
		"user %d is not an administrator in chat %d",
		userID,
		chatID,
	)
}

// GetRights returns the administrator rights for userID.
//
// Chat creators implicitly have all administrator permissions.
func GetRights(
	client *td.Client,
	chatID, userID int64,
	forceReload bool,
) (*td.ChatAdministratorRights, error) {
	admin, err := GetUserAdmin(client, chatID, userID, forceReload)
	if err != nil {
		return nil, err
	}

	switch status := admin.Status.(type) {
	case *td.ChatMemberStatusAdministrator:
		return status.Rights, nil

	case *td.ChatMemberStatusCreator:
		// Return a copy so callers cannot mutate the package-level value.
		rights := allCreatorRights
		return &rights, nil

	default:
		return nil, fmt.Errorf(
			"user %d has unexpected member status in chat %d",
			userID,
			chatID,
		)
	}
}

// ClearAdminCache removes cached administrators.
//
// Passing chatID == 0 clears the entire administrator cache.
func ClearAdminCache(chatID int64) {
	if chatID == 0 {
		AdminCache.Clear()
		return
	}

	AdminCache.Delete(adminCacheKey(chatID))
}

func UpdateAdminCache(chatID int64, member *td.ChatMember) {
	if member == nil {
		return
	}

	key := adminCacheKey(chatID)

	admins, ok := AdminCache.Get(key)
	if !ok {
		return
	}

	userID, ok := memberID(member)
	if !ok {
		return
	}

	admin := isAdministrator(member)

	for i, existing := range admins {
		existingID, ok := memberID(existing)
		if !ok || existingID != userID {
			continue
		}

		if admin {
			updated := make([]*td.ChatMember, len(admins))
			copy(updated, admins)
			updated[i] = member
			AdminCache.Set(key, updated)
		} else {
			updated := make([]*td.ChatMember, 0, len(admins)-1)
			updated = append(updated, admins[:i]...)
			updated = append(updated, admins[i+1:]...)
			AdminCache.Set(key, updated)
		}

		return
	}

	if admin {
		updated := make([]*td.ChatMember, 0, len(admins)+1)
		updated = append(updated, admins...)
		updated = append(updated, member)
		AdminCache.Set(key, updated)
	}
}
