package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kustbots/kustmusic/internal/store"
	"github.com/kustbots/kustmusic/internal/telegram"
)

// Clone bots: anyone can hand this bot a BotFather token with /clone and get
// their own copy of the music bot running under it.
//
// The main bot keeps /webhook. Each clone is served at /clone/<token>, so the
// token arrives with every update and nothing has to be kept in memory or in
// config — the token also sits in Mongo (clone_bots) purely so webhooks can
// be re-registered later with /resetclones, and so an unknown token in the
// path is refused rather than trusted.

const maxClonesPerUser = 5

var botTokenRE = regexp.MustCompile(`^[0-9]{5,15}:[A-Za-z0-9_-]{30,60}$`)

type cloneRegistry struct {
	mu      sync.RWMutex
	apps    map[string]*App // token -> clone
	chatBot map[int64]*App  // chat -> the bot that last spoke in it
}

func newCloneRegistry() *cloneRegistry {
	return &cloneRegistry{apps: make(map[string]*App), chatBot: make(map[int64]*App)}
}

func (r *cloneRegistry) get(token string) *App {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.apps[token]
}

func (r *cloneRegistry) put(token string, app *App) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.apps[token]; old != nil {
		for chatID, owner := range r.chatBot {
			if owner == old {
				r.chatBot[chatID] = app
			}
		}
	}
	r.apps[token] = app
}

func (r *cloneRegistry) drop(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.apps[token]
	delete(r.apps, token)
	for chatID, owner := range r.chatBot {
		if owner == old {
			delete(r.chatBot, chatID)
		}
	}
}

func (r *cloneRegistry) remember(chatID int64, app *App) {
	r.mu.Lock()
	r.chatBot[chatID] = app
	r.mu.Unlock()
}

// appFor is the bot that should speak in chatID: whichever one last handled
// an update from it, else fallback. play-api's stream-ended events carry only
// a chat id, and the "next song" message has to come from the bot that is
// actually in that chat.
func (r *cloneRegistry) appFor(chatID int64, fallback *App) *App {
	if r == nil {
		return fallback
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if app := r.chatBot[chatID]; app != nil {
		return app
	}
	return fallback
}

// newChild builds a clone App that shares everything chat-keyed with the main
// bot (router, queue, Mongo, premium) but speaks through its own token.
func (a *App) newChild(token string) *App {
	tg := telegram.New(token)
	c := &App{
		tg:         tg,
		router:     a.router,
		yt:         a.yt,
		queue:      a.queue,
		db:         a.db,
		log:        a.log,
		ownerID:    a.ownerID,
		startedAt:  time.Now(),
		broadcast:  a.broadcast,
		premium:    a.premium,
		playLocks:  a.playLocks,
		progress:   a.progress,
		precache:   a.precache,
		limiter:    a.limiter,
		adminCache: make(map[string]adminVerdict),
		isClone:    true,
		clones:     a.clones,
		mainToken:  a.mainToken,
		webhookURL: a.webhookURL,
	}
	c.assistants = newAssistantGuard(tg, a.router, a.log)
	c.promo = newPromoter(tg)
	return c
}

// cloneFor returns the running clone for token, loading it from Mongo the
// first time it is seen after a restart. nil means the token is not a
// registered clone.
func (a *App) cloneFor(token string) *App {
	if a.clones == nil {
		return nil
	}
	if c := a.clones.get(token); c != nil {
		return c
	}
	if a.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rec, err := a.db.GetCloneBot(ctx, token)
	if err != nil || rec == nil {
		return nil
	}
	c := a.newChild(token)
	a.clones.put(token, c)
	return c
}

func (a *App) cloneBaseURL() string {
	return strings.TrimSuffix(strings.TrimRight(a.webhookURL, "/"), "/webhook")
}

// handleCloneWebhook serves POST /clone/<token>.
func (a *App) handleCloneWebhook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	token := strings.TrimPrefix(r.URL.Path, "/clone/")
	if !botTokenRE.MatchString(token) {
		http.NotFound(w, r)
		return
	}
	child := a.cloneFor(token)
	if child == nil {
		http.NotFound(w, r)
		return
	}
	var u telegram.Update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	go child.processUpdate(u)
}

