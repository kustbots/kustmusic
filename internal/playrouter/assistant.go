package playrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AssistantInfo is who a play-api server streams through — the Telegram
// user account it logs in as. The bot needs the id to ask Telegram whether
// that account is actually in a group, and the username to tell people
// which account to add when it isn't.
type AssistantInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Mention  string `json:"mention"`
}

// assistantTTL bounds how long a server's identity stays cached. The
// account behind a server only changes on redeploy, so this is generous;
// it exists so a re-sessioned server is picked up eventually without the
// bot needing a restart.
const assistantTTL = 30 * time.Minute

type assistantEntry struct {
	info    AssistantInfo
	fetched time.Time
}

// Assistant returns which account server streams through, cached for
// assistantTTL.
//
// This is on the hot path — the bot checks assistant membership before
// every play — so it must not cost a network round-trip each time.
// Failures are deliberately not cached: a 503 here means the server's
// assistant is still connecting, and the next attempt should re-ask rather
// than inherit a stale failure.
func (r *Router) Assistant(server string) (AssistantInfo, error) {
	r.mu.Lock()
	if e, ok := r.assistants[server]; ok && time.Since(e.fetched) < assistantTTL {
		r.mu.Unlock()
		return e.info, nil
	}
	r.mu.Unlock()

	resp, err := r.http.Get(server + "/assistant")
	if err != nil {
		return AssistantInfo{}, fmt.Errorf("playrouter: assistant lookup failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var body struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &body)
		if body.Error == "" {
			body.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return AssistantInfo{}, fmt.Errorf("playrouter: assistant unavailable: %s", body.Error)
	}

	var info AssistantInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return AssistantInfo{}, fmt.Errorf("playrouter: assistant lookup returned non-JSON")
	}
	if info.ID == 0 {
		return AssistantInfo{}, fmt.Errorf("playrouter: assistant lookup returned no user id")
	}

	r.mu.Lock()
	r.assistants[server] = assistantEntry{info: info, fetched: time.Now()}
	r.mu.Unlock()
	return info, nil
}

// ForgetAssistant drops server's cached identity, so the next lookup
// re-resolves it. Used when a server starts failing in a way that suggests
// its assistant changed or went away.
func (r *Router) ForgetAssistant(server string) {
	r.mu.Lock()
	delete(r.assistants, server)
	r.mu.Unlock()
}

// Servers returns the audio server pool, so callers can reason about the
// whole fleet (e.g. "is this assistant in the group on any server?").
func (r *Router) Servers() []string {
	return append([]string{}, r.servers...)
}

// ServersSharingAssistant lists the servers already known to stream through
// account id.
//
// The fleet does not run one account per server — some pairs share one, on
// purpose. That makes a failover that only avoids the failed *server*
// wasteful: if a chat can't play because @someaccount isn't in it, moving
// to the other server running @someaccount fails for exactly the same
// reason, burning one of the few attempts a request gets. Callers use this
// to skip the whole account, not just the one server.
//
// Only cached identities are consulted — this is called on the play path
// and must not add round-trips. That's enough in practice: each attempt
// caches the server it tried, so the set converges as the request walks
// the fleet.
func (r *Router) ServersSharingAssistant(id int64) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for server, e := range r.assistants {
		if e.info.ID == id {
			out = append(out, server)
		}
	}
	return out
}
