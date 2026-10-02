/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package db

import (
	"github.com/kustbots/kustmusic/internal/cache"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// AddAuthUser adds a user to the list of authorized users for a chat.
func (db *Database) AddAuthUser(chatID, userID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.authDB.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$addToSet": bson.M{"user_ids": userID}},
		options.UpdateOne().SetUpsert(true),
	)

	if err == nil {
		db.authCache.Delete(toKey(chatID))
	}
	return err
}

// RemoveAuthUser removes a user from the list of authorized users for a chat.
func (db *Database) RemoveAuthUser(chatID, userID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.authDB.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$pull": bson.M{"user_ids": userID}},
	)
	if err == nil {
		db.authCache.Delete(toKey(chatID))
	}
	return err
}

// GetAuthUsers retrieves a list of all authorized users for a chat.
func (db *Database) GetAuthUsers(chatID int64) []int64 {
	key := toKey(chatID)
	if cached, ok := db.authCache.Get(key); ok {
		return cached
	}

	ctx, cancel := db.ctx()
	defer cancel()

	var doc struct {
		UserIDs []int64 `bson:"user_ids"`
	}
	err := db.authDB.FindOne(ctx, bson.M{"_id": chatID}).Decode(&doc)
	if err != nil {
		return []int64{}
	}
	db.authCache.Set(key, doc.UserIDs)
	return doc.UserIDs
}

// IsAuthUser checks if a specific user is in the list of authorized users for a chat.
func (db *Database) IsAuthUser(chatID, userID int64) bool {
	admins, err := cache.GetChatAdminIDs(chatID)
	if err != nil || admins == nil {
		admins = []int64{}
	}

	if slices.Contains(admins, userID) {
		return true
	}

	users := db.GetAuthUsers(chatID)
	return slices.Contains(users, userID)
}

// IsAdmin checks if a specific user is an administrator in a chat.
func (db *Database) IsAdmin(chatID, userID int64) bool {
	admins, err := cache.GetChatAdminIDs(chatID)
	if err != nil || admins == nil {
		admins = []int64{}
	}
	return slices.Contains(admins, userID)
}
