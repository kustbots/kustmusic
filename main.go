// kust-music-bot is the Go rewrite of main-music-rx: /start, /play, /skip,
// /stop, /pause, /resume, /queue, /playlist (MongoDB-backed, same
// database/collections as the Python bot), admin moderation (/ban /unban
// /kick /mute /unmute /tmute), /prime (premium users, overriding to a
// dedicated play-api server) and /broadcast (/bstatus /bcancel), all served
// over a Telegram Bot API webhook (not long-polling/TDLib) so the bot is a
// plain, fast HTTP service instead of a persistent poller.
//
// Scope note (deliberate, not an oversight): i18n (the original bot's
// multi-language UI text) is not ported — everything here is English only.
// Couples and welcome-image features are dropped entirely, per the
// project's explicit decision to remove them in this rewrite. /vplay
// (video playback) has server routing wired up (playrouter.VideoServerFor)
// but no command handler yet.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kustbots/kustmusic/internal/core/ytdlp"
	engine "github.com/kustbots/kustmusic/internal/playapi/app"
	"github.com/kustbots/kustmusic/internal/playapi/config"
	"github.com/kustbots/kustmusic/internal/playrouter"
	"github.com/kustbots/kustmusic/internal/queue"
	"github.com/kustbots/kustmusic/internal/store"
	"github.com/kustbots/kustmusic/internal/telegram"
	"github.com/kustbots/kustmusic/internal/ytapi"
)

type App struct {
	tg        *telegram.Client
	router    *playrouter.Router
	yt        *ytapi.Client
	queue     *queue.Manager
	db        *store.Store
	log       *slog.Logger
	ownerID   int64
	startedAt time.Time

	// Clone-bot support (see clone.go). isClone marks an App that speaks
	// through a user-supplied token rather than the main one.
	isClone    bool
	clones     *cloneRegistry
	mainToken  string
	webhookURL string

	broadcast  *broadcastState
	premium    *premiumCache
	playLocks  *chatLocks
	progress   *progressTracker
	precache   *precacher
	assistants *assistantGuard
	promo      *promoter
	limiter    *commandLimiter

	adminMu    sync.Mutex
	adminCache map[string]adminVerdict

	// pendingPlay holds the track a chat was trying to play when every
	// assistant account turned out to be rate-limited, so the "I've added
	// it" button can carry on from there instead of asking the user to
	// type the search again.
	//
	// It is deliberately not the queue: a play that never started is
	// dropped from the queue so it can't wedge the chat, and parking a
	// copy here keeps that property while still making the button useful.
	pendingMu   sync.Mutex
	pendingPlay map[int64]queue.Song
}

// rememberPendingPlay parks song as chatID's resume target.
func (a *App) rememberPendingPlay(chatID int64, song queue.Song) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if a.pendingPlay == nil {
		a.pendingPlay = make(map[int64]queue.Song)
	}
	a.pendingPlay[chatID] = song
}

// takePendingPlay removes and returns chatID's resume target, if it has
// one. Taking rather than reading means a second tap of the button can't
// start the same song twice.
func (a *App) takePendingPlay(chatID int64) (queue.Song, bool) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	song, ok := a.pendingPlay[chatID]
	if ok {
		delete(a.pendingPlay, chatID)
	}
	return song, ok
}

