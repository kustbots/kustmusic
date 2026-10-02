/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package sessions

import (
	"encoding/base64"
	"fmt"

	"github.com/amarnathcjd/gogram/telegram"
)

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
		return nil, fmt.Errorf("failed to decode the base64 string: %w", err)
	}

	expectedSize := dcIDSize + apiIDSize + testModeSize + authKeySize + userIDSize + isBotSize
	if len(packedData) != expectedSize {
		return nil, fmt.Errorf("unexpected data length: received %d, expected %d", len(packedData), expectedSize)
	}

	appID := int32(uint32(packedData[1])<<24 | uint32(packedData[2])<<16 | uint32(packedData[3])<<8 | uint32(packedData[4]))
	if appID < 0 {
		return nil, fmt.Errorf("the app ID is invalid: %d", appID)
	}
	return &telegram.Session{
		Hostname: telegram.ResolveDC(int(packedData[0]), packedData[5] != 0, false),
		AppID:    appID,
		Key:      packedData[6 : 6+authKeySize],
	}, nil
}
