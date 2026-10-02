package ntgcalls

//#include "ntgcalls.h"
import "C"
import "sync"

type Client struct {
	handle                       *C.ntg_instance
	mutex                        sync.RWMutex
	connectionChangeCallbacks    []ConnectionChangeCallback
	streamEndCallbacks           []StreamEndCallback
	upgradeCallbacks             []UpgradeCallback
	signalCallbacks              []SignalCallback
	frameCallbacks               []FrameCallback
	remoteSourceCallbacks        []RemoteSourceCallback
	broadcastTimestampCallbacks  []BroadcastTimestampCallback
	broadcastPartCallbacks       []BroadcastPartCallback
	emojisCallbacks              []EmojisCallback
	requestParticipantsCallbacks []RequestParticipantsCallback
	outboundBlockCallbacks       []OutboundBlockCallback
	subchainRequestCallbacks     []SubchainRequestCallback
}

var (
	clientsMutex sync.RWMutex
	clients      = make(map[*C.ntg_instance]*Client)
)

func registerClient(instance *Client) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	clients[instance.handle] = instance
}

func unregisterClient(instance *Client) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	delete(clients, instance.handle)
}

func lookupClient(handle *C.ntg_instance) *Client {
	clientsMutex.RLock()
	defer clientsMutex.RUnlock()
	return clients[handle]
}
