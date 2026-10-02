// @generated from schema/ntgcalls.ntl -- DO NOT EDIT
// NOLINTBEGIN
#pragma once

#if defined _WIN32 || defined __CYGWIN__
#ifdef NTG_EXPORTS
#define NTG_C_EXPORT __declspec(dllexport)
#else
#define NTG_C_EXPORT __declspec(dllimport)
#endif
#else
#ifdef NTG_EXPORTS
#define NTG_C_EXPORT __attribute__((visibility("default")))
#else
#define NTG_C_EXPORT
#endif
#endif

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

typedef struct ntg_bytes {
    uint8_t* data;
    size_t len;
} ntg_bytes;

typedef enum {
    NTG_OK = 0,
    NTG_ERR_UNKNOWN = -1,
    NTG_ERR_NULL_POINTER = -2,
    NTG_ERR_RTC = -100,
    NTG_ERR_SDP_PARSE = -101,
    NTG_ERR_TRANSPORT_PARSE = -102,
    NTG_ERR_RTMP_STREAMING_UNSUPPORTED = -103,
    NTG_ERR_CONNECTION = -200,
    NTG_ERR_CONNECTION_NOT_FOUND = -201,
    NTG_ERR_CONNECTION_ERROR = -202,
    NTG_ERR_CRYPTO_ERROR = -203,
    NTG_ERR_TELEGRAM_SERVER_ERROR = -204,
    NTG_ERR_RTC_CONNECTION_NEEDED = -205,
    NTG_ERR_INVALID_PARAMS = -206,
    NTG_ERR_SIGNALING = -300,
    NTG_ERR_SIGNALING_ERROR = -301,
    NTG_ERR_SIGNALING_UNSUPPORTED = -302,
    NTG_ERR_MEDIA = -400,
    NTG_ERR_FILE_ERROR = -401,
    NTG_ERR_FFMPEG_ERROR = -402,
    NTG_ERR_SHELL_ERROR = -403,
    NTG_ERR_MEDIA_DEVICE_ERROR = -404,
} ntg_result;

typedef enum {
    NTG_LOG_DEBUG = 1,
    NTG_LOG_INFO = 2,
    NTG_LOG_WARNING = 4,
    NTG_LOG_ERROR = 8
} ntg_log_level;

typedef enum {
    NTG_LOG_SOURCE_WEBRTC = 1,
    NTG_LOG_SOURCE_SELF = 2
} ntg_log_source;

typedef struct ntg_log_message {
    ntg_log_level level;
    ntg_log_source source;
    const char* file;
    uint32_t line;
    const char* message;
} ntg_log_message;

typedef void (*ntg_log_cb)(ntg_log_message message, void* user_data);

NTG_C_EXPORT void ntg_set_log_callback(ntg_log_cb callback, void* user_data);

NTG_C_EXPORT const char* ntg_get_version(void);

NTG_C_EXPORT const char* ntg_last_error(void);

typedef enum {
    NTG_MEDIA_SOURCE_UNKNOWN,
    NTG_MEDIA_SOURCE_FILE,
    NTG_MEDIA_SOURCE_SHELL,
    NTG_MEDIA_SOURCE_FFMPEG,
    NTG_MEDIA_SOURCE_DEVICE,
    NTG_MEDIA_SOURCE_DESKTOP,
    NTG_MEDIA_SOURCE_EXTERNAL,
} ntg_media_source;

typedef enum {
    NTG_STREAM_TYPE_AUDIO,
    NTG_STREAM_TYPE_VIDEO,
} ntg_stream_type;

typedef enum {
    NTG_STREAM_STATUS_ACTIVE,
    NTG_STREAM_STATUS_PAUSED,
    NTG_STREAM_STATUS_IDLING,
} ntg_stream_status;

typedef enum {
    NTG_STREAM_MODE_CAPTURE,
    NTG_STREAM_MODE_PLAYBACK,
} ntg_stream_mode;

typedef enum {
    NTG_STREAM_DEVICE_MICROPHONE,
    NTG_STREAM_DEVICE_SPEAKER,
    NTG_STREAM_DEVICE_CAMERA,
    NTG_STREAM_DEVICE_SCREEN,
} ntg_stream_device;

