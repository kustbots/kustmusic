// Package telegram is a minimal Telegram Bot API client — just enough to
// receive webhook updates and send/edit messages with inline keyboards. No
// TDLib/gotdbot dependency: webhook mode only needs plain HTTPS calls to
// api.telegram.org, which is both simpler and exactly what "use webhooks,
// not long polling" calls for.
package telegram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	token string
	http  *http.Client

	// photoIDs remembers the file_id Telegram gave each uploaded picture, so
	// a picture is uploaded once per bot and then re-sent by reference.
	// file_ids are per bot, which is why this lives on the client.
	photoMu  sync.Mutex
	photoIDs map[string]string
}

// New builds a client tuned for one thing: not falling over when
// connecting to api.telegram.org is slow.
//
// The default transport keeps only two idle connections per host, so a busy
// bot opens a new TCP+TLS connection for almost every call. That was fine
// until dialling itself started timing out ("dial tcp 149.154.166.110:443:
// i/o timeout") — at which point nearly every request was paying for a
// fresh handshake on a path that was failing, and the bot went quiet in
// chats while its own health endpoint answered fine. Holding a real pool of
// warm connections means the common case never dials at all.
func New(token string) *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// Telegram is the only host this client ever talks to, so the
		// per-host pool is the one that matters. 2 (the default) is far
		// too small for a bot handling several chats at once.
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &Client{
		token:    token,
		http:     &http.Client{Timeout: 20 * time.Second, Transport: transport},
		photoIDs: make(map[string]string),
	}
}

// networkRetries is how many extra attempts a call gets when it fails at
// the network level (dial timeout, connection reset, TLS handshake).
//
// Only network failures are retried. A Telegram-level error — "message is
// not modified", "not enough rights" — is a real answer and retrying it
// just burns quota and time.
const networkRetries = 2

// IsTransportError reports whether err is a connection-level failure —
// the network being unreachable — rather than Telegram answering "no".
//
// Callers need the difference: "the bot was kicked from this chat" is
// permanent and worth acting on, while "dial tcp: i/o timeout" says
// nothing at all about the chat and must not be treated as if it did.
func IsTransportError(err error) bool { return isTransportError(err) }

func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := err.Error()
	for _, marker := range []string{
		"connection reset",
		"connection refused",
		"unexpected EOF",
		"EOF",
		"broken pipe",
		"no such host",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// call sends one Bot API request, retrying transport-level failures.
func (c *Client) call(method string, params map[string]any, out any) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = c.callOnce(method, params, out)
		if err == nil || attempt >= networkRetries || !isTransportError(err) {
			return err
		}
		// Short, escalating pause. The failures being retried here are
		// connection-level blips lasting seconds, not minutes.
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
}

func (c *Client) callOnce(method string, params map[string]any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("telegram: marshal params: %w", err)
	}
	url := "https://api.telegram.org/bot" + c.token + "/" + method
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: %s: %w", method, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram: %s: read response: %w", method, err)
	}
	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("telegram: %s: decode envelope: %w (body: %s)", method, err, respBody)
	}
	if !envelope.OK {
		return fmt.Errorf("telegram: %s failed: %s", method, envelope.Description)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("telegram: %s: decode result: %w", method, err)
		}
	}
	return nil
}

func (c *Client) SetWebhook(url string) error {
	return c.call("setWebhook", map[string]any{"url": url}, nil)
}

// GetUpdates long-polls for new updates (used when no webhook is configured).
func (c *Client) GetUpdates(offset int64, timeoutSeconds int) ([]Update, error) {
	var out []Update
	err := c.call("getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message", "callback_query"},
	}, &out)
	return out, err
}

func (c *Client) DeleteWebhook() error {
	return c.call("deleteWebhook", map[string]any{"drop_pending_updates": true}, nil)
}

// InlineKeyboard is a grid of buttons; each row is a slice of buttons.
type InlineKeyboard [][]InlineButton

// Button colours Telegram renders on inline buttons (Bot API 9.4+).
const (
	StylePrimary = "primary" // blue
	StyleSuccess = "success" // green
	StyleDanger  = "danger"  // red
)

type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
	Style        string `json:"style,omitempty"`
}

type SentMessage struct {
	MessageID int `json:"message_id"`
	Photo     []struct {
		FileID string `json:"file_id"`
	} `json:"photo"`
}

