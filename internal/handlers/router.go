/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package handlers

import (
	"time"

	"github.com/AshokShau/gotdbot"
	"github.com/AshokShau/gotdbot/filters/callbackquery"
)

var startTime = time.Now()

func LoadModules(c *gotdbot.Client) {
	c.OnCommand("reload", reloadAdminCacheHandler)
	c.OnCommand("join", joinHandler)
	c.OnCommand("link", joinHandler)
	c.OnCommand("authList", authListHandler)
	c.OnCommand("auths", authListHandler)
	c.OnCommand("auth", addAuthHandler)
	c.OnCommand("addAuth", addAuthHandler)
	c.OnCommand("removeAuth", removeAuthHandler)
	c.OnCommand("rmAuth", removeAuthHandler)
	c.OnCommand("broadcast", broadcastHandler)
	c.OnCommand("gCast", broadcastHandler)
	c.OnCommand("stop_gcast", cancelBroadcastHandler)
	c.OnCommand("stop_broadcast", cancelBroadcastHandler)
	c.OnCommand("av", activeVcHandler)
	c.OnCommand("active_vc", activeVcHandler)
	c.OnCommand("clearass", clearAssistantsHandler)
	c.OnCommand("clearAssistants", clearAssistantsHandler)
	c.OnCommand("leaveAll", leaveAllHandler)
	c.OnCommand("logger", loggerHandler)
	c.OnCommand("privacy", privacyHandler)
	c.OnCommand("autoplay", autoplayHandler)
	c.OnCommand("loop", loopHandler)
	c.OnCommand("pause", pauseHandler)
	c.OnCommand("resume", resumeHandler)
	c.OnCommand("cplist", createPlaylistHandler)
	c.OnCommand("createplaylist", createPlaylistHandler)
	c.OnCommand("deleteplaylist", deletePlaylistHandler)
	c.OnCommand("queue", queueHandler)
	c.OnCommand("seek", seekHandler)
	c.OnCommand("sh", shellCommand)
	c.OnCommand("skip", skipHandler)
	c.OnCommand("stop", stopHandler)
	c.OnCommand("end", stopHandler)
	c.OnCommand("start", startHandler)
	c.OnCommand("help", startHandler)
	c.OnCommand("ping", pingHandler)
	c.OnCommand("mix", mixHandler)
	c.OnCommand("play", playHandler)
	c.OnCommand("p", playHandler)
	c.OnCommand("fplay", fPlayHandler)
	c.OnCommand("fp", fPlayHandler)
	c.OnCommand("vplay", vPlayHandler)
	c.OnCommand("v", vPlayHandler)
	c.OnCommand("fvplay", fVPlayHandler)
	c.OnCommand("fvp", fVPlayHandler)
	c.OnCommand("remove", removeHandler)
	c.OnCommand("mute", muteHandler)
	c.OnCommand("unmute", unmuteHandler)
	c.OnCommand("settings", settingsHandler)
	c.OnCommand("addtoplaylist", addToPlaylistHandler)
	c.OnCommand("addtoplist", addToPlaylistHandler)
	c.OnCommand("removefromplaylist", removeFromPlaylistHandler)
	c.OnCommand("rmplist", removeFromPlaylistHandler)
	c.OnCommand("plistinfo", playlistInfoHandler)
	c.OnCommand("playlistinfo", playlistInfoHandler)
	c.OnCommand("myplaylists", myPlaylistsHandler)
	c.OnCommand("myplist", myPlaylistsHandler)
	c.OnCommand("stats", statsHandler)

	c.OnUpdateNewCallbackQuery(helpCallbackHandler, callbackquery.Prefix("help_"))
	c.OnUpdateNewCallbackQuery(playCallbackHandler, callbackquery.Prefix("play_"))
	c.OnUpdateNewCallbackQuery(vcPlayHandler, callbackquery.Prefix("vcplay_"))
	c.OnUpdateNewCallbackQuery(settingsCallbackHandler, callbackquery.Prefix("settings_"))
	c.OnUpdateNewCallbackQuery(autoplayCallbackHandler, callbackquery.Equal("autoplay_toggle"))

	c.OnUpdateChatMember(handleParticipant, nil)
	c.OnUpdateNewChat(handleNewChat, nil)
	c.OnUpdateNewMessage(handleVoiceChatMessage, nil)

	c.Logger.Debug("Handlers loaded successfully")
}
