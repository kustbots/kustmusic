package ytdlp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain lets this test binary stand in for yt-dlp: when FAKE_YTDLP is set
// it behaves like the real program instead of running tests, so the client
// can be exercised without yt-dlp installed or any network.
func TestMain(m *testing.M) {
	if mode := os.Getenv("FAKE_YTDLP"); mode != "" {
		fakeYTDLP(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeYTDLP(mode string) {
	args := os.Args[1:]
	if f := os.Getenv("FAKE_CALL_LOG"); f != "" {
		if h, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			h.WriteString("call\n")
			h.Close()
		}
	}
	switch mode {
	case "search":
		out, _ := json.Marshal(map[string]any{
			"id": "abc123", "title": strings.Join(args, "|"), "duration": 213.0,
		})
		os.Stdout.Write(out)
	case "flat":
		out, _ := json.Marshal(map[string]any{"entries": []map[string]any{
			{"id": "xyz789", "title": "Flat Song", "duration": 61.0},
		}})
		os.Stdout.Write(out)
	case "unavailable":
		os.Stderr.WriteString("WARNING: noise\nERROR: [youtube] abc: Video unavailable\n")
		os.Exit(1)
	case "flaky":
		os.Stderr.WriteString("ERROR: unable to download webpage: temporary failure\n")
		os.Exit(1)
	case "audio", "image":
		for i, a := range args {
			if a == "-o" && i+1 < len(args) {
				var data []byte
				if mode == "image" {
					data = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 2048)...)
				} else {
					data = make([]byte, 4096)
					for j := range data {
						data[j] = byte(j%250 + 3)
					}
				}
				os.WriteFile(args[i+1], data, 0o600)
			}
		}
	}
}

func fake(t *testing.T, mode string) *Client {
	t.Helper()
	t.Setenv("FAKE_YTDLP", mode)
	return New(Config{Bin: os.Args[0], Concurrency: 2, DownloadTimeout: 10 * time.Second})
}

