// Package store wraps the same MongoDB database main-music-rx already uses
// (playlists, premium users, broadcast chat list/state) so this rewrite
// reads/writes the exact collections and document shapes the Python bot
// used — no migration needed, and either bot can read the other's data.
package store

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// dbName is the database used, from MONGO_DB (default "kustmusic").
// same database, so either bot can read the other's data.
func dbName() string {
	if v := os.Getenv("MONGO_DB"); v != "" {
		return v
	}
	return "kustmusic"
}

type Store struct {
	client         *mongo.Client
	playlists      *mongo.Collection
	broadcastChats *mongo.Collection
	broadcastState *mongo.Collection
	stateBackup    *mongo.Collection
	queueBackups   *mongo.Collection
	cloneBots      *mongo.Collection
}

func Connect(uri string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(dbName())
	return &Store{
		client:         client,
		playlists:      db.Collection("playlists"),
		broadcastChats: db.Collection("broadcast"),
		broadcastState: db.Collection("broadcast_state"),
		stateBackup:    db.Collection("state_backup"),
		queueBackups:   db.Collection("queue_backups"),
		cloneBots:      db.Collection("clone_bots"),
	}, nil
}

// --- Cloned bots ---

// CloneBot is one user-supplied bot that runs as a clone of this one. The
// token is the document id: it is what the webhook path carries, so it is
// the natural lookup key, and it makes a second /clone of the same bot an
// upsert instead of a duplicate.
type CloneBot struct {
	Token     string `bson:"_id"`
	BotID     int64  `bson:"bot_id"`
	Username  string `bson:"username"`
	OwnerID   int64  `bson:"owner_id"`
	CreatedAt int64  `bson:"created_at"`
}

func (s *Store) SaveCloneBot(ctx context.Context, b CloneBot) error {
	_, err := s.cloneBots.UpdateOne(ctx,
		bson.M{"_id": b.Token},
		bson.M{"$set": bson.M{"bot_id": b.BotID, "username": b.Username, "owner_id": b.OwnerID},
			"$setOnInsert": bson.M{"created_at": b.CreatedAt}},
		options.Update().SetUpsert(true),
	)
	return err
}

// GetCloneBot returns nil, nil when the token is not a registered clone.
func (s *Store) GetCloneBot(ctx context.Context, token string) (*CloneBot, error) {
	var b CloneBot
	err := s.cloneBots.FindOne(ctx, bson.M{"_id": token}).Decode(&b)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) ListCloneBots(ctx context.Context) ([]CloneBot, error) {
	cur, err := s.cloneBots.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []CloneBot
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RemoveCloneBot(ctx context.Context, token string) error {
	_, err := s.cloneBots.DeleteOne(ctx, bson.M{"_id": token})
	return err
}

// --- Playlists ---

type PlaylistEntry struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	ChatID    int64              `bson:"chat_id"`
	UserID    int64              `bson:"user_id"`
	SongTitle string             `bson:"song_title"`
	URL       string             `bson:"url"`
	Duration  string             `bson:"duration"`
	Timestamp float64            `bson:"timestamp"`
}

func (s *Store) AddToPlaylist(ctx context.Context, e PlaylistEntry) (bool, error) {
	existing := s.playlists.FindOne(ctx, bson.M{"chat_id": e.ChatID, "user_id": e.UserID, "song_title": e.SongTitle})
	if existing.Err() == nil {
		return false, nil // already saved
	}
	e.Timestamp = float64(time.Now().Unix())
	_, err := s.playlists.InsertOne(ctx, e)
	return err == nil, err
}

