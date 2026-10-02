package ntgcalls

//#include "ntgcalls.h"
import "C"

type DhConfig struct {
	G      int32
	P      []byte
	Random []byte
}

func (ctx *DhConfig) ParseToC() (C.ntg_dh_config, func()) {
	var x C.ntg_dh_config
	x.g = C.int32_t(ctx.G)
	x.p, x.p_len = parseBytes(ctx.P)
	x.random, x.random_len = parseBytes(ctx.Random)
	return x, func() {
		freeBytes(x.p)
		freeBytes(x.random)
	}
}
