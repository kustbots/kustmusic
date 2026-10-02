package idle

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ActivityStore remembers when each assistant account last played in each
// group, outside the process.
//
// The sweeper's clock used to live only in memory, which broke it twice
// over. Heroku restarts every dyno at least daily, and each boot reseeded
// every group as "active just now" — so any window longer than a day could
// never elapse, and a shorter one (14h) was the only setting that ever
// fired, which is what kept evicting assistants from groups that were
// merely quiet overnight and then flood-waiting them on the rejoin. And
// each account serves two servers, but each server only knew about its own
// plays, so one server could leave a group the other was actively using.
// A shared, persistent record fixes both: restarts don't reset anything,
// and a play on either server counts for the account.
type ActivityStore interface {
	// Load returns every group the account has a recorded time for.
	Load(ctx context.Context, accountID int64) (map[int64]time.Time, error)
	// LastActive reports the recorded time for one group; found is false
	// when there is no record (never seen, or already left and forgotten).
	LastActive(ctx context.Context, accountID, chatID int64) (at time.Time, found bool, err error)
	// Touch records a play. It never moves a record backwards, so two
	// servers on the same account can't overwrite a fresher time with a
	// staler one.
	Touch(ctx context.Context, accountID, chatID int64, at time.Time) error
	// TouchIfMissing starts a group's clock only if nothing is recorded
	// yet, so seeding at boot can't clobber a real play time.
	TouchIfMissing(ctx context.Context, accountID, chatID int64, at time.Time) error
	// Forget drops a group's record once the account has left it.
	Forget(ctx context.Context, accountID, chatID int64) error
}

// activityCollection lives alongside the bot's own collections in the same
// database, so no new infrastructure is needed.
const activityCollection = "assistant_group_activity"

// MongoActivityStore is the ActivityStore backed by the bot's MongoDB.
type MongoActivityStore struct {
	coll *mongo.Collection
}

type activityDoc struct {
	ID         string    `bson:"_id"`
	AccountID  int64     `bson:"account_id"`
	ChatID     int64     `bson:"chat_id"`
	LastActive time.Time `bson:"last_active"`
}

// ConnectMongoActivityStore dials MongoDB and returns a ready store.
func ConnectMongoActivityStore(uri, dbName string) (*MongoActivityStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	coll := client.Database(dbName).Collection(activityCollection)
	// Load filters by account; without this every boot scans the lot.
	// Failure to create it is harmless (it may already exist), so it isn't
	// treated as a connection failure.
	_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "account_id", Value: 1}}})
	return &MongoActivityStore{coll: coll}, nil
}

func activityID(accountID, chatID int64) string {
	return fmt.Sprintf("%d:%d", accountID, chatID)
}

func (s *MongoActivityStore) Load(ctx context.Context, accountID int64) (map[int64]time.Time, error) {
	cur, err := s.coll.Find(ctx, bson.M{"account_id": accountID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := make(map[int64]time.Time)
	for cur.Next(ctx) {
		var d activityDoc
		if err := cur.Decode(&d); err != nil {
			continue
		}
		out[d.ChatID] = d.LastActive
	}
	return out, cur.Err()
}

func (s *MongoActivityStore) LastActive(ctx context.Context, accountID, chatID int64) (time.Time, bool, error) {
	var d activityDoc
	err := s.coll.FindOne(ctx, bson.M{"_id": activityID(accountID, chatID)}).Decode(&d)
	if err == mongo.ErrNoDocuments {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return d.LastActive, true, nil
}

func (s *MongoActivityStore) Touch(ctx context.Context, accountID, chatID int64, at time.Time) error {
	_, err := s.coll.UpdateOne(ctx,
		bson.M{"_id": activityID(accountID, chatID)},
		bson.M{
			"$set": bson.M{"account_id": accountID, "chat_id": chatID},
			"$max": bson.M{"last_active": at},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (s *MongoActivityStore) TouchIfMissing(ctx context.Context, accountID, chatID int64, at time.Time) error {
	_, err := s.coll.UpdateOne(ctx,
		bson.M{"_id": activityID(accountID, chatID)},
		bson.M{"$setOnInsert": bson.M{"account_id": accountID, "chat_id": chatID, "last_active": at}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (s *MongoActivityStore) Forget(ctx context.Context, accountID, chatID int64) error {
	_, err := s.coll.DeleteOne(ctx, bson.M{"_id": activityID(accountID, chatID)})
	return err
}
