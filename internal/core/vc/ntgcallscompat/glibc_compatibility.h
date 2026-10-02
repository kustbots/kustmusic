// Compatibility shim for glibc >= 2.28, which stopped exporting the private
// __dn_expand/__res_nquery symbols that the prebuilt libntgcalls.a's vendored
// GLib resolver (gthreadedresolver.c) links against. Defines real functions
// under those names that forward to the modern public resolv.h API
// (res_query/res_nquery), so the linker resolves them without needing the
// hidden glibc-internal aliases. Adapted verbatim from the reference
// implementation: github.com/AshokShau/TgMusicBot, src/vc/glibc_compatibility.h
// (Laky64, 2025-04-23), under GPLv3 — this header must be #include'd in
// exactly one cgo preamble in this package (binding.go), since it contains
// real function definitions, not just declarations.
#pragma once

#ifdef __GLIBC__
    #if __GLIBC__ > 2 || (__GLIBC__ == 2 && __GLIBC_MINOR__ >= 28)
		#include <resolv.h>
        int __dn_expand(const unsigned char *src, const unsigned char *src_end,
		                 unsigned char *dst, int dst_len, int options) {
		    int n = res_query((char *)src, C_IN, T_PTR, dst, dst_len);
		    if (n < 0) {
		        return -1;
		    }
		    return n;
		}

		int __res_nquery(const res_state statp, const char *dname, int class, int type,
		                 unsigned char *answer, int anslen) {
		    return res_nquery(statp, dname, class, type, answer, anslen);
		}
	#endif
#endif
