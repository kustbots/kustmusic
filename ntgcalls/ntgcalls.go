package ntgcalls

/*
#include "ntgcalls.h"
#include <stdlib.h>
extern void handleStreamEnd(ntg_instance* handle, int64_t chatID, ntg_stream_type streamType, ntg_stream_device streamDevice, void* userData);
extern void handleUpgrade(ntg_instance* handle, int64_t chatID, ntg_media_state state, void* userData);
extern void handleConnectionChange(ntg_instance* handle, int64_t chatID, ntg_connection_info info, void* userData);
extern void handleSignal(ntg_instance* handle, int64_t chatID, uint8_t* data, size_t size, void* userData);
extern void handleFrames(ntg_instance* handle, int64_t chatID, ntg_stream_mode streamMode, ntg_stream_device streamDevice, ntg_frame* frames, size_t size, void* userData);
extern void handleRemoteSourceChange(ntg_instance* handle, int64_t chatID, ntg_remote_source remoteSource, void* userData);
extern void handleRequestBroadcastTimestamp(ntg_instance* handle, int64_t chatID, void* userData);
extern void handleRequestBroadcastPart(ntg_instance* handle, int64_t chatID, ntg_segment_part_request request, void* userData);
extern void handleUpdateEmojis(ntg_instance* handle, int64_t chatID, char* emojis, void* userData);
extern void handleRequestParticipants(ntg_instance* handle, int64_t chatID, void* userData);
extern void handleOutboundBlock(ntg_instance* handle, int64_t chatID, uint8_t* block, size_t size, void* userData);
extern void handleSubchainRequest(ntg_instance* handle, int64_t chatID, ntg_subchain_request request, void* userData);
extern void handleLogs(ntg_log_message message, void* userData);
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func init() {
	C.ntg_set_log_callback((C.ntg_log_cb)(unsafe.Pointer(C.handleLogs)), nil)
}

var (
	loggerNTGCalls = NewLogger("ntgcalls", LevelInfo)
	loggerWebRTC   = NewLogger("webrtc", LevelFatal)
)

func NTgCalls() *Client {
	instance := &Client{
		handle: C.ntg_instance_create(),
	}

	registerClient(instance)
	C.ntg_on_stream_end_callback(instance.handle, (C.ntg_stream_end_callback_cb)(unsafe.Pointer(C.handleStreamEnd)), nil)
	C.ntg_on_upgrade_callback(instance.handle, (C.ntg_upgrade_callback_cb)(unsafe.Pointer(C.handleUpgrade)), nil)
	C.ntg_on_signaling_data_callback(instance.handle, (C.ntg_signaling_data_callback_cb)(unsafe.Pointer(C.handleSignal)), nil)
	C.ntg_on_connection_change_callback(instance.handle, (C.ntg_connection_change_callback_cb)(unsafe.Pointer(C.handleConnectionChange)), nil)
	C.ntg_on_frames_callback(instance.handle, (C.ntg_frames_callback_cb)(unsafe.Pointer(C.handleFrames)), nil)
	C.ntg_on_remote_source_change_callback(instance.handle, (C.ntg_remote_source_change_callback_cb)(unsafe.Pointer(C.handleRemoteSourceChange)), nil)
	C.ntg_on_request_broadcast_timestamp_callback(instance.handle, (C.ntg_request_broadcast_timestamp_callback_cb)(unsafe.Pointer(C.handleRequestBroadcastTimestamp)), nil)
	C.ntg_on_request_broadcast_part_callback(instance.handle, (C.ntg_request_broadcast_part_callback_cb)(unsafe.Pointer(C.handleRequestBroadcastPart)), nil)
	C.ntg_on_update_emojis_callback(instance.handle, (C.ntg_update_emojis_callback_cb)(unsafe.Pointer(C.handleUpdateEmojis)), nil)
	C.ntg_on_request_participants_callback(instance.handle, (C.ntg_request_participants_callback_cb)(unsafe.Pointer(C.handleRequestParticipants)), nil)
	C.ntg_on_outbound_block_callback(instance.handle, (C.ntg_outbound_block_callback_cb)(unsafe.Pointer(C.handleOutboundBlock)), nil)
	C.ntg_on_subchain_request_callback(instance.handle, (C.ntg_subchain_request_callback_cb)(unsafe.Pointer(C.handleSubchainRequest)), nil)

	return instance
}

//export handleLogs
func handleLogs(message C.ntg_log_message, _ unsafe.Pointer) {
	text := fmt.Sprintf(
		"(%s:%d) %s",
		C.GoString(message.file),
		uint32(message.line),
		C.GoString(message.message),
	)

	var lg *Logger
	if message.source == C.NTG_LOG_SOURCE_WEBRTC {
		lg = loggerWebRTC
	} else {
		lg = loggerNTGCalls
	}

	loggerInstance := lg
	switch message.level {
	case C.NTG_LOG_DEBUG:
		loggerInstance.Debug(text)
	case C.NTG_LOG_INFO:
		loggerInstance.Info(text)
	case C.NTG_LOG_WARNING:
		loggerInstance.Warn(text)
	case C.NTG_LOG_ERROR:
		loggerInstance.Error(text)
	}
}

//export handleStreamEnd
func handleStreamEnd(handle *C.ntg_instance, chatID C.int64_t, streamType C.ntg_stream_type, streamDevice C.ntg_stream_device, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goStreamType := VideoStream
	if streamType == C.NTG_STREAM_TYPE_AUDIO {
		goStreamType = AudioStream
	}
	goDevice := parseStreamDevice(streamDevice)
	for _, callback := range self.copyStreamEndCallbacks() {
		go callback(int64(chatID), goStreamType, goDevice)
	}
}

//export handleUpgrade
func handleUpgrade(handle *C.ntg_instance, chatID C.int64_t, state C.ntg_media_state, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goState := parseMediaState(state)
	for _, callback := range self.copyUpgradeCallbacks() {
		go callback(int64(chatID), goState)
	}
}

//export handleSignal
func handleSignal(handle *C.ntg_instance, chatID C.int64_t, data *C.uint8_t, size C.size_t, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}

	goData := C.GoBytes(unsafe.Pointer(data), C.int(size))
	for _, callback := range self.copySignalCallbacks() {
		go callback(int64(chatID), goData)
	}
}

//export handleConnectionChange
func handleConnectionChange(handle *C.ntg_instance, chatID C.int64_t, info C.ntg_connection_info, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goState := NetworkInfo{
		State: parseConnectionState(info.state),
	}
	switch info.kind {
	case C.NTG_CONNECTION_KIND_NORMAL:
		goState.Kind = NormalConnection
	case C.NTG_CONNECTION_KIND_PRESENTATION:
		goState.Kind = PresentationConnection
	}
	for _, callback := range self.copyConnectionChangeCallbacks() {
		go callback(int64(chatID), goState)
	}
}

//export handleFrames
func handleFrames(handle *C.ntg_instance, chatID C.int64_t, streamMode C.ntg_stream_mode, streamDevice C.ntg_stream_device, frames *C.ntg_frame, size C.size_t, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goStreamMode := PlaybackStream
	if streamMode == C.NTG_STREAM_MODE_CAPTURE {
		goStreamMode = CaptureStream
	}
	goFrames := make([]Frame, size)
	for i := 0; i < int(size); i++ {
		rawFrame := *(*C.ntg_frame)(unsafe.Pointer(uintptr(unsafe.Pointer(frames)) + uintptr(i)*unsafe.Sizeof(*frames)))
		goFrames[i] = Frame{
			Ssrc: uint32(rawFrame.ssrc),
			Data: C.GoBytes(unsafe.Pointer(rawFrame.data), C.int(rawFrame.data_len)),
			FrameData: FrameData{
				AbsoluteCaptureTimestampMs: int64(rawFrame.frame_data.absolute_capture_timestamp_ms),
				Width:                      uint16(rawFrame.frame_data.width),
				Height:                     uint16(rawFrame.frame_data.height),
				Rotation:                   uint16(rawFrame.frame_data.rotation),
			},
		}
	}
	goDevice := parseStreamDevice(streamDevice)
	for _, callback := range self.copyFrameCallbacks() {
		go callback(int64(chatID), goStreamMode, goDevice, goFrames)
	}
}

//export handleRemoteSourceChange
func handleRemoteSourceChange(handle *C.ntg_instance, chatID C.int64_t, remoteSource C.ntg_remote_source, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goRemoteSource := RemoteSource{
		Ssrc:   uint32(remoteSource.ssrc),
		State:  parseStreamStatus(remoteSource.state),
		Device: parseStreamDevice(remoteSource.device),
	}
	for _, callback := range self.copyRemoteSourceCallbacks() {
		go callback(int64(chatID), goRemoteSource)
	}
}

//export handleRequestBroadcastTimestamp
func handleRequestBroadcastTimestamp(handle *C.ntg_instance, chatID C.int64_t, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	for _, callback := range self.copyBroadcastTimestampCallbacks() {
		go callback(int64(chatID))
	}
}

//export handleRequestBroadcastPart
func handleRequestBroadcastPart(handle *C.ntg_instance, chatID C.int64_t, request C.ntg_segment_part_request, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	var goQuality MediaSegmentQuality
	switch request.quality {
	case C.NTG_MEDIA_SEGMENT_QUALITY_NONE:
		goQuality = SegmentQualityNone
	case C.NTG_MEDIA_SEGMENT_QUALITY_THUMBNAIL:
		goQuality = SegmentQualityThumbnail
	case C.NTG_MEDIA_SEGMENT_QUALITY_MEDIUM:
		goQuality = SegmentQualityMedium
	case C.NTG_MEDIA_SEGMENT_QUALITY_FULL:
		goQuality = SegmentQualityFull
	}
	goRequest := SegmentPartRequest{
		SegmentID:     int64(request.segment_id),
		PartID:        int32(request.part_id),
		Limit:         int32(request.limit),
		Timestamp:     int64(request.timestamp),
		QualityUpdate: bool(request.quality_update),
		ChannelID:     int32(request.channel_id),
		Quality:       goQuality,
	}
	for _, callback := range self.copyBroadcastPartCallbacks() {
		go callback(int64(chatID), goRequest)
	}
}

//export handleUpdateEmojis
func handleUpdateEmojis(handle *C.ntg_instance, chatID C.int64_t, emojis *C.char, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goEmojis := C.GoString(emojis)
	for _, callback := range self.copyEmojisCallbacks() {
		go callback(int64(chatID), goEmojis)
	}
}

//export handleRequestParticipants
func handleRequestParticipants(handle *C.ntg_instance, chatID C.int64_t, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	for _, callback := range self.copyRequestParticipantsCallbacks() {
		go callback(int64(chatID))
	}
}

//export handleOutboundBlock
func handleOutboundBlock(handle *C.ntg_instance, chatID C.int64_t, block *C.uint8_t, size C.size_t, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goBlock := C.GoBytes(unsafe.Pointer(block), C.int(size))
	for _, callback := range self.copyOutboundBlockCallbacks() {
		go callback(int64(chatID), goBlock)
	}
}

//export handleSubchainRequest
func handleSubchainRequest(handle *C.ntg_instance, chatID C.int64_t, request C.ntg_subchain_request, _ unsafe.Pointer) {
	self := lookupClient(handle)
	if self == nil {
		return
	}
	goRequest := SubchainRequest{
		Subchain: int32(request.subchain),
		Height:   int32(request.height),
		Limit:    int32(request.limit),
	}
	for _, callback := range self.copySubchainRequestCallbacks() {
		go callback(int64(chatID), goRequest)
	}
}

func (ctx *Client) GetState(chatId int64) (MediaState, error) {
	var buffer C.ntg_media_state
	if err := parseResult(C.ntg_get_state(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return MediaState{}, err
	}
	defer C.ntg_media_state_free(&buffer)
	return parseMediaState(buffer), nil
}

func (ctx *Client) GetConnectionMode(chatId int64) (ConnectionMode, error) {
	var buffer C.ntg_connection_mode
	if err := parseResult(C.ntg_get_connection_mode(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return ConnectionMode(0), err
	}
	switch buffer {
	case C.NTG_CONNECTION_MODE_RTC:
		return RtcConnection, nil
	case C.NTG_CONNECTION_MODE_STREAM:
		return StreamConnection, nil
	case C.NTG_CONNECTION_MODE_RTMP:
		return RTMPConnection, nil
	default:
		return ConnectionMode(0), fmt.Errorf("unknown connection mode")
	}
}

func (ctx *Client) GetEmojisFingerprint(chatId int64) (string, error) {
	var buffer *C.char
	if err := parseResult(C.ntg_get_emojis_fingerprint(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return "", err
	}
	defer C.ntg_string_free(buffer)
	return C.GoString(buffer), nil
}

func (ctx *Client) GetCallType(chatId int64) (CallType, error) {
	var buffer C.ntg_call_type
	if err := parseResult(C.ntg_get_call_type(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return CallType(0), err
	}
	switch buffer {
	case C.NTG_CALL_TYPE_GROUP:
		return GroupCallType, nil
	case C.NTG_CALL_TYPE_P2P:
		return P2PCallType, nil
	case C.NTG_CALL_TYPE_CONFERENCE:
		return ConferenceCallType, nil
	default:
		return CallType(0), fmt.Errorf("unknown call type")
	}
}

func (ctx *Client) CreateCall(chatId int64) (string, error) {
	var buffer *C.char
	if err := parseResult(C.ntg_create_call(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return "", err
	}
	defer C.ntg_string_free(buffer)
	return C.GoString(buffer), nil
}

func (ctx *Client) InitPresentation(chatId int64) (string, error) {
	var buffer *C.char
	if err := parseResult(C.ntg_init_presentation(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return "", err
	}
	defer C.ntg_string_free(buffer)
	return C.GoString(buffer), nil
}

func (ctx *Client) InitConference(chatId int64, userId int64, lastBlock []byte) (ConferenceJoinParams, error) {
	lastBlockC, lastBlockSize := parseBytes(lastBlock)
	defer freeBytes(lastBlockC)
	var buffer C.ntg_conference_join_params
	if err := parseResult(C.ntg_init_conference(ctx.handle, C.int64_t(chatId), C.int64_t(userId), lastBlockC, lastBlockSize, &buffer)); err != nil {
		return ConferenceJoinParams{}, err
	}
	defer C.ntg_conference_join_params_free(&buffer)
	return ConferenceJoinParams{
		Payload:   C.GoString(buffer.payload),
		PublicKey: C.GoBytes(unsafe.Pointer(buffer.public_key), C.int(buffer.public_key_len)),
		Block:     C.GoBytes(unsafe.Pointer(buffer.block), C.int(buffer.block_len)),
	}, nil
}

func (ctx *Client) StopPresentation(chatId int64) error {
	return parseResult(C.ntg_stop_presentation(ctx.handle, C.int64_t(chatId)))
}

func (ctx *Client) AddIncomingVideo(chatId, userId int64, endpoint string, ssrcGroups []SsrcGroup) (uint32, error) {
	endpointC := C.CString(endpoint)
	defer C.free(unsafe.Pointer(endpointC))
	rawGroups := parseSsrcGroups(ssrcGroups)
	defer freeSsrcGroups(rawGroups)
	var groupsC *C.ntg_ssrc_group
	if len(rawGroups) > 0 {
		groupsC = &rawGroups[0]
	}
	var buffer C.uint32_t
	if err := parseResult(C.ntg_add_incoming_video(ctx.handle, C.int64_t(chatId), C.int64_t(userId), endpointC, groupsC, C.size_t(len(rawGroups)), &buffer)); err != nil {
		return 0, err
	}
	return uint32(buffer), nil
}

func (ctx *Client) RemoveIncomingVideo(chatId int64, endpoint string) (bool, error) {
	endpointC := C.CString(endpoint)
	defer C.free(unsafe.Pointer(endpointC))
	var buffer C.bool
	if err := parseResult(C.ntg_remove_incoming_video(ctx.handle, C.int64_t(chatId), endpointC, &buffer)); err != nil {
		return false, err
	}
	return bool(buffer), nil
}

func (ctx *Client) CreateP2PCall(userId int64) error {
	return parseResult(C.ntg_create_p2p_call(ctx.handle, C.int64_t(userId)))
}

func (ctx *Client) InitExchange(userId int64, dhConfig DhConfig, gAHash []byte) ([]byte, error) {
	dhConfigC, freeDhConfig := dhConfig.ParseToC()
	defer freeDhConfig()
	gAHashC, gAHashSize := parseBytes(gAHash)
	defer freeBytes(gAHashC)
	var buffer *C.uint8_t
	var size C.size_t
	if err := parseResult(C.ntg_init_exchange(ctx.handle, C.int64_t(userId), dhConfigC, gAHashC, gAHashSize, &buffer, &size)); err != nil {
		return nil, err
	}
	defer C.ntg_bytes_free(unsafe.Pointer(buffer))
	return C.GoBytes(unsafe.Pointer(buffer), C.int(size)), nil
}

func (ctx *Client) ExchangeKeys(userId int64, gAB []byte, fingerprint int64) (AuthParams, error) {
	gABC, gABSize := parseBytes(gAB)
	defer freeBytes(gABC)
	var buffer C.ntg_auth_params
	if err := parseResult(C.ntg_exchange_keys(ctx.handle, C.int64_t(userId), gABC, gABSize, C.int64_t(fingerprint), &buffer)); err != nil {
		return AuthParams{}, err
	}
	defer C.ntg_auth_params_free(&buffer)
	return AuthParams{
		GAOrB:          C.GoBytes(unsafe.Pointer(buffer.g_a_or_b), C.int(buffer.g_a_or_b_len)),
		KeyFingerprint: int64(buffer.key_fingerprint),
	}, nil
}

func (ctx *Client) SkipExchange(userId int64, encryptionKey []byte, isOutgoing bool) error {
	encryptionKeyC, encryptionKeySize := parseBytes(encryptionKey)
	defer freeBytes(encryptionKeyC)
	return parseResult(C.ntg_skip_exchange(ctx.handle, C.int64_t(userId), encryptionKeyC, encryptionKeySize, C.bool(isOutgoing)))
}

func (ctx *Client) ConnectP2P(userId int64, rtcServers []RTCServer, versions []string, p2pAllowed bool, customParameters string) error {
	rawServers := parseRtcServers(rtcServers)
	defer freeRtcServers(rawServers)
	var serversC *C.ntg_rtc_server
	if len(rawServers) > 0 {
		serversC = &rawServers[0]
	}
	rawVersions, versionsC, versionsSize := parseStringVectorC(versions)
	defer freeStringVectorC(rawVersions)
	var customParametersC *C.char
	if len(customParameters) > 0 {
		customParametersC = C.CString(customParameters)
		defer C.free(unsafe.Pointer(customParametersC))
	}
	return parseResult(C.ntg_connect_p2p(ctx.handle, C.int64_t(userId), serversC, C.size_t(len(rawServers)), versionsC, versionsSize, C.bool(p2pAllowed), customParametersC))
}

func (ctx *Client) SendSignalingData(chatId int64, data []byte) error {
	dataC, dataSize := parseBytes(data)
	defer freeBytes(dataC)
	return parseResult(C.ntg_send_signaling_data(ctx.handle, C.int64_t(chatId), dataC, dataSize))
}

//goland:noinspection GoUnusedExportedFunction
func GetProtocol() (Protocol, error) {
	var buffer C.ntg_protocol
	if err := parseResult(C.ntg_get_protocol(&buffer)); err != nil {
		return Protocol{}, err
	}
	defer C.ntg_protocol_free(&buffer)
	return Protocol{
		MinLayer:     int32(buffer.min_layer),
		MaxLayer:     int32(buffer.max_layer),
		UdpP2P:       bool(buffer.udp_p2p),
		UdpReflector: bool(buffer.udp_reflector),
		Versions:     parseStringVector(buffer.library_versions, buffer.library_versions_len),
	}, nil
}

func (ctx *Client) Connect(chatId int64, params string, isPresentation bool) error {
	paramsC := C.CString(params)
	defer C.free(unsafe.Pointer(paramsC))
	return parseResult(C.ntg_connect(ctx.handle, C.int64_t(chatId), paramsC, C.bool(isPresentation)))
}

func (ctx *Client) SetStreamSources(chatId int64, streamMode StreamMode, desc MediaDescription) error {
	descC, freeDesc := desc.ParseToC()
	defer freeDesc()
	return parseResult(C.ntg_set_stream_sources(ctx.handle, C.int64_t(chatId), streamMode.ParseToC(), descC))
}

func (ctx *Client) SendExternalFrame(chatId int64, streamDevice StreamDevice, data []byte, frameData FrameData) error {
	dataC, dataSize := parseBytes(data)
	defer freeBytes(dataC)
	return parseResult(C.ntg_send_external_frame(ctx.handle, C.int64_t(chatId), streamDevice.ParseToC(), dataC, dataSize, frameData.ParseToC()))
}

func (ctx *Client) SendBroadcastTimestamp(chatId int64, timestamp int64) error {
	return parseResult(C.ntg_send_broadcast_timestamp(ctx.handle, C.int64_t(chatId), C.int64_t(timestamp)))
}

func (ctx *Client) SendBroadcastPart(chatId int64, segmentID int64, partID int32, status MediaSegmentStatus, qualityUpdate bool, data []byte) error {
	dataC, dataSize := parseBytes(data)
	defer freeBytes(dataC)
	return parseResult(C.ntg_send_broadcast_part(ctx.handle, C.int64_t(chatId), C.int64_t(segmentID), C.int32_t(partID), status.ParseToC(), C.bool(qualityUpdate), dataC, dataSize))
}

func (ctx *Client) UpdateAudioSsrcMappings(chatId int64, mappings []SsrcMapping) error {
	rawMappings := parseSsrcMappings(mappings)
	var mappingsC *C.ntg_ssrc_mapping
	if len(rawMappings) > 0 {
		mappingsC = &rawMappings[0]
	}
	return parseResult(C.ntg_update_audio_ssrc_mappings(ctx.handle, C.int64_t(chatId), mappingsC, C.size_t(len(rawMappings))))
}

func (ctx *Client) ApplyBlocks(chatId int64, subchain int32, nextOffset int32, blocks [][]byte, fromShortPoll bool) error {
	rawBlocks := parseBlocks(blocks)
	defer freeBlocks(rawBlocks)
	var blocksC *C.ntg_bytes
	if len(rawBlocks) > 0 {
		blocksC = &rawBlocks[0]
	}
	return parseResult(C.ntg_apply_blocks(ctx.handle, C.int64_t(chatId), C.int32_t(subchain), C.int32_t(nextOffset), blocksC, C.size_t(len(rawBlocks)), C.bool(fromShortPoll)))
}

func (ctx *Client) FinishSubchainRequest(chatId int64, subchain int32) error {
	return parseResult(C.ntg_finish_subchain_request(ctx.handle, C.int64_t(chatId), C.int32_t(subchain)))
}

func (ctx *Client) Pause(chatId int64) (bool, error) {
	var buffer C.bool
	if err := parseResult(C.ntg_pause(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return false, err
	}
	return bool(buffer), nil
}

func (ctx *Client) Resume(chatId int64) (bool, error) {
	var buffer C.bool
	if err := parseResult(C.ntg_resume(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return false, err
	}
	return bool(buffer), nil
}

func (ctx *Client) Mute(chatId int64) (bool, error) {
	var buffer C.bool
	if err := parseResult(C.ntg_mute(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return false, err
	}
	return bool(buffer), nil
}

func (ctx *Client) UnMute(chatId int64) (bool, error) {
	var buffer C.bool
	if err := parseResult(C.ntg_unmute(ctx.handle, C.int64_t(chatId), &buffer)); err != nil {
		return false, err
	}
	return bool(buffer), nil
}

func (ctx *Client) Stop(chatId int64) error {
	return parseResult(C.ntg_stop(ctx.handle, C.int64_t(chatId)))
}

func (ctx *Client) Time(chatId int64, streamMode StreamMode) (uint64, error) {
	var buffer C.uint64_t
	if err := parseResult(C.ntg_time(ctx.handle, C.int64_t(chatId), streamMode.ParseToC(), &buffer)); err != nil {
		return 0, err
	}
	return uint64(buffer), nil
}

//goland:noinspection GoUnusedExportedFunction
func GetMediaDevices() (MediaDevices, error) {
	var buffer C.ntg_media_devices
	if err := parseResult(C.ntg_get_media_devices(&buffer)); err != nil {
		return MediaDevices{}, err
	}
	defer C.ntg_media_devices_free(&buffer)
	return MediaDevices{
		Microphone: parseDeviceInfoVector(buffer.microphone, buffer.microphone_len),
		Speaker:    parseDeviceInfoVector(buffer.speaker, buffer.speaker_len),
		Camera:     parseDeviceInfoVector(buffer.camera, buffer.camera_len),
		Screen:     parseDeviceInfoVector(buffer.screen, buffer.screen_len),
	}, nil
}

func (ctx *Client) CpuUsage() (float64, error) {
	var buffer C.double
	if err := parseResult(C.ntg_cpu_usage(ctx.handle, &buffer)); err != nil {
		return 0, err
	}
	return float64(buffer), nil
}

//goland:noinspection GoUnusedExportedFunction
func EnableGLibLoop(enable bool) error {
	return parseResult(C.ntg_enable_glib_loop(C.bool(enable)))
}

//goland:noinspection GoUnusedExportedFunction
func Ping() (string, error) {
	var buffer *C.char
	if err := parseResult(C.ntg_ping(&buffer)); err != nil {
		return "", err
	}
	defer C.ntg_string_free(buffer)
	return C.GoString(buffer), nil
}

func (ctx *Client) Calls() (map[int64]CallInfo, error) {
	var buffer *C.ntg_call_info_entry
	var size C.size_t
	if err := parseResult(C.ntg_calls(ctx.handle, &buffer, &size)); err != nil {
		return nil, err
	}
	defer C.ntg_call_info_entry_free(buffer, size)
	result := make(map[int64]CallInfo, int(size))
	for i := 0; i < int(size); i++ {
		entry := *(*C.ntg_call_info_entry)(unsafe.Pointer(uintptr(unsafe.Pointer(buffer)) + uintptr(i)*unsafe.Sizeof(*buffer)))
		result[int64(entry.key)] = CallInfo{
			Playback: parseStreamStatus(entry.value.playback),
			Capture:  parseStreamStatus(entry.value.capture),
		}
	}
	return result, nil
}

func Version() string {
	return C.GoString(C.ntg_get_version())
}

func (ctx *Client) Free() {
	unregisterClient(ctx)
	C.ntg_instance_destroy(ctx.handle)
	ctx.handle = nil
}
