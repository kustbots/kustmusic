/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package db

import (
	"errors"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// GetAssistant retrieves the index of the assistant for a chat.
// Returns -1 if no assistant is assigned.
func (db *Database) GetAssistant(chatID int64) (int, error) {
	key := toKey(chatID)
	if cached, ok := db.assistantCache.Get(key); ok {
		return cached, nil
	}
	var doc struct {
		Num int `bson:"num"`
	}

	ctx, cancel := db.ctx()
	defer cancel()

	err := db.assistantDB.FindOne(ctx, bson.M{"_id": chatID}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return -1, nil
		}
		return -1, err
	}
	db.assistantCache.Set(key, doc.Num)
	return doc.Num, nil
}

// SetAssistant sets the assistant index for a given chat.
func (db *Database) SetAssistant(chatID int64, num int) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.assistantDB.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": bson.M{"num": num}}, options.UpdateOne().SetUpsert(true))
	if err == nil {
		db.assistantCache.Set(toKey(chatID), num)
	}

	return err
}

// RemoveAssistant removes the assistant from a chat's settings.
func (db *Database) RemoveAssistant(chatID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.assistantDB.DeleteOne(ctx, bson.M{"_id": chatID})
	if err == nil {
		db.assistantCache.Delete(toKey(chatID))
	}
	return err
}

// AssignAssistant attempts to set the assistant for a chat if it is not currently set.
func (db *Database) AssignAssistant(chatID int64, proposedAssistant int) (int, error) {
	ctx, cancel := db.ctx()
	defer cancel()

	filter := bson.M{
		"_id": chatID,
		"$or": bson.A{
			bson.M{"num": bson.M{"$exists": false}},
			bson.M{"num": -1},
		},
	}
	update := bson.M{"$set": bson.M{"num": proposedAssistant}}
	opts := options.UpdateOne().SetUpsert(true)

	result, err := db.assistantDB.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return db.GetAssistant(chatID)
		}
		return -1, err
	}

	if result.ModifiedCount > 0 || result.UpsertedCount > 0 {
		db.assistantCache.Set(toKey(chatID), proposedAssistant)
		return proposedAssistant, nil
	}

	return db.GetAssistant(chatID)
}

// ClearAllAssistants removes all assistant assignments.
func (db *Database) ClearAllAssistants() (int64, error) {
	ctx, cancel := db.ctx()
	defer cancel()

	result, err := db.assistantDB.DeleteMany(ctx, bson.M{})
	if err != nil {
		slog.Info("[DB] Error clearing assistants", "error", err)
		return 0, err
	}

	db.assistantCache.Clear()
	return result.DeletedCount, nil
}