// SendCachedPhoto sends a picture from memory with an HTML caption. The first
// send uploads the bytes; Telegram's file_id for them is kept, and every later
// send passes that id instead, so the picture is never uploaded twice. key
// names the picture (one id is kept per key).
func (c *Client) SendCachedPhoto(chatID int64, key string, data []byte, caption string, kb InlineKeyboard) (*SentMessage, error) {
	c.photoMu.Lock()
	id := c.photoIDs[key]
	c.photoMu.Unlock()
	if id != "" {
		if sent, err := c.SendPhoto(chatID, id, caption, kb); err == nil {
			return sent, nil
		}
		// A rejected id (for example after the bot's token was reissued)
		// falls through to a fresh upload.
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = mw.WriteField("caption", caption)
	_ = mw.WriteField("parse_mode", "HTML")
	if kb != nil {
		markup, err := json.Marshal(map[string]any{"inline_keyboard": kb})
		if err != nil {
			return nil, fmt.Errorf("telegram: sendPhoto: marshal keyboard: %w", err)
		}
		_ = mw.WriteField("reply_markup", string(markup))
	}
	part, err := mw.CreateFormFile("photo", key+".jpg")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	resp, err := c.http.Post("https://api.telegram.org/bot"+c.token+"/sendPhoto", mw.FormDataContentType(), &body)
	if err != nil {
		return nil, fmt.Errorf("telegram: sendPhoto upload: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("telegram: sendPhoto upload: read response: %w", err)
	}
	var envelope struct {
		OK          bool        `json:"ok"`
		Description string      `json:"description"`
		Result      SentMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("telegram: sendPhoto upload: decode: %w", err)
	}
	if !envelope.OK {
		return nil, fmt.Errorf("telegram: sendPhoto upload failed: %s", envelope.Description)
	}
	if n := len(envelope.Result.Photo); n > 0 {
		c.photoMu.Lock()
		c.photoIDs[key] = envelope.Result.Photo[n-1].FileID
		c.photoMu.Unlock()
	}
	return &envelope.Result, nil
}

func (c *Client) SendMessage(chatID int64, text string, kb InlineKeyboard) (*SentMessage, error) {
	params := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if kb != nil {
		params["reply_markup"] = map[string]any{"inline_keyboard": kb}
	}
	var out SentMessage
	if err := c.call("sendMessage", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendAnimation sends a GIF/animation with an HTML caption — used for the
// /start home screen. See buildHomeScreen's comment for why this is HTML
// and not the Python original's Markdown.
func (c *Client) SendAnimation(chatID int64, animationURL, caption string, kb InlineKeyboard) (*SentMessage, error) {
	params := map[string]any{
		"chat_id":    chatID,
		"animation":  animationURL,
		"caption":    caption,
		"parse_mode": "HTML",
	}
	if kb != nil {
		params["reply_markup"] = map[string]any{"inline_keyboard": kb}
	}
	var out SentMessage
	if err := c.call("sendAnimation", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendPhoto sends a photo with an HTML caption — used for the now-playing
// card when the song has a thumbnail, matching setup_player_ui's
// send_photo-then-fallback-to-text behavior.
func (c *Client) SendPhoto(chatID int64, photoURL, caption string, kb InlineKeyboard) (*SentMessage, error) {
	params := map[string]any{
		"chat_id":    chatID,
		"photo":      photoURL,
		"caption":    caption,
		"parse_mode": "HTML",
	}
	if kb != nil {
		params["reply_markup"] = map[string]any{"inline_keyboard": kb}
	}
	var out SentMessage
	if err := c.call("sendPhoto", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) EditMessageText(chatID int64, messageID int, text string, kb InlineKeyboard) error {
	params := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if kb != nil {
		params["reply_markup"] = map[string]any{"inline_keyboard": kb}
	}
	return c.call("editMessageText", params, nil)
}

// EditMessageReplyMarkup swaps just the inline keyboard on an existing
// message, leaving its text/caption alone. Used to animate the player
// card's progress bar without touching (or needing to know) whether the
// card was sent as a photo or as plain text.
// EditMessageCaption edits the caption of a media message.
//
// Bot API keeps this strictly separate from editMessageText: calling
// editMessageText on a photo or animation fails with "there is no text in
// the message to edit". The Python bot never hit this because it went
// through MTProto, where one edit call covers both — which is exactly how
// the help button ended up broken in this port (see cbShowHelp).
func (c *Client) EditMessageCaption(chatID int64, messageID int, caption string, kb InlineKeyboard) error {
	params := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"caption":    caption,
		"parse_mode": "HTML",
	}
	if kb != nil {
		params["reply_markup"] = map[string]any{"inline_keyboard": kb}
	}
	return c.call("editMessageCaption", params, nil)
}

// BotCommand is one entry in the "/" command menu Telegram shows in chats.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// SetMyCommands publishes the "/" menu for scope ("" for the default
// scope, or e.g. "all_group_chats"). Without this the menu keeps whatever
// was set by hand in BotFather long ago, so commands the bot really
// supports simply don't exist as far as users typing "/" can tell.
func (c *Client) SetMyCommands(commands []BotCommand, scope string) error {
	params := map[string]any{"commands": commands}
	if scope != "" {
		params["scope"] = map[string]any{"type": scope}
	}
	return c.call("setMyCommands", params, nil)
}

func (c *Client) EditMessageReplyMarkup(chatID int64, messageID int, kb InlineKeyboard) error {
	return c.call("editMessageReplyMarkup", map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"reply_markup": map[string]any{"inline_keyboard": kb},
	}, nil)
}

func (c *Client) AnswerCallbackQuery(callbackQueryID, text string) error {
	params := map[string]any{"callback_query_id": callbackQueryID}
	if text != "" {
		params["text"] = text
	}
	return c.call("answerCallbackQuery", params, nil)
}

func (c *Client) DeleteMessage(chatID int64, messageID int) error {
	return c.call("deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}, nil)
}

// ChatMember mirrors Telegram's getChatMember result — just the status
// field, which is all IsAdmin needs.
type ChatMember struct {
	Status string `json:"status"` // "creator", "administrator", "member", "restricted", "left", "kicked"
}

func (c *Client) GetChatMember(chatID, userID int64) (*ChatMember, error) {
	var out ChatMember
	if err := c.call("getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IsAdmin reports whether userID is the creator or an administrator of
// chatID — matches main-music-rx's is_user_admin (minus its hardcoded
// support-account bypass IDs, which aren't ported here).
func (c *Client) IsAdmin(chatID, userID int64) (bool, error) {
	m, err := c.GetChatMember(chatID, userID)
	if err != nil {
		return false, err
	}
	return m.Status == "creator" || m.Status == "administrator", nil
}

// GetChatByUsername resolves a public @username to its Chat (works for
// users too, per Bot API's getChat) — used to resolve /ban @username-style
// arguments when the command isn't a reply.
func (c *Client) GetChatByUsername(username string) (*Chat, error) {
	var out Chat
	if err := c.call("getChat", map[string]any{"chat_id": "@" + username}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) BanChatMember(chatID, userID int64) error {
	return c.call("banChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, nil)
}

func (c *Client) UnbanChatMember(chatID, userID int64) error {
	return c.call("unbanChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "only_if_banned": true}, nil)
}

// ChatPermissions mirrors the subset of Telegram's ChatPermissions this bot
// sets for /mute, /unmute, /tmute.
type ChatPermissions struct {
	CanSendMessages       bool `json:"can_send_messages"`
	CanSendMediaMessages  bool `json:"can_send_media_messages"`
	CanSendOtherMessages  bool `json:"can_send_other_messages"`
	CanAddWebPagePreviews bool `json:"can_add_web_page_previews"`
}

// RestrictChatMember applies perms to userID in chatID. untilUnix is a Unix
// timestamp (0 means "forever", matching Bot API semantics) — used by
// /tmute for a timed restriction.
func (c *Client) RestrictChatMember(chatID, userID int64, perms ChatPermissions, untilUnix int64) error {
	params := map[string]any{
		"chat_id":     chatID,
		"user_id":     userID,
		"permissions": perms,
	}
	if untilUnix > 0 {
		params["until_date"] = untilUnix
	}
	return c.call("restrictChatMember", params, nil)
}

// ExportChatInviteLink returns a primary invite link for chatID — used to
// re-invite the assistant to a group it previously left (see the
// idle-group sweeper in play-api). Requires the bot to have invite-users
// rights in that chat.
//
// Prefer CreateChatInviteLink: Bot API's exportChatInviteLink *revokes* the
// group's existing primary link as a side effect, so calling it to let the
// assistant back in quietly invalidates whatever link the group has been
// sharing with its members.
func (c *Client) ExportChatInviteLink(chatID int64) (string, error) {
	var link string
	if err := c.call("exportChatInviteLink", map[string]any{"chat_id": chatID}, &link); err != nil {
		return "", err
	}
	return link, nil
}

// CreateChatInviteLink makes an additional, short-lived invite link for
// chatID without touching the group's primary one.
//
// This is how the assistant gets let back into a group it was swept out
// of. It expires within minutes, which is the only guard it needs.
//
// It deliberately does NOT set member_limit. A one-use link is brittle:
// anything that touches it first — a retry, a race between two servers —
// burns it, and Telegram then answers the real attempt with "invite link
// is invalid or expired", which is exactly the failure groups were
// hitting. The short expiry already stops the link being useful to anyone
// else, so the member cap bought nothing and cost joins.
func (c *Client) CreateChatInviteLink(chatID int64, expiresUnix int64) (string, error) {
	var out struct {
		InviteLink string `json:"invite_link"`
	}
	params := map[string]any{
		"chat_id": chatID,
		"name":    "assistant",
	}
	if expiresUnix > 0 {
		params["expire_date"] = expiresUnix
	}
	if err := c.call("createChatInviteLink", params, &out); err != nil {
		return "", err
	}
	return out.InviteLink, nil
}

func (c *Client) ForwardMessage(toChatID, fromChatID int64, messageID int) error {
	return c.call("forwardMessage", map[string]any{
		"chat_id":      toChatID,
		"from_chat_id": fromChatID,
		"message_id":   messageID,
	}, nil)
}

func (c *Client) GetMe() (*User, error) {
	var out User
	if err := c.call("getMe", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

// Update mirrors the subset of Telegram's Update object this bot handles.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from"`
	Text      string `json:"text"`
	// Caption is set instead of Text when the message carries media. It's
	// how callback handlers tell an animation/photo card apart from a
	// plain text message — the two need different edit methods.
	Caption        string   `json:"caption"`
	ReplyToMessage *Message `json:"reply_to_message"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}
