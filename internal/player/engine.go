/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package player

import (
	"context"
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/cache"
	"github.com/kustbots/kustmusic/ntgcalls"
	"slices"
	"strings"
	"time"

	"github.com/AshokShau/gotdbot"
	tg "github.com/amarnathcjd/gogram/telegram"
)

func (c *TelegramCalls) startCallStream(ctx context.Context, acc *AssistantAccount, chatId int64, mediaDesc ntgcalls.MediaDescription) error {
	calls, err := acc.binding.Calls()
	if err != nil {
		return err
	}

	if _, ok := calls[chatId]; ok {
		return acc.binding.SetStreamSources(chatId, ntgcalls.CaptureStream, mediaDesc)
	}

	if err = c.connectCall(ctx, acc, chatId, mediaDesc, ""); err != nil {
		return err
	}

	if chatId < 0 {
		return c.joinPresentation(ctx, acc, chatId, mediaDesc.Screen != nil)
	}

	return nil
}

func (c *TelegramCalls) stopAssistantCall(acc *AssistantAccount, chatId int64, banned bool) error {
	acc.mu.Lock()
	acc.presentations = stdRemove(acc.presentations, chatId)
	delete(acc.pendingConnections, chatId)
	inputGroupCall := acc.inputGroupCalls[chatId]
	acc.mu.Unlock()

	if err := acc.binding.Stop(chatId); err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}

	if banned || inputGroupCall == nil {
		return nil
	}

	_, err := acc.App.PhoneLeaveGroupCall(inputGroupCall, 0)
	return err
}

func (c *TelegramCalls) connectCall(ctx context.Context, acc *AssistantAccount, chatId int64, mediaDesc ntgcalls.MediaDescription, jsonParams string) (err error) {
	connectCh := make(chan error, 1)
	acc.mu.Lock()
	acc.waitConnect[chatId] = connectCh
	acc.mu.Unlock()

	defer func() {
		acc.mu.Lock()
		delete(acc.waitConnect, chatId)
		acc.mu.Unlock()
	}()

	if chatId < 0 {
		if acc.self == nil {
			err = errors.New("assistant is not ready")
			return
		}

		if len(jsonParams) == 0 {
			jsonParams, err = acc.binding.CreateCall(chatId)
			if err != nil {
				_ = acc.binding.Stop(chatId)
				return err
			}
		}

		if err = acc.binding.SetStreamSources(chatId, ntgcalls.CaptureStream, mediaDesc); err != nil {
			_ = acc.binding.Stop(chatId)
			return err
		}

		var inputGroupCall tg.InputGroupCall
		inputGroupCall, err = c.getInputGroupCall(acc, chatId)
		if err != nil {
			_ = acc.binding.Stop(chatId)
			return err
		}

		resultParams := "{\"transport\": null}"
		var callResRaw tg.Updates
		callResRaw, err = acc.App.PhoneJoinGroupCall(
			&tg.PhoneJoinGroupCallParams{
				Muted:        false,
				VideoStopped: mediaDesc.Camera == nil,
				Call:         inputGroupCall,
				Params: &tg.DataJson{
					Data: jsonParams,
				},
				JoinAs: &tg.InputPeerUser{
					UserID:     acc.self.ID,
					AccessHash: acc.self.AccessHash,
				},
			},
		)
		if err != nil {
			return err
		}

		if callRes, ok := callResRaw.(*tg.UpdatesObj); ok {
			for _, update := range callRes.Updates {
				if connUpdate, ok := update.(*tg.UpdateGroupCallConnection); ok {
					resultParams = connUpdate.Params.Data
				}
			}
		}

		if err = acc.binding.Connect(chatId, resultParams, false); err != nil {
			return err
		}

		connectionMode, modeErr := acc.binding.GetConnectionMode(chatId)
		if modeErr != nil {
			return modeErr
		}

		if connectionMode == ntgcalls.StreamConnection && len(jsonParams) > 0 {
			acc.mu.Lock()
			acc.pendingConnections[chatId] = &pendingConnection{
				MediaDesc: mediaDesc,
				Payload:   jsonParams,
			}
			acc.mu.Unlock()
		}
	} else {
		err = errors.New("p2p is not supported")
		return err
	}

	select {
	case err = <-connectCh:
		return err
	case <-ctx.Done():
		err = ctx.Err()
		return err
	case <-time.After(connectWaitTimeout):
		err = fmt.Errorf("connection timeout")
		return err
	}
}

