// Package playrouter picks which play-api server handles a chat and talks
// to it over HTTP — a direct port of main-music-rx's select_api_server /
// execute_playback_api_request (Python), including the premium-server
// override and the separate video/vplay server pool.
package playrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Router struct {
	botID        string // sent with each play request so the engine can tell which bot asked
	servers      []string
	videoServers []string
	http         *http.Client

	mu          sync.Mutex
	sticky      map[int64]string // chatID -> audio server base URL, sticky once assigned
	videoSticky map[int64]string
	rrIndex     int
	vrrIndex    int
	loads       map[string]int            // server -> last-known active_calls count
	unhealthy   map[string]bool           // server -> last probe failed, or it said it can't stream
	assistants  map[string]assistantEntry // server -> which account it streams through
}

// SetBotID records the bot's own Telegram id, learned at startup.
func (r *Router) SetBotID(id string) { r.botID = id }

func New(servers []string) *Router {
	return NewWithVideo(servers, nil)
}

// NewWithVideo also configures the video (/vplay) server pool, matching
// main-music-rx's separate vplay_api_servers list and its own
// least-loaded/round-robin picker.
func NewWithVideo(servers, videoServers []string) *Router {
	return &Router{
		servers:      servers,
		videoServers: videoServers,
		// Must exceed play-api's own PlayRequestBudget (150s default) —
		// play-api now keep-alive-pings /play while it waits on a fresh
		// join's WebRTC handshake (up to CONNECT_WAIT_SECONDS, 90s
		// default), so a client-side timeout shorter than that gave up
		// and errored out on exactly the requests that would otherwise
		// have succeeded, reading as "lag"/flakiness.
		http:        &http.Client{Timeout: 170 * time.Second},
		sticky:      make(map[int64]string),
		videoSticky: make(map[int64]string),
		loads:       make(map[string]int),
		unhealthy:   make(map[string]bool),
		assistants:  make(map[string]assistantEntry),
	}
}

// ServerFor returns the audio server assigned to chatID, picking a fresh
// least-loaded one (tie-broken by round robin) if this chat doesn't have
// one yet. Sticky once assigned, matching the Python bot's reuse_cache
// behavior, so a chat's queue/skip/stop calls always land on the same
// server as its original /play. isPremium is accepted but unused: this build
// has one playback engine, so there is no separate premium server.
func (r *Router) ServerFor(chatID int64, isPremium bool) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sticky[chatID]; ok {
		return s
	}
	s := r.pickLeastLoadedLocked(r.servers, &r.rrIndex)
	r.sticky[chatID] = s
	return s
}

// Reassign drops chatID's current server and picks a different one, so a
// failed play can be retried somewhere else instead of giving up. Returns
// "" when there's no alternative left to try.
//
// Without this a single unhealthy server (dead assistant session, a bad
// upstream moment) meant every play in every chat pinned to it failed —
// even though four other servers were sitting idle and able to serve it.
func (r *Router) Reassign(chatID int64, exclude map[string]bool) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var pool []string
	for _, s := range r.servers {
		if !exclude[s] {
			pool = append(pool, s)
		}
	}
	if len(pool) == 0 {
		return ""
	}
	s := r.pickLeastLoadedLocked(pool, &r.rrIndex)
	r.sticky[chatID] = s
	return s
}

// DisplayIndex returns server's 1-based position in the audio server pool
// (matching main-music-rx's display_server, e.g. server_id = idx + 1), or 0
// if server isn't in the pool (e.g. it's the premium override).
func (r *Router) DisplayIndex(server string) int {
	for i, s := range r.servers {
		if s == server {
			return i + 1
		}
	}
	return 0
}

// VideoServerFor is ServerFor's counterpart for /vplay, using the separate
// video server pool.
func (r *Router) VideoServerFor(chatID int64) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.videoSticky[chatID]; ok {
		return s
	}
	s := r.pickLeastLoadedLocked(r.videoServers, &r.vrrIndex)
	r.videoSticky[chatID] = s
	return s
}

