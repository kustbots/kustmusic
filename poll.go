package main

import "time"

// pollUpdates fetches updates by long polling. It runs when WEBHOOK_URL is
// not set, which is the easy way to run the bot at home or on a VPS with no
// public address.
func (a *App) pollUpdates() {
	var offset int64
	for {
		updates, err := a.tg.GetUpdates(offset, 15)
		if err != nil {
			a.log.Warn("getUpdates failed, retrying", "err", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			go a.processUpdate(u)
		}
	}
}