func (c *TelegramCalls) joinPresentation(ctx context.Context, acc *AssistantAccount, chatId int64, join bool) error {
	connectionMode, err := acc.binding.GetConnectionMode(chatId)
	if err != nil {
		return err
	}
	if connectionMode == ntgcalls.StreamConnection || connectionMode != ntgcalls.RtcConnection {
		return nil
	}

	if join {
		acc.mu.RLock()
		already := slices.Contains(acc.presentations, chatId)
		acc.mu.RUnlock()
		if already {
			return nil
		}

		connectCh := make(chan error, 1)
		acc.mu.Lock()
		acc.waitConnect[chatId] = connectCh
		acc.mu.Unlock()

		defer func() {
			acc.mu.Lock()
			delete(acc.waitConnect, chatId)
			acc.mu.Unlock()
		}()

		jsonParams, err := acc.binding.InitPresentation(chatId)
		if err != nil {
			return err
		}

		resultParams := "{\"transport\": null}"
		inputGroupCall, err := c.getInputGroupCall(acc, chatId)
		if err != nil {
			return err
		}

		callResRaw, err := acc.App.PhoneJoinGroupCallPresentation(
			inputGroupCall,
			&tg.DataJson{
				Data: jsonParams,
			},
		)
		if err != nil {
			return err
		}

		if callRes, ok := callResRaw.(*tg.UpdatesObj); ok {
			for _, update := range callRes.Updates {
				if connUpdate, ok := update.(*tg.UpdateGroupCallConnection); ok {
					resultParams = connUpdate.Params.Data
				}
			}
		}

		if err = acc.binding.Connect(chatId, resultParams, true); err != nil {
			return err
		}

		select {
		case err = <-connectCh:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(connectWaitTimeout):
			return fmt.Errorf("presentation connection timeout")
		}

		acc.mu.Lock()
		acc.presentations = append(acc.presentations, chatId)
		acc.mu.Unlock()
		return nil
	}

	acc.mu.RLock()
	inPresentation := slices.Contains(acc.presentations, chatId)
	acc.mu.RUnlock()
	if inPresentation {
		acc.mu.Lock()
		acc.presentations = stdRemove(acc.presentations, chatId)
		acc.mu.Unlock()

		if err = acc.binding.StopPresentation(chatId); err != nil {
			return err
		}

		acc.mu.RLock()
		inputGroupCall := acc.inputGroupCalls[chatId]
		acc.mu.RUnlock()

		if inputGroupCall != nil {
			_, err = acc.App.PhoneLeaveGroupCallPresentation(inputGroupCall)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *TelegramCalls) getInputGroupCall(acc *AssistantAccount, chatId int64) (tg.InputGroupCall, error) {
	acc.mu.RLock()
	call, ok := acc.inputGroupCalls[chatId]
	acc.mu.RUnlock()
	if ok {
		if call == nil {
			return nil, fmt.Errorf("group call for chatId %d is closed", chatId)
		}
		return call, nil
	}

	peer, err := acc.App.ResolvePeer(chatId)
	if err != nil {
		return nil, err
	}
	switch chatPeer := peer.(type) {
	case *tg.InputPeerChannel:
		fullChat, err := acc.App.ChannelsGetFullChannel(
			&tg.InputChannelObj{
				ChannelID:  chatPeer.ChannelID,
				AccessHash: chatPeer.AccessHash,
			},
		)
		if err != nil {
			return nil, err
		}
		acc.mu.Lock()
		acc.inputGroupCalls[chatId] = fullChat.FullChat.(*tg.ChannelFull).Call
		acc.mu.Unlock()
	case *tg.InputPeerChat:
		fullChat, err := acc.App.MessagesGetFullChat(chatPeer.ChatID)
		if err != nil {
			return nil, err
		}
		acc.mu.Lock()
		acc.inputGroupCalls[chatId] = fullChat.FullChat.(*tg.ChatFullObj).Call
		acc.mu.Unlock()
	default:
		return nil, fmt.Errorf("chatId %d is not a group call", chatId)
	}

	acc.mu.RLock()
	defer acc.mu.RUnlock()
	if call, ok := acc.inputGroupCalls[chatId]; ok && call == nil {
		return nil, fmt.Errorf("group call for chatId %d is closed", chatId)
	} else if ok {
		return call, nil
	}

	return nil, fmt.Errorf("group call for chatId %d not found", chatId)
}

func (c *TelegramCalls) setCallStatus(acc *AssistantAccount, call tg.InputGroupCall, state ntgcalls.MediaState) error {
	_, err := acc.App.PhoneEditGroupCallParticipant(
		&tg.PhoneEditGroupCallParticipantParams{
			Call: call,
			Participant: &tg.InputPeerUser{
				UserID:     acc.self.ID,
				AccessHash: acc.self.AccessHash,
			},
			Muted:              state.Muted,
			VideoPaused:        state.VideoPaused,
			VideoStopped:       state.VideoStopped,
			PresentationPaused: state.PresentationPaused,
		},
	)
	return err
}

func (c *TelegramCalls) parseChatId(acc *AssistantAccount, chatId any) (int64, error) {
	if chatId == nil {
		return 0, fmt.Errorf("chatId cannot be nil")
	}

	var parsedChatId int64
	switch v := chatId.(type) {
	case tg.Peer:
		switch p := v.(type) {
		case *tg.PeerUser:
			parsedChatId = p.UserID
		case *tg.PeerChat:
			parsedChatId = -p.ChatID
		case *tg.PeerChannel:
			parsedChatId = -1000000000000 - p.ChannelID
		}
	case int64:
		parsedChatId = v
	case int:
		parsedChatId = int64(v)
	case int32:
		parsedChatId = int64(v)
	case int16:
		parsedChatId = int64(v)
	case int8:
		parsedChatId = int64(v)
	case string:
		rawChat, err := acc.App.ResolveUsername(v)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve username: %w", err)
		}
		switch chat := rawChat.(type) {
		case *tg.UserObj:
			parsedChatId = chat.ID
		case *tg.ChatObj:
			parsedChatId = -chat.ID
		case *tg.Channel:
			parsedChatId = -1000000000000 - chat.ID
		}
	default:
		return 0, fmt.Errorf("unsupported chatId type: %T", chatId)
	}

	switch chatId.(type) {
	case int64, int, int32, int16, int8:
		rawChat, err := acc.App.GetInputPeer(parsedChatId)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve peer: %w", err)
		}
		switch peer := rawChat.(type) {
		case *tg.InputPeerUser:
			parsedChatId = peer.UserID
		case *tg.InputPeerChat:
			parsedChatId = -peer.ChatID
		case *tg.InputPeerChannel:
			parsedChatId = -1000000000000 - peer.ChannelID
		}
	}
	return parsedChatId, nil
}

