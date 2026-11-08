package api

import (
	"net/http"

	"github.com/chixi/harbor/internal/mailbox"
	"github.com/chixi/harbor/internal/netx"
)

// handleBalance 查接码平台余额。
func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if st.APIKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "还没填 API Key"})
		return
	}
	raw, err := mailbox.New(st.APIKey, st.Domain, "").Balance(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "raw": raw})
}

// handleRegions 下发可选地区。
func (s *Server) handleRegions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"regions": netx.Regions})
}
