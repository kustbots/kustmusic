package ntgcalls

type RTCServer struct {
	ID                 int64
	Ipv4, Ipv6         string
	Username, Password string
	Port               int32
	Turn, Stun, Tcp    bool
	PeerTag            []byte
}
