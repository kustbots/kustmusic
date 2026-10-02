// Package admin ports main-music-rx's moderation-command helpers:
// resolving which user a /ban, /mute, etc. command targets, and checking
// whether the caller is allowed to run it.
package admin

import (
	"strconv"
	"strings"

	"github.com/kustbots/kustmusic/internal/telegram"
)

// ExtractTargetUser resolves the user a moderation command targets: a
// reply's author takes priority (matches the Python bot exactly), else the
// command's first argument as either a raw numeric ID or an @username
// (resolved via getChat). Returns 0 and a user-facing message if nothing
// could be resolved.
func ExtractTargetUser(tg *telegram.Client, m *telegram.Message, args string) (int64, string) {
	if m.ReplyToMessage != nil && m.ReplyToMessage.From != nil {
		return m.ReplyToMessage.From.ID, ""
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return 0, "🤔 Who exactly? Reply to them, or give me their @username."
	}
	target := strings.Fields(args)[0]
	target = strings.TrimPrefix(target, "@")

	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		return id, ""
	}
	chat, err := tg.GetChatByUsername(target)
	if err != nil {
		return 0, "🔍 No idea who that is — check the @username?"
	}
	return chat.ID, ""
}