typedef enum {
    NTG_CONNECTION_STATE_CONNECTING,
    NTG_CONNECTION_STATE_CONNECTED,
    NTG_CONNECTION_STATE_FAILED,
    NTG_CONNECTION_STATE_TIMEOUT,
    NTG_CONNECTION_STATE_CLOSED,
} ntg_connection_state;

typedef enum {
    NTG_CONNECTION_KIND_NORMAL,
    NTG_CONNECTION_KIND_PRESENTATION,
} ntg_connection_kind;

typedef enum {
    NTG_CALL_TYPE_GROUP,
    NTG_CALL_TYPE_OUTGOING,
    NTG_CALL_TYPE_INCOMING,
    NTG_CALL_TYPE_P2P,
    NTG_CALL_TYPE_CONFERENCE,
} ntg_call_type;

typedef enum {
    NTG_MEDIA_SEGMENT_QUALITY_NONE,
    NTG_MEDIA_SEGMENT_QUALITY_THUMBNAIL,
    NTG_MEDIA_SEGMENT_QUALITY_MEDIUM,
    NTG_MEDIA_SEGMENT_QUALITY_FULL,
} ntg_media_segment_quality;

typedef enum {
    NTG_MEDIA_SEGMENT_PART_STATUS_NOT_READY,
    NTG_MEDIA_SEGMENT_PART_STATUS_RESYNC_NEEDED,
    NTG_MEDIA_SEGMENT_PART_STATUS_DOWNLOADING,
    NTG_MEDIA_SEGMENT_PART_STATUS_SUCCESS,
} ntg_media_segment_part_status;

typedef enum {
    NTG_CONNECTION_MODE_NONE,
    NTG_CONNECTION_MODE_RTC,
    NTG_CONNECTION_MODE_STREAM,
    NTG_CONNECTION_MODE_RTMP,
} ntg_connection_mode;

typedef enum {
    NTG_VIDEO_ROTATION_VIDEO_ROTATION_0,
    NTG_VIDEO_ROTATION_VIDEO_ROTATION_90,
    NTG_VIDEO_ROTATION_VIDEO_ROTATION_180,
    NTG_VIDEO_ROTATION_VIDEO_ROTATION_270,
} ntg_video_rotation;

typedef struct ntg_connection_info {
    ntg_connection_state state;
    ntg_connection_kind kind;
} ntg_connection_info;

typedef struct ntg_remote_source {
    uint32_t ssrc;
    ntg_stream_status state;
    ntg_stream_device device;
} ntg_remote_source;

typedef struct ntg_subchain_request {
    int32_t subchain;
    int32_t height;
    int32_t limit;
} ntg_subchain_request;

typedef struct ntg_audio_description {
    ntg_media_source media_source;
    uint32_t sample_rate;
    uint8_t channel_count;
    char* input;
    bool keep_open;
} ntg_audio_description;

typedef struct ntg_call_info {
    ntg_stream_status playback;
    ntg_stream_status capture;
} ntg_call_info;

typedef struct ntg_device_info {
    char* name;
    char* metadata;
} ntg_device_info;

typedef struct ntg_media_devices {
    ntg_device_info* microphone;
    size_t microphone_len;
    ntg_device_info* speaker;
    size_t speaker_len;
    ntg_device_info* camera;
    size_t camera_len;
    ntg_device_info* screen;
    size_t screen_len;
} ntg_media_devices;

typedef struct ntg_media_state {
    bool muted;
    bool video_paused;
    bool video_stopped;
    bool presentation_paused;
    bool presentation_stopped;
} ntg_media_state;

typedef struct ntg_video_description {
    ntg_media_source media_source;
    int16_t width;
    int16_t height;
    uint8_t fps;
    char* input;
    bool keep_open;
} ntg_video_description;

typedef struct ntg_media_description {
    ntg_audio_description* microphone;
    ntg_audio_description* speaker;
    ntg_video_description* camera;
    ntg_video_description* screen;
} ntg_media_description;

typedef struct ntg_frame_data {
    int64_t absolute_capture_timestamp_ms;
    ntg_video_rotation rotation;
    uint16_t width;
    uint16_t height;
} ntg_frame_data;