func main() {
	log := slog.Default()

	token := firstEnv("BOT_TOKEN", "TOKEN")
	if token == "" {
		log.Error("BOT_TOKEN is not set. Create a bot with @BotFather and put its token there")
		os.Exit(1)
	}
	cfg := config.Load()
	if cfg.APIID == 0 || cfg.APIHash == "" {
		log.Error("API_ID and API_HASH are not set. Get them at https://my.telegram.org")
		os.Exit(1)
	}
	port := getenv("PORT", "8000")
	webhookURL := os.Getenv("WEBHOOK_URL")
	ownerID, _ := strconv.ParseInt(getenv("OWNER_ID", "0"), 10, 64)
	mongoURI := os.Getenv("MONGO_URI")

	// The music source and the playback engine both run inside this process.
	// The engine only listens on 127.0.0.1 and the bot is its one client.
	dl := ytdlp.New(ytdlp.Config{
		Bin:         cfg.YTDLPPath,
		CookiesFile: cookiesFile(cfg.CookiesFile),
		ExtraArgs:   cfg.YTDLPArgs,
	})
	cfg.BotWebhookURL = "http://127.0.0.1:" + port + "/playapi-events"
	engine.Start(cfg, dl)
	servers := []string{"http://127.0.0.1:" + cfg.Port}

	var db *store.Store
	if mongoURI == "" {
		log.Info("MONGO_URI is not set, so playlists, premium users, broadcast and clone bots are off")
	} else {
		var err error
		db, err = store.Connect(mongoURI)
		if err != nil {
			log.Error("mongo connect failed, so playlists, premium users, broadcast and clone bots are off", "err", err)
			db = nil
		}
	}

	app := &App{
		tg:        telegram.New(token),
		router:    playrouter.New(servers),
		yt:        ytapi.New(dl),
		queue:     queue.New(),
		db:        db,
		log:       log,
		ownerID:   ownerID,
		startedAt: time.Now(),
		broadcast: newBroadcastState(),
		premium:   newPremiumCache(),
		playLocks: newChatLocks(),
		progress:  newProgressTracker(),
		precache:  newPrecacher("http://127.0.0.1:"+cfg.Port+"/cache?url=", log),

		clones:     newCloneRegistry(),
		mainToken:  token,
		webhookURL: webhookURL,
	}
	me, err := app.tg.GetMe()
	if err != nil {
		log.Error("Telegram rejected BOT_TOKEN", "err", err)
		os.Exit(1)
	}
	addToGroupURL = "https://t.me/" + me.Username + "?startgroup=true"
	app.router.SetBotID(strconv.FormatInt(me.ID, 10))
	log.Info("bot identified", "username", me.Username)
	app.assistants = newAssistantGuard(app.tg, app.router, log)
	app.promo = newPromoter(app.tg)
	app.limiter = newCommandLimiter()
	app.adminCache = make(map[string]adminVerdict)
	go func() {
		for range time.Tick(30 * time.Minute) {
			app.limiter.Sweep()
		}
	}()
	go app.publishCommands() // refresh the "/" menu without delaying startup

	if db != nil {
		app.premium.refresh(db)
		go func() {
			for range time.Tick(2 * time.Minute) {
				app.premium.refresh(db)
			}
		}()
	}

	go app.router.PollLoadsForever(15*time.Second, log)

	if webhookURL != "" {
		if err := app.tg.SetWebhook(webhookURL); err != nil {
			log.Warn("failed to register webhook", "err", err)
		} else {
			log.Info("webhook registered", "url", webhookURL)
		}
	} else {
		// No public address to receive updates on, so ask Telegram for them.
		_ = app.tg.DeleteWebhook()
		go app.pollUpdates()
		log.Info("WEBHOOK_URL is not set, receiving updates by polling")
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	http.HandleFunc("/webhook", app.handleWebhook)
	http.HandleFunc("/clone/", app.handleCloneWebhook)
	http.HandleFunc("/playapi-events", app.handlePlayAPIEvent)

	// Heroku restarts dynos daily and on every deploy, announcing it with
	// SIGTERM before killing the process. Catch it, write the queues to
	// Mongo, and exit — otherwise every group's lined-up songs disappear
	// at that moment with nothing said in the chat.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-stop
		log.Info("shutdown signal received, saving queues", "signal", sig.String())
		app.saveQueues()
		os.Exit(0)
	}()

	// Pick up whatever the previous run was playing. In the background so
	// the webhook server starts accepting updates immediately — Telegram
	// retries on a closed port and a slow boot would pile up a backlog.
	go app.restoreQueues()

	log.Info("kust-music-bot starting", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Error("server exited", "err", err)
		os.Exit(1)
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// cookiesFile returns the cookies file to hand to yt-dlp: COOKIES_FILE when
// set, otherwise a private temporary file written from the COOKIES text, so
// the cookies never have to live in the image or the repository.
func cookiesFile(path string) string {
	if path != "" {
		return path
	}
	text := os.Getenv("COOKIES")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	f, err := os.CreateTemp("", "cookies-*.txt")
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		return ""
	}
	return f.Name()
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (a *App) handleWebhook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var u telegram.Update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		w.WriteHeader(http.StatusOK) // never make Telegram retry-storm on a bad body
		return
	}
	w.WriteHeader(http.StatusOK)

	// Handle in the background so Telegram's webhook POST returns
	// immediately — playback can take a while (see play-api's own ping
	// mechanism for why), and Telegram expects a fast 200.
	go a.processUpdate(u)
}

// processUpdate runs one update through this bot — the main one or a clone.
func (a *App) processUpdate(u telegram.Update) {
	defer func() {
		if rec := recover(); rec != nil {
			a.log.Error("panic handling update", "recover", rec)
		}
	}()
	switch {
	case u.Message != nil:
		a.clones.remember(u.Message.Chat.ID, a)
		// Clone chats stay out of the main bot's broadcast list: it isn't a
		// member of them, so a broadcast would fail and prune them.
		if !a.isClone {
			a.registerBroadcastChat(u.Message.Chat)
		}
		a.handleMessage(u.Message)
	case u.CallbackQuery != nil:
		if u.CallbackQuery.Message != nil {
			a.clones.remember(u.CallbackQuery.Message.Chat.ID, a)
		}
		a.handleCallback(u.CallbackQuery)
	}
}

