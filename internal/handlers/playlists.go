/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package handlers

import (
	"fmt"
	"github.com/kustbots/kustmusic/internal/db"
	"github.com/kustbots/kustmusic/internal/sources"
	"html"
	"strconv"
	"strings"

	td "github.com/AshokShau/gotdbot"
)

func createPlaylistHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	userID := m.SenderID()

	args := Args(m)
	if args == "" {
		_, err := m.ReplyText(c, "<b>Usage:</b> /createplaylist [playlist name]", replyOpts)
		return err
	}

	userPlaylists, err := db.Instance.GetUserPlaylists(userID)
	if err != nil {
		_, err = m.ReplyText(c, "Unable to fetch your playlists. Please try again later.", nil)
		return err
	}

	if len(userPlaylists) >= 10 {
		_, _ = m.ReplyText(c, "You have reached the maximum limit of 10 playlists.", nil)
		return td.EndGroups
	}

	if len([]rune(args)) > 40 {
		args = string([]rune(args)[:40])
	}

	playlistID, err := db.Instance.CreatePlaylist(args, userID)
	if err != nil {
		_, err = m.ReplyText(c, fmt.Sprintf("Failed to create playlist: %s", err.Error()), nil)
		return err
	}

	_, err = m.ReplyText(
		c,
		fmt.Sprintf(
			"Playlist <b>%s</b> has been created successfully.\nID: <code>%s</code>",
			args,
			playlistID,
		),
		replyOpts,
	)

	return td.EndGroups
}

func deletePlaylistHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	userID := m.SenderID()

	args := Args(m)
	if args == "" {
		_, err := m.ReplyText(
			c,
			"<b>Usage:</b> /deleteplaylist [playlist id]",
			&td.SendTextMessageOpts{ParseMode: "HTML"},
		)
		return err
	}

	playlist, err := db.Instance.GetPlaylist(args)
	if err != nil {
		_, err := m.ReplyText(
			c,
			"The specified playlist could not be found. Please check the playlist ID.",
			nil,
		)
		return err
	}

	if playlist.UserID != userID {
		_, err := m.ReplyText(
			c,
			"You can only delete playlists that you created.",
			nil,
		)
		return err
	}

	err = db.Instance.DeletePlaylist(args, userID)
	if err != nil {
		_, err := m.ReplyText(
			c,
			fmt.Sprintf("Failed to delete the playlist: %s", err.Error()),
			nil,
		)
		return err
	}

	_, err = m.ReplyText(
		c,
		fmt.Sprintf("Playlist <b>%s</b> has been deleted successfully.", playlist.Name),
		&td.SendTextMessageOpts{ParseMode: "HTML"},
	)

	return err
}
func addToPlaylistHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	userID := m.SenderID()

	args := strings.SplitN(Args(m), " ", 2)
	if len(args) != 2 {
		_, err := m.ReplyText(
			c,
			"<b>Usage:</b> /addtoplaylist [playlist id] [song url]",
			&td.SendTextMessageOpts{ParseMode: "HTML"},
		)
		return err
	}

	playlistID := args[0]
	songURL := args[1]

	playlist, err := db.Instance.GetPlaylist(playlistID)
	if err != nil {
		_, err := m.ReplyText(
			c,
			"The specified playlist could not be found. Please verify the playlist ID.",
			nil,
		)
		return err
	}

	if playlist.UserID != userID {
		_, err := m.ReplyText(
			c,
			"You can only modify playlists that you created.",
			nil,
		)
		return err
	}

	wrapper := sources.NewDlWrapper(songURL)
	if !wrapper.IsValid() {
		_, err := m.ReplyText(
			c,
			"The provided URL is invalid or the platform is not supported.",
			nil,
		)
		return err
	}

	trackInfo, err := wrapper.GetInfo()
	if err != nil {
		_, err := m.ReplyText(
			c,
			fmt.Sprintf("Unable to retrieve track information: %s", err.Error()),
			nil,
		)
		return err
	}

	if trackInfo.Results == nil || len(trackInfo.Results) == 0 {
		_, err := m.ReplyText(
			c,
			"No playable tracks were found for the provided link.",
			nil,
		)
		return err
	}

	song := db.Song{
		URL:      trackInfo.Results[0].Url,
		Name:     trackInfo.Results[0].Title,
		TrackID:  trackInfo.Results[0].Id,
		Duration: trackInfo.Results[0].Duration,
		Platform: trackInfo.Results[0].Platform,
	}

	err = db.Instance.AddSongToPlaylist(playlistID, song)
	if err != nil {
		_, err := m.ReplyText(
			c,
			fmt.Sprintf("Failed to add the track to the playlist: %s", err.Error()),
			nil,
		)
		return err
	}

	_, err = m.ReplyText(
		c,
		fmt.Sprintf(
			"Track <b>%s</b> has been added to playlist <b>%s</b>.",
			song.Name,
			playlist.Name,
		),
		replyOpts,
	)

	return err
}

func removeFromPlaylistHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	userID := m.SenderID()

	args := strings.SplitN(Args(m), " ", 2)
	if len(args) != 2 {
		_, err := m.ReplyText(
			c,
			"<b>Usage:</b> /removefromplaylist [playlist id] [song number or url]",
			&td.SendTextMessageOpts{ParseMode: "HTML"},
		)
		return err
	}

	playlistID := args[0]
	songIdentifier := args[1]

	playlist, err := db.Instance.GetPlaylist(playlistID)
	if err != nil {
		_, err = m.ReplyText(c, "Playlist not found.", nil)
		return err
	}

	if playlist.UserID != userID {
		_, err = m.ReplyText(c, "You do not own this playlist.", nil)
		return err
	}

	songIndex, err := strconv.Atoi(songIdentifier)
	var trackID string

	if err == nil {
		if songIndex < 1 || songIndex > len(playlist.Songs) {
			_, err := m.ReplyText(c, "Invalid song number.", nil)
			return err
		}
		trackID = playlist.Songs[songIndex-1].TrackID
	} else {
		for _, song := range playlist.Songs {
			if song.URL == songIdentifier || song.TrackID == songIdentifier {
				trackID = song.TrackID
				break
			}
		}
	}

	if trackID == "" {
		_, err = m.ReplyText(c, "Song not found in playlist.", nil)
		return err
	}

	err = db.Instance.RemoveSongFromPlaylist(playlistID, trackID)
	if err != nil {
		_, err = m.ReplyText(c, fmt.Sprintf("Error removing song: %s", err.Error()), nil)
		return err
	}

	_, err = m.ReplyText(c, fmt.Sprintf("Song removed from playlist '%s'.", playlist.Name), nil)
	return err
}

func playlistInfoHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	args := Args(m)
	if args == "" {
		_, err := m.ReplyText(
			c,
			"<b>Usage:</b> /playlistinfo [playlist id]",
			&td.SendTextMessageOpts{ParseMode: "HTML"},
		)
		return err
	}

	playlist, err := db.Instance.GetPlaylist(args)
	if err != nil {
		_, err = m.ReplyText(c, "Playlist not found.", nil)
		return err
	}

	ownerName := "Unknown"
	if owner, err := c.GetUser(playlist.UserID); err == nil && owner != nil {
		ownerName = owner.FirstName
	}

	var songs []string
	for i, song := range playlist.Songs {
		songs = append(songs, fmt.Sprintf("%d. %s (%s)", i+1, song.Name, song.URL))
	}

	text := fmt.Sprintf(
		"<b>Playlist Info</b>\n\n<b>Name:</b> %s\n<b>Owner:</b> %s\n<b>Songs:</b> %d\n\n%s",
		playlist.Name,
		ownerName,
		len(playlist.Songs),
		strings.Join(songs, "\n"),
	)

	if len([]rune(text)) > 3000 {
		var sb strings.Builder
		sb.WriteString("<h5>🎵 Playlist Info</h5>")
		sb.WriteString(fmt.Sprintf("<p><b>Name:</b> %s<br><b>Owner:</b> %s<br><b>Songs:</b> %d</p>", html.EscapeString(playlist.Name), html.EscapeString(ownerName), len(playlist.Songs)))
		sb.WriteString("<details>")
		sb.WriteString("<summary><b>📜 Click to View Songs</b></summary>")
		sb.WriteString("<br>")
		sb.WriteString("<table bordered striped>")
		sb.WriteString("<tr>")
		sb.WriteString("<th align='center'><b>#</b></th>")
		sb.WriteString("<th align='left'><b>Song Title</b></th>")
		sb.WriteString("</tr>")

		for i, song := range playlist.Songs {
			trackName := html.EscapeString(song.Name)
			trackURL := html.EscapeString(song.URL)
			var trackLink string
			if trackURL != "" {
				trackLink = fmt.Sprintf("<a href='%s'>%s</a>", trackURL, trackName)
			} else {
				trackLink = trackName
			}

			sb.WriteString("<tr>")
			sb.WriteString(fmt.Sprintf("<td align='center'>%d</td>", i+1))
			sb.WriteString(fmt.Sprintf("<td align='left'>%s</td>", trackLink))
			sb.WriteString("</tr>")
		}
		sb.WriteString("</table>")
		sb.WriteString("</details>")

		richMessage := &td.InputRichMessage{Source: &td.RichMessageSourceHtml{Text: sb.String()}}
		_, err = m.ReplyRichMessage(c, richMessage, nil)
		return err
	}

	_, err = m.ReplyText(c, text, &td.SendTextMessageOpts{ParseMode: "HTML"})
	return err
}

func myPlaylistsHandler(c *td.Client, m *td.Message) error {
	deleteCmd(c, m)

	userID := m.SenderID()

	playlists, err := db.Instance.GetUserPlaylists(userID)
	if err != nil {
		_, err := m.ReplyText(c, fmt.Sprintf("Error fetching playlists: %s", err.Error()), nil)
		return err
	}

	if len(playlists) == 0 {
		_, err := m.ReplyText(c, "You do not have any playlists.", nil)
		return err
	}

	var playlistInfo []string
	for _, playlist := range playlists {
		playlistInfo = append(
			playlistInfo,
			fmt.Sprintf("- %s (<code>%s</code>)", playlist.Name, playlist.ID),
		)
	}

	_, err = m.ReplyText(
		c,
		fmt.Sprintf("<b>My Playlists</b>\n\n%s", strings.Join(playlistInfo, "\n")),
		&td.SendTextMessageOpts{ParseMode: "HTML"},
	)

	return err
}
