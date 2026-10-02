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
	"encoding/binary"
	"fmt"
	"net"

	"github.com/amarnathcjd/gogram/telegram"
)

func DecodeTelethonSessionString(sessionString string) (*telegram.Session, error) {
	data, err := base64.URLEncoding.DecodeString(sessionString[1:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %v", err)
	}

	ipLen := 4
	if len(data) == 352 {
		ipLen = 16
	}

	expectedLen := 1 + ipLen + 2 + 256
	if len(data) != expectedLen {
		return nil, fmt.Errorf("invalid session string length")
	}

	offset := 1

	ipData := data[offset : offset+ipLen]
	ip := net.IP(ipData)
	ipAddress := ip.String()
	offset += ipLen

	port := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	var authKey [256]byte
	copy(authKey[:], data[offset:offset+256])

	return &telegram.Session{
		Hostname: ipAddress + ":" + fmt.Sprint(port),
		Key:      authKey[:],
	}, nil
}