// registerBroadcastChat records chatID so /broadcast can reach it later —
// matches main-music-rx's registration in start_handler, but done on every
// incoming message instead of just /start so a chat isn't missed if a user
// never explicitly ran /start.
func (a *App) registerBroadcastChat(chat telegram.Chat) {
	if a.db == nil {
		return
	}
	kind := "group"
	if chat.Type == "private" {
		kind = "private"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.db.RegisterBroadcastChat(ctx, chat.ID, kind); err != nil {
		a.log.Debug("registerBroadcastChat failed", "chat_id", chat.ID, "err", err)
	}
}

func (a *App) handleMessage(m *telegram.Message) {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return
	}
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	if at := strings.IndexByte(cmd, '@'); at != -1 {
		cmd = cmd[:at]
	}
	args := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))

	// Throttle before doing any work. Ported from the Python bot's
	// check_abuse: per user, four commands per six seconds. Anything past
	// that is someone leaning on a button, and every one of these commands
	// costs real Telegram calls and play-api work.
	if strings.HasPrefix(cmd, "/") && m.From != nil {
		if allowed, warn := a.limiter.Allow(m.From.ID); !allowed {
			if warn {
				_, _ = a.tg.SendMessage(m.Chat.ID, "⏳ Easy — give me a second to catch up.", nil)
			}
			return
		}
	}

	// Owner tooling belongs to the main bot only.
	if a.isClone {
		switch cmd {
		case "/broadcast", "/bstatus", "/bcancel", "/prime", "/live", "/clones", "/resetclones":
			return
		}
	}

	switch cmd {
	case "/clone":
		a.cmdClone(m, args)
	case "/unclone":
		a.cmdUnclone(m, args)
	case "/clones":
		a.cmdClones(m)
	case "/resetclones":
		a.cmdResetClones(m)
	case "/start":
		a.cmdStart(m)
	case "/play":
		a.cmdPlay(m, args)
	case "/skip", "/next":
		if a.requireControlAdminCmd(m) {
			a.cmdSkip(m.Chat.ID, actorOf(m.From))
		}
	case "/stop", "/end":
		a.cmdStopGuarded(m)
	case "/pause":
		if a.requireControlAdminCmd(m) {
			a.cmdPause(m.Chat.ID, actorOf(m.From))
		}
	case "/resume", "/unpause":
		if a.requireControlAdminCmd(m) {
			a.cmdResume(m.Chat.ID, actorOf(m.From))
		}
	case "/queue":
		a.cmdQueue(m.Chat.ID)
	case "/clear":
		if a.requireControlAdminCmd(m) {
			a.cmdClear(m.Chat.ID, actorOf(m.From))
		}
	case "/ping":
		a.cmdPing(m.Chat.ID)
	case "/help":
		a.cmdHelp(m.Chat.ID)
	case "/playlist":
		a.cmdPlaylist(m)
	case "/ban":
		a.cmdBan(m, args)
	case "/unban":
		a.cmdUnban(m, args)
	case "/kick":
		a.cmdKick(m, args)
	case "/mute":
		a.cmdMute(m, args)
	case "/unmute":
		a.cmdUnmute(m, args)
	case "/tmute":
		a.cmdTmute(m, args)
	case "/prime":
		a.cmdPrime(m, args)
	case "/broadcast":
		a.cmdBroadcast(m, args)
	case "/bstatus":
		a.cmdBroadcastStatus(m)
	case "/bcancel":
		a.cmdBroadcastCancel(m)
	case "/live":
		a.cmdLive(m)
	}
}

