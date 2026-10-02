# KustMusic

A Telegram music bot that streams audio and video into group video chats. Written in Go on top of TDLib and NTgCalls.

![KustMusic](assets/start.jpg)

## Features

- Plays from YouTube by name or link, plus Telegram audio and video files
- Player card with artwork, and pause, skip, stop and mute buttons
- Queue, loop, seek, mixes and autoplay
- Personal playlists
- Up to 10 assistant accounts, rotated automatically
- YouTube cookies with a retry ladder: every cookie file is tried in turn, then cookieless fallbacks

## Requirements

| Thing | Where to get it |
|-------|-----------------|
| `API_ID`, `API_HASH` | [my.telegram.org](https://my.telegram.org) |
| `TOKEN` | [@BotFather](https://t.me/BotFather) |
| `STRING1` | A Pyrogram, Telethon or Gogram session string for the assistant account |
| `MONGO_URI` | A MongoDB connection string |
| `OWNER_ID` | Your numeric Telegram user ID |

Copy `sample.env` to `.env` and fill it in. Every other variable in that file is optional.

## Cookies

YouTube usually blocks downloads from server IPs unless cookies are supplied. Put one or more Netscape-format cookie files in `cookies/` as `*.txt` before building, or set `COOKIES_URL` to a comma-separated list of paste links. See [docs/cookies.md](docs/cookies.md) for how to export them.

Cookie files are ignored by git. Do not commit them.

## Run with Docker

```bash
docker compose up --build
```

## Deploy to Heroku

The app uses the container stack and a single `web` dyno. The web process also serves the start banner at `/start.jpg`.

```bash
heroku stack:set container -a your-app
```

```bash
git push heroku main
```

Then set the config vars from the table above.

## Layout

| Path | What it holds |
|------|---------------|
| `main.go`, `assets.go` | Start-up and the small asset server |
| `internal/handlers` | Commands, callbacks and chat events |
| `internal/player` | Voice chat engine, assistants and stream control |
| `internal/sources` | Search and download for each platform |
| `internal/utils` | Player cards, keyboards and shared types |
| `internal/cache`, `internal/db` | In-memory queues and MongoDB storage |
| `internal/config` | Environment and cookie loading |
| `ntgcalls` | Go bindings for NTgCalls |

## Community

- Updates: [t.me/kustbots](https://t.me/kustbots)
- Support: [t.me/kustbhai](https://t.me/kustbhai)
- Community: [t.me/kustbotschat](https://t.me/kustbotschat)

## License

GNU GPL v3, see [LICENSE](LICENSE). KustMusic is a modified version of TgMusicBot, copyright (c) 2025-2026 Ashok Shau, and keeps the original copyright notices in the source files as the license requires.
