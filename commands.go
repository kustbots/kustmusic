package main

import "github.com/kustbots/kustmusic/internal/telegram"

// botCommands is the "/" menu Telegram shows when someone types a slash.
//
// It was four entries — play, end, ping, help — set by hand in BotFather
// long before this rewrite, so most of what the bot actually does was
// invisible to anyone discovering it that way. This publishes the real
// list on every start, which keeps the menu honest as commands change
// instead of drifting again.
//
// Admin-only commands are deliberately left out: they'd show for everyone
// while working for almost nobody, which reads as broken. Group members
// who need them already know them, and /help documents them in full.
var botCommands = []telegram.BotCommand{
	{Command: "play", Description: "Play a song — /play <name or link>"},
	{Command: "skip", Description: "Skip to the next song in the queue"},
	{Command: "pause", Description: "Pause the current song"},
	{Command: "resume", Description: "Resume a paused song"},
	{Command: "stop", Description: "Stop playback and clear the queue"},
	{Command: "queue", Description: "Show what's playing and what's next"},
	{Command: "playlist", Description: "Your saved songs"},
	{Command: "ping", Description: "Check the bot is alive and how fast"},
	{Command: "clone", Description: "Make your own copy of this bot"},
	{Command: "help", Description: "All commands, by category"},
	{Command: "start", Description: "Show the home screen"},
}

// publishCommands pushes the menu for both the default scope and group
// chats. Group chats need their own call: a scope set there once (even to
// an empty list, which is what this bot had) shadows the default, so
// updating only the default would leave groups — where the bot is actually
// used — with nothing.
func (a *App) publishCommands() {
	if err := a.tg.SetMyCommands(botCommands, ""); err != nil {
		a.log.Warn("couldn't publish the default command menu", "err", err)
	}
	if err := a.tg.SetMyCommands(botCommands, "all_group_chats"); err != nil {
		a.log.Warn("couldn't publish the group command menu", "err", err)
	}
}