func (c *TelegramCalls) handleUpdates(acc *AssistantAccount) {
	acc.App.AddRawHandler(&tg.UpdateGroupCallParticipants{}, func(m tg.Update, _ *tg.Client) error {
		return c.onGroupCallParticipants(acc, m)
	})

	acc.App.AddRawHandler(&tg.UpdateGroupCall{}, func(m tg.Update, _ *tg.Client) error {
		return c.onGroupCall(acc, m)
	})

	acc.binding.OnRequestBroadcastTimestamp(func(chatId int64) {
		c.onRequestBroadcastTimestamp(acc, chatId)
	})

	acc.binding.OnRequestBroadcastPart(func(chatId int64, req ntgcalls.SegmentPartRequest) {
		c.onRequestBroadcastPart(acc, chatId, req)
	})

	acc.binding.OnConnectionChange(func(chatId int64, info ntgcalls.NetworkInfo) {
		c.onConnectionChange(acc, chatId, info)
	})

	acc.binding.OnUpgrade(func(chatId int64, state ntgcalls.MediaState) {
		c.onUpgrade(acc, chatId, state)
	})

	acc.binding.OnStreamEnd(func(chatId int64, streamType ntgcalls.StreamType, device ntgcalls.StreamDevice) {
		c.mu.RLock()
		callbacks := c.streamEndCallbacks
		c.mu.RUnlock()
		for _, callback := range callbacks {
			go callback(chatId, streamType, device)
		}
	})
}

