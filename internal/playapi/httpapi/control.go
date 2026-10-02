package httpapi

import (
	"net/http"
	"strconv"
)

func parseChatID(r *http.Request) (int64, string, bool) {
	chatIDStr := r.URL.Query().Get("chatid")
	if chatIDStr == "" {
		return 0, "", false
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return 0, chatIDStr, false
	}
	return chatID, chatIDStr, true
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	chatID, chatIDStr, ok := parseChatID(r)
	if chatIDStr == "" {
		writeError(w, http.StatusBadRequest, "Missing chatid parameter")
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid chatid parameter")
		return
	}

	s.Idle.Cancel(chatID)
	s.Gen.Bump(chatID) // invalidate any in-flight backup-download-and-switch
	if err := s.Voice.Stop(r.Context(), chatID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.activeStream.pop(chatID)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Stopped media", "chatid": chatIDStr})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	chatID, chatIDStr, ok := parseChatID(r)
	if chatIDStr == "" {
		writeError(w, http.StatusBadRequest, "Missing chatid parameter")
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid chatid parameter")
		return
	}
	if err := s.Voice.Pause(r.Context(), chatID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Paused media", "chatid": chatIDStr})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	chatID, chatIDStr, ok := parseChatID(r)
	if chatIDStr == "" {
		writeError(w, http.StatusBadRequest, "Missing chatid parameter")
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid chatid parameter")
		return
	}
	if err := s.Voice.Resume(r.Context(), chatID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Resumed media", "chatid": chatIDStr})
}