func (a *App) handleCallback(cq *telegram.CallbackQuery) {
	if cq.Message == nil {
		return
	}
	chatID := cq.Message.Chat.ID
	switch {
	case cq.Data == "skip", cq.Data == "stop", cq.Data == "pause", cq.Data == "resume":
		// Same rule as the typed commands — the player card's buttons are
		// visible to everyone in the group, so without this they were the
		// easy way around the check.
		if !a.requireControlAdminCallback(cq) {
			return
		}
		switch cq.Data {
		case "skip":
			a.cmdSkip(chatID, actorOf(&cq.From))
		case "stop":
			a.cmdStop(chatID, actorOf(&cq.From))
		case "pause":
			a.cmdPause(chatID, actorOf(&cq.From))
		case "resume":
			a.cmdResume(chatID, actorOf(&cq.From))
		}
	case cq.Data == "add_to_playlist":
		a.cbAddToPlaylist(cq)
	case strings.HasPrefix(cq.Data, "playlist_page|"):
		a.cbPlaylistPage(cq)
	case strings.HasPrefix(cq.Data, "playlist_detail|"):
		a.cbPlaylistDetail(cq)
	case strings.HasPrefix(cq.Data, "play_song|"):
		a.cbPlaySong(cq)
	case strings.HasPrefix(cq.Data, "remove_from_playlist|"):
		a.cbRemoveFromPlaylist(cq)
	case strings.HasPrefix(cq.Data, "queue_playnow|"):
		// This jumps the queue and skips the current song, so it's the
		// same privilege as pressing skip.
		if !a.requireControlAdminCallback(cq) {
			return
		}
		a.cbQueuePlayNow(cq)
	case strings.HasPrefix(cq.Data, "queue_remove|"):
		// Binning a song someone else queued is the same kind of power as
		// skipping one, so it answers to the same rule.
		if !a.requireControlAdminCallback(cq) {
			return
		}
		a.cbQueueRemove(cq)
	case cq.Data == "clear":
		if !a.requireControlAdminCallback(cq) {
			return
		}
		a.cmdClear(chatID, actorOf(&cq.From))
	case cq.Data == "progress":
		// The progress bar is a button purely so it can render inside the
		// keyboard; tapping it just reports where the track is, as an
		// alert, rather than doing anything.
		_ = a.tg.AnswerCallbackQuery(cq.ID, a.progressText(chatID))
		return
	case cq.Data == "assistant_added":
		a.cbAssistantAdded(cq)
	case cq.Data == "show_help":
		a.cbShowHelp(cq)
	case cq.Data == "help_music":
		a.cbHelpPage(cq, helpMusicText)
	case cq.Data == "help_admin":
		a.cbHelpPage(cq, helpAdminText)
	case cq.Data == "help_playlist":
		a.cbHelpPage(cq, helpPlaylistText)
	case cq.Data == "help_util":
		a.cbHelpPage(cq, helpUtilText)
	case cq.Data == "go_back":
		a.cbGoBack(cq)
	case cq.Data == "playlist_back":
		a.sendPlaylistPage(chatID, 0, cq.From.ID)
	}
	_ = a.tg.AnswerCallbackQuery(cq.ID, "")
}

func (a *App) cmdStart(m *telegram.Message) {
	name := "Music Lover"
	var userID int64
	if m.From != nil {
		userID = m.From.ID
		if m.From.FirstName != "" {
			name = m.From.FirstName
		}
	}
	userLink := fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, userID, htmlEscape(name))
	caption, kb := buildHomeScreen(userLink)
	a.sendHome(m.Chat.ID, caption, kb)
}

func (a *App) cmdPlay(m *telegram.Message, query string) {
	chatID := m.Chat.ID
	if query == "" {
		_, _ = a.tg.SendMessage(chatID, "Usage: /play <song name or link>", nil)
		return
	}

	// Stage 1: a bare snowflake, out immediately so the chat sees the
	// command registered before the search round-trip even starts.
	processing, _ := a.tg.SendMessage(chatID, processingSnowflake, nil)

	result, err := a.yt.Resolve(query)
	if err != nil {
		a.log.Warn("search failed", "chat_id", chatID, "query", query, "err", err)
		a.replaceMessage(chatID, processing,
			"🔍 Couldn't find anything for that. Check the spelling, or paste a link instead?", nil)
		return
	}

	song := queue.Song{Title: result.Title, URL: result.URL, Duration: result.Duration, Thumbnail: result.Thumbnail, Query: query}
	if m.From != nil {
		song.RequesterID = m.From.ID
		song.RequesterName = m.From.FirstName
	}
	position := a.queue.Push(chatID, song)

	if position > 1 {
		// Warm the edge cache now, while the current track is still
		// playing, so this one is a cache hit by the time it comes up
		// instead of a ~10s cold fetch.
		a.precache.Warm(song.URL)
		requester := mentionUser(song.RequesterID, song.RequesterName)
		a.replaceMessage(chatID, processing,
			queueAddedCaption(song.Title, formatTime(iso8601ToSeconds(song.Duration)), requester, position-1),
			queueAddedKeyboard())
		return
	}

	a.startPlayback(chatID, processing, song)
}

// cbAssistantAdded handles the "I've added it" button on the rate-limited
// card: someone has walked an assistant into the group by hand, so the
// cached "not a member here" verdicts are now wrong, and the song that was
// interrupted can be picked up.
func (a *App) cbAssistantAdded(cq *telegram.CallbackQuery) {
	chatID := cq.Message.Chat.ID
	_ = a.tg.AnswerCallbackQuery(cq.ID, "Checking…")

	// Whatever the guard last worked out about this chat, it was worked
	// out before the account was added. Forget it so membership is
	// re-checked rather than answered from a stale "absent".
	a.assistants.Forget(chatID)

	song, ok := a.takePendingPlay(chatID)
	if !ok {
		a.editCard(cq, "<b>✅ ᴛʜᴀɴᴋs</b>\n\nNothing was left waiting — send <code>/play</code> and I'll pick it up from here.", nil)
		return
	}

	// Back onto the queue it goes. If something else is already playing,
	// this belongs behind it rather than cutting in.
	if pos := a.queue.Push(chatID, song); pos > 1 {
		a.editCard(cq, "<b>✅ ᴀᴅᴅᴇᴅ ʙᴀᴄᴋ ᴛᴏ ᴛʜᴇ ǫᴜᴇᴜᴇ</b>\n\n"+
			htmlEscape(oneLineTitle(song.Title))+" will play when the current track finishes.", nil)
		return
	}

	a.editCard(cq, "<b>🎧 ᴘɪᴄᴋɪɴɢ ᴜᴘ ᴡʜᴇʀᴇ ᴡᴇ ʟᴇғᴛ ᴏғғ…</b>", nil)
	processing, _ := a.tg.SendMessage(chatID, processingSnowflake, nil)
	go a.startPlayback(chatID, processing, song)
}