func TestSearchByNameUsesOneResultSearchAndEndOfOptions(t *testing.T) {
	c := fake(t, "search")
	info, err := c.Search(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if info.Seconds != 213 || info.URL != "https://www.youtube.com/watch?v=abc123" {
		t.Fatalf("unexpected info: %+v", info)
	}
	args := strings.Split(info.Title, "|") // the fake reports its own arguments as the title
	if args[len(args)-1] != "ytsearch1:hello world" || args[len(args)-2] != "--" {
		t.Fatalf("query must be passed after --: %v", args)
	}
}

func TestSearchQueryStartingWithDashCannotBeAFlag(t *testing.T) {
	c := fake(t, "search")
	info, err := c.Search(context.Background(), "--exec rm -rf")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(info.Title, "|")
	if args[len(args)-1] != "ytsearch1:--exec rm -rf" || args[len(args)-2] != "--" {
		t.Fatalf("a leading dash must not become a flag: %v", args)
	}
}

func TestSearchLinkIsNotWrappedInASearch(t *testing.T) {
	c := fake(t, "search")
	info, err := c.Search(context.Background(), "https://youtu.be/abc123")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(info.Title, "|")
	if args[len(args)-1] != "https://youtu.be/abc123" {
		t.Fatalf("link should be passed as is: %v", args)
	}
}

func TestSearchFlatResultTakesFirstEntry(t *testing.T) {
	c := fake(t, "flat")
	info, err := c.Search(context.Background(), "anything")
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "Flat Song" || info.URL != "https://www.youtube.com/watch?v=xyz789" ||
		info.Thumbnail != "https://i.ytimg.com/vi/xyz789/hqdefault.jpg" || info.Seconds != 61 {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	if _, err := New(Config{}).Search(context.Background(), "   "); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDownloadWritesFileAndRunsValidate(t *testing.T) {
	c := fake(t, "audio")
	dest := filepath.Join(t.TempDir(), "song.audio")
	validated := false
	err := c.Download(context.Background(), "https://youtu.be/abc", dest, func(p string) error {
		validated = true
		fi, err := os.Stat(p)
		if err != nil || fi.Size() != 4096 {
			t.Errorf("validate saw a bad file: %v %v", fi, err)
		}
		return nil
	})
	if err != nil || !validated {
		t.Fatalf("download failed: err=%v validated=%v", err, validated)
	}
	if fi, err := os.Stat(dest); err != nil || fi.Size() != 4096 {
		t.Fatalf("destination missing or wrong: %v %v", fi, err)
	}
	if left, _ := filepath.Glob(dest + ".part-*"); len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}

func TestDownloadRejectedByValidateRetriesThenFailsAndLeavesNothing(t *testing.T) {
	c := fake(t, "audio")
	dir := t.TempDir()
	dest := filepath.Join(dir, "song.audio")
	calls := 0
	err := c.Download(context.Background(), "https://youtu.be/abc", dest, func(string) error {
		calls++
		return errors.New("too short")
	})
	if err == nil || calls != downloadRetries {
		t.Fatalf("want %d attempts then an error, got calls=%d err=%v", downloadRetries, calls, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a rejected download must leave nothing behind: %v", entries)
	}
}

func TestDownloadPermanentFailureIsNotRetried(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_CALL_LOG", log)
	c := fake(t, "unavailable")
	err := c.Download(context.Background(), "https://youtu.be/abc", filepath.Join(t.TempDir(), "s"), nil)
	if err == nil || !errors.Is(err, errPermanent) {
		t.Fatalf("want a permanent error, got %v", err)
	}
	if !strings.Contains(Strip(err), "Video unavailable") {
		t.Fatalf("user-facing text should keep yt-dlp's reason: %q", Strip(err))
	}
	data, _ := os.ReadFile(log)
	if n := strings.Count(string(data), "call"); n != 1 {
		t.Fatalf("a permanent failure must not be retried, yt-dlp ran %d times", n)
	}
}

func TestDownloadTransientFailureIsRetried(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_CALL_LOG", log)
	c := fake(t, "flaky")
	err := c.Download(context.Background(), "https://youtu.be/abc", filepath.Join(t.TempDir(), "s"), nil)
	if err == nil || errors.Is(err, errPermanent) {
		t.Fatalf("want a plain error, got %v", err)
	}
	data, _ := os.ReadFile(log)
	if n := strings.Count(string(data), "call"); n != downloadRetries {
		t.Fatalf("want %d attempts, yt-dlp ran %d times", downloadRetries, n)
	}
}

func TestDownloadRejectsAnImageBody(t *testing.T) {
	c := fake(t, "image")
	dest := filepath.Join(t.TempDir(), "s")
	err := c.Download(context.Background(), "https://youtu.be/abc", dest, nil)
	if err == nil || !errors.Is(err, errPermanent) {
		t.Fatalf("a thumbnail must be rejected as permanent, got %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("rejected download must not be published")
	}
}

func TestDownloadRefusesNonLinks(t *testing.T) {
	if err := New(Config{}).Download(context.Background(), "-o /etc/passwd", "x", nil); err == nil {
		t.Fatal("a non-link must be refused before yt-dlp runs")
	}
}

func TestMissingBinaryHasAClearMessage(t *testing.T) {
	c := New(Config{Bin: filepath.Join(t.TempDir(), "no-such-yt-dlp")})
	_, err := c.Search(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("want a not-installed message, got %v", err)
	}
}

func TestErrorTextNeverContainsTheCookiesPath(t *testing.T) {
	t.Setenv("FAKE_YTDLP", "flaky")
	c := New(Config{Bin: os.Args[0], CookiesFile: "/secret/cookies-123.txt"})
	_, err := c.Search(context.Background(), "x")
	if err == nil || strings.Contains(err.Error(), "cookies") {
		t.Fatalf("error must not leak the command line: %v", err)
	}
}

func TestToInfoFallbacks(t *testing.T) {
	r := rawInfo{ID: "id1", Title: "T"}
	i := r.toInfo()
	if i.URL != "https://www.youtube.com/watch?v=id1" || i.Thumbnail != "https://i.ytimg.com/vi/id1/hqdefault.jpg" {
		t.Fatalf("id fallbacks wrong: %+v", i)
	}
	r = rawInfo{ID: "id1", WebpageURL: "https://example.com/w", Thumbnail: "https://example.com/t.jpg"}
	if i := r.toInfo(); i.URL != "https://example.com/w" || i.Thumbnail != "https://example.com/t.jpg" {
		t.Fatalf("explicit fields should win: %+v", i)
	}
	if i := (rawInfo{}).toInfo(); i.URL != "" {
		t.Fatalf("an empty result must stay empty: %+v", i)
	}
}

func TestLastErrorLine(t *testing.T) {
	got := lastErrorLine("WARNING: x\nERROR: first\nERROR: " + strings.Repeat("a", 500) + "\n")
	if len(got) != 200 {
		t.Fatalf("want the last ERROR line cut to 200 chars, got %d", len(got))
	}
	if lastErrorLine("nothing here") != "" {
		t.Fatal("no ERROR line means no message")
	}
}