func (r *Router) pickLeastLoadedLocked(pool []string, rrIndex *int) string {
	if len(pool) == 0 {
		return ""
	}
	// Health first, load second. A server whose assistant is dead hosts no
	// calls, so it reports — or, when the probe itself fails, defaults to —
	// zero active calls, which made it permanently the *emptiest* and
	// therefore most attractive server in the fleet. Every new chat got
	// black-holed onto exactly the servers that could not play anything,
	// and because assignment is sticky it stayed there; with two dead
	// servers, failover would even hand a chat straight from one to the
	// other. Filtering first is what stops "least loaded" from meaning
	// "most broken".
	//
	// If nothing is currently known-healthy the full pool is still used
	// rather than returning "": a stale or failed probe round must not be
	// able to wedge playback fleet-wide, and /play's own error path already
	// fails over.
	healthy := make([]string, 0, len(pool))
	for _, s := range pool {
		if !r.unhealthy[s] {
			healthy = append(healthy, s)
		}
	}
	if len(healthy) > 0 {
		pool = healthy
	}

	minLoad := -1
	var candidates []string
	for _, s := range pool {
		load := r.loads[s]
		if minLoad == -1 || load < minLoad {
			minLoad = load
			candidates = []string{s}
		} else if load == minLoad {
			candidates = append(candidates, s)
		}
	}
	chosen := candidates[*rrIndex%len(candidates)]
	*rrIndex++
	return chosen
}

// RefreshLoads polls every server's /active_calls and updates the
// least-loaded picker's view of real load. Meant to run on a timer in the
// background, same as the Python bot's server_load_poll_loop.
func (r *Router) RefreshLoads() {
	for _, s := range append(append([]string{}, r.servers...), r.videoServers...) {
		count, ok, reason := r.probeLoad(s)
		r.mu.Lock()
		was := r.unhealthy[s]
		r.loads[s] = count
		r.unhealthy[s] = !ok
		r.mu.Unlock()

		// Log the edges, not every poll: this runs on a timer, and a server
		// that stays dead for hours would otherwise bury everything else in
		// the log. The transition is the part worth seeing.
		if was == ok { // health just changed
			if ok {
				slog.Info("play server is healthy again, routing restored", "server", s)
			} else {
				slog.Warn("play server says it can't stream — routing around it", "server", s, "reason", reason)
			}
		}
	}
}

