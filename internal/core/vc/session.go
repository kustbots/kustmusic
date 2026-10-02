package vc

import (
	"encoding/base64"
	"fmt"

	"github.com/amarnathcjd/gogram/telegram"
)

// DecodePyrogramSessionString decodes a Pyrogram-format string session into
// gogram's Session format. Adapted from gogram's own official example
// (examples/sessions/pyrogram/main.go) — verified this session against a
// real production ASSISTANT_SESSION earlier in this project and confirmed
// it resolves to the correct account (see the project's Milestone 0 check).
func DecodePyrogramSessionString(encodedString string) (*telegram.Session, error) {
	const (
		dcIDSize     = 1
		apiIDSize    = 4
		testModeSize = 1
		authKeySize  = 256
		userIDSize   = 8
		isBotSize    = 1
	)

	for len(encodedString)%4 != 0 {
		encodedString += "="
	}

	packedData, err := base64.URLEncoding.DecodeString(encodedString)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 string: %w", err)
	}

	expectedSize := dcIDSize + apiIDSize + testModeSize + authKeySize + userIDSize + isBotSize
	if len(packedData) != expectedSize {
		return nil, fmt.Errorf("unexpected data length: got %d, want %d", len(packedData), expectedSize)
	}

	return &telegram.Session{
		Hostname: telegram.ResolveDC(int(uint8(packedData[0])), packedData[5] != 0, false),
		AppID:    int32(uint32(packedData[1])<<24 | uint32(packedData[2])<<16 | uint32(packedData[3])<<8 | uint32(packedData[4])),
		Key:      packedData[6 : 6+authKeySize],
	}, nil
}