// startPlayback calls play-api for song and updates processing (the
// "searching..." message) into the final now-playing card or an error.
func (a *App) startPlayback(chatID int64, processing *telegram.SentMessage, song queue.Song) {
	// Serialize per chat — see chatlock.go for why (concurrent setups in
	// one chat invalidate each other on play-api's side).
	a.playLocks.Lock(chatID)
	defer a.playLocks.Unlock(chatID)

	// A song that never actually started must not be left at the head of
	// the queue. The caller pushes first and plays second, so a failed
	// setup used to leave the track parked at position 0 with nothing
	// playing it — and every subsequent /play in that chat was then
	// announced as "added to queue" *behind* the phantom instead of
	// starting, until someone ran /stop. Cleaning up here is what stops one
	// failed play from wedging the whole chat.
	//
	// dropOnExit stays true for every failure exit below, and is cleared
	// only on success and on the superseded case — there a newer request
	// legitimately owns the queue now, and this attempt must not touch it.
	dropOnExit := true
	defer func() {
		if dropOnExit {
			a.queue.DropCurrent(chatID, song.URL)
		}
	}()

	isPremium := a.premium.isPremium(song.RequesterID)
	server := a.router.ServerFor(chatID, isPremium)
	if server == "" {
		a.replaceMessage(chatID, processing, "❌ No playback servers are switched on right now. Support will want to hear about this one.", nil)
		return
	}

	// Stage 2: the search resolved and playback setup is starting — edit
	// the snowflake into the fuller status, so the chat isn't staring at a
	// lone emoji for the whole (potentially slow) join+buffer wait.
	if processing != nil {
		status := processingStatus
		if isPremium {
			status = processingStatusPremium
		}
		_ = a.tg.EditMessageText(chatID, processing.MessageID, status, nil)
	}

	result, err, server := a.playWithFailover(chatID, song, server, isPremium)
	if err != nil {
		// "The assistant isn't in your group" is the one failure the chat
		// can fix itself, so it gets its own message naming the account to
		// add, rather than the generic sanitised error.
		var missing *assistantMissingError
		if errors.As(err, &missing) {
			// Park the song either way: whichever wording the card uses, it
			// offers an "I've added it" button, and that button is only
			// worth anything if there's something to resume.
			a.rememberPendingPlay(chatID, song)
			until, _, allLimited := a.assistants.FloodSummary()
			text, kb := assistantMissingCard(missing, until, allLimited)
			a.replaceMessage(chatID, processing, text, kb)
			return
		}
		a.replaceMessage(chatID, processing, "❌ "+htmlEscape(userFacingError(err.Error())), supportKeyboard())
		return
	}
	if !result.OK && playrouter.IsSuperseded(result.Error) {
		// Another /play for this chat took over while this one was still
		// setting up. That's the newer request winning a race, not a
		// failure — the user who triggered it is already getting their own
		// card. Surfacing it as "❌ Playback failed" (what this used to do)
		// meant a busy chat filled with scary red errors for something
		// entirely normal. Quietly drop the stale attempt's message.
		//
		// The newer request owns the queue now, so this stale attempt must
		// not clean up after itself — doing so would delete the song the
		// winner is about to play.
		dropOnExit = false
		if processing != nil {
			_ = a.tg.DeleteMessage(chatID, processing.MessageID)
		}
		return
	}
	if !result.OK && playrouter.NeedsAssistantRejoin(result.Error) {
		// The assistant isn't in this chat any more — most likely the
		// idle-group sweeper left it after a quiet spell. Re-invite it and
		// retry once, so a group coming back to life just works instead of
		// needing someone to re-add the assistant by hand.
		a.assistants.Forget(chatID) // the cached "it's in there" answer was wrong
		if a.reinviteAssistant(server, chatID) {
			result, err = a.router.Play(server, chatID, song.URL, int(iso8601ToSeconds(song.Duration)))
			if err != nil {
				a.replaceMessage(chatID, processing, "❌ "+htmlEscape(userFacingError(err.Error())), supportKeyboard())
				return
			}
		}
	}
	if !result.OK {
		a.log.Warn("playback failed", "chat_id", chatID, "url", song.URL, "err", result.Error)
		a.replaceMessage(chatID, processing, "❌ "+htmlEscape(userFacingError(result.Error)), supportKeyboard())
		return
	}

	// Playing for real — the song belongs at the head of the queue now.
	dropOnExit = false

	requesterMention := "Someone"
	if song.RequesterID != 0 {
		name := song.RequesterName
		if name == "" {
			name = strconv.FormatInt(song.RequesterID, 10)
		}
		requesterMention = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, song.RequesterID, htmlEscape(name))
	}
	displayServer := "Premium"
	modeText := "𝐏𝐫𝐞ᴍɪᴜᴍ⚡"
	if !isPremium {
		displayServer = strconv.Itoa(a.router.DisplayIndex(server))
		modeText = "sᴛᴀɴᴅᴀʀᴅ"
	}

	total := iso8601ToSeconds(song.Duration)
	caption := playerCaption(song.Title, requesterMention, displayServer, modeText)
	kb := playerKeyboard(total)
	card := a.sendPlayerCard(chatID, processing, caption, kb, song.Thumbnail)
	if card != nil {
		a.progress.Start(chatID, card.MessageID, total, a.renderProgress)
	}

	// Only ever after the song someone asked for is already playing, and
	// only once in a long while — see promo.go for the pacing.
	a.promo.MaybeSend(chatID)
}

