package ntgcalls

//#include "ntgcalls.h"
//#include <stdlib.h>
import "C"
import "unsafe"

type MediaDescription struct {
	Microphone *AudioDescription
	Speaker    *AudioDescription
	Camera     *VideoDescription
	Screen     *VideoDescription
}

func (ctx *MediaDescription) ParseToC() (C.ntg_media_description, func()) {
	var x C.ntg_media_description
	var cleanups []func()
	if ctx.Microphone != nil {
		x.microphone, cleanups = copyAudioDescription(ctx.Microphone, cleanups)
	}
	if ctx.Speaker != nil {
		x.speaker, cleanups = copyAudioDescription(ctx.Speaker, cleanups)
	}
	if ctx.Camera != nil {
		x.camera, cleanups = copyVideoDescription(ctx.Camera, cleanups)
	}
	if ctx.Screen != nil {
		x.screen, cleanups = copyVideoDescription(ctx.Screen, cleanups)
	}
	return x, func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}
}

func copyAudioDescription(desc *AudioDescription, cleanups []func()) (*C.ntg_audio_description, []func()) {
	value, cleanup := desc.ParseToC()
	pointer := (*C.ntg_audio_description)(C.malloc(C.size_t(unsafe.Sizeof(value))))
	*pointer = value
	return pointer, append(cleanups, cleanup, func() { C.free(unsafe.Pointer(pointer)) })
}

func copyVideoDescription(desc *VideoDescription, cleanups []func()) (*C.ntg_video_description, []func()) {
	value, cleanup := desc.ParseToC()
	pointer := (*C.ntg_video_description)(C.malloc(C.size_t(unsafe.Sizeof(value))))
	*pointer = value
	return pointer, append(cleanups, cleanup, func() { C.free(unsafe.Pointer(pointer)) })
}
