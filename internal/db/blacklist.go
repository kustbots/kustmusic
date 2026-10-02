/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package db

import (
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// AddBlacklistedChat adds a chat to the blacklist.
func (db *Database) AddBlacklistedChat(chatID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.cacheDB.UpdateOne(ctx,
		bson.M{"_id": "bl_chats"},
		bson.M{"$addToSet": bson.M{"chat_ids": chatID}},
		options.UpdateOne().SetUpsert(true),
	)
	if err == nil {
		db.blChatsCache.Delete("bl_chats")
	}
	return err
}

// RemoveBlacklistedChat removes a chat from the blacklist.
func (db *Database) RemoveBlacklistedChat(chatID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.cacheDB.UpdateOne(ctx,
		bson.M{"_id": "bl_chats"},
		bson.M{"$pull": bson.M{"chat_ids": chatID}},
	)
	if err == nil {
		db.blChatsCache.Delete("bl_chats")
	}
	return err
}

// GetBlacklistedChats retrieves the list of blacklisted chat IDs.
func (db *Database) GetBlacklistedChats() []int64 {
	if cached, ok := db.blChatsCache.Get("bl_chats"); ok {
		return cached
	}
	var doc struct {
		ChatIDs []int64 `bson:"chat_ids"`
	}

	ctx, cancel := db.ctx()
	defer cancel()

	err := db.cacheDB.FindOne(ctx, bson.M{"_id": "bl_chats"}).Decode(&doc)
	if err != nil {
		return []int64{}
	}
	db.blChatsCache.Set("bl_chats", doc.ChatIDs)
	return doc.ChatIDs
}

// IsBlacklistedChat checks if a chat is blacklisted.
func (db *Database) IsBlacklistedChat(chatID int64) bool {
	chats := db.GetBlacklistedChats()
	return slices.Contains(chats, chatID)
}

// AddBlacklistedUser adds a user to the blacklist.
func (db *Database) AddBlacklistedUser(userID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.cacheDB.UpdateOne(ctx,
		bson.M{"_id": "bl_users"},
		bson.M{"$addToSet": bson.M{"user_ids": userID}},
		options.UpdateOne().SetUpsert(true),
	)
	if err == nil {
		db.blUsersCache.Delete("bl_users")
	}
	return err
}

// RemoveBlacklistedUser removes a user from the blacklist.
func (db *Database) RemoveBlacklistedUser(userID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.cacheDB.UpdateOne(ctx,
		bson.M{"_id": "bl_users"},
		bson.M{"$pull": bson.M{"user_ids": userID}},
	)
	if err == nil {
		db.blUsersCache.Delete("bl_users")
	}
	return err
}

// GetBlacklistedUsers retrieves the list of blacklisted user IDs.
func (db *Database) GetBlacklistedUsers() []int64 {
	if cached, ok := db.blUsersCache.Get("bl_users"); ok {
		return cached
	}
	var doc struct {
		UserIDs []int64 `bson:"user_ids"`
	}

	ctx, cancel := db.ctx()
	defer cancel()

	err := db.cacheDB.FindOne(ctx, bson.M{"_id": "bl_users"}).Decode(&doc)
	if err != nil {
		return []int64{}
	}
	db.blUsersCache.Set("bl_users", doc.UserIDs)
	return doc.UserIDs
}

// IsBlacklistedUser checks if a user is blacklisted.
func (db *Database) IsBlacklistedUser(userID int64) bool {
	users := db.GetBlacklistedUsers()
	return slices.Contains(users, userID)
}
