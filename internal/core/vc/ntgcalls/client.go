package ntgcalls

/*
#include <stdlib.h>
#include "ntgcalls.h"
*/
import "C"

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"unsafe"
)

// ConnectionState mirrors ntg_connection_state_enum. NTgCalls' own async
// completion of ntg_connect does not mean the WebRTC connection is actually
// ready for media yet — the real "ready" signal is this state reaching
// Connected via the connection-change callback. Both known-working
// references (AshokShau/TgMusicBot's Go binding and pytgcalls' own
// production code, same v2.2.5 engine) explicitly wait for this after
// connect, with a ~25s timeout — skipping it (this code's original
// approach, relying on PlayedTime alone) meant confirmBuffering's 12s
// window consistently expired on a fresh join, even though the stream
// was fine and would have started playing given more time.
type ConnectionState int

const (
	StateConnecting ConnectionState = iota
	StateConnected
	StateTimeout
	StateFailed
	StateClosed
)

// Client wraps one NTgCalls native instance (one process typically holds
// exactly one — it manages many chatIDs' calls internally).
type Client struct {
	ptr C.uintptr_t

	mu                 sync.Mutex
	onStreamEnd        func(chatID int64)
	onConnectionChange func(chatID int64, state ConnectionState)
}

// streamEndRegistry maps a Client's native ptr to its Go callback, since
// NTgCalls' C callback signature carries no Go-side context we control
// beyond the userData we register it with.
var (
	streamEndMu  sync.Mutex
	streamEndReg = map[C.uintptr_t]*Client{}
)

//export goStreamEndCallback
func goStreamEndCallback(ptr C.uintptr_t, chatID C.int64_t, streamType C.ntg_stream_type_enum, device C.ntg_stream_device_enum, userData unsafe.Pointer) {
	_, _, _ = streamType, device, userData // unused, kept named to avoid a cgo codegen parameter-name collision
	streamEndMu.Lock()
	c, ok := streamEndReg[ptr]
	streamEndMu.Unlock()
	if !ok {
		return
	}
	c.mu.Lock()
	cb := c.onStreamEnd
	c.mu.Unlock()
	if cb != nil {
		cb(int64(chatID))
	}
}

//export goConnectionChangeCallback
func goConnectionChangeCallback(ptr C.uintptr_t, chatID C.int64_t, info C.ntg_network_info_struct, userData unsafe.Pointer) {
	_ = userData
	streamEndMu.Lock()
	c, ok := streamEndReg[ptr]
	streamEndMu.Unlock()
	if !ok {
		return
	}
	c.mu.Lock()
	cb := c.onConnectionChange
	c.mu.Unlock()
	if cb != nil {
		cb(int64(chatID), ConnectionState(info.state))
	}
}

// NewClient creates one native NTgCalls instance.
func NewClient() (*Client, error) {
	ptr := C.ntg_init()
	if ptr == 0 {
		return nil, fmt.Errorf("ntgcalls: ntg_init failed")
	}
	c := &Client{ptr: ptr}
	streamEndMu.Lock()
	streamEndReg[ptr] = c
	streamEndMu.Unlock()

	rc := registerStreamEndCallback(ptr)
	if rc < 0 {
		return nil, fmt.Errorf("ntgcalls: ntg_on_stream_end failed (code %d)", int(rc))
	}
	rc = registerConnectionChangeCallback(ptr)
	if rc < 0 {
		return nil, fmt.Errorf("ntgcalls: ntg_on_connection_change failed (code %d)", int(rc))
	}
	return c, nil
}

// Close releases the native instance. Not safe to use the Client afterward.
func (c *Client) Close() error {
	streamEndMu.Lock()
	delete(streamEndReg, c.ptr)
	streamEndMu.Unlock()
	rc := C.ntg_destroy(c.ptr)
	if rc < 0 {
		return fmt.Errorf("ntgcalls: ntg_destroy failed (code %d)", int(rc))
	}
	return nil
}

// OnStreamEnd registers cb to run whenever any chat's stream naturally ends.
func (c *Client) OnStreamEnd(cb func(chatID int64)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStreamEnd = cb
}

// OnConnectionChange registers cb to run whenever any chat's underlying
// WebRTC connection state changes — the only reliable way to know a call is
// actually ready for media, per ConnectionState's comment.
func (c *Client) OnConnectionChange(cb func(chatID int64, state ConnectionState)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConnectionChange = cb
}

// CreateCall asks the native engine to prepare join parameters for chatID.
// The returned JSON string must be sent to Telegram's phone.joinGroupCall
// RPC (via gogram) — NTgCalls does not talk to Telegram itself.
func (c *Client) CreateCall(chatID int64) (string, error) {
	var buf *C.char
	err := call(func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int {
		return C.ntg_create(c.ptr, C.int64_t(chatID), &buf, makeAsync(userData, errCode, errMsg))
	})
	if err != nil {
		return "", err
	}
	defer C.free(unsafe.Pointer(buf))
	return C.GoString(buf), nil
}