// probeLoad returns server's active-call count and whether that server is
// actually in a state to take work. The second value is the important one:
// an unreachable server, a non-JSON answer, and a server whose assistant
// session is dead all look identical to a plain count — zero — and zero is
// the number a least-loaded picker likes best. Reporting health separately
// is what keeps pickLeastLoadedLocked from treating "broken" as "free".
func (r *Router) probeLoad(server string) (int, bool, string) {
	resp, err := r.http.Get(server + "/active_calls")
	if err != nil {
		return 0, false, "unreachable: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Sprintf("health probe returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Count int `json:"count"`
		// Pointer, not bool: an older play-api that predates this field
		// omits it entirely, and a missing field must mean "assume usable"
		// rather than silently marking that whole server unhealthy.
		Ready  *bool  `json:"ready"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, false, "unreadable health response: " + err.Error()
	}
	if body.Ready != nil && !*body.Ready {
		reason := body.Reason
		if reason == "" {
			reason = "server reported itself not ready"
		}
		return body.Count, false, reason
	}
	return body.Count, true, ""
}

// PlayResult mirrors what callers need from a /play response.
type PlayResult struct {
	OK    bool
	Error string
	Raw   string
}

// Play calls server's /play endpoint for chatID/watchURL, matching
// main-music-rx's execute_playback_api_request contract exactly (same
// query params, same "bot" ID). Checks the JSON body's "error" field
// regardless of HTTP status — play-api's slow path (see its play.go) can
// return a real error inside a 200 once it's started sending keep-alive
// pings, so status-code-only error detection would miss those.
// durationSeconds is how long the track actually runs; 0 when the search
// API didn't give a duration (a live stream, say). It's forwarded so
// play-api can reject a download that came back shorter than the real song
// — the download endpoint streams chunked with no Content-Length, so a
// fetch cut short is indistinguishable from a complete one until you look
// at the audio itself, and the bot is the side that knows the true length.
func (r *Router) Play(server string, chatID int64, watchURL string, durationSeconds int) (*PlayResult, error) {
	endpoint := fmt.Sprintf("%s/play?chatid=%d&url=%s&api=1&bot=%s",
		server, chatID, url.QueryEscape(watchURL), r.botID)
	if durationSeconds > 0 {
		endpoint += fmt.Sprintf("&duration=%d", durationSeconds)
	}

	resp, err := r.http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("playrouter: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("playrouter: read response: %w", err)
	}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if jsonErr := json.Unmarshal(raw, &body); jsonErr != nil {
		return nil, fmt.Errorf("playrouter: non-JSON response (status %d): %s", resp.StatusCode, raw)
	}
	if body.Error != "" {
		return &PlayResult{OK: false, Error: body.Error, Raw: string(raw)}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return &PlayResult{OK: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode), Raw: string(raw)}, nil
	}
	return &PlayResult{OK: true, Raw: string(raw)}, nil
}

// JoinChat asks server's assistant to join chatOrInvite (an invite link or
// @username) — used to re-invite the assistant to a group it left, so a
// chat that went quiet long enough to be swept can start playing again
// without anyone re-adding the assistant by hand.
func (r *Router) JoinChat(server, chatOrInvite string) error {
	endpoint := fmt.Sprintf("%s/join?chat=%s", server, url.QueryEscape(chatOrInvite))
	resp, err := r.http.Get(endpoint)
	if err != nil {
		return fmt.Errorf("playrouter: join failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("playrouter: join returned HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

// IsSuperseded reports whether a /play error just means a newer /play for
// the same chat took over — play-api's ErrStale. It's a race outcome, not
// a failure, and shouldn't be shown to users as one.
func IsSuperseded(errMsg string) bool {
	return strings.Contains(errMsg, "superseded")
}

// NeedsAssistantRejoin reports whether a /play error looks like "the
// assistant isn't in that chat any more" — the specific failure the
// idle-group sweeper creates, and the one worth retrying after a re-join.
// Matches the same error strings main-music-rx's own retry path keyed off.
func NeedsAssistantRejoin(errMsg string) bool {
	for _, marker := range []string{
		"Peer id invalid",
		"CHANNEL_INVALID",
		"no active group call",
		"missing from cache",
		"USER_NOT_PARTICIPANT",
	} {
		if strings.Contains(errMsg, marker) {
			return true
		}
	}
	return false
}

func (r *Router) simpleGet(server, path string, chatID int64) error {
	endpoint := fmt.Sprintf("%s/%s?chatid=%d", server, path, chatID)
	resp, err := r.http.Get(endpoint)
	if err != nil {
		return fmt.Errorf("playrouter: %s failed: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("playrouter: %s returned HTTP %d: %s", path, resp.StatusCode, body)
	}
	return nil
}

func (r *Router) Stop(server string, chatID int64) error { return r.simpleGet(server, "stop", chatID) }
func (r *Router) Pause(server string, chatID int64) error {
	return r.simpleGet(server, "pause", chatID)
}
func (r *Router) Resume(server string, chatID int64) error {
	return r.simpleGet(server, "resume", chatID)
}

// PollLoadsForever calls RefreshLoads on interval until ctx-less stop is
// never needed in practice — the bot process lives for the dyno's life.
func (r *Router) PollLoadsForever(interval time.Duration, log *slog.Logger) {
	r.RefreshLoads()
	ticker := time.NewTicker(interval)
	for range ticker.C {
		r.RefreshLoads()
	}
}
