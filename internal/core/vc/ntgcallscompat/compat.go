// Package ntgcallscompat exists purely to compile glibc_compatibility.h's
// __dn_expand/__res_nquery shim into the final binary via a blank import
// (see vc/assistant.go). It must stay a separate package with no
// //export'ed functions of its own: cgo generates one _cgo_export.c per
// package that concatenates every file's preamble in that package, and
// compiling this header there too — alongside the ntgcalls package's real
// //export functions — produced "multiple definition of __dn_expand" at
// link time. A package with zero //export functions never gets a
// _cgo_export.c, so the shim compiles exactly once.
package ntgcallscompat

/*
#include <stdlib.h>
#include "glibc_compatibility.h"

// A plain C-side static array of function pointers forces the linker to
// need __dn_expand/__res_nquery resolved, without ever calling them —
// libntgcalls.a's vendored GLib resolver calls them for real at runtime.
// Referencing them this way (a C-level relocation, not a cgo Go-side
// C.xxx lookup) sidesteps cgo's type probe failing to resolve
// __res_nquery's res_state-typed signature ("could not determine what
// C.__res_nquery refers to", hit when this was attempted directly from
// Go). keepalive() itself has a trivial void(void) signature cgo has no
// trouble typing, and calling it from Go's init() (below) is what makes
// Go's linker keep this whole translation unit — an unreferenced cgo
// preamble was otherwise silently dropped from the final link (confirmed
// empirically: the "undefined reference to __dn_expand" error reappeared
// when this file had zero Go-level usage of anything in it).
static void (*ntgcallscompat_keepalive_ptrs[2])(void) = {
    (void (*)(void))__dn_expand,
    (void (*)(void))__res_nquery,
};

static void ntgcallscompat_keepalive(void) {
    (void)ntgcallscompat_keepalive_ptrs;
}
*/
import "C"

func init() {
	C.ntgcallscompat_keepalive()
}
