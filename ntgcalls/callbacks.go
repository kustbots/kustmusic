package ntgcalls

func (ctx *Client) OnStreamEnd(callback StreamEndCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.streamEndCallbacks = append(ctx.streamEndCallbacks, callback)
}

func (ctx *Client) OnUpgrade(callback UpgradeCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.upgradeCallbacks = append(ctx.upgradeCallbacks, callback)
}

func (ctx *Client) OnConnectionChange(callback ConnectionChangeCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.connectionChangeCallbacks = append(ctx.connectionChangeCallbacks, callback)
}

func (ctx *Client) OnSignal(callback SignalCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.signalCallbacks = append(ctx.signalCallbacks, callback)
}

func (ctx *Client) OnFrame(callback FrameCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.frameCallbacks = append(ctx.frameCallbacks, callback)
}

func (ctx *Client) OnRemoteSourceChange(callback RemoteSourceCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.remoteSourceCallbacks = append(ctx.remoteSourceCallbacks, callback)
}

func (ctx *Client) OnRequestBroadcastTimestamp(callback BroadcastTimestampCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.broadcastTimestampCallbacks = append(ctx.broadcastTimestampCallbacks, callback)
}

func (ctx *Client) OnRequestBroadcastPart(callback BroadcastPartCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.broadcastPartCallbacks = append(ctx.broadcastPartCallbacks, callback)
}

func (ctx *Client) OnUpdateEmojis(callback EmojisCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.emojisCallbacks = append(ctx.emojisCallbacks, callback)
}

func (ctx *Client) OnRequestParticipants(callback RequestParticipantsCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.requestParticipantsCallbacks = append(ctx.requestParticipantsCallbacks, callback)
}

func (ctx *Client) OnOutboundBlock(callback OutboundBlockCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.outboundBlockCallbacks = append(ctx.outboundBlockCallbacks, callback)
}

func (ctx *Client) OnSubchainRequest(callback SubchainRequestCallback) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.subchainRequestCallbacks = append(ctx.subchainRequestCallbacks, callback)
}

func (ctx *Client) copyStreamEndCallbacks() []StreamEndCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]StreamEndCallback(nil), ctx.streamEndCallbacks...)
}

func (ctx *Client) copyUpgradeCallbacks() []UpgradeCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]UpgradeCallback(nil), ctx.upgradeCallbacks...)
}

func (ctx *Client) copyConnectionChangeCallbacks() []ConnectionChangeCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]ConnectionChangeCallback(nil), ctx.connectionChangeCallbacks...)
}

func (ctx *Client) copySignalCallbacks() []SignalCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]SignalCallback(nil), ctx.signalCallbacks...)
}

func (ctx *Client) copyFrameCallbacks() []FrameCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]FrameCallback(nil), ctx.frameCallbacks...)
}

func (ctx *Client) copyRemoteSourceCallbacks() []RemoteSourceCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]RemoteSourceCallback(nil), ctx.remoteSourceCallbacks...)
}

func (ctx *Client) copyBroadcastTimestampCallbacks() []BroadcastTimestampCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]BroadcastTimestampCallback(nil), ctx.broadcastTimestampCallbacks...)
}

func (ctx *Client) copyBroadcastPartCallbacks() []BroadcastPartCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]BroadcastPartCallback(nil), ctx.broadcastPartCallbacks...)
}

func (ctx *Client) copyEmojisCallbacks() []EmojisCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]EmojisCallback(nil), ctx.emojisCallbacks...)
}

func (ctx *Client) copyRequestParticipantsCallbacks() []RequestParticipantsCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]RequestParticipantsCallback(nil), ctx.requestParticipantsCallbacks...)
}

func (ctx *Client) copyOutboundBlockCallbacks() []OutboundBlockCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]OutboundBlockCallback(nil), ctx.outboundBlockCallbacks...)
}

func (ctx *Client) copySubchainRequestCallbacks() []SubchainRequestCallback {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return append([]SubchainRequestCallback(nil), ctx.subchainRequestCallbacks...)
}
