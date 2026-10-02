//
// Created by Laky64 on 23/04/2025.
// This header file provides compatibility for the glibc >= 2.28 version for _dn_expand and __res_nquery functions.

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