func (s *Store) ListPlaylist(ctx context.Context, userID int64) ([]PlaylistEntry, error) {
	cur, err := s.playlists.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []PlaylistEntry
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RemoveFromPlaylist(ctx context.Context, id primitive.ObjectID) error {
	_, err := s.playlists.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (s *Store) GetPlaylistEntry(ctx context.Context, id primitive.ObjectID) (*PlaylistEntry, error) {
	var e PlaylistEntry
	if err := s.playlists.FindOne(ctx, bson.M{"_id": id}).Decode(&e); err != nil {
		return nil, err
	}
	return &e, nil
}

// --- Premium users --- (stored as a single document, matching the Python
// bot's state_backup singleton pattern for premium_users)

func (s *Store) LoadPremiumUsers(ctx context.Context) (map[int64]bool, error) {
	var doc struct {
		State struct {
			PremiumUsers map[string]any `bson:"premium_users"`
		} `bson:"state"`
	}
	err := s.stateBackup.FindOne(ctx, bson.M{"_id": "singleton"}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return map[int64]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(doc.State.PremiumUsers))
	for k := range doc.State.PremiumUsers {
		if id, err := strconv.ParseInt(k, 10, 64); err == nil {
			out[id] = true
		}
	}
	return out, nil
}

func (s *Store) SavePremiumUser(ctx context.Context, userID, addedBy int64) error {
	key := strconv.FormatInt(userID, 10)
	_, err := s.stateBackup.UpdateOne(ctx,
		bson.M{"_id": "singleton"},
		bson.M{"$set": bson.M{
			"state.premium_users." + key: bson.M{"added_by": addedBy, "timestamp": time.Now().Unix()},
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

// --- Broadcast chat registry ---

// chatIDForms returns every representation chatID might be stored as.
//
// This collection was written by several generations of bot, and they
// didn't agree on a type: of ~5,600 documents, most hold chat_id as an
// int64, some as an int32, and a few dozen as a *string*. Queries have to
// match all of them or they silently miss chats.
func chatIDForms(chatID int64) []any {
	return []any{chatID, int32(chatID), strconv.FormatInt(chatID, 10)}
}

func (s *Store) RegisterBroadcastChat(ctx context.Context, chatID int64, kind string) error {
	existing := s.broadcastChats.FindOne(ctx, bson.M{"chat_id": bson.M{"$in": chatIDForms(chatID)}})
	if existing.Err() == nil {
		return nil // already registered, in whichever type it was stored as
	}
	_, err := s.broadcastChats.InsertOne(ctx, bson.M{"chat_id": chatID, "type": kind})
	return err
}

// ListBroadcastChats returns every registered chat id.
//
// It decodes chat_id field by field rather than through a struct, because
// a struct binds one Go type and the collection genuinely holds three (see
// chatIDForms). Decoding into `int64` made the driver fail the entire
// cursor the moment it reached one of the string documents — which is what
// "cannot decode string into an integer type" was, and why /broadcast
// couldn't reach a single one of the 5,600+ chats over a few dozen
// malformed ones. Anything unparseable is now skipped instead of taking
// the whole list down with it.
func (s *Store) ListBroadcastChats(ctx context.Context) ([]int64, error) {
	cur, err := s.broadcastChats.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"chat_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []int64
	seen := make(map[int64]bool)
	for cur.Next(ctx) {
		id, ok := decodeChatID(cur.Current.Lookup("chat_id"))
		if !ok || seen[id] {
			continue // unreadable, or the same chat stored twice under different types
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, cur.Err()
}

// decodeChatID reads a chat id whatever BSON type it was written as.
func decodeChatID(v bson.RawValue) (int64, bool) {
	switch v.Type {
	case bsontype.Int64:
		return v.Int64(), true
	case bsontype.Int32:
		return int64(v.Int32()), true
	case bsontype.Double:
		return int64(v.Double()), true
	case bsontype.String:
		id, err := strconv.ParseInt(strings.TrimSpace(v.StringValue()), 10, 64)
		return id, err == nil
	default:
		return 0, false
	}
}

func (s *Store) RemoveBroadcastChat(ctx context.Context, chatID int64) error {
	// DeleteMany, not DeleteOne: a chat can be present under more than one
	// stored type, and leaving a duplicate behind means a chat that blocked
	// the bot keeps getting broadcast to.
	_, err := s.broadcastChats.DeleteMany(ctx, bson.M{"chat_id": bson.M{"$in": chatIDForms(chatID)}})
	return err
}

// --- Broadcast state (checkpointed periodically so a crash mid-broadcast
// loses at most a handful of sends, matching the Python bot's approach) ---

func (s *Store) PersistBroadcastState(ctx context.Context, state bson.M) error {
	state["_id"] = "current"
	_, err := s.broadcastState.ReplaceOne(ctx, bson.M{"_id": "current"}, state, options.Replace().SetUpsert(true))
	return err
}

func (s *Store) ClearBroadcastState(ctx context.Context) error {
	_, err := s.broadcastState.DeleteOne(ctx, bson.M{"_id": "current"})
	return err
}

// --- Queue persistence across restarts ---
//
// Heroku restarts a dyno at least once a day, plus on every deploy, and it
// gives the process a SIGTERM and a few seconds' grace first. Without
// this, every group's queue vanished at that moment — songs people had
// lined up simply gone, with no explanation in the chat. The bot now
// writes them here on the way down and picks them back up on boot, the
// same shape the dream bot uses.

// SavedQueue is one chat's pending songs.
type SavedQueue struct {
	ChatID  int64       `bson:"chat_id"`
	Songs   []SavedSong `bson:"songs"`
	SavedAt int64       `bson:"saved_at"`
}

// SavedSong mirrors queue.Song. It's spelled out separately so the on-disk
// shape is an explicit, stable contract rather than whatever the in-memory
// struct happens to look like after a refactor.
type SavedSong struct {
	Title         string `bson:"title"`
	URL           string `bson:"url"`
	Duration      string `bson:"duration"`
	Thumbnail     string `bson:"thumbnail"`
	Query         string `bson:"query"`
	RequesterID   int64  `bson:"requester_id"`
	RequesterName string `bson:"requester_name"`
}

// SaveQueues replaces the stored queues with the ones given. Called during
// shutdown, so it rewrites wholesale rather than merging: what's in memory
// at that moment is the truth, and a chat that emptied its queue must not
// be resurrected from an older snapshot.
func (s *Store) SaveQueues(ctx context.Context, queues []SavedQueue) error {
	if _, err := s.queueBackups.DeleteMany(ctx, bson.M{}); err != nil {
		return err
	}
	if len(queues) == 0 {
		return nil
	}
	now := time.Now().Unix()
	docs := make([]any, 0, len(queues))
	for _, q := range queues {
		if len(q.Songs) == 0 {
			continue
		}
		q.SavedAt = now
		docs = append(docs, q)
	}
	if len(docs) == 0 {
		return nil
	}
	_, err := s.queueBackups.InsertMany(ctx, docs)
	return err
}

// LoadQueues reads back everything SaveQueues wrote.
func (s *Store) LoadQueues(ctx context.Context) ([]SavedQueue, error) {
	cur, err := s.queueBackups.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []SavedQueue
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ClearQueues empties the backup, so a restored queue can't be restored a
// second time if the bot restarts again before saving.
func (s *Store) ClearQueues(ctx context.Context) error {
	_, err := s.queueBackups.DeleteMany(ctx, bson.M{})
	return err
}