typedef struct ntg_ssrc_group {
    char* semantics;
    uint32_t* ssrcs;
    size_t ssrcs_len;
} ntg_ssrc_group;

typedef struct ntg_frame {
    int64_t ssrc;
    uint8_t* data;
    size_t data_len;
    ntg_frame_data frame_data;
} ntg_frame;

typedef struct ntg_segment_part_request {
    int64_t segment_id;
    int32_t part_id;
    int32_t limit;
    int64_t timestamp;
    bool quality_update;
    int32_t channel_id;
    ntg_media_segment_quality quality;
} ntg_segment_part_request;

typedef struct ntg_ssrc_mapping {
    int64_t user_id;
    int32_t ssrc;
} ntg_ssrc_mapping;

typedef struct ntg_auth_params {
    int64_t key_fingerprint;
    uint8_t* g_a_or_b;
    size_t g_a_or_b_len;
} ntg_auth_params;

typedef struct ntg_conference_join_params {
    char* payload;
    uint8_t* public_key;
    size_t public_key_len;
    uint8_t* block;
    size_t block_len;
} ntg_conference_join_params;

typedef struct ntg_dh_config {
    int32_t g;
    uint8_t* p;
    size_t p_len;
    uint8_t* random;
    size_t random_len;
} ntg_dh_config;

typedef struct ntg_protocol {
    int32_t min_layer;
    int32_t max_layer;
    bool udp_p2p;
    bool udp_reflector;
    char** library_versions;
    size_t library_versions_len;
} ntg_protocol;

typedef struct ntg_rtc_server {
    uint64_t id;
    char* ipv4;
    char* ipv6;
    uint16_t port;
    char* username;
    char* password;
    bool turn;
    bool stun;
    bool tcp;
    uint8_t* peer_tag;
    size_t peer_tag_len;
} ntg_rtc_server;

typedef struct ntg_call_info_entry {
    int64_t key;
    ntg_call_info value;
} ntg_call_info_entry;

typedef struct ntg_instance ntg_instance;

NTG_C_EXPORT ntg_instance* ntg_instance_create(void);
NTG_C_EXPORT void ntg_instance_destroy(ntg_instance* handle);

NTG_C_EXPORT void ntg_connection_info_free(ntg_connection_info* value);
NTG_C_EXPORT void ntg_remote_source_free(ntg_remote_source* value);
NTG_C_EXPORT void ntg_subchain_request_free(ntg_subchain_request* value);
NTG_C_EXPORT void ntg_audio_description_free(ntg_audio_description* value);
NTG_C_EXPORT void ntg_call_info_free(ntg_call_info* value);
NTG_C_EXPORT void ntg_device_info_free(ntg_device_info* value);
NTG_C_EXPORT void ntg_media_devices_free(ntg_media_devices* value);
NTG_C_EXPORT void ntg_media_state_free(ntg_media_state* value);
NTG_C_EXPORT void ntg_video_description_free(ntg_video_description* value);
NTG_C_EXPORT void ntg_media_description_free(ntg_media_description* value);
NTG_C_EXPORT void ntg_frame_data_free(ntg_frame_data* value);
NTG_C_EXPORT void ntg_ssrc_group_free(ntg_ssrc_group* value);
NTG_C_EXPORT void ntg_frame_free(ntg_frame* value);
NTG_C_EXPORT void ntg_segment_part_request_free(ntg_segment_part_request* value);
NTG_C_EXPORT void ntg_ssrc_mapping_free(ntg_ssrc_mapping* value);
NTG_C_EXPORT void ntg_auth_params_free(ntg_auth_params* value);
NTG_C_EXPORT void ntg_conference_join_params_free(ntg_conference_join_params* value);
NTG_C_EXPORT void ntg_dh_config_free(ntg_dh_config* value);
NTG_C_EXPORT void ntg_protocol_free(ntg_protocol* value);
NTG_C_EXPORT void ntg_rtc_server_free(ntg_rtc_server* value);
NTG_C_EXPORT void ntg_call_info_entry_free(ntg_call_info_entry* entries, size_t len);
NTG_C_EXPORT void ntg_string_free(char* value);
NTG_C_EXPORT void ntg_bytes_free(void* value);

