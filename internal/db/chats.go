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
	"github.com/kustbots/kustmusic/internal/utils"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Chats represents a chat document in the database.
type Chats struct {
	ID        int64  `bson:"_id"`
	PlayType  int    `bson:"play_type"`
	AdminPlay bool   `bson:"admin_play"`
	AdminMode string `bson:"admin_mode"`
	CmdDelete bool   `bson:"cmd_delete"`
}

// getChat retrieves a chat's data from the cache or database.
func (db *Database) getChat(chatID int64) (*Chats, error) {
	key := toKey(chatID)
	if cached, ok := db.chatCache.Get(key); ok {
		return cached, nil
	}

	var chat Chats
	var err error

	ctx, cancel := db.ctx()
	defer cancel()

	for range 3 {
		err = db.chatDB.FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat)
		if err == nil {
			break
		}
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err != nil {
		slog.Info("[DB] An error occurred while getting the chat", "error", err)
		return nil, err
	}

	db.chatCache.Set(key, &chat)
	return &chat, nil
}

// AddChat adds a new chat to the database if it does not already exist.
func (db *Database) AddChat(chatID int64) error {
	chat, _ := db.getChat(chatID)
	if chat != nil {
		return nil // Chat already exists.
	}

	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.chatDB.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$setOnInsert": bson.M{}}, options.UpdateOne().SetUpsert(true))
	if err == nil {
		slog.Info("[DB] A new chat has been added", "id", chatID)
	}
	return err
}

func (db *Database) GetPlayType(chatID int64) int {
	chat, _ := db.getChat(chatID)
	if chat == nil {
		return 0
	}
	return chat.PlayType
}

func (db *Database) SetPlayType(chatID int64, playType int) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.chatDB.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": bson.M{"play_type": playType}}, options.UpdateOne().SetUpsert(true))
	if err == nil {
		db.chatCache.Delete(toKey(chatID))
	}
	return err
}

func (db *Database) GetPlayMode(chatID int64) bool {
	chat, _ := db.getChat(chatID)
	if chat == nil {
		return false
	}
	return chat.AdminPlay
}

func (db *Database) SetPlayMode(chatID int64, adminPlay bool) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.chatDB.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": bson.M{"admin_play": adminPlay}}, options.UpdateOne().SetUpsert(true))
	if err == nil {
		db.chatCache.Delete(toKey(chatID))
	}
	return err
}

func (db *Database) GetAdminMode(chatID int64) string {
	chat, _ := db.getChat(chatID)
	if chat == nil || chat.AdminMode == "" {
		return utils.Everyone
	}
	return chat.AdminMode
}

func (db *Database) SetAdminMode(chatID int64, adminMode string) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.chatDB.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": bson.M{"admin_mode": adminMode}}, options.UpdateOne().SetUpsert(true))
	if err == nil {
		db.chatCache.Delete(toKey(chatID))
	}
	return err
}

func (db *Database) GetCmdDelete(chatID int64) bool {
	chat, _ := db.getChat(chatID)
	if chat == nil {
		return false
	}
	return chat.CmdDelete
}

func (db *Database) SetCmdDelete(chatID int64, cmdDelete bool) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.chatDB.UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": bson.M{"cmd_delete": cmdDelete}}, options.UpdateOne().SetUpsert(true))
	if err == nil {
		db.chatCache.Delete(toKey(chatID))
	}
	return err
}

func (db *Database) GetAllChats() ([]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := db.chatDB.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		_ = cursor.Close(ctx)
	}(cursor, ctx)

	var chats []int64
	for cursor.Next(ctx) {
		var doc Chats
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		chats = append(chats, doc.ID)
		db.chatCache.Set(toKey(doc.ID), &doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return chats, nil
}