// playWithFailover tries the assigned server, then moves the chat to a
// different one and tries again if the failure looks like something another
// server could succeed at.
//
// One unhealthy server used to fail every play pinned to it — a dead
// assistant session, or a bad upstream moment, and that chat was stuck even
// though the other four were idle and healthy. Now the chat gets reassigned
// and retried. Returns the server that actually served the request so the
// player card reports the right one.
//
// Deliberately does NOT retry a "superseded" result (a newer /play already
// won — retrying would fight it) or an assistant-not-in-chat error (that's
// handled by re-inviting on the same server, just below).
func (a *App) playWithFailover(chatID int64, song queue.Song, server string, isPremium bool) (*playrouter.PlayResult, error, string) {
	tried := map[string]bool{}
	var lastResult *playrouter.PlayResult
	var lastErr error

	// attempt counts real /play calls, not servers looked at. Skipping a
	// server because its account can't be used costs nothing and shouldn't
	// eat the budget for actually trying to play something — see the guard
	// branch below.
	for attempt := 0; attempt < maxServerAttempts; {
		tried[server] = true

		// Confirm this server's assistant is actually in the group before
		// asking it to play. Without this the server could report success
		// for a chat its account had been removed from, and the bot posted
		// a now-playing card for silence (see assistants.go).
		if gerr := a.assistants.Ensure(chatID, server); gerr != nil {
			lastResult, lastErr = nil, gerr

			// A missing assistant is specific to an account, and a chat
			// that one account can't reach another may already be in — so
			// move rather than give up. Rule out every server running the
			// same account, though: some pairs share one, and retrying the
			// twin fails identically while spending an attempt.
			var missing *assistantMissingError
			if errors.As(gerr, &missing) {
				for _, twin := range a.router.ServersSharingAssistant(missing.ID) {
					tried[twin] = true
				}
			}
			if isPremium {
				break
			}
			next := a.router.Reassign(chatID, tried)
			if next == "" {
				break
			}
			a.log.Warn("assistant unusable for chat, trying another server",
				"chat_id", chatID, "from", server, "to", next, "err", gerr)
			server = next
			// Deliberately no attempt++: nothing was played, and an account
			// sitting out a flood wait shouldn't burn a chat's chances.
			// This walks the whole fleet if it has to, which is what lets
			// a flood-limited account hand its traffic to a free one. It
			// still terminates — tried[] only grows, so Reassign runs out.
			continue
		}

		attempt++
		result, err := a.router.Play(server, chatID, song.URL, int(iso8601ToSeconds(song.Duration)))
		if err == nil && result.OK {
			return result, nil, server
		}
		lastResult, lastErr = result, err

		// These two are resolved on the current server, not by moving.
		if err == nil && (playrouter.IsSuperseded(result.Error) ||
			playrouter.NeedsAssistantRejoin(result.Error)) {
			return result, nil, server
		}

		// Premium chats are pinned to their dedicated server on purpose —
		// moving them off it would silently downgrade the tier.
		if isPremium {
			break
		}

		next := a.router.Reassign(chatID, tried)
		if next == "" {
			break // every server has been tried
		}
		a.log.Warn("play failed, failing over to another server",
			"chat_id", chatID, "from", server, "to", next,
			"err", errText(err, result))
		server = next
	}

	return lastResult, lastErr, server
}

// maxServerAttempts caps how many servers one request will actually try to
// play on before giving up, so a chat can't spend forever walking a broken
// fleet. Servers skipped because their account is unusable don't count
// toward it — those cost nothing and are how traffic moves off a
// flood-limited account onto a free one.
const maxServerAttempts = 3

func errText(err error, result *playrouter.PlayResult) string {
	if err != nil {
		return err.Error()
	}
	if result != nil {
		return result.Error
	}
	return "unknown"
}