NTG_C_EXPORT ntg_result ntg_create_p2p_call(
    ntg_instance* handle,
    int64_t user_id
);
NTG_C_EXPORT ntg_result ntg_init_exchange(
    ntg_instance* handle,
    int64_t user_id,
    ntg_dh_config dh_config,
    const uint8_t* ga_hash, size_t ga_hash_len,
    uint8_t** out, size_t* out_len
);
NTG_C_EXPORT ntg_result ntg_exchange_keys(
    ntg_instance* handle,
    int64_t user_id,
    const uint8_t* g_a_or_b, size_t g_a_or_b_len,
    int64_t fingerprint,
    ntg_auth_params* out
);
NTG_C_EXPORT ntg_result ntg_skip_exchange(
    ntg_instance* handle,
    int64_t user_id,
    const uint8_t* encryption_key, size_t encryption_key_len,
    bool is_outgoing
);
NTG_C_EXPORT ntg_result ntg_connect_p2p(
    ntg_instance* handle,
    int64_t user_id,
    const ntg_rtc_server* servers, size_t servers_len,
    const char** versions, size_t versions_len,
    bool p2p_allowed,
    const char* custom_parameters
);
NTG_C_EXPORT ntg_result ntg_create_call(
    ntg_instance* handle,
    int64_t chat_id,
    char** out
);
NTG_C_EXPORT ntg_result ntg_init_presentation(
    ntg_instance* handle,
    int64_t chat_id,
    char** out
);
NTG_C_EXPORT ntg_result ntg_init_conference(
    ntg_instance* handle,
    int64_t chat_id,
    int64_t user_id,
    const uint8_t* last_block, size_t last_block_len,
    ntg_conference_join_params* out
);
NTG_C_EXPORT ntg_result ntg_connect(
    ntg_instance* handle,
    int64_t chat_id,
    const char* params,
    bool is_presentation
);
NTG_C_EXPORT ntg_result ntg_add_incoming_video(
    ntg_instance* handle,
    int64_t chat_id,
    int64_t user_id,
    const char* endpoint,
    const ntg_ssrc_group* ssrc_groups, size_t ssrc_groups_len,
    uint32_t* out
);
NTG_C_EXPORT ntg_result ntg_remove_incoming_video(
    ntg_instance* handle,
    int64_t chat_id,
    const char* endpoint,
    bool* out
);
NTG_C_EXPORT ntg_result ntg_set_stream_sources(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_stream_mode mode,
    ntg_media_description media
);
NTG_C_EXPORT ntg_result ntg_pause(
    ntg_instance* handle,
    int64_t chat_id,
    bool* out
);
NTG_C_EXPORT ntg_result ntg_resume(
    ntg_instance* handle,
    int64_t chat_id,
    bool* out
);
NTG_C_EXPORT ntg_result ntg_mute(
    ntg_instance* handle,
    int64_t chat_id,
    bool* out
);
NTG_C_EXPORT ntg_result ntg_unmute(
    ntg_instance* handle,
    int64_t chat_id,
    bool* out
);
NTG_C_EXPORT ntg_result ntg_stop(
    ntg_instance* handle,
    int64_t chat_id
);
NTG_C_EXPORT ntg_result ntg_stop_presentation(
    ntg_instance* handle,
    int64_t chat_id
);
NTG_C_EXPORT ntg_result ntg_get_emojis_fingerprint(
    ntg_instance* handle,
    int64_t chat_id,
    char** out
);
NTG_C_EXPORT ntg_result ntg_time(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_stream_mode mode,
    uint64_t* out
);
NTG_C_EXPORT ntg_result ntg_get_state(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_media_state* out
);
NTG_C_EXPORT ntg_result ntg_get_call_type(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_call_type* out
);
NTG_C_EXPORT ntg_result ntg_get_connection_mode(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_connection_mode* out
);
NTG_C_EXPORT ntg_result ntg_cpu_usage(
    ntg_instance* handle,
    double* out
);
NTG_C_EXPORT ntg_result ntg_ping(
    char** out
);
NTG_C_EXPORT ntg_result ntg_get_media_devices(
    ntg_media_devices* out
);
NTG_C_EXPORT ntg_result ntg_get_protocol(
    ntg_protocol* out
);
NTG_C_EXPORT ntg_result ntg_enable_glib_loop(
    bool enable
);
NTG_C_EXPORT ntg_result ntg_send_broadcast_timestamp(
    ntg_instance* handle,
    int64_t chat_id,
    int64_t timestamp
);
NTG_C_EXPORT ntg_result ntg_send_broadcast_part(
    ntg_instance* handle,
    int64_t chat_id,
    int64_t segment_id,
    int32_t part_id,
    ntg_media_segment_part_status status,
    bool quality_update,
    const uint8_t* data, size_t data_len
);
NTG_C_EXPORT ntg_result ntg_send_signaling_data(
    ntg_instance* handle,
    int64_t chat_id,
    const uint8_t* msg_key, size_t msg_key_len
);
NTG_C_EXPORT ntg_result ntg_send_external_frame(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_stream_device device,
    const uint8_t* data, size_t data_len,
    ntg_frame_data frame_data
);
NTG_C_EXPORT ntg_result ntg_update_audio_ssrc_mappings(
    ntg_instance* handle,
    int64_t chat_id,
    const ntg_ssrc_mapping* ssrc_groups, size_t ssrc_groups_len
);
NTG_C_EXPORT ntg_result ntg_apply_blocks(
    ntg_instance* handle,
    int64_t chat_id,
    int32_t subchain,
    int32_t next_offset,
    const ntg_bytes* blocks, size_t blocks_len,
    bool from_short_poll
);
NTG_C_EXPORT ntg_result ntg_finish_subchain_request(
    ntg_instance* handle,
    int64_t chat_id,
    int32_t subchain
);
NTG_C_EXPORT ntg_result ntg_calls(
    ntg_instance* handle,
    ntg_call_info_entry** out, size_t* out_len
);
typedef void (*ntg_upgrade_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_media_state state,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_upgrade_callback(ntg_instance* handle, ntg_upgrade_callback_cb callback, void* user_data);

typedef void (*ntg_stream_end_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_stream_type type,
    ntg_stream_device device,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_stream_end_callback(ntg_instance* handle, ntg_stream_end_callback_cb callback, void* user_data);

typedef void (*ntg_connection_change_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_connection_info state,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_connection_change_callback(ntg_instance* handle, ntg_connection_change_callback_cb callback, void* user_data);

typedef void (*ntg_frames_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_stream_mode mode,
    ntg_stream_device device,
    const ntg_frame* frames, size_t frames_len,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_frames_callback(ntg_instance* handle, ntg_frames_callback_cb callback, void* user_data);

typedef void (*ntg_signaling_data_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    const uint8_t* data, size_t data_len,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_signaling_data_callback(ntg_instance* handle, ntg_signaling_data_callback_cb callback, void* user_data);

typedef void (*ntg_remote_source_change_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_remote_source state,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_remote_source_change_callback(ntg_instance* handle, ntg_remote_source_change_callback_cb callback, void* user_data);

typedef void (*ntg_request_broadcast_part_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_segment_part_request request,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_request_broadcast_part_callback(ntg_instance* handle, ntg_request_broadcast_part_callback_cb callback, void* user_data);

typedef void (*ntg_request_broadcast_timestamp_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_request_broadcast_timestamp_callback(ntg_instance* handle, ntg_request_broadcast_timestamp_callback_cb callback, void* user_data);

typedef void (*ntg_request_participants_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_request_participants_callback(ntg_instance* handle, ntg_request_participants_callback_cb callback, void* user_data);

typedef void (*ntg_outbound_block_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    const uint8_t* block, size_t block_len,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_outbound_block_callback(ntg_instance* handle, ntg_outbound_block_callback_cb callback, void* user_data);

typedef void (*ntg_subchain_request_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    ntg_subchain_request subchain_request,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_subchain_request_callback(ntg_instance* handle, ntg_subchain_request_callback_cb callback, void* user_data);

typedef void (*ntg_update_emojis_callback_cb)(
    ntg_instance* handle,
    int64_t chat_id,
    const char* emojis,
    void* user_data
);
NTG_C_EXPORT ntg_result ntg_on_update_emojis_callback(ntg_instance* handle, ntg_update_emojis_callback_cb callback, void* user_data);

#ifdef __cplusplus
}
#endif
// NOLINTEND