// Connect finishes joining chatID's call using params obtained from
// Telegram's response to the phone.joinGroupCall RPC seeded by CreateCall.
func (c *Client) Connect(chatID int64, params string) error {
	cParams := C.CString(params)
	defer C.free(unsafe.Pointer(cParams))

	return call(func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int {
		return C.ntg_connect(c.ptr, C.int64_t(chatID), cParams, C.bool(false), makeAsync(userData, errCode, errMsg))
	})
}

// buildFFmpegShellCommand constructs the exact ffmpeg invocation NTgCalls'
// NTG_SHELL media source expects: a single shell command string whose stdout
// is raw PCM. Verified against two independent, real, same-version (v2.2.5)
// production implementations — pytgcalls' MediaStream (site-packages
// ffmpeg.py build_command, run through this project's already-deployed
// dream/boge/main-music-rx bots) and the TgMusicBot Go reference's
// getMediaDescription — both of which build an ffmpeg SHELL command for any
// plain URL/file source; neither ever uses NTG_FFMPEG for this. Using
// NTG_FFMPEG directly with a raw URL as `input` (this function's original,
// wrong approach) got "ntg_set_stream_sources ... call rejected
// synchronously" on a real deploy — NTG_FFMPEG is not a "give me a
// URL/path, I'll decode it" mode.
func buildFFmpegShellCommand(source string) string {
	var cmd strings.Builder
	cmd.WriteString("ffmpeg ")

	if _, err := os.Stat(source); err != nil {
		// Not a local file — treat as a live/remote URL and let ffmpeg
		// retry through transient network hiccups instead of dying.
		cmd.WriteString("-reconnect 1 -reconnect_at_eof 1 -reconnect_streamed 1 -reconnect_delay_max 2 ")
	}

	cmd.WriteString(fmt.Sprintf("-i %q ", source))
	cmd.WriteString("-v quiet -f s16le -ac 2 -ar 48000 pipe:1")
	return cmd.String()
}

// SetStreamSources starts (or seamlessly switches) audio playback of source
// (a URL or local file path) in chatID's call, by spawning ffmpeg as a real
// OS subprocess via NTG_SHELL and reading its raw PCM stdout — see
// buildFFmpegShellCommand's comment for why, not NTG_FFMPEG directly.
func (c *Client) SetStreamSources(chatID int64, source string) error {
	// Deliberately NOT freed: ntg_set_stream_sources is async and the header
	// alone doesn't reveal whether the native engine copies `input` before
	// or after the completion promise fires (it's plausibly held for the
	// life of the spawned ffmpeg process, not just the call). Freeing this
	// eagerly risks a use-after-free deep in the native engine that would
	// surface as a hard-to-diagnose mid-playback crash; a small per-call
	// leak (one string per song) is the much safer failure mode until this
	// is verified against real behavior. Revisit once this can be tested
	// against a real build — if the native lib is confirmed to copy input
	// synchronously, this can switch back to a normal defer C.free.
	cInput := C.CString(buildFFmpegShellCommand(source))

	audio := C.ntg_audio_description_struct{
		mediaSource:  C.NTG_SHELL,
		input:        cInput,
		sampleRate:   48000,
		channelCount: 2,
		keepOpen:     C.bool(false),
	}
	desc := C.ntg_media_description_struct{
		microphone: &audio,
	}

	return call(func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int {
		return C.ntg_set_stream_sources(c.ptr, C.int64_t(chatID), C.NTG_STREAM_CAPTURE, desc, makeAsync(userData, errCode, errMsg))
	})
}

func (c *Client) Pause(chatID int64) error {
	return call(func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int {
		return C.ntg_pause(c.ptr, C.int64_t(chatID), makeAsync(userData, errCode, errMsg))
	})
}

func (c *Client) Resume(chatID int64) error {
	return call(func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int {
		return C.ntg_resume(c.ptr, C.int64_t(chatID), makeAsync(userData, errCode, errMsg))
	})
}

func (c *Client) Stop(chatID int64) error {
	return call(func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int {
		return C.ntg_stop(c.ptr, C.int64_t(chatID), makeAsync(userData, errCode, errMsg))
	})
}

// Time returns how long chatID's stream has actually been playing, in
// milliseconds — the primitive the buffering-confirmation poll uses.
func (c *Client) Time(chatID int64) (int64, error) {
	var ms C.int64_t
	err := call(func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int {
		return C.ntg_time(c.ptr, C.int64_t(chatID), C.NTG_STREAM_CAPTURE, &ms, makeAsync(userData, errCode, errMsg))
	})
	if err != nil {
		return 0, err
	}
	return int64(ms), nil
}
