package playrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ServerStat is one playback server's line in an operational report.
//
// Two different counts matter and they are not the same number:
//
//	Live     - streams actually running on that server right now, read
//	           straight from its /active_calls. This is the server's own
//	           truth and includes anything it is playing, whoever asked.
//	Assigned - chats this bot has sticky-routed to that server. A chat
//	           keeps its server once assigned, so this is the load the
//	           router believes it has placed there, which drifts above
//	           Live as chats finish playing without being reassigned.
//
// Reporting only one of them would be misleading: Live alone hides how the
// router has distributed chats, and Assigned alone claims playback that may
// have long since ended.
type ServerStat struct {
	Index     int    // 1-based position within its pool
	URL       string //
	Video     bool   // true for the /vplay pool rather than the audio pool
	Online    bool   // /active_calls answered
	Live      int    // streams running on the server right now
	Assigned  int    // chats sticky-routed here by this bot
	Assistant string // account the server streams through, empty if unavailable
	Note      string // why the server is unusable, when it is
}

// Snapshot probes every configured playback server and reports its current
// state. Servers are probed concurrently and the whole call is bounded by
// timeout, because this backs an interactive command.
func (r *Router) Snapshot(timeout time.Duration) []ServerStat {
	// The Router's own client carries a 170s timeout sized for /play, which
	// has to outlast a fresh join's WebRTC handshake. A status report must
	// never block that long, so probe with a short-lived client instead.
	client := &http.Client{Timeout: timeout}

	r.mu.Lock()
	assigned := make(map[string]int, len(r.sticky)+len(r.videoSticky))
	for _, s := range r.sticky {
		assigned[s]++
	}
	for _, s := range r.videoSticky {
		assigned[s]++
	}
	audio := append([]string{}, r.servers...)
	video := append([]string{}, r.videoServers...)
	r.mu.Unlock()

	out := make([]ServerStat, 0, len(audio)+len(video))
	for i, s := range audio {
		out = append(out, ServerStat{Index: i + 1, URL: s, Assigned: assigned[s]})
	}
	for i, s := range video {
		out = append(out, ServerStat{Index: i + 1, URL: s, Video: true, Assigned: assigned[s]})
	}

	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func(st *ServerStat) {
			defer wg.Done()
			st.Live, st.Online, st.Assistant, st.Note = probeStat(client, st.URL)
		}(&out[i])
	}
	wg.Wait()
	return out
}

// probeStat reads one server's live stream count and the account it streams
// through. The two are separate endpoints on purpose: a server can be up and
// answering /active_calls while its assistant session is missing, in which
// case it cannot actually play anything and the report should say so.
func probeStat(client *http.Client, server string) (live int, online bool, assistant, note string) {
	resp, err := client.Get(server + "/active_calls")
	if err != nil {
		return 0, false, "", "unreachable"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false, "", fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, false, "", "unreadable /active_calls"
	}
	live, online = body.Count, true

	// Deliberately not Router.Assistant: that caches for assistantTTL, and a
	// report exists precisely to show the current state rather than a value
	// that may be up to half an hour stale.
	ar, err := client.Get(server + "/assistant")
	if err != nil {
		return live, online, "", "assistant lookup failed"
	}
	defer ar.Body.Close()
	raw, _ := io.ReadAll(ar.Body)
	if ar.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error != "" {
			return live, online, "", e.Error
		}
		return live, online, "", fmt.Sprintf("assistant HTTP %d", ar.StatusCode)
	}
	var info AssistantInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return live, online, "", "unreadable /assistant"
	}
	return live, online, info.Username, ""
}
