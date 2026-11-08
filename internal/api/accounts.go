package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type accountView struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Secret   string `json:"secret"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	MFAState string `json:"mfa_state"`
	Attempts int    `json:"attempts"`
	Error    string `json:"error"`
	Created  int64  `json:"created_at"`
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	kind := kindOf(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	q := r.URL.Query().Get("q")

	rows, total, err := s.Store.ListAccounts(r.Context(), kind, page, size, q)
	if err != nil {
		s.fail(w, err)
		return
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	if page < 1 {
		page = 1
	}

	out := make([]accountView, 0, len(rows))
	for _, a := range rows {
		out = append(out, accountView{
			ID: a.ID, Email: a.Email, Password: a.Password, Secret: a.TotpSecret,
			Name: a.Name, IP: a.IP, MFAState: a.MFAState, Attempts: a.Attempts,
			Error: a.Error, Created: a.CreatedAt,
		})
	}
	pages := (total + size - 1) / size
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out, "total": total, "page": page, "size": size, "pages": pages,
	})
}

func (s *Server) handleSelectAll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind string `json:"kind"`
		Q    string `json:"q"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	ids, err := s.Store.AllIDs(r.Context(), in.Kind, in.Q)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": ids, "total": len(ids)})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind string  `json:"kind"`
		IDs  []int64 `json:"ids"`
		All  bool    `json:"all"`
		Q    string  `json:"q"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	kind := in.Kind
	if kind == "" {
		kind = "plain"
	}
	ids := in.IDs
	if in.All || len(ids) == 0 {
		var err error
		ids, err = s.Store.AllIDs(r.Context(), kind, in.Q)
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	lines, err := s.Store.ExportLines(r.Context(), kind, ids)
	if err != nil {
		s.fail(w, err)
		return
	}
	name := "accounts-plain"
	if kind == "mfa" {
		name = "accounts-mfa"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.txt"`, name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

func (s *Server) handleAccountStats(w http.ResponseWriter, r *http.Request) {
	mfa, err := s.Store.CountKind(r.Context(), "mfa")
	if err != nil {
		s.fail(w, err)
		return
	}
	plain, err := s.Store.CountKind(r.Context(), "plain")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"mfa": mfa, "plain": plain})
}

func kindOf(r *http.Request) string {
	if r.URL.Query().Get("kind") == "mfa" {
		return "mfa"
	}
	return "plain"
}