func (c *TelegramCalls) onRequestBroadcastTimestamp(acc *AssistantAccount, chatId int64) {
	acc.mu.RLock()
	inputGroupCall := acc.inputGroupCalls[chatId]
	acc.mu.RUnlock()
	if inputGroupCall != nil {
		channels, err := acc.App.PhoneGetGroupCallStreamChannels(inputGroupCall)
		if err == nil && len(channels.Channels) > 0 {
			_ = acc.binding.SendBroadcastTimestamp(chatId, channels.Channels[0].LastTimestampMs)
		}
	}
}

func (c *TelegramCalls) onRequestBroadcastPart(acc *AssistantAccount, chatId int64, req ntgcalls.SegmentPartRequest) {
	acc.mu.RLock()
	inputGroupCall := acc.inputGroupCalls[chatId]
	acc.mu.RUnlock()
	if inputGroupCall != nil {
		file, err := acc.App.UploadGetFile(
			&tg.UploadGetFileParams{
				Location: &tg.InputGroupCallStream{
					Call:         inputGroupCall,
					TimeMs:       req.Timestamp,
					Scale:        0,
					VideoChannel: req.ChannelID,
					VideoQuality: max(int32(req.Quality), 0),
				},
				Offset: 0,
				Limit:  req.Limit,
			},
		)
		status := ntgcalls.SegmentStatusNotReady
		var data []byte
		if err != nil {
			if tg.GetFloodWait(err) == 0 {
				status = ntgcalls.SegmentStatusResyncNeeded
			}
		} else if fileObj, ok := file.(*tg.UploadFileObj); ok {
			data = fileObj.Bytes
			status = ntgcalls.SegmentStatusSuccess
		}
		_ = acc.binding.SendBroadcastPart(
			chatId,
			req.SegmentID,
			req.PartID,
			status,
			req.QualityUpdate,
			data,
		)
	}
}

func (c *TelegramCalls) onGroupCallParticipants(acc *AssistantAccount, m tg.Update) error {
	participantsUpdate := m.(*tg.UpdateGroupCallParticipants)
	inputCall, ok := participantsUpdate.Call.(*tg.InputGroupCallObj)
	if !ok {
		return nil
	}

	chatId, err := c.convertGroupCallId(acc, inputCall.ID)
	if err != nil || acc.self == nil {
		return nil
	}

	for _, participant := range participantsUpdate.Participants {
		participantId := getParticipantId(participant.Peer)
		if participantId != acc.self.ID {
			continue
		}

		connectionMode, err := acc.binding.GetConnectionMode(chatId)
		if err == nil && connectionMode == ntgcalls.StreamConnection && participant.CanSelfUnmute {
			acc.mu.RLock()
			pending := acc.pendingConnections[chatId]
			acc.mu.RUnlock()

			if pending != nil {
				err = c.connectCall(
					context.Background(),
					acc,
					chatId,
					pending.MediaDesc,
					pending.Payload,
				)
				if err != nil {
					acc.App.Log.Warnf("failed to reconnect pending_call: %v", err)
				}
				acc.mu.Lock()
				delete(acc.pendingConnections, chatId)
				acc.mu.Unlock()
			}
			break
		}

		if !participant.CanSelfUnmute {
			acc.mu.Lock()
			if !slices.Contains(acc.mutedByAdmin, chatId) {
				acc.mutedByAdmin = append(acc.mutedByAdmin, chatId)
			}
			acc.mu.Unlock()
			break
		}

		acc.mu.RLock()
		wasMuted := slices.Contains(acc.mutedByAdmin, chatId)
		acc.mu.RUnlock()

		if wasMuted {
			state, stateErr := acc.binding.GetState(chatId)
			if stateErr != nil {
				acc.App.Log.Warnf("failed to get call state: %v", stateErr)
				break
			}

			if statusErr := c.setCallStatus(acc, participantsUpdate.Call, state); statusErr != nil {
				acc.App.Log.Warnf("failed to update call status: %v", statusErr)
				break
			}

			acc.mu.Lock()
			acc.mutedByAdmin = stdRemove(acc.mutedByAdmin, chatId)
			acc.mu.Unlock()
		}
		break
	}

	return nil
}

