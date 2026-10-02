// Package assets holds pictures compiled into the binary.
package assets

import _ "embed"

// DP is the bot's profile picture, also used on the home screen.
//
//go:embed dp.jpg
var DP []byte