// reinviteAssistant adds server's assistant back into chatID after a play
// failed because it wasn't there. Reports whether the join looks like it
// worked, so the caller knows whether retrying the play is worthwhile.
//
// Delegates to the guard so both re-invite paths share one implementation
// — this one used to export (and so revoke) the group's primary invite
// link, while the guard created a single-use one.
func (a *App) reinviteAssistant(server string, chatID int64) bool {
	info, err := a.router.Assistant(server)
	if err != nil {
		a.log.Warn("couldn't identify assistant to re-invite", "chat_id", chatID, "err", err)
		return false
	}
	if err := a.assistants.invite(chatID, server, info.ID); err != nil {
		return false
	}
	a.log.Info("re-invited assistant to chat", "chat_id", chatID)
	return true
}

// sendPlayerCard replicates setup_player_ui's exact send sequence: delete
// the "processing" message, try sending the card as a photo (thumbnail)
// with an HTML caption, and only fall back to a plain text message if
// there's no thumbnail or the photo send fails.
// Returns the sent card so the caller can animate its progress bar.
func (a *App) sendPlayerCard(chatID int64, processing *telegram.SentMessage, caption string, kb telegram.InlineKeyboard, thumbnailURL string) *telegram.SentMessage {
	if processing != nil {
		_ = a.tg.DeleteMessage(chatID, processing.MessageID)
	}
	if thumbnailURL != "" && (strings.HasPrefix(thumbnailURL, "http://") || strings.HasPrefix(thumbnailURL, "https://")) {
		if sent, err := a.tg.SendPhoto(chatID, thumbnailURL, caption, kb); err == nil {
			return sent
		}
	}
	sent, err := a.tg.SendMessage(chatID, caption, kb)
	if err != nil {
		return nil
	}
	return sent
}

// renderProgress redraws the player card's keyboard with the bar advanced
// to elapsed — the callback the progress tracker ticks.
func (a *App) renderProgress(chatID int64, messageID int, elapsed, total float64) error {
	return a.tg.EditMessageReplyMarkup(chatID, messageID, playerKeyboardAt(elapsed, total))
}

func (a *App) cmdSkip(chatID int64, actor string) {
	a.progress.Stop(chatID)
	server := a.router.ServerFor(chatID, false)
	next, hasNext := a.queue.Advance(chatID)
	if !hasNext {
		_ = a.router.Stop(server, chatID)
		_, _ = a.tg.SendMessage(chatID, queueEndedText+byActor(actor), queueEndedKeyboard())
		return
	}
	processing, _ := a.tg.SendMessage(chatID, fmt.Sprintf("⏭ <b>Skipped</b> — playing <b>%s</b>%s", htmlEscape(next.Title), byActor(actor)), nil)
	a.startPlayback(chatID, processing, next)
}

// cmdStopGuarded is /stop as a plain command in a group: main-music-rx
// requires the caller be a chat admin for this specific path (unlike the
// "stop" inline button, which doesn't re-check — matches the Python bot's
// own asymmetry between stop_handler and the callback handler).
func (a *App) cmdStopGuarded(m *telegram.Message) {
	if m.Chat.Type == "group" || m.Chat.Type == "supergroup" {
		if m.From == nil {
			return
		}
		ok, err := a.tg.IsAdmin(m.Chat.ID, m.From.ID)
		if err != nil || !ok {
			_, _ = a.tg.SendMessage(m.Chat.ID, "🔒 Admins only, sorry.", nil)
			return
		}
	}
	a.cmdStop(m.Chat.ID, actorOf(m.From))
}

func (a *App) cmdStop(chatID int64, actor string) {
	a.progress.Stop(chatID)
	server := a.router.ServerFor(chatID, false)
	if err := a.router.Stop(server, chatID); err != nil {
		a.log.Warn("stop failed", "chat_id", chatID, "err", err)
	}
	a.queue.Clear(chatID)
	_, _ = a.tg.SendMessage(chatID, "⏹ <b>Stopped</b> — out of the voice chat, queue wiped."+byActor(actor), nil)
}

