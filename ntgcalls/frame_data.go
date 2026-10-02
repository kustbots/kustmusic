package ntgcalls

//#include "ntgcalls.h"
import "C"

type FrameData struct {
	AbsoluteCaptureTimestampMs int64
	Width, Height, Rotation    uint16
}

func (ctx *FrameData) ParseToC() C.ntg_frame_data {
	var x C.ntg_frame_data
	x.absolute_capture_timestamp_ms = C.int64_t(ctx.AbsoluteCaptureTimestampMs)
	x.width = C.uint16_t(ctx.Width)
	x.height = C.uint16_t(ctx.Height)
	x.rotation = C.ntg_video_rotation(ctx.Rotation)
	return x
}
