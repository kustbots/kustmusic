package main

import (
	_ "embed"
	"log/slog"
	"net/http"
)

//go:embed assets/start.jpg
var startImage []byte

// serveAssets serves the start image so Telegram can fetch it by URL.
func serveAssets(port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start.jpg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(startImage)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Warn("asset server stopped", "error", err)
	}
}
