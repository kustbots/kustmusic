package main

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kustbots/kustmusic/internal/playrouter"
	"github.com/kustbots/kustmusic/internal/telegram"
)

// presenceTTL is how long a confirmed "the assistant is in this group"
// answer is trusted before being re-checked.
//
// Long enough that the common case — the same group playing song after
// song — costs one Telegram call every few minutes rather than one per
// play, short enough that an assistant removed from a group is noticed
// soon after. A wrong positive here is not silent: the play still runs,
// and a play that fails because the assistant is gone invalidates this
// cache immediately (see Forget), so the next attempt re-checks.
const presenceTTL = 10 * time.Minute

// Every other verdict is cached too, which is the whole point of this
// rewrite. Caching only the happy answer meant a chat the assistant
// couldn't get into re-ran the full check-invite-recheck sequence on every
// single /play — across three servers, since failover retried each one —
// and that hammering is what got the accounts FLOOD_WAIT'd on
// MessagesImportChatInvite.
const (
	// absentTTL: confirmed not in the chat and the invite didn't work.
	// Nothing will change in the next few minutes, so stop asking.
	absentTTL = 5 * time.Minute
	// unknownTTL: Telegram didn't answer. Assume present (fail open, as
	// before) but don't re-ask on every play while the API is unhappy —
	// that turned a network blip into a request storm.
	unknownTTL = 1 * time.Minute
	// Invite attempts are held off for however long the failure actually
	// warrants — see inviteBackoff. A flat cooldown was the wrong tool:
	// Telegram answered a burst with FLOOD_WAIT "please wait 3 seconds"
	// and this code turned that into a ten-minute lockout, so every retry
	// for the next ten minutes returned "couldn't get set up" without
	// even trying. The advice in that message — wait a few seconds and
	// try again — was true of Telegram and false of this bot.
	transientInviteBackoff = 30 * time.Second
	permanentInviteBackoff = 10 * time.Minute
)

// inviteBackoff works out how long to leave a chat alone after a failed
// invite, from what actually went wrong.
//
// Telegram is specific when it rate-limits — FLOOD_WAIT_X names the number
// of seconds — so that number is used directly rather than guessed at.
// Failures nothing will fix (no permission, not a group) get a long hold,
// because retrying them is pure waste.
func inviteBackoff(err error) time.Duration {
	if err == nil {
		return 0
	}
	msg := strings.ToLower(err.Error())

	if secs, ok := parseFloodWait(msg); ok {
		// A small margin on top: the clock Telegram is measuring against
		// isn't this one.
		return time.Duration(secs)*time.Second + 2*time.Second
	}

	for _, permanent := range []string{
		"not enough rights", "chat_admin_required",
		"can't invite members to a private chat",
		"user_banned_in_channel", "channels_too_much",
		"chat not found",
	} {
		if strings.Contains(msg, permanent) {
			return permanentInviteBackoff
		}
	}
	return transientInviteBackoff
}

// parseFloodWait pulls the seconds out of "[FLOOD_WAIT_X] Please wait 3
// seconds before repeating the action".
func parseFloodWait(msg string) (int, bool) {
	if !strings.Contains(msg, "flood_wait") {
		return 0, false
	}
	m := floodWaitSeconds.FindStringSubmatch(msg)
	if len(m) != 2 {
		return 0, false
	}
	secs, err := strconv.Atoi(m[1])
	if err != nil || secs <= 0 {
		return 0, false
	}
	if secs > 3600 {
		secs = 3600 // don't take an absurd value at face value
	}
	return secs, true
}

var floodWaitSeconds = regexp.MustCompile(`wait (\d+) second`)