func (a *App) cmdClone(m *telegram.Message, args string) {
	chatID := m.Chat.ID
	if m.From == nil {
		return
	}
	token := strings.TrimSpace(args)
	if token == "" {
		_, _ = a.tg.SendMessage(chatID,
			"🤖 <b>Make your own music bot</b>\n\nCreate a bot in @BotFather, then send me:\n<code>/clone 123456:ABC-your-bot-token</code>\n\nDo it here in private — a bot token works like a password.", nil)
		return
	}
	// The token is already in the chat history by now; removing the message
	// at least keeps it off the screen.
	_ = a.tg.DeleteMessage(chatID, m.MessageID)
	if m.Chat.Type != "private" {
		_, _ = a.tg.SendMessage(chatID, "🔒 That token was just posted in a group, so treat it as leaked: revoke it in @BotFather (/revoke), then send the new one to me in a private chat.", nil)
		return
	}
	if !botTokenRE.MatchString(token) {
		_, _ = a.tg.SendMessage(chatID, "❌ That doesn't look like a bot token. Copy it from @BotFather exactly as given.", nil)
		return
	}
	if token == a.mainToken {
		_, _ = a.tg.SendMessage(chatID, "😄 That's me! Send a token from a bot of your own.", nil)
		return
	}
	base := a.cloneBaseURL()
	if a.db == nil || a.clones == nil || base == "" {
		_, _ = a.tg.SendMessage(chatID, "⚠️ Cloning isn't available right now. Try again later.", nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	existing, err := a.db.GetCloneBot(ctx, token)
	if err != nil {
		a.log.Warn("clone lookup failed", "err", err)
		_, _ = a.tg.SendMessage(chatID, "💾 Database hiccup — try again in a moment.", nil)
		return
	}
	userID := m.From.ID
	if existing != nil && existing.OwnerID != userID && userID != a.ownerID {
		_, _ = a.tg.SendMessage(chatID, "❌ That bot is already set up by someone else.", nil)
		return
	}
	if existing == nil && userID != a.ownerID {
		all, err := a.db.ListCloneBots(ctx)
		if err == nil {
			n := 0
			for _, b := range all {
				if b.OwnerID == userID {
					n++
				}
			}
			if n >= maxClonesPerUser {
				_, _ = a.tg.SendMessage(chatID, fmt.Sprintf("❌ You already have %d clones — that's the limit.", maxClonesPerUser), nil)
				return
			}
		}
	}

	childTG := telegram.New(token)
	me, err := childTG.GetMe()
	if err != nil || !me.IsBot {
		_, _ = a.tg.SendMessage(chatID, "❌ Telegram didn't accept that token. Check it and try again.", nil)
		return
	}
	if err := childTG.SetWebhook(base + "/clone/" + token); err != nil {
		a.log.Warn("clone webhook failed", "bot", me.Username, "err", err)
		_, _ = a.tg.SendMessage(chatID, "❌ Couldn't point that bot at me. Try again in a moment.", nil)
		return
	}
	rec := store.CloneBot{Token: token, BotID: me.ID, Username: me.Username, OwnerID: userID, CreatedAt: time.Now().Unix()}
	if err := a.db.SaveCloneBot(ctx, rec); err != nil {
		a.log.Error("saving clone failed — webhook is set but the token is not stored", "bot", me.Username, "err", err)
		_ = childTG.DeleteWebhook()
		_, _ = a.tg.SendMessage(chatID, "💾 Couldn't save your bot. Nothing was changed — try again.", nil)
		return
	}
	child := a.newChild(token)
	a.clones.put(token, child)
	go child.publishCommands()

	_, _ = a.tg.SendMessage(chatID, fmt.Sprintf(
		"✅ <b>@%s is live!</b>\n\nAdd it to a group, make it an admin, and use <code>/play</code>. To remove it later: <code>/unclone</code> with the token.", htmlEscape(me.Username)), nil)
}

func (a *App) cmdUnclone(m *telegram.Message, args string) {
	chatID := m.Chat.ID
	if m.From == nil || a.db == nil || a.clones == nil {
		return
	}
	arg := strings.TrimSpace(args)
	if arg == "" {
		_, _ = a.tg.SendMessage(chatID, "Usage: <code>/unclone &lt;bot token&gt;</code>", nil)
		return
	}
	if botTokenRE.MatchString(arg) {
		_ = a.tg.DeleteMessage(chatID, m.MessageID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token := arg
	if !botTokenRE.MatchString(arg) {
		// Only the bot's owner may remove by @username — a username is public.
		if m.From.ID != a.ownerID {
			_, _ = a.tg.SendMessage(chatID, "❌ Send the bot token to remove a clone.", nil)
			return
		}
		token = ""
		all, _ := a.db.ListCloneBots(ctx)
		for _, b := range all {
			if strings.EqualFold(b.Username, strings.TrimPrefix(arg, "@")) {
				token = b.Token
			}
		}
		if token == "" {
			_, _ = a.tg.SendMessage(chatID, "❌ No clone with that username.", nil)
			return
		}
	}
	rec, err := a.db.GetCloneBot(ctx, token)
	if err != nil || rec == nil || (rec.OwnerID != m.From.ID && m.From.ID != a.ownerID) {
		_, _ = a.tg.SendMessage(chatID, "❌ No such clone for you.", nil)
		return
	}
	_ = telegram.New(token).DeleteWebhook()
	if err := a.db.RemoveCloneBot(ctx, token); err != nil {
		_, _ = a.tg.SendMessage(chatID, "💾 Couldn't remove it. Try again.", nil)
		return
	}
	a.clones.drop(token)
	_, _ = a.tg.SendMessage(chatID, fmt.Sprintf("🗑 @%s removed.", htmlEscape(rec.Username)), nil)
}

func (a *App) cmdClones(m *telegram.Message) {
	if m.From == nil || m.From.ID != a.ownerID || a.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	all, err := a.db.ListCloneBots(ctx)
	if err != nil {
		_, _ = a.tg.SendMessage(m.Chat.ID, "💾 Couldn't read the clone list.", nil)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🤖 <b>%d clone bots</b>\n", len(all))
	for i, c := range all {
		if i >= 60 {
			fmt.Fprintf(&b, "…and %d more\n", len(all)-i)
			break
		}
		fmt.Fprintf(&b, "• @%s — owner <code>%d</code>\n", htmlEscape(c.Username), c.OwnerID)
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, b.String(), nil)
}

// cmdResetClones re-registers every stored clone's webhook against the
// current WEBHOOK_URL — for after this app is renamed, moved or redeployed
// somewhere new.
func (a *App) cmdResetClones(m *telegram.Message) {
	if m.From == nil || m.From.ID != a.ownerID || a.db == nil {
		return
	}
	base := a.cloneBaseURL()
	if base == "" {
		_, _ = a.tg.SendMessage(m.Chat.ID, "⚠️ WEBHOOK_URL isn't set, so there's nothing to point the clones at.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	all, err := a.db.ListCloneBots(ctx)
	if err != nil {
		_, _ = a.tg.SendMessage(m.Chat.ID, "💾 Couldn't read the clone list.", nil)
		return
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, fmt.Sprintf("🔄 Resetting %d webhooks…", len(all)), nil)
	ok, failed := 0, 0
	var bad []string
	for _, c := range all {
		if err := telegram.New(c.Token).SetWebhook(base + "/clone/" + c.Token); err != nil {
			failed++
			bad = append(bad, "@"+c.Username)
		} else {
			ok++
		}
		time.Sleep(60 * time.Millisecond)
	}
	text := fmt.Sprintf("✅ Done. Reset: %d, failed: %d.", ok, failed)
	if len(bad) > 0 {
		if len(bad) > 30 {
			bad = bad[:30]
		}
		text += "\nFailed: " + htmlEscape(strings.Join(bad, ", "))
	}
	_, _ = a.tg.SendMessage(m.Chat.ID, text, nil)
}
