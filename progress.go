package main

import (
	"sync"
	"time"
)

// progressUpdateInterval matches the Python bot's own 18s cadence. Telegram
// rate-limits edits per chat, so this stays deliberately slow — the bar has
// 14 segments, so on a typical 3-4 minute track it still advances visibly
// every tick.
const progressUpdateInterval = 18 * time.Second

// progressTracker animates the player card's progress-bar button.
//
// The bar used to be drawn once, at 0:00, and never touched again — so it
// sat frozen for the whole song. This runs a per-chat goroutine that
// re-renders the keyboard on an interval, and cancels the previous one
// whenever a new song starts or playback stops, so two songs can't fight
// over the same card.
type progressTracker struct {
	mu      sync.Mutex
	cancels map[int64]chan struct{}
}

func newProgressTracker() *progressTracker {
	return &progressTracker{cancels: make(map[int64]chan struct{})}
}

// Stop halts any running updater for chatID.
func (p *progressTracker) Stop(chatID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ch, ok := p.cancels[chatID]; ok {
		close(ch)
		delete(p.cancels, chatID)
	}
}

// Start begins animating messageID's keyboard for a track of totalSeconds,
// replacing any updater already running for this chat. A non-positive
// totalSeconds (live stream, or a duration the search API didn't return)
// is skipped entirely rather than animating a bar that can't be
// meaningful.
func (p *progressTracker) Start(chatID int64, messageID int, totalSeconds float64, render func(chatID int64, messageID int, elapsed, total float64) error) {
	if totalSeconds <= 0 {
		return
	}
	p.Stop(chatID)

	done := make(chan struct{})
	p.mu.Lock()
	p.cancels[chatID] = done
	p.mu.Unlock()

	go func() {
		started := time.Now()
		ticker := time.NewTicker(progressUpdateInterval)
		defer ticker.Stop()
		lastBar := ""
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(started).Seconds()
				if elapsed > totalSeconds {
					elapsed = totalSeconds
				}
				// Skip the edit when the bar would render identically.
				// On a long track a 14-segment bar doesn't advance every
				// 18s tick, and re-sending the same markup makes Telegram
				// reject it as "message is not modified" — which, treated
				// as a real error, killed the updater partway through the
				// song and froze the bar again.
				bar := progressBarStyled(elapsed, totalSeconds)
				if bar != lastBar {
					if err := render(chatID, messageID, elapsed, totalSeconds); err != nil {
						// A genuine failure — the card was deleted, or the
						// chat is gone. Nothing to keep updating.
						return
					}
					lastBar = bar
				}
				if elapsed >= totalSeconds {
					return
				}
			}
		}
	}()
}