// assistantGuard answers "is this server's assistant actually in this
// group?" before the bot asks that server to play anything.
//
// This is the fix for the worst failure mode this bot had: with the
// assistant absent, play-api could still report success — its native call
// engine happily accepts a new stream source for a chat it believes it is
// still joined to — so the bot posted a full now-playing card, with a
// progress bar ticking along, for a song nobody could hear. Users had no
// way to tell that from a real playback, and no reason to suspect the
// assistant needed adding.
//
// Checking membership is cheap (one getChatMember, which the bot can make
// because it is in the group itself) but not free, so answers are cached
// per chat+assistant pair.
type assistantGuard struct {
	tg     *telegram.Client
	router *playrouter.Router
	log    *slog.Logger

	mu          sync.Mutex
	verdicts    map[string]verdict  // "chatID:assistantID" -> what we last worked out
	inviteAfter map[int64]time.Time // chatID -> earliest time another invite is worth trying
	accountNext map[int64]time.Time // assistant user id -> earliest time IT may invite anywhere
	// globalInvites is a sliding window of every invite attempt the bot has
	// made recently, whatever chat or account it was for. See
	// globalInviteBudget.
	globalInvites []time.Time
	// accountNames is every assistant account the bot has seen, by id.
	// Needed because when all of them are rate-limited the only useful
	// thing to tell a group is *which accounts to add by hand* — a manual
	// add bypasses the limit entirely, since Telegram restricts an account
	// inviting itself in, not somebody else adding it.
	accountNames map[int64]string
}

// verdict is a cached answer about one assistant in one chat.
type verdict struct {
	present bool
	expires time.Time
}

func newAssistantGuard(tg *telegram.Client, router *playrouter.Router, log *slog.Logger) *assistantGuard {
	return &assistantGuard{
		tg:           tg,
		router:       router,
		log:          log,
		verdicts:     make(map[string]verdict),
		inviteAfter:  make(map[int64]time.Time),
		accountNext:  make(map[int64]time.Time),
		accountNames: make(map[int64]string),
	}
}

// noteAccountName remembers an assistant's @username so FloodSummary can
// name the accounts a group should add by hand.
func (g *assistantGuard) noteAccountName(id int64, username string) {
	if id == 0 || username == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.accountNames[id] = username
}

// FloodSummary reports whether *every* assistant account is currently
// rate-limited, when the first one frees up, and what those accounts are
// called.
//
// The distinction matters for what the chat gets told. One limited account
// is routine — the caller just moves to a server backed by a different
// one. All of them limited at once is a different situation: no amount of
// retrying will help, and repeating /play actively makes it worse, because
// each attempt is another invite that Telegram counts against the account.
// So the chat needs a wait time and the option to skip the wait by adding
// an account manually, not "try again in a few seconds".
func (g *assistantGuard) FloodSummary() (until time.Time, usernames []string, allLimited bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.accountNames) == 0 {
		return time.Time{}, nil, false
	}

	now := time.Now()
	var soonest time.Time
	every := true
	for id, name := range g.accountNames {
		usernames = append(usernames, name)
		next, seen := g.accountNext[id]
		if !seen || !now.Before(next) {
			every = false // this one could be used right now
			continue
		}
		if soonest.IsZero() || next.Before(soonest) {
			soonest = next
		}
	}
	sort.Strings(usernames)
	if !every {
		return time.Time{}, usernames, false
	}
	return soonest, usernames, true
}

// assistantMissingError means the account is confirmed absent from the
// group and could not be invited back. It carries the assistant's identity
// so the bot can tell people exactly which account to add — an error that
// says "add @thisaccount" is actionable, "playback failed" is not.
type assistantMissingError struct {
	Username string
	ID       int64
	// Cause is why the automatic invite didn't work, when it was tried.
	// Almost always a missing bot permission — the message shown in the
	// chat depends on it, because "make me an admin" and "try again" are
	// very different instructions.
	Cause error
}

func (e *assistantMissingError) Error() string {
	who := e.Username
	if who == "" {
		who = fmt.Sprintf("id %d", e.ID)
	}
	msg := "couldn't add assistant account @" + who + " to this group"
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *assistantMissingError) Unwrap() error { return e.Cause }