func (c *TelegramCalls) onGroupCall(acc *AssistantAccount, m tg.Update) error {
	updateGroupCall := m.(*tg.UpdateGroupCall)
	groupCallRaw := updateGroupCall.Call
	if groupCallRaw == nil {
		return nil
	}

	var chatID int64
	var err error

	if updateGroupCall.Peer != nil {
		chatID, err = c.parseChatId(acc, updateGroupCall.Peer)
		if err != nil {
			return err
		}
	} else {
		var callID int64
		switch call := groupCallRaw.(type) {
		case *tg.GroupCallObj:
			callID = call.ID
		case *tg.GroupCallDiscarded:
			callID = call.ID
		}

		if callID != 0 {
			acc.mu.RLock()
			for id, inputCall := range acc.inputGroupCalls {
				if obj, ok := inputCall.(*tg.InputGroupCallObj); ok && obj.ID == callID {
					chatID = id
					break
				}
			}
			acc.mu.RUnlock()
		}
	}

	if chatID == 0 {
		return nil
	}

	switch groupCall := groupCallRaw.(type) {
	case *tg.GroupCallObj:
		acc.mu.Lock()
		acc.inputGroupCalls[chatID] = &tg.InputGroupCallObj{
			ID:         groupCall.ID,
			AccessHash: groupCall.AccessHash,
		}
		acc.mu.Unlock()
		return nil
	case *tg.GroupCallDiscarded:
		acc.mu.Lock()
		delete(acc.inputGroupCalls, chatID)
		acc.mu.Unlock()
		_ = acc.binding.Stop(chatID)
		return nil
	}
	return nil
}

func (c *TelegramCalls) onConnectionChange(acc *AssistantAccount, chatId int64, info ntgcalls.NetworkInfo) {
	acc.mu.RLock()
	waitCh := acc.waitConnect[chatId]
	acc.mu.RUnlock()
	if waitCh == nil {
		return
	}

	var err error
	switch info.State {
	case ntgcalls.Connected:
		err = nil
	case ntgcalls.Closed, ntgcalls.Failed:
		err = fmt.Errorf("connection failed")
	case ntgcalls.Timeout:
		err = fmt.Errorf("connection timeout")
	default:
		return
	}

	select {
	case waitCh <- err:
	default:
	}
}
func (c *TelegramCalls) onUpgrade(acc *AssistantAccount, chatId int64, state ntgcalls.MediaState) {
	acc.App.Logger.Infof("chatId %d, state %+v", chatId, state)

	acc.mu.RLock()
	inputGroupCall := acc.inputGroupCalls[chatId]
	acc.mu.RUnlock()

	if inputGroupCall == nil {
		return
	}

	if err := c.setCallStatus(acc, inputGroupCall, state); err != nil {
		acc.App.Log.Warnf("failed to update call status: %v", err)
	}
}
func (c *TelegramCalls) convertGroupCallId(acc *AssistantAccount, callId int64) (int64, error) {
	acc.mu.RLock()
	defer acc.mu.RUnlock()
	for chatId, inputCallInterface := range acc.inputGroupCalls {
		if inputCall, ok := inputCallInterface.(*tg.InputGroupCallObj); ok {
			if inputCall.ID == callId {
				return chatId, nil
			}
		}
	}
	return 0, fmt.Errorf("group call id %d not found", callId)
}

