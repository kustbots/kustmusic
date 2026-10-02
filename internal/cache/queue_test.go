package cache

import (
	"github.com/kustbots/kustmusic/internal/utils"
	"testing"
)

func TestChatCacherQueueOperations(t *testing.T) {
	c := newChatCacher()
	chatID := int64(1001)

	if c.IsActive(chatID) {
		t.Fatalf("expected chat to be inactive initially")
	}

	song1 := &utils.PlayerCache{TrackID: "t1", Name: "Song 1", URL: "http://song1.mp3", Duration: 120}
	song2 := &utils.PlayerCache{TrackID: "t2", Name: "Song 2", URL: "http://song2.mp3", Duration: 180}

	length := c.AddSong(chatID, song1)
	if length != 1 {
		t.Fatalf("expected queue length 1, got %d", length)
	}

	if !c.IsActive(chatID) {
		t.Fatalf("expected chat to be active")
	}

	length = c.AddSong(chatID, song2)
	if length != 2 {
		t.Fatalf("expected queue length 2, got %d", length)
	}

	playing := c.GetPlayingTrack(chatID)
	if playing == nil || playing.TrackID != "t1" {
		t.Fatalf("expected playing track to be t1, got %v", playing)
	}

	upcoming := c.GetUpcomingTrack(chatID)
	if upcoming == nil || upcoming.TrackID != "t2" {
		t.Fatalf("expected upcoming track to be t2, got %v", upcoming)
	}

	song3 := &utils.PlayerCache{TrackID: "t3", Name: "Song 3"}
	c.AddSongToFront(chatID, song3)
	// Now queue should be: song1 (playing, idx 0), song3 (idx 1), song2 (idx 2)
	if c.GetUpcomingTrack(chatID).TrackID != "t3" {
		t.Fatalf("expected upcoming track after AddSongToFront to be t3")
	}

	moved := c.MoveTrackToFront(chatID, "t2")
	if !moved {
		t.Fatalf("expected MoveTrackToFront to succeed")
	}
	// Now queue should be: song1 (idx 0), song2 (idx 1), song3 (idx 2)
	if c.GetUpcomingTrack(chatID).TrackID != "t2" {
		t.Fatalf("expected upcoming track after MoveTrackToFront to be t2")
	}

	removed := c.RemoveCurrentSong(chatID)
	if removed == nil || removed.TrackID != "t1" {
		t.Fatalf("expected removed song to be t1")
	}

	// Now queue: song2 (idx 0), song3 (idx 1)
	if c.GetPlayingTrack(chatID).TrackID != "t2" {
		t.Fatalf("expected playing track to be t2")
	}

	c.RemoveTrack(chatID, 0)
	// Now queue: song3 (idx 0)
	if c.GetPlayingTrack(chatID).TrackID != "t3" {
		t.Fatalf("expected playing track to be t3")
	}

	c.ClearChat(chatID)
	if c.IsActive(chatID) {
		t.Fatalf("expected chat to be inactive after ClearChat")
	}
}

func TestChatCacherAutoplayAndLoop(t *testing.T) {
	c := newChatCacher()
	chatID := int64(2002)

	c.SetAutoplay(chatID, true)
	if !c.GetAutoplay(chatID) {
		t.Fatalf("expected autoplay to be true")
	}

	c.AddAutoplayHistory(chatID, "track1", "track2")
	history := c.GetAutoplayHistory(chatID)
	if len(history) != 2 || history[0] != "track1" || history[1] != "track2" {
		t.Fatalf("unexpected history: %v", history)
	}

	if c.GetLastAutoplayTrackID(chatID) != "track2" {
		t.Fatalf("expected last autoplay track ID to be track2")
	}

	c.ClearAutoplayHistory(chatID)
	if c.GetAutoplayHistory(chatID) != nil {
		t.Fatalf("expected history to be nil after clear")
	}

	// Test Loop count
	song := &utils.PlayerCache{TrackID: "t1", Loop: 0}
	c.AddSong(chatID, song)

	c.SetLoopCount(chatID, 3)
	if c.GetLoopCount(chatID) != 3 {
		t.Fatalf("expected loop count 3, got %d", c.GetLoopCount(chatID))
	}
}
