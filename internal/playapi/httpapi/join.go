package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Joining a group by invite link is the single operation Telegram
// rate-limits hardest, and the penalty escalates with how many attempts an
// account has made recently — measured in production climbing 8s → 55s →
// 71s → 117s → 214s → 233s → 1319s as attempts piled up, until every
// account in the fleet was locked out at once.
//
// The caller has its own limits, but they only protect against the caller
// misbehaving. This process is the one thing that maps one-to-one onto a
// Telegram account, so it is the only place a limit can be enforced no
// matter who is asking or how many callers there are. Refusing here costs
// a failover to another server; not refusing costs the account.
const (
	// joinMinSpacing is the floor between two joins by this assistant.
	joinMinSpacing = 20 * time.Second
	// joinBudget/joinWindow bound the slower burn as well, so a steady
	// drip spaced just over the floor still can't accumulate into a
	// penalty over the course of a few minutes.
	joinBudget = 5
	joinWindow = 10 * time.Minute
)

// joinLimiter throttles this assistant's joins.
type joinLimiter struct {
	mu     sync.Mutex
	last   time.Time
	recent []time.Time
}

// Allow claims a join slot, reporting how long to wait when it can't.
//
// Attempts are counted, not successes: Telegram's limit is on asking, so a
// refused or failed join costs exactly as much as one that worked, and
// counting only successes would leave the real rate unmeasured.
func (l *joinLimiter) Allow() (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.last.IsZero() {
		if since := now.Sub(l.last); since < joinMinSpacing {
			return false, joinMinSpacing - since
		}
	}

	kept := l.recent[:0]
	for _, t := range l.recent {
		if now.Sub(t) < joinWindow {
			kept = append(kept, t)
		}
	}
	l.recent = kept

	if len(kept) >= joinBudget {
		// The oldest attempt still in the window is what frees the next
		// slot when it ages out.
		return false, joinWindow - now.Sub(kept[0])
	}

	l.recent = append(kept, now)
	l.last = now
	return true, 0
}

// normalizeJoinTarget puts a chat reference into a form gogram's
// JoinChannel actually accepts.
//
// This used to strip the "https://t.me/" prefix off links and the "@" off
// usernames — exactly the two markers gogram matches on. Its join regexes
// are anchored on them (TgJoinRe wants ".../+hash", UsernameRe wants "@" or
// a t.me link), so a stripped value matched neither and every join came
// back "invalid channel or chat". That silently broke the whole automatic
// re-invite path: the bot would export an invite link, hand it here, get a
// failure, and fall back to telling the group to add the assistant by
// hand — for a join that never had a chance of working.
func normalizeJoinTarget(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i] // invite links sometimes carry tracking params
	}
	s = strings.TrimSuffix(s, "/")

	switch {
	case s == "":
		return ""
	case strings.HasPrefix(s, "+"), strings.HasPrefix(s, "joinchat/"):
		// A bare invite hash — put the link back together.
		return "https://t.me/" + s
	case strings.Contains(s, "t.me/"), strings.HasPrefix(s, "@"):
		return s // already in a form gogram parses
	default:
		return "@" + s // bare username
	}
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	chat := normalizeJoinTarget(r.URL.Query().Get("chat"))
	if chat == "" {
		writeError(w, http.StatusBadRequest, "Missing chat parameter")
		return
	}

	// Slow down before Telegram makes us. A refusal here is cheap — the
	// caller fails over to a server backed by a different account, which
	// has its own budget — whereas letting the attempt through when this
	// account is already going too fast is what earns the escalating
	// penalty that takes the whole fleet down.
	if ok, retry := s.joins.Allow(); !ok {
		retry = retry.Round(time.Second)
		s.Log.Warn("join refused by local rate limit", "chat", chat, "retry_in", retry)
		writeError(w, http.StatusTooManyRequests,
			fmt.Sprintf("this assistant is joining too often; it may join again in %s", retry))
		return
	}

	if err := s.Voice.Join(r.Context(), chat); err != nil {
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "USER_ALREADY_PARTICIPANT"):
			// Already in — the caller wanted the assistant present, and it
			// is. Reporting this as a failure made the bot treat a
			// perfectly good chat as unreachable.
			writeJSON(w, http.StatusOK, map[string]any{"message": "Already a member of " + chat + "."})
		case strings.Contains(errStr, "USERNAME_INVALID"):
			writeError(w, http.StatusBadRequest, "Invalid username or link.")
		case strings.Contains(errStr, "INVITE_HASH_INVALID"), strings.Contains(errStr, "INVITE_HASH_EXPIRED"):
			writeError(w, http.StatusBadRequest, "Invalid or expired invite link.")
		case strings.Contains(errStr, "USER_BANNED_IN_CHANNEL"), strings.Contains(errStr, "CHANNEL_PRIVATE"):
			writeError(w, http.StatusForbidden, "The assistant is banned from that chat.")
		case strings.Contains(errStr, "CHANNELS_TOO_MUCH"):
			writeError(w, http.StatusServiceUnavailable, "CHANNELS_TOO_MUCH: this assistant is in too many chats to join another.")
		default:
			writeError(w, http.StatusInternalServerError, errStr)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"message": "Successfully Joined: " + chat})
}
