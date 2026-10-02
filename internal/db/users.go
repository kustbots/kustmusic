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
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Users represents a user document in the database.
type Users struct {
	ID int64 `bson:"_id"`
}

// AddUser adds a new user to the database if they do not already exist.
func (db *Database) AddUser(userID int64) error {
	key := toKey(userID)
	if _, ok := db.userCache.Get(key); ok {
		return nil
	}

	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.userDB.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$setOnInsert": bson.M{}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return err
	}

	db.userCache.Set(key, &Users{ID: userID})
	return nil
}

// RemoveUser removes a user from the database and cache.
func (db *Database) RemoveUser(userID int64) error {
	ctx, cancel := db.ctx()
	defer cancel()

	_, err := db.userDB.DeleteOne(ctx, bson.M{"_id": userID})
	if err != nil {
		return err
	}

	db.userCache.Delete(toKey(userID))
	return nil
}

// IsUserExist checks if a user exists in the database.
func (db *Database) IsUserExist(userID int64) (bool, error) {
	key := toKey(userID)
	if _, ok := db.userCache.Get(key); ok {
		return true, nil
	}

	ctx, cancel := db.ctx()
	defer cancel()

	var user Users
	err := db.userDB.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	db.userCache.Set(key, &user)
	return true, nil
}

// GetAllUsers retrieves a list of all user IDs from the database.
func (db *Database) GetAllUsers() ([]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := db.userDB.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		_ = cursor.Close(ctx)
	}(cursor, ctx)

	var users []int64
	for cursor.Next(ctx) {
		var doc Users
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		users = append(users, doc.ID)
		db.userCache.Set(toKey(doc.ID), &doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
