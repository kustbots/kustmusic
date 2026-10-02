// Package notify tells whichever bot requested a stream that it ended (or
// the assistant left idle) — play-api has no persistent connection back to
// the caller otherwise. This used to DM the bot account directly via the
// assistant (Sender.SendMessage), relying on the bot to notice and parse a
// JSON-payload private message — fragile, and the reported cause of
// "auto-skip not working" (nothing on the bot side was actually listening
// for it). Replaced with a direct HTTP POST to the bot's own webhook
// server, which is strictly simpler and faster: no round-trip through
// Telegram's message delivery at all.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Notifier struct {
	webhookURL string
	http       *http.Client
}

// New builds a Notifier that POSTs events to webhookURL. An empty
// webhookURL makes Send a no-op (matches the old "botID == 0" no-op case —
// there's simply nowhere configured to tell).
func New(webhookURL string) *Notifier {
	return &Notifier{webhookURL: webhookURL, http: &http.Client{Timeout: 10 * time.Second}}
}

func (n *Notifier) Send(ctx context.Context, botID int64, event map[string]any) error {
	if n.webhookURL == "" || botID == 0 {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("notify: marshal event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.http.Do(req)
	if err != nil {
		return fmt.Errorf("notify: webhook post failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify: webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
