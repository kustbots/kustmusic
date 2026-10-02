package playback

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// probeTimeout bounds one ffprobe run. The files involved are ~1-2MB, so a
// healthy probe finishes in well under a second; this only exists so a
// wedged ffprobe can't hold a play request open.
const probeTimeout = 20 * time.Second

// minDurationRatio is how much shorter than the real track a download may
// be before it's treated as truncated. Loose on purpose — a track's
// metadata duration and its audio stream legitimately disagree by a second
// or two, and trailing silence is often trimmed — so this only fires on
// files that are missing a meaningful chunk of the song.
const minDurationRatio = 0.85

// audioDurationSeconds reports how much audio the file at path actually
// contains.
//
// It reads packet timestamps rather than the container's declared duration,
// and that distinction is the whole point. A truncated WebM still carries
// the *original* Duration in its header — that field is written up front,
// before the bytes that never arrived — so "-show_entries format=duration"
// cheerfully reports a full 3:53 for a file that stops at 1:48, which is
// precisely the case this is meant to catch. The timestamp on the last
// packet actually present is the only honest answer.
func audioDurationSeconds(path string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "packet=pts_time",
		"-of", "csv=p=0",
		path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var last float64
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSuffix(strings.TrimSpace(line), ",")
		if line == "" || line == "N/A" {
			continue
		}
		if v, convErr := strconv.ParseFloat(line, 64); convErr == nil && v > last {
			last = v
		}
	}
	if last <= 0 {
		return 0, fmt.Errorf("ffprobe reported no packet timestamps")
	}
	return last, nil
}

// verifyDownloadLength rejects a download holding materially less audio than
// the track really runs for.
//
// This is the check that catches what neither the HTTP layer nor a size
// floor can see: the download API streams chunked, with no Content-Length
// to compare against, and when its own upstream fetch is cut short it still
// closes the stream cleanly. Go sees an ordinary EOF, the short file clears
// every size check, gets cached, and from then on every request for that
// song replays the same stub — the song starts, then stops partway, at the
// exact same second every time.
//
// Probe failures deliberately pass. If ffprobe is missing or confused by a
// container, that is not evidence the audio is bad, and refusing to play
// over it would turn a diagnostic gap into an outage.
func (e *Engine) verifyDownloadLength(path string, expectedSeconds float64) error {
	if expectedSeconds <= 0 {
		return nil // caller didn't say how long the track is; nothing to check
	}
	actual, err := audioDurationSeconds(path)
	if err != nil {
		e.Log.Warn("couldn't probe downloaded audio, accepting it as-is", "path", path, "err", err)
		return nil
	}
	if actual < expectedSeconds*minDurationRatio {
		return fmt.Errorf("truncated download: %.0fs of audio for a %.0fs track", actual, expectedSeconds)
	}
	return nil
}
