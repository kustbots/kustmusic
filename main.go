/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package main

/*
#cgo linux LDFLAGS: -L . -lntgcalls -lm -lz -lresolv
#cgo darwin LDFLAGS: -L . -lntgcalls -lc++ -lz -lbz2 -liconv -framework AVFoundation -framework AudioToolbox -framework CoreAudio -framework QuartzCore -framework CoreMedia -framework VideoToolbox -framework AppKit -framework Metal -framework MetalKit -framework OpenGL -framework IOSurface -framework ScreenCaptureKit

// Currently is supported only dynamically linked library on Windows due to
// https://github.com/golang/go/issues/63903
#cgo windows LDFLAGS: -L. -lntgcalls
#include "ntgcalls/ntgcalls.h"
#include "glibc_compatibility.h"
*/
import "C"
import (
	"fmt"
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/db"
	"github.com/kustbots/kustmusic/internal/handlers"
	"github.com/kustbots/kustmusic/internal/player"
	"github.com/kustbots/kustmusic/internal/sources"
	"github.com/kustbots/kustmusic/ntgcalls"
	"os"
	"time"

	"github.com/AshokShau/gotdbot"
)

//go:generate go run github.com/AshokShau/gotdbot/scripts/tools

func main() {
	if port := os.Getenv("PORT"); port != "" {
		go serveAssets(port)
	}

	if err := config.LoadEnv(); err != nil {
		panic(err)
	}

	if err := db.InitDatabase(); err != nil {
		panic("failed to connect database: " + err.Error())
	}

	tdDir := "td"
	_ = os.Remove(tdDir)
	libPath := "./libtdjson.so.1.8.67"
	manager := gotdbot.NewClientManager(libPath)
	clientConfig := gotdbot.DefaultClientConfig()
	clientConfig.AutoRetry = &gotdbot.AutoRetry{
		ChatNotFound: true,
		MaxFloodWait: 5 * time.Second,
	}

	clientConfig.DatabaseDirectory = tdDir
	clientConfig.ParseMode = gotdbot.ParseModeHTML
	client, err := manager.RegisterClient(config.ApiId, config.ApiHash, config.Token, clientConfig)
	if err != nil {
		panic("failed to register client: " + err.Error())
	}

	if config.DlBotToken != "" {
		dlClientConfig := gotdbot.DefaultClientConfig()
		dlClientConfig.AutoRetry = &gotdbot.AutoRetry{
			ChatNotFound: true,
		}

		dlClientConfig.DatabaseDirectory = tdDir + "_dl"
		_ = os.Remove(dlClientConfig.DatabaseDirectory)

		dlClient, err := manager.RegisterClient(config.ApiId, config.ApiHash, config.DlBotToken, dlClientConfig)
		if err != nil {
			client.Logger.Warnf("failed to register dl client: %s", err.Error())
			sources.DlBot = client
		} else {
			sources.DlBot = dlClient
			dlClient.Logger.Infof("dl client registered")
		}
	}

	for i, session := range config.SessionStrings {
		err = player.Calls.StartClient(config.ApiId, config.ApiHash, session, fmt.Sprintf("_%d", i))
		if err != nil {
			panic("failed to start client: " + err.Error())
		}
	}

	player.Calls.RegisterHandlers(client)
	handlers.LoadModules(client)
	msg := fmt.Sprintf("Bot started\nNtgCalls %s", ntgcalls.Version())
	client.Logger.Info(msg)
	if config.LoggerId != 0 {
		_, _ = client.SendTextMessage(config.LoggerId, msg, nil)
	}
	manager.Idle()
	client.Logger.Info("The bot is shutting down...")
	player.Calls.StopAllClients()
	_ = os.Remove(config.DownloadsDir)
}
