package main

import (
	"math/rand"
	"sync"
	"time"

	"github.com/kustbots/kustmusic/internal/telegram"
)

// Promo pacing. These are deliberately conservative — a promo that shows
// up often enough to notice is a promo people mute the bot over, and a
// music bot in a busy group can play dozens of songs an hour.
const (
	// promoEverySongs is the minimum songs a chat must play between
	// promos.
	promoEverySongs = 8
	// promoCooldown is the minimum wall-clock gap on top of that, so a
	// chat blasting through short tracks doesn't get promos back to back.
	promoCooldown = 3 * time.Hour
	// promoChance keeps it from being clockwork — once a chat is eligible,
	// it still only fires sometimes, so it reads as incidental rather than
	// as a counter ticking over.
	promoChance = 0.5
)

// promoter posts an occasional one-line note about the bot's own group,
// its sibling bots, and the updates channel.
//
// The whole design goal is not being annoying. It only ever appears after
// a song has already started (so it's never in the way of what someone
// asked for), only in groups, at most once every few hours per chat, and
// never twice in a row with the same message. Everything is one short
// line plus buttons — no images, no walls of text, nothing that pushes the
// player card off screen.
type promoter struct {
	tg  *telegram.Client
	rng *rand.Rand

	mu    sync.Mutex
	state map[int64]*promoChat
}

type promoChat struct {
	songsSince int
	lastSent   time.Time
	lastIndex  int // which promo went out last, so the next one differs
}

func newPromoter(tg *telegram.Client) *promoter {
	return &promoter{
		tg:    tg,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
		state: make(map[int64]*promoChat),
	}
}

// promo is one rotation entry: a line of text and the buttons that make it
// actionable. Every one has a call to action — a promo you can't act on is
// just noise in someone's group.
type promo struct {
	text string
	kb   telegram.InlineKeyboard
}

// promoEnabled turns the occasional promo note on. Off by default.
var promoEnabled = getenv("PROMO_ENABLED", "") == "true"

// promos is built on each use because the add-to-group link is only known
// once the bot has started.
func promos() []promo {
	return []promo{
		{
			text: "💡 <i>Liking the sound? I'm free in every group, so add me to yours.</i>",
			kb: telegram.InlineKeyboard{
				{{Text: "➕ Add Me To A Group", URL: addToGroupURL}},
				{{Text: "📢 Updates", URL: updatesURL}},
			},
		},
		{
			text: "💡 <i>New features land here first. Follow the updates channel so your group gets them early.</i>",
			kb: telegram.InlineKeyboard{
				{{Text: "📢 Updates Channel", URL: updatesURL}},
				{
					{Text: "💬 Community", URL: communityURL},
					{Text: "➕ Add Me", URL: addToGroupURL},
				},
			},
		},
	}
}

// MaybeSend counts a song for chatID and posts a promo if this chat is due
// for one. Safe to call on every play; it decides for itself whether
// anything happens, and does nothing far more often than not.
func (p *promoter) MaybeSend(chatID int64) {
	// Groups only. A promo telling someone to add the bot to a group makes
	// no sense in the DM where they just added it, and DMs are where
	// unsolicited messages are most unwelcome.
	if !promoEnabled || chatID >= 0 {
		return
	}

	pick, ok := p.due(chatID)
	if !ok {
		return
	}

	if _, err := p.tg.SendMessage(chatID, promos()[pick].text, promos()[pick].kb); err != nil {
		// Not worth retrying or reporting — it's a nicety, and a chat
		// where sending fails has bigger problems than a missed promo.
		return
	}
}

// due advances chatID's counters and reports whether a promo should go out
// now, along with which one. Kept separate from sending so the locking
// stays tight and the decision is testable on its own.
func (p *promoter) due(chatID int64) (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	st, known := p.state[chatID]
	if !known {
		// A chat's first songs are the worst possible moment: someone is
		// still deciding whether this bot is worth keeping. Start the
		// clock, say nothing.
		p.state[chatID] = &promoChat{lastSent: time.Now(), lastIndex: -1}
		return 0, false
	}

	st.songsSince++
	if st.songsSince < promoEverySongs || time.Since(st.lastSent) < promoCooldown {
		return 0, false
	}
	if p.rng.Float64() > promoChance {
		return 0, false
	}

	// Rotate rather than repeat — the same line twice running is what
	// makes this feel like spam instead of a tip.
	next := (st.lastIndex + 1) % len(promos())
	st.lastIndex = next
	st.songsSince = 0
	st.lastSent = time.Now()
	return next, true
}
