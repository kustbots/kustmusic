package ntgcalls

//#include "ntgcalls.h"
//#include <stdlib.h>
import "C"
import "unsafe"

type AudioDescription struct {
	MediaSource  MediaSource
	Input        string
	SampleRate   uint32
	ChannelCount uint8
	KeepOpen     bool
}

func (ctx *AudioDescription) ParseToC() (C.ntg_audio_description, func()) {
	var x C.ntg_audio_description
	x.media_source = ctx.MediaSource.ParseToC()
	x.input = C.CString(ctx.Input)
	x.sample_rate = C.uint32_t(ctx.SampleRate)
	x.channel_count = C.uint8_t(ctx.ChannelCount)
	x.keep_open = C.bool(ctx.KeepOpen)
	return x, func() {
		C.free(unsafe.Pointer(x.input))
	}
}
