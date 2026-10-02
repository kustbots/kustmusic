package main

import "strings"

// This file is the only place raw internal errors are allowed to turn into
// something a group actually reads.
//
// Two things went wrong before it covered everything. Errors leaked
// infrastructure into public chats — a failed download once printed the
// full backend endpoint, URL and query string included, because Go's http
// errors embed the request URL and the bot echoed err.Error() verbatim.
// And the ones that didn't leak were still gibberish: a group tapping
// pause got 'ntgcalls: Connection with chat id "-100…" not found', which
// tells a user nothing except that something they can't see is broken.
//
// So: known failures get plain language, everything else gets a generic
// line, and the real text goes to the log where it belongs.

// genericPlaybackError is the fallback for anything unrecognised on the
// playback path.
const genericPlaybackError = "Something went wrong on my end. Give it another go with a different song — " +
	"and if it keeps happening, poke support and we'll dig into it."

// genericActionError is the fallback for the control commands. "Try a
// different song" would be nonsense advice for a failed pause.
const genericActionError = "That didn't go through. Try again in a sec — if it keeps up, give support a shout."

// userFacingError turns an internal error into something worth reading.
// Use it for anything on the play path.
func userFacingError(raw string) string {
	if msg, ok := knownFailure(raw); ok {
		return msg
	}
	return genericPlaybackError
}

// controlError is userFacingError for pause/resume/stop/skip, where the
// most common failure by far is "there's nothing playing" and the advice
// to try another song makes no sense.
func controlError(raw string) string {
	if msg, ok := knownFailure(raw); ok {
		return msg
	}
	return genericActionError
}

// knownFailure maps the failures worth explaining onto plain language.
// Anything not listed is deliberately not forwarded: internal errors are
// built by wrapping through several packages ("playback: … fallback
// failed: cdn: …"), so even the ones with no URL in them expose backend
// structure and read as noise to someone who just wanted a song.
func knownFailure(raw string) (string, bool) {
	lower := strings.ToLower(raw)

	switch {
	// Nothing is playing. This is the one that used to surface as raw
	// ntgcalls text on every stray pause tap.
	case strings.Contains(lower, "connection with chat id"),
		strings.Contains(lower, "not found in the call"),
		strings.Contains(lower, "no stream"):
		return "Nothing's playing right now — start something with /play first.", true

	case strings.Contains(lower, "no active group call"),
		strings.Contains(lower, "groupcall_invalid"),
		strings.Contains(lower, "groupcall_forbidden"):
		return "There's no voice chat running here. Someone start one and I'll hop in.", true

	case strings.Contains(lower, "restricted or unavailable"),
		strings.Contains(lower, "not audio"),
		strings.Contains(lower, "non-media url"):
		return "That one's locked down — age-gated, private, or blocked where my servers live. Pick another track and we're good.", true

	case strings.Contains(lower, "timed out"),
		strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "timeout"):
		return "That took way too long to load, so I bailed. Give it another shot?", true

	case strings.Contains(lower, "assistant not connected"),
		strings.Contains(lower, "no assistant available"):
		return "My streaming servers are having a moment. Give it a few seconds and try again.", true

	case strings.Contains(lower, "channels_too_much"):
		return "I'm stretched across too many groups at the moment. Support can sort this one out.", true

	case strings.Contains(lower, "no playback servers"):
		return "No playback servers are switched on right now. Support will want to hear about this one.", true

	// Moderation failures. These are the ones an admin can actually act on.
	case strings.Contains(lower, "not enough rights"),
		strings.Contains(lower, "chat_admin_required"),
		strings.Contains(lower, "need administrator rights"):
		return "I don't have the permissions for that. Bump me up in the admin settings and I'll handle it.", true

	case strings.Contains(lower, "user_admin_invalid"),
		strings.Contains(lower, "is an administrator"),
		strings.Contains(lower, "can't remove chat owner"):
		return "Can't touch another admin — that's above my pay grade.", true

	case strings.Contains(lower, "participant_id_invalid"),
		strings.Contains(lower, "user not found"),
		strings.Contains(lower, "user_not_participant"):
		return "That user isn't in this group.", true
	}

	return "", false
}
