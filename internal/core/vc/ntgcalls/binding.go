// Package ntgcalls is a cgo binding against the real NTgCalls v2.2.5 C ABI
// (github.com/pytgcalls/ntgcalls) — the exact native voice-call engine
// already running in production for every existing bot in this project
// (confirmed: "PyTgCalls v2.3.3 powered by NTgCalls v2.2.5"). The header
// (ntgcalls.h) and static library are fetched at build time by
// setup_ntgcalls.go from the project's official GitHub releases, matching
// the same distribution mechanism the TgMusicBot reference project uses.
//
// NTgCalls itself does NOT speak Telegram's MTProto — ntg_create returns
// opaque join parameters that must be sent to Telegram's phone.joinGroupCall
// RPC (via gogram, see the bot/playapi assistant packages), and Telegram's
// response is fed into ntg_connect to actually establish the WebRTC
// connection. This package only wraps the native engine's C ABI into
// idiomatic, synchronous Go calls — it knows nothing about Telegram.
//
// IMPORTANT — build status: this package cannot be compiled on the machine
// that wrote it (no C compiler available in this environment). It has been
// written directly against the real, verified ntgcalls.h header (not
// guessed), but its FIRST compile attempt will be the Heroku container
// build — flag any cgo build failure there as expected-possible, not a
// surprise, and fix forward from the real compiler error rather than
// guessing again blind.
package ntgcalls

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo LDFLAGS: -L${SRCDIR}/lib -lntgcalls -lstdc++ -lm -lz

#include <stdlib.h>
#include "ntgcalls.h"

// A Go //export does not by itself make the function resolvable as C.name
// from Go code, even within the same file that exports it — cgo needs an
// explicit C declaration to type it. These forward-declare the two
// //export'ed functions (goPromiseTrampoline here, goStreamEndCallback in
// client.go) so makeAsync and registerStreamEndCallback below can pass them
// as C function pointers.
extern void goPromiseTrampoline(void* userData);
extern void goStreamEndCallback(uintptr_t ptr, int64_t chatId, ntg_stream_type_enum type, ntg_stream_device_enum device, void* userData);
extern void goConnectionChangeCallback(uintptr_t ptr, int64_t chatId, ntg_network_info_struct info, void* userData);
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// pending tracks in-flight async calls keyed by the userData pointer we hand
// to NTgCalls, so the C trampoline can find and signal the right waiter.
var (
	pendingMu sync.Mutex
	pending   = map[unsafe.Pointer]chan struct{}{}
)

//export goPromiseTrampoline
func goPromiseTrampoline(userData unsafe.Pointer) {
	pendingMu.Lock()
	ch, ok := pending[userData]
	pendingMu.Unlock()
	if ok {
		close(ch)
	}
}

// callTimeout bounds how long call() waits for the async promise before
// falling back to the synchronous return code. A synchronous rejection
// (negative return) is not always followed by a promise callback — verified
// empirically: waiting on it unconditionally (matching the reference
// implementation, see below) hung a real /play request indefinitely on a
// genuine synchronous rejection, past both the client's own timeout and
// Heroku's router timeout, with no response ever sent. The promise clearly
// isn't guaranteed to fire for every rejection, whatever the reference
// project's assumption is built on.
const callTimeout = 10 * time.Second

// call runs a single ntg_* async operation synchronously from Go's
// perspective: it registers a waiter keyed by a fresh token, invokes fn with
// that token plus error-output pointers, then waits for either the C
// trampoline to fire (translating NTgCalls' errorCode/errorMessage into a
// Go error — this carries the real, descriptive message) or callTimeout to
// elapse, in which case it falls back to fn's own synchronous return value
// (still better than hanging forever, even without a descriptive message).
func call(fn func(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.int) error {
	// A small heap-allocated byte serves as a unique, stable pointer identity
	// to key the pending map by — cgo forbids passing Go pointers that
	// contain other Go pointers to C, so this must not be a pointer to a
	// struct containing Go-managed memory; a lone byte is safe. Not freed
	// synchronously: if the timeout path below fires first, the promise may
	// still arrive later and dereference this token as the map key (a stale
	// but harmless lookup once we've already deleted the pending entry).
	token := C.malloc(1)

	ch := make(chan struct{})
	pendingMu.Lock()
	pending[token] = ch
	pendingMu.Unlock()

	var errCode C.int
	var errMsg *C.char

	rc := fn(token, &errCode, &errMsg)

	select {
	case <-ch:
		pendingMu.Lock()
		delete(pending, token)
		pendingMu.Unlock()
		C.free(token)

		if errCode < 0 {
			msg := ""
			if errMsg != nil {
				msg = C.GoString(errMsg)
				C.free(unsafe.Pointer(errMsg))
			}
			if msg == "" {
				msg = fmt.Sprintf("error code: %d", int(errCode))
			}
			return fmt.Errorf("ntgcalls: %s", msg)
		}
		return nil

	case <-time.After(callTimeout):
		pendingMu.Lock()
		delete(pending, token)
		pendingMu.Unlock()
		if rc < 0 {
			return fmt.Errorf("ntgcalls: call rejected synchronously (code %d), no promise callback within %s", int(rc), callTimeout)
		}
		return fmt.Errorf("ntgcalls: promise did not fire within %s", callTimeout)
	}
}

// registerStreamEndCallback wires goStreamEndCallback (client.go,
// //export'ed) directly as the native stream-end callback — no C-side
// trampoline needed, since an //export'ed Go function already gets a real
// non-static C declaration cgo emits automatically for the whole package.
func registerStreamEndCallback(ptr C.uintptr_t) C.int {
	return C.ntg_on_stream_end(ptr, C.ntg_stream_callback(C.goStreamEndCallback), nil)
}

// registerConnectionChangeCallback wires goConnectionChangeCallback
// (client.go, //export'ed) as the native connection-state-change callback —
// see ConnectionState's comment for why this matters.
func registerConnectionChangeCallback(ptr C.uintptr_t) C.int {
	return C.ntg_on_connection_change(ptr, C.ntg_connection_callback(C.goConnectionChangeCallback), nil)
}

// makeAsync builds the ntg_async_struct every ntg_* async call needs, wiring
// its promise callback to goPromiseTrampoline. This used to be a `static
// inline` C helper in a shared header (bridge.h), but cgo's static-function
// wrapping only reliably picks up functions written directly in the calling
// file's own preamble comment — not ones reached transitively through an
// #include — which caused "undefined: make_async" from client.go despite
// the header being correctly included. A plain Go function has no such
// per-file boundary, so it's the safe way to share this across the package.
func makeAsync(userData unsafe.Pointer, errCode *C.int, errMsg **C.char) C.ntg_async_struct {
	return C.ntg_async_struct{
		userData:     userData,
		errorCode:    errCode,
		errorMessage: errMsg,
		promise:      C.ntg_async_callback(C.goPromiseTrampoline),
	}
}