// NeedsInviteRights reports whether the invite failed because the bot
// isn't allowed to add members — the one case the group has to fix, and
// the only one worth showing a message for.
func (e *assistantMissingError) NeedsInviteRights() bool {
	if e.Cause == nil {
		return false
	}
	cause := strings.ToLower(e.Cause.Error())
	for _, marker := range []string{
		"not enough rights",
		"chat_admin_required",
		"need administrator rights",
		"can't remove chat owner",
		"method is available for supergroup",
	} {
		if strings.Contains(cause, marker) {
			return true
		}
	}
	return false
}

// Mention renders the assistant for display, preferring its @username.
func (e *assistantMissingError) Mention() string {
	if e.Username != "" {
		return "@" + e.Username
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">the assistant account</a>`, e.ID)
}

// Ensure reports whether server's assistant can stream into chatID,
// re-inviting it if it has been removed. A nil return means the account is
// present and the play is worth attempting.
//
// It fails closed only on a definite answer. A transient Telegram or
// play-api hiccup returns nil with a logged warning rather than blocking a
// group that is probably fine — a check that guesses "missing" on a
// network blip would break more playbacks than the bug it guards against.
func (g *assistantGuard) Ensure(chatID int64, server string) error {
	info, err := g.router.Assistant(server)
	if err != nil {
		// The server can't say who it is, which in practice means its
		// assistant isn't connected. Treat that as this server being
		// unusable so the caller moves the chat elsewhere; it's the same
		// condition /play itself would reject with a 503.
		return fmt.Errorf("no assistant available on this server: %w", err)
	}
	g.noteAccountName(info.ID, info.Username)

	key := fmt.Sprintf("%d:%d", chatID, info.ID)
	if v, ok := g.cached(key); ok {
		if v.present {
			return nil
		}
		// Known absent and recently unfixable. Answer from cache rather
		// than repeating a sequence that just failed.
		return &assistantMissingError{Username: info.Username, ID: info.ID}
	}

	switch g.check(chatID, info.ID) {
	case presenceIn:
		g.remember(key, true, presenceTTL)
		return nil
	case presenceUnknown:
		// Telegram didn't answer. Fail open as before, but hold that
		// decision briefly so a flaky API doesn't mean a fresh round of
		// calls on every single play.
		g.remember(key, true, unknownTTL)
		return nil
	}

	// Confirmed absent. The idle-group sweeper leaves quiet groups on
	// purpose, so this is the ordinary path back in — the assistant gets
	// added automatically and nobody in the chat is asked to do anything.
	//
	// But joining by invite link is the operation Telegram rate-limits
	// hardest, so it gets at most one attempt per chat per cooldown, no
	// matter how many servers ask or how many people type /play.
	if !g.mayInvite(chatID) {
		g.remember(key, false, absentTTL)
		return &assistantMissingError{Username: info.Username, ID: info.ID}
	}

	// The account this server uses may be cooling down from a flood wait
	// it earned in some other group. Don't queue behind it — fail this
	// server fast so the caller moves to a server backed by a different
	// account, which has its own budget.
	if ok, wait := g.reserveInvite(info.ID); !ok {
		g.remember(key, false, wait)
		return &assistantMissingError{Username: info.Username, ID: info.ID}
	}

	// Last gate: the bot-wide budget. Checked after the chat and account
	// limits so a request already destined to be refused doesn't spend a
	// slot from the shared pool on its way out.
	if ok, retry := g.reserveGlobalInvite(); !ok {
		g.log.Warn("bot-wide invite budget spent, holding this chat off",
			"chat_id", chatID, "retry_in", retry.Round(time.Second))
		g.holdOffInvites(chatID, retry)
		g.remember(key, false, retry)
		return &assistantMissingError{Username: info.Username, ID: info.ID}
	}

	if err := g.invite(chatID, server, info.ID); err != nil {
		g.noteAccountFlood(info.ID, err)
		// Hold off for exactly as long as this particular failure is
		// worth waiting out, and let the cached verdict expire at the same
		// moment — otherwise the chat stays "broken" in memory after the
		// reason to think so has passed.
		backoff := inviteBackoff(err)
		g.holdOffInvites(chatID, backoff)
		g.remember(key, false, backoff)
		return &assistantMissingError{Username: info.Username, ID: info.ID, Cause: err}
	}

	// Telegram takes a moment to reflect a fresh join, so a single
	// immediate re-check can read as "still not here" for a join that
	// actually worked, and send the chat a pointless error.
	for attempt := 0; attempt < joinConfirmAttempts; attempt++ {
		if g.check(chatID, info.ID) != presenceOut {
			g.log.Info("added assistant to chat automatically", "chat_id", chatID, "assistant", info.Username)
			g.remember(key, true, presenceTTL)
			return nil
		}
		time.Sleep(joinConfirmDelay)
	}

	// The join reported success but the membership never showed up. Give
	// it a short hold rather than the full absent window — this is more
	// often Telegram lagging than a real refusal.
	g.holdOffInvites(chatID, transientInviteBackoff)
	g.remember(key, false, transientInviteBackoff)
	return &assistantMissingError{Username: info.Username, ID: info.ID}
}

// joinConfirmAttempts/joinConfirmDelay bound how long Ensure waits for a
// join to show up in getChatMember before deciding it didn't work.
const (
	joinConfirmAttempts = 3
	joinConfirmDelay    = 700 * time.Millisecond
)

// cached returns the stored verdict for key if it hasn't expired.
func (g *assistantGuard) cached(key string) (verdict, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	v, ok := g.verdicts[key]
	if !ok || time.Now().After(v.expires) {
		return verdict{}, false
	}
	return v, true
}

// minInviteSpacing is how far apart one account's invites are spaced.
//
// Telegram rate-limits MessagesImportChatInvite per *account*, not per
// chat — which is the thing the first version of this throttle missed
// entirely. Five different groups each had their own per-chat budget and
// all of them spent it on the same account inside four seconds; Telegram
// answered every one with FLOOD_WAIT and then escalated the penalty from
// 3 seconds to 239. Spacing them out at the account is the only place the
// limit is actually visible.
const minInviteSpacing = 15 * time.Second

// A ceiling on invite attempts across the whole bot, whatever chat or
// account they belong to.
//
// The other limits each bound one dimension: a chat can't retry quickly
// (mayInvite), and one account can't be asked twice in a row
// (minInviteSpacing). Neither bounds the total. Many chats, each perfectly
// within its own budget, each landing on a different account, still add up
// to a burst — and Telegram's penalty is a function of that aggregate. It
// was measured escalating 8s -> 55s -> 71s -> 117s -> 214s -> 233s ->
// 1319s as attempts piled on, which is what took every account out at once
// and left groups unable to play anything.
//
// Eight per minute is comfortably above what normal use produces (a group
// resuming after the idle sweeper needs exactly one) and far below the
// rate that earns a penalty, so this only ever bites during the kind of
// storm it exists to stop.
const (
	globalInviteWindow = time.Minute
	globalInviteBudget = 8
)

// reserveGlobalInvite claims one slot from the bot-wide invite budget.
//
// Claimed rather than merely checked, and claimed last of the three gates,
// so a request that is going to be refused by an earlier limit doesn't
// spend from the shared pool on its way out.
func (g *assistantGuard) reserveGlobalInvite() (ok bool, retryAfter time.Duration) {
	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	kept := g.globalInvites[:0]
	for _, t := range g.globalInvites {
		if now.Sub(t) < globalInviteWindow {
			kept = append(kept, t)
		}
	}
	g.globalInvites = kept

	if len(kept) >= globalInviteBudget {
		// The oldest attempt in the window is the one whose expiry frees a
		// slot, so that is how long there is to wait.
		return false, globalInviteWindow - now.Sub(kept[0])
	}
	g.globalInvites = append(kept, now)
	return true, 0
}

// reserveInvite claims the next invite slot for accountID, or reports that
// the account is still cooling down.
//
// The slot is claimed before the attempt, not after, so several chats
// resuming at once can't all pass the check and then pile onto the same
// account — which is exactly what happened during a restart, when queue
// restore woke several groups simultaneously.
func (g *assistantGuard) reserveInvite(accountID int64) (ok bool, wait time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if next, seen := g.accountNext[accountID]; seen && now.Before(next) {
		return false, next.Sub(now)
	}
	g.accountNext[accountID] = now.Add(minInviteSpacing)
	return true, 0
}

// noteAccountFlood applies a FLOOD_WAIT to the whole account.
//
// Only flood waits go here. "Not enough rights" or "can't invite to a
// private chat" are facts about one chat — punishing the account for them
// would stop it serving every other group.
func (g *assistantGuard) noteAccountFlood(accountID int64, err error) {
	if err == nil {
		return
	}
	secs, isFlood := parseFloodWait(strings.ToLower(err.Error()))
	if !isFlood {
		return
	}
	until := time.Now().Add(time.Duration(secs)*time.Second + 2*time.Second)

	g.mu.Lock()
	defer g.mu.Unlock()
	if cur, seen := g.accountNext[accountID]; !seen || until.After(cur) {
		g.accountNext[accountID] = until
		g.log.Warn("assistant account is flood-limited, pausing its invites everywhere",
			"assistant_id", accountID, "for", time.Duration(secs)*time.Second)
	}
}

// mayInvite reports whether chatID's hold-off has elapsed.
func (g *assistantGuard) mayInvite(chatID int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.inviteAfter[chatID]
	return !ok || time.Now().After(until)
}

// holdOffInvites parks further invite attempts for this chat.
func (g *assistantGuard) holdOffInvites(chatID int64, d time.Duration) {
	g.mu.Lock()
	g.inviteAfter[chatID] = time.Now().Add(d)
	g.mu.Unlock()
}

type presence int

const (
	presenceIn presence = iota
	presenceOut
	presenceUnknown
)

// check asks Telegram whether userID is in chatID.
func (g *assistantGuard) check(chatID, userID int64) presence {
	member, err := g.tg.GetChatMember(chatID, userID)
	if err != nil {
		// Telegram reports "never been here" as an error rather than a
		// status for some chat types, so those descriptions count as a
		// definite absence. Anything else is a genuine unknown.
		desc := strings.ToLower(err.Error())
		for _, definite := range []string{
			"participant_id_invalid",
			"user not found",
			"user_not_participant",
			"member not found",
		} {
			if strings.Contains(desc, definite) {
				return presenceOut
			}
		}
		g.log.Warn("couldn't check assistant membership", "chat_id", chatID, "user_id", userID, "err", err)
		return presenceUnknown
	}

	switch member.Status {
	case "left", "kicked":
		return presenceOut
	default: // creator, administrator, member, restricted
		return presenceIn
	}
}

// invite adds server's assistant to chatID without involving anyone in the
// chat: clear a stale ban if there is one, mint a single-use invite link,
// and have the assistant join through it.
//
// The link is created, not exported. Bot API's exportChatInviteLink — what
// this used to call — revokes the group's existing primary link as a side
// effect, so letting the assistant back in silently invalidated whatever
// link the group had been handing out to its own members.
func (g *assistantGuard) invite(chatID int64, server string, assistantID int64) error {
	// A previously-banned assistant can't accept any link. unbanChatMember
	// is only_if_banned, so this is a no-op in the normal case.
	if err := g.tg.UnbanChatMember(chatID, assistantID); err != nil {
		g.log.Debug("assistant unban skipped", "chat_id", chatID, "err", err)
	}

	// Two attempts, because "Invalid or expired invite link" turned out to
	// be a thing that happens to a link created seconds earlier. Whatever
	// the cause on Telegram's side, a second freshly-minted link usually
	// works, and one extra call beats telling a group to come back later.
	var lastErr error
	for attempt := 0; attempt < inviteAttempts; attempt++ {
		link, err := g.tg.CreateChatInviteLink(chatID, time.Now().Add(inviteLinkTTL).Unix())
		if err != nil {
			g.log.Warn("couldn't create an invite link for the assistant", "chat_id", chatID, "err", err)
			return fmt.Errorf("invite link: %w", err)
		}

		joinErr := g.router.JoinChat(server, link)
		if joinErr == nil {
			return nil
		}
		lastErr = fmt.Errorf("join: %w", joinErr)

		// Telegram states exactly how long to hold off, and for a join it
		// is routinely two or three seconds. Reporting that as a failed
		// join was actively harmful: the caller reads it as "this server's
		// assistant is unusable" and fails over, which sends a *different*
		// account at the same group, which earns its own flood wait — so a
		// three-second pause turned into a lap of the whole fleet burning
		// every account's invite quota, and the user saw "couldn't get set
		// up". Waiting it out here is both faster and cheaper.
		//
		// Long waits are a genuinely different situation: those really do
		// mean "this account is spent, use another one", so they are still
		// returned to the caller to fail over on. Either way the flood is
		// recorded against the account so its existing spacing applies
		// everywhere else too.
		if secs, isFlood := parseFloodWait(strings.ToLower(joinErr.Error())); isFlood {
			g.noteAccountFlood(assistantID, joinErr)
			if secs <= maxInlineFloodWait && attempt < inviteAttempts-1 {
				g.log.Warn("join flood-limited, waiting it out instead of failing over",
					"chat_id", chatID, "assistant_id", assistantID, "wait_seconds", secs)
				time.Sleep(time.Duration(secs)*time.Second + 500*time.Millisecond)
				continue
			}
			g.log.Warn("assistant join flood-limited for too long, letting the caller try another server",
				"chat_id", chatID, "assistant_id", assistantID, "wait_seconds", secs)
			return lastErr
		}

		if !isBadInviteLink(joinErr) {
			g.log.Warn("assistant join failed", "chat_id", chatID, "err", joinErr)
			return lastErr
		}
		g.log.Warn("invite link rejected, minting a fresh one", "chat_id", chatID, "attempt", attempt+1, "err", joinErr)
	}
	return lastErr
}

// inviteAttempts is how many times invite() will try. Three, not two: one
// for the original link, one for the "invalid or expired link" re-mint that
// prompted the retry loop in the first place, and one spare so a short
// flood wait doesn't consume the re-mint's turn.
const inviteAttempts = 3

// maxInlineFloodWait is the longest FLOOD_WAIT worth sitting out inside a
// live /play request. A few seconds beats a fleet-wide failover; beyond
// that the user is better served by another account picking it up, and the
// request shouldn't be held open waiting.
const maxInlineFloodWait = 6

// isBadInviteLink reports whether the join failed because of the link
// itself rather than anything about the chat or the account — the one case
// where making a new link and trying again is worth a round trip.
func isBadInviteLink(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid or expired invite link") ||
		strings.Contains(msg, "invite_hash_expired") ||
		strings.Contains(msg, "invite_hash_invalid")
}

// inviteLinkTTL keeps the generated link short-lived — it exists for one
// join by one account and shouldn't outlive that.
const inviteLinkTTL = 10 * time.Minute

func (g *assistantGuard) remember(key string, present bool, ttl time.Duration) {
	g.mu.Lock()
	g.verdicts[key] = verdict{present: present, expires: time.Now().Add(ttl)}
	g.mu.Unlock()
}

// Forget drops every cached answer for chatID, so the next play re-checks
// from scratch. Called when a play fails in a way that suggests the
// assistant left after the cache said otherwise.
func (g *assistantGuard) Forget(chatID int64) {
	prefix := fmt.Sprintf("%d:", chatID)
	g.mu.Lock()
	for key := range g.verdicts {
		if strings.HasPrefix(key, prefix) {
			delete(g.verdicts, key)
		}
	}
	g.mu.Unlock()
}
