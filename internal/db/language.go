/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package db

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// GetLanguage retrieves the language code for a chat.
func (db *Database) GetLanguage(chatID int64) (string, error) {
	key := toKey(chatID)
	if cached, ok := db.langCache.Get(key); ok {
		return cached, nil
	}

	ctx, cancel := db.ctx()
	defer cancel()

	var doc struct {
		Lang string `bson:"lang"`
	}
	err := db.langDB.FindOne(ctx, bson.M{"_id": chatID}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "en", nil
		}
		return "", err
	}
	db.langCache.Set(key, doc.Lang)
	return doc.Lang, nil
}

// SetLanguage sets the language code for a chat.
func (db *Database) SetLanguage(ctx context.Context, chatID int64, langCode string) error {
	_, err := db.langDB.UpdateOne(ctx,
		bson.M{"_id": chatID},
		bson.M{"$set": bson.M{"lang": langCode}},
		options.UpdateOne().SetUpsert(true),
	)

	if err == nil {
		db.langCache.Set(toKey(chatID), langCode)
	}
	return err
}
