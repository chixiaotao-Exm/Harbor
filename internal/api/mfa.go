package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/chixi/harbor/internal/flow"
	"github.com/chixi/harbor/internal/mfa"
	"github.com/chixi/harbor/internal/store"
)

// handleAccountCode 现算某个账号的 6 位动态码。
func (s *Server) handleAccountCode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id 不合法")
		return
	}
	acc, err := s.Store.GetAccount(r.Context(), id)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if acc.TotpSecret == "" {
		writeErr(w, http.StatusBadRequest, "这个账号还没绑双因子")
		return
	}
	code, err := totpCode(acc.TotpSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "密钥无法解析")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":  code,
		"left":  secondsLeft(),
		"email": acc.Email,
	})
}

// totpCode / secondsLeft 由 flow 与时间推出，测试可替换。
var (
	totpCode    = func(secret string) (string, error) { return flow.TOTPCode(secret) }
	secondsLeft = func() int { return 30 - int(time.Now().Unix()%30) }
)

// handleMFAStart 投递异步任务。
func (s *Server) handleMFAStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs   []int64 `json:"ids"`
		All   bool    `json:"all"`
		Proxy string  `json:"proxy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	ids := in.IDs
	if in.All || len(ids) == 0 {
		var err error
		ids, err = s.Store.PendingMFAIDs(r.Context())
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	proxy := in.Proxy
	if proxy == "" {
		if st, err := s.Store.GetSettings(r.Context()); err == nil {
			proxy = st.Proxy
		}
	}
	n, err := s.MFA.Start(r.Context(), ids, proxy)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "queued": n})
}

func (s *Server) handleMFABoard(w http.ResponseWriter, r *http.Request) {
	rows := s.MFA.Board()
	out := make([]mfa.BoardRow, 0, 16)
	for q := mfa.Q1; q <= mfa.Q5; q++ {
		out = append(out, rows[q]...)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running": s.MFA.Running(),
		"rows":    out,
	})
}

func (s *Server) handleMFAFailed(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.FailedMFA(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleMFARetry(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	ids := in.IDs
	if in.All || len(ids) == 0 {
		list, err := s.Store.FailedMFA(r.Context())
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, f := range list {
			ids = append(ids, f.ID)
		}
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "queued": 0})
		return
	}
	// 失败的要先清状态才好重投。
	for _, id := range ids {
		_ = s.Store.SetMFAState(r.Context(), id, "none", "")
	}
	n, err := s.MFA.Start(r.Context(), ids, "")
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "queued": n})
}
