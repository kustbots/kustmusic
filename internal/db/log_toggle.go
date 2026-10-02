/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package db

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// GetLoggerStatus retrieves the logger status for a given bot.
func (db *Database) GetLoggerStatus() bool {
	if cached, ok := db.loggerCache.Get("logger"); ok {
		return cached
	}

	ctx, cancel := db.ctx()
	defer cancel()

	var doc struct {
		Status bool `bson:"status"`
	}
	err := db.cacheDB.FindOne(ctx, bson.M{"_id": "logger"}).Decode(&doc)
	if err != nil {
		return false
	}
	db.loggerCache.Set("logger", doc.Status)
	return doc.Status
}

// SetLoggerStatus enables or disables the logger for a bot.
func (db *Database) SetLoggerStatus(status bool) error {
	ctx, cancel := db.ctx()
	defer cancel()
	_, err := db.cacheDB.UpdateOne(ctx,
		bson.M{"_id": "logger"},
		bson.M{"$set": bson.M{"status": status}},
		options.UpdateOne().SetUpsert(true),
	)
	if err == nil {
		db.loggerCache.Set("logger", status)
	}
	return err
}