func getParticipantId(peer tg.Peer) int64 {
	var participantId int64
	switch chatObj := peer.(type) {
	case *tg.PeerUser:
		participantId = chatObj.UserID
	case *tg.PeerChannel:
		participantId = chatObj.ChannelID
	case *tg.PeerChat:
		participantId = chatObj.ChatID
	}
	return participantId
}

func stdRemove[T comparable](slice []T, val T) []T {
	return slices.DeleteFunc(slice, func(e T) bool {
		return e == val
	})
}

// Stop halts media playback in a voice chat and clears the chat's cache.
func (c *TelegramCalls) Stop(chatId int64, banned bool) error {
	c.clearPlayedTimeOffset(chatId)
	cache.ChatCache.SetAutoplay(chatId, false)
	cache.ChatCache.ClearChat(chatId)

	acc, index, err := c.GetAccount(chatId)
	if err != nil {
		return err
	}

	err = c.stopAssistantCall(acc, chatId, banned)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}

		acc.App.Logger.Info("[Stop] Failed to stop the call", "error", err, "index", index)
		return fmt.Errorf("failed to stop call: %w", err)
	}
	return nil
}

// Pause temporarily stops media playback in a voice chat.
func (c *TelegramCalls) Pause(chatId int64) (bool, error) {
	acc, index, err := c.GetAccount(chatId)
	if err != nil {
		return false, err
	}

	res, err := acc.binding.Pause(chatId)
	if err != nil {
		acc.App.Logger.Warn("[Pause] Failed to pause the call", "error", err, "index", index)
		return res, fmt.Errorf("failed to pause: %w", err)
	}
	return res, err
}

// Resume continues a paused media playback in a voice chat.
func (c *TelegramCalls) Resume(chatId int64) (bool, error) {
	acc, index, err := c.GetAccount(chatId)
	if err != nil {
		return false, err
	}

	res, err := acc.binding.Resume(chatId)
	if err != nil {
		logger.Warn("Failed to resume the call", "error", err, "index", index)
		return res, fmt.Errorf("failed to resume: %w", err)
	}

	return res, err
}

// Mute silences the media playback in a voice chat.
func (c *TelegramCalls) Mute(chatId int64) (bool, error) {
	acc, index, err := c.GetAccount(chatId)
	if err != nil {
		return false, err
	}

	res, err := acc.binding.Mute(chatId)
	if err != nil {
		logger.Warn("Failed to mute the call", "error", err, "index", index)
		return res, fmt.Errorf("failed to mute: %w", err)
	}

	return res, err
}

// Unmute restores the audio of a muted media playback in a voice chat.
func (c *TelegramCalls) Unmute(chatId int64) (bool, error) {
	acc, index, err := c.GetAccount(chatId)
	if err != nil {
		return false, err
	}

	res, err := acc.binding.UnMute(chatId)
	if err != nil {
		logger.Warn("Failed to unmute the call", "error", err, "index", index)
		return res, fmt.Errorf("failed to unmute: %w", err)
	}

	return res, err
}

func (c *TelegramCalls) SeekStream(bot *gotdbot.Client, chatId int64, seekSec int) error {
	if seekSec < 0 {
		return errors.New("seek position must be >= 0")
	}

	track := cache.ChatCache.GetPlayingTrack(chatId)
	if track == nil {
		return errors.New("no playing track found in chat")
	}

	currTime, err := c.PlayedTime(chatId)
	if err != nil {
		return fmt.Errorf("failed to get played time: %w", err)
	}

	toSeek := int(currTime) + seekSec

	if toSeek < 0 || track.Duration <= 0 {
		return errors.New("invalid seek position or duration. The position must be positive and the duration must be greater than 0")
	}

	c.setPlayedTimeOffset(chatId, uint64(toSeek))

	ffmpegParams := fmt.Sprintf("-ss %d -to %d", toSeek, track.Duration)

	acc, index, err := c.GetAccount(chatId)
	if err != nil {
		return err
	}

	return c.playMediaWithAssistant(bot, chatId, track.FilePath, track.IsVideo, ffmpegParams, acc, index)
}
