package db

import (
	"github.com/kustbots/kustmusic/internal/cache"
	"testing"
	"time"
)

func newTestDB() *Database {
	return &Database{
		chatCache:      cache.NewCache[*Chats](20 * time.Minute),
		userCache:      cache.NewCache[*Users](20 * time.Minute),
		assistantCache: cache.NewCache[int](20 * time.Minute),
		authCache:      cache.NewCache[[]int64](20 * time.Minute),
		langCache:      cache.NewCache[string](20 * time.Minute),
		loggerCache:    cache.NewCache[bool](20 * time.Minute),
		blChatsCache:   cache.NewCache[[]int64](20 * time.Minute),
		blUsersCache:   cache.NewCache[[]int64](20 * time.Minute),
	}
}

func TestDatabaseCacheHits(t *testing.T) {
	db := newTestDB()

	// Language Cache Hit
	db.langCache.Set(toKey(101), "hi")
	lang, err := db.GetLanguage(101)
	if err != nil || lang != "hi" {
		t.Fatalf("expected lang 'hi', got '%s' (err: %v)", lang, err)
	}

	// Assistant Cache Hit
	db.assistantCache.Set(toKey(101), 3)
	ass, err := db.GetAssistant(101)
	if err != nil || ass != 3 {
		t.Fatalf("expected assistant 3, got %d (err: %v)", ass, err)
	}

	// Logger Status Cache Hit
	db.loggerCache.Set("logger", true)
	if !db.GetLoggerStatus() {
		t.Fatalf("expected logger status true")
	}

	// Blacklist Chats Cache Hit
	db.blChatsCache.Set("bl_chats", []int64{1001, 1002})
	blChats := db.GetBlacklistedChats()
	if len(blChats) != 2 || blChats[0] != 1001 {
		t.Fatalf("unexpected blacklisted chats: %v", blChats)
	}
	if !db.IsBlacklistedChat(1001) {
		t.Fatalf("expected chat 1001 to be blacklisted")
	}
	if db.IsBlacklistedChat(9999) {
		t.Fatalf("expected chat 9999 NOT to be blacklisted")
	}

	// Blacklist Users Cache Hit
	db.blUsersCache.Set("bl_users", []int64{5001})
	blUsers := db.GetBlacklistedUsers()
	if len(blUsers) != 1 || blUsers[0] != 5001 {
		t.Fatalf("unexpected blacklisted users: %v", blUsers)
	}
	if !db.IsBlacklistedUser(5001) {
		t.Fatalf("expected user 5001 to be blacklisted")
	}

	// User Exist Cache Hit
	db.userCache.Set(toKey(2001), &Users{ID: 2001})
	exists, err := db.IsUserExist(2001)
	if err != nil || !exists {
		t.Fatalf("expected user 2001 to exist")
	}

	// Auth Users Cache Hit
	db.authCache.Set(toKey(3001), []int64{7001, 7002})
	auths := db.GetAuthUsers(3001)
	if len(auths) != 2 || auths[0] != 7001 {
		t.Fatalf("unexpected auth users: %v", auths)
	}

	// Chat Cache Hits
	chatObj := &Chats{
		ID:        3001,
		PlayType:  1,
		AdminPlay: true,
		AdminMode: "admin_only",
		CmdDelete: true,
	}
	db.chatCache.Set(toKey(3001), chatObj)

	if db.GetPlayType(3001) != 1 {
		t.Fatalf("expected play_type 1")
	}
	if !db.GetPlayMode(3001) {
		t.Fatalf("expected admin_play true")
	}
	if db.GetAdminMode(3001) != "admin_only" {
		t.Fatalf("expected admin_mode admin_only")
	}
	if !db.GetCmdDelete(3001) {
		t.Fatalf("expected cmd_delete true")
	}
}

func TestPlaylistUtils(t *testing.T) {
	songs := []Song{
		{URL: "https://example.com/s1", Name: "Song 1", TrackID: "id1", Duration: 100, Platform: "youtube"},
		{URL: "https://example.com/s2", Name: "Song 2", TrackID: "id2", Duration: 200, Platform: "spotify"},
	}

	tracks := ConvertSongsToTracks(songs)
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if tracks[0].Url != "https://example.com/s1" || tracks[0].Title != "Song 1" {
		t.Fatalf("unexpected converted track 0: %v", tracks[0])
	}

	id1 := generateUniquePlaylistID()
	id2 := generateUniquePlaylistID()
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("generateUniquePlaylistID generated invalid or duplicate IDs: %s, %s", id1, id2)
	}
}
