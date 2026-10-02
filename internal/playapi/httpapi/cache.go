package httpapi

import "net/http"

// handleCache pre-downloads a song into the engine's cache ahead of time.
// The bot calls it to prefetch the next queued song.
func (s *Server) handleCache(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		writeError(w, http.StatusBadRequest, "Missing url parameter")
		return
	}
	if err := s.Engine.Prefetch(r.Context(), url); err != nil {
		writeError(w, http.StatusInternalServerError, "Download failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Song cached successfully", "url": url})
}
