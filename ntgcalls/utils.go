package ntgcalls

//#include "ntgcalls.h"
//#include <stdlib.h>
import "C"
import (
	"fmt"
	"unsafe"
)

func parseResult(result C.ntg_result) error {
	if result == C.NTG_OK {
		return nil
	}
	if message := C.GoString(C.ntg_last_error()); len(message) > 0 {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("error code: %d", int32(result))
}

func parseConnectionState(state C.ntg_connection_state) ConnectionState {
	switch state {
	case C.NTG_CONNECTION_STATE_CONNECTING:
		return Connecting
	case C.NTG_CONNECTION_STATE_CONNECTED:
		return Connected
	case C.NTG_CONNECTION_STATE_FAILED:
		return Failed
	case C.NTG_CONNECTION_STATE_TIMEOUT:
		return Timeout
	case C.NTG_CONNECTION_STATE_CLOSED:
		return Closed
	}
	return Connecting
}

func parseStreamDevice(device C.ntg_stream_device) StreamDevice {
	var goDevice StreamDevice
	switch device {
	case C.NTG_STREAM_DEVICE_MICROPHONE:
		goDevice = MicrophoneStream
	case C.NTG_STREAM_DEVICE_SPEAKER:
		goDevice = SpeakerStream
	case C.NTG_STREAM_DEVICE_CAMERA:
		goDevice = CameraStream
	case C.NTG_STREAM_DEVICE_SCREEN:
		goDevice = ScreenStream
	}
	return goDevice
}

func parseStreamStatus(status C.ntg_stream_status) StreamStatus {
	switch status {
	case C.NTG_STREAM_STATUS_ACTIVE:
		return ActiveStream
	case C.NTG_STREAM_STATUS_PAUSED:
		return PausedStream
	case C.NTG_STREAM_STATUS_IDLING:
		return IdlingStream
	}
	return ActiveStream
}

func parseMediaState(state C.ntg_media_state) MediaState {
	return MediaState{
		Muted:               bool(state.muted),
		VideoPaused:         bool(state.video_paused),
		VideoStopped:        bool(state.video_stopped),
		PresentationPaused:  bool(state.presentation_paused),
		PresentationStopped: bool(state.presentation_stopped),
	}
}

func parseBytes(data []byte) (*C.uint8_t, C.size_t) {
	if len(data) > 0 {
		return (*C.uint8_t)(C.CBytes(data)), C.size_t(len(data))
	}
	return nil, 0
}

func freeBytes(data *C.uint8_t) {
	if data != nil {
		C.free(unsafe.Pointer(data))
	}
}

func parseStringVector(data **C.char, size C.size_t) []string {
	result := make([]string, size)
	for i := 0; i < int(size); i++ {
		pointer := *(**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(data)) + uintptr(i)*unsafe.Sizeof(*data)))
		result[i] = C.GoString(pointer)
	}
	return result
}

func parseUint32VectorC(data []uint32) (*C.uint32_t, C.size_t) {
	if len(data) == 0 {
		return nil, 0
	}
	cData := C.malloc(C.size_t(len(data)) * C.size_t(unsafe.Sizeof(C.uint32_t(0))))
	if cData == nil {
		return nil, 0
	}
	ssrcs := (*C.uint32_t)(cData)
	for i, v := range data {
		*(*C.uint32_t)(unsafe.Pointer(uintptr(cData) + uintptr(i)*unsafe.Sizeof(C.uint32_t(0)))) = C.uint32_t(v)
	}
	return ssrcs, C.size_t(len(data))
}

func parseStringVectorC(data []string) ([]*C.char, **C.char, C.size_t) {
	if len(data) == 0 {
		return nil, nil, 0
	}
	rawData := make([]*C.char, len(data))
	for i, v := range data {
		rawData[i] = C.CString(v)
	}
	return rawData, &rawData[0], C.size_t(len(data))
}

func freeStringVectorC(data []*C.char) {
	for _, value := range data {
		C.free(unsafe.Pointer(value))
	}
}

func parseRtcServers(rtcServers []RTCServer) []C.ntg_rtc_server {
	rawServers := make([]C.ntg_rtc_server, len(rtcServers))
	for i, server := range rtcServers {
		peerTagC, peerTagSize := parseBytes(server.PeerTag)
		rawServers[i] = C.ntg_rtc_server{
			id:           C.uint64_t(server.ID),
			ipv4:         C.CString(server.Ipv4),
			ipv6:         C.CString(server.Ipv6),
			username:     C.CString(server.Username),
			password:     C.CString(server.Password),
			port:         C.uint16_t(server.Port),
			turn:         C.bool(server.Turn),
			stun:         C.bool(server.Stun),
			tcp:          C.bool(server.Tcp),
			peer_tag:     peerTagC,
			peer_tag_len: peerTagSize,
		}
	}
	return rawServers
}

func freeRtcServers(rawServers []C.ntg_rtc_server) {
	for _, server := range rawServers {
		C.free(unsafe.Pointer(server.ipv4))
		C.free(unsafe.Pointer(server.ipv6))
		C.free(unsafe.Pointer(server.username))
		C.free(unsafe.Pointer(server.password))
		freeBytes(server.peer_tag)
	}
}

func parseSsrcGroups(ssrcGroups []SsrcGroup) []C.ntg_ssrc_group {
	rawGroups := make([]C.ntg_ssrc_group, len(ssrcGroups))
	for i, group := range ssrcGroups {
		ssrcsC, ssrcsSize := parseUint32VectorC(group.Ssrcs)
		rawGroups[i] = C.ntg_ssrc_group{
			semantics: C.CString(group.Semantics),
			ssrcs:     ssrcsC,
			ssrcs_len: ssrcsSize,
		}
	}
	return rawGroups
}

func freeSsrcGroups(rawGroups []C.ntg_ssrc_group) {
	for _, group := range rawGroups {
		C.free(unsafe.Pointer(group.semantics))
		if group.ssrcs != nil {
			C.free(unsafe.Pointer(group.ssrcs))
		}
	}
}

func parseSsrcMappings(mappings []SsrcMapping) []C.ntg_ssrc_mapping {
	rawMappings := make([]C.ntg_ssrc_mapping, len(mappings))
	for i, mapping := range mappings {
		rawMappings[i] = C.ntg_ssrc_mapping{
			user_id: C.int64_t(mapping.UserID),
			ssrc:    C.int32_t(mapping.Ssrc),
		}
	}
	return rawMappings
}

func parseBlocks(blocks [][]byte) []C.ntg_bytes {
	rawBlocks := make([]C.ntg_bytes, len(blocks))
	for i, block := range blocks {
		blockC, blockSize := parseBytes(block)
		rawBlocks[i] = C.ntg_bytes{
			data: blockC,
			len:  blockSize,
		}
	}
	return rawBlocks
}

func freeBlocks(rawBlocks []C.ntg_bytes) {
	for _, block := range rawBlocks {
		freeBytes(block.data)
	}
}

func parseDeviceInfoVector(devices *C.ntg_device_info, size C.size_t) []DeviceInfo {
	rawDevices := make([]DeviceInfo, size)
	for i := 0; i < int(size); i++ {
		device := *(*C.ntg_device_info)(unsafe.Pointer(uintptr(unsafe.Pointer(devices)) + uintptr(i)*unsafe.Sizeof(*devices)))
		rawDevices[i] = DeviceInfo{
			Name:     C.GoString(device.name),
			Metadata: C.GoString(device.metadata),
		}
	}
	return rawDevices
}