func (a *App) cmdPause(chatID int64, actor string) {
	server := a.router.ServerFor(chatID, false)
	if err := a.router.Pause(server, chatID); err != nil {
		a.log.Warn("pause failed", "chat_id", chatID, "err", err)
		_, _ = a.tg.SendMessage(chatID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_, _ = a.tg.SendMessage(chatID, "⏸ <b>Paused</b>"+byActor(actor), nil)
}

func (a *App) cmdResume(chatID int64, actor string) {
	server := a.router.ServerFor(chatID, false)
	if err := a.router.Resume(server, chatID); err != nil {
		a.log.Warn("resume failed", "chat_id", chatID, "err", err)
		_, _ = a.tg.SendMessage(chatID, "❌ "+htmlEscape(controlError(err.Error())), nil)
		return
	}
	_, _ = a.tg.SendMessage(chatID, "▶️ <b>Resumed</b>"+byActor(actor), nil)
}

func (a *App) cmdQueue(chatID int64) {
	songs := a.queue.List(chatID)
	if len(songs) == 0 {
		_, _ = a.tg.SendMessage(chatID, "📭 Queue's empty. Line something up with <code>/play</code>.", nil)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🎵 <b>ǫᴜєᴜє</b> · <i>%d track(s)</i>\n<blockquote>", len(songs))
	var kb telegram.InlineKeyboard
	for i, s := range songs {
		marker := strconv.Itoa(i + 1)
		if i == 0 {
			marker = "▶️"
		}
		fmt.Fprintf(&b, "%s  %s", marker, htmlEscape(s.Title))
		if i < len(songs)-1 {
			b.WriteString("\n")
		}
		if i > 0 {
			idx := strconv.Itoa(i)
			kb = append(kb, []telegram.InlineButton{
				{Text: fmt.Sprintf("#%d ▶️ Play Now", i+1), CallbackData: "queue_playnow|" + idx, Style: telegram.StyleSuccess},
				{Text: "🗑 Remove", CallbackData: "queue_remove|" + idx, Style: telegram.StyleDanger},
			})
		}
	}
	b.WriteString("</blockquote>")
	_, _ = a.tg.SendMessage(chatID, b.String(), kb)
}

// cmdClear empties the queue without stopping what's already playing —
// matches the Python bot's /clear and its "🗑 Clear" button.
func (a *App) cmdClear(chatID int64, actor string) {
	current, hadCurrent := a.queue.Current(chatID)
	a.queue.Clear(chatID)
	if hadCurrent {
		// Keep the currently-playing track so /clear only drops what's
		// waiting, rather than silently killing the live stream too.
		a.queue.Push(chatID, current)
	}
	_, _ = a.tg.SendMessage(chatID, "🗑 <b>Queue cleared</b> — whatever's playing keeps going."+byActor(actor), nil)
}

func (a *App) cmdPing(chatID int64) {
	start := time.Now()
	msg, err := a.tg.SendMessage(chatID, "🏓 Pinging…", nil)
	if err != nil {
		return
	}
	latency := time.Since(start)
	uptime := time.Since(a.startedAt).Round(time.Second)
	text := fmt.Sprintf("🏓 <b>Pong!</b>\n❍ <b>Latency:</b> %dms\n❍ <b>Uptime:</b> %s",
		latency.Milliseconds(), uptime)
	_ = a.tg.EditMessageText(chatID, msg.MessageID, text, nil)
}

func (a *App) cmdHelp(chatID int64) {
	_, _ = a.tg.SendMessage(chatID, helpMenuText, helpMenuKeyboard())
}

// progressText reports how far into the current track playback is, for the
// progress button's tap-through alert.
func (a *App) progressText(chatID int64) string {
	song, ok := a.queue.Current(chatID)
	if !ok {
		return "Nothing's playing right now."
	}
	total := iso8601ToSeconds(song.Duration)
	if total <= 0 {
		return song.Title
	}
	return fmt.Sprintf("%s — %s", oneLineTitle(song.Title), formatTime(total))
}

// cbQueuePlayNow handles the queue's "▶️ Play Now" button: moves the
// selected song to play right after whatever's currently playing, then
// skips straight to it — reusing cmdSkip's existing advance-and-play path
// rather than duplicating it.
func (a *App) cbQueuePlayNow(cq *telegram.CallbackQuery) {
	idx := parseQueueIndex(cq.Data)
	if idx <= 0 {
		return
	}
	chatID := cq.Message.Chat.ID
	if _, ok := a.queue.MoveToNext(chatID, idx); !ok {
		return
	}
	a.cmdSkip(chatID, actorOf(&cq.From))
}

func (a *App) cbQueueRemove(cq *telegram.CallbackQuery) {
	idx := parseQueueIndex(cq.Data)
	if idx <= 0 {
		return
	}
	chatID := cq.Message.Chat.ID
	if removed, ok := a.queue.RemoveAt(chatID, idx); ok {
		_, _ = a.tg.SendMessage(chatID, "🗑 Removed from queue: <b>"+htmlEscape(removed.Title)+"</b>", nil)
	}
	a.cmdQueue(chatID)
}

func parseQueueIndex(callbackData string) int {
	parts := strings.SplitN(callbackData, "|", 2)
	if len(parts) != 2 {
		return 0
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return idx
}

// replaceMessage edits `processing` in place if it was actually sent, or
// falls back to sending a new message (e.g. if the initial SendMessage
// itself failed for some reason).
func (a *App) replaceMessage(chatID int64, processing *telegram.SentMessage, text string, kb telegram.InlineKeyboard) {
	if processing != nil {
		if err := a.tg.EditMessageText(chatID, processing.MessageID, text, kb); err == nil {
			return
		}
	}
	_, _ = a.tg.SendMessage(chatID, text, kb)
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(s)
}
