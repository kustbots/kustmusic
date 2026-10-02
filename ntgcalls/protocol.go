package ntgcalls

type Protocol struct {
	MinLayer     int32
	MaxLayer     int32
	UdpP2P       bool
	UdpReflector bool
	Versions     []string
}
