// Package api 提供 HTTP 路由与处理函数。
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/chixi/harbor/internal/config"
	"github.com/chixi/harbor/internal/events"
	"github.com/chixi/harbor/internal/mfa"
	"github.com/chixi/harbor/internal/netx"
	"github.com/chixi/harbor/internal/runner"
	"github.com/chixi/harbor/internal/store"
)

// Server 聚合依赖。
type Server struct {
	Store  *store.Store
	Cfg    *config.Config
	Log    *slog.Logger
	WebDir string
	WebFS  fs.FS
	Hub    *events.Hub
	Runner *runner.Runner
	MFA    *mfa.Engine
}

// New 构造路由。
func New(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	r.Get("/api/health", s.handleHealth)
	r.Route("/api", func(api chi.Router) {
		api.Get("/settings", s.handleGetSettings)
		api.Put("/settings", s.handlePutSettings)

		api.Get("/events", s.handleEvents)
		api.Get("/events/history", s.handleEventHistory)
		api.Get("/balance", s.handleBalance)
		api.Get("/regions", s.handleRegions)
		api.Post("/run/start", s.handleRunStart)
		api.Post("/run/stop", s.handleRunStop)
		api.Get("/run/stat", s.handleRunStat)
		api.Get("/run/items", s.handleRunItems)

		api.Get("/accounts", s.handleListAccounts)
		api.Get("/accounts/stats", s.handleAccountStats)
		api.Get("/accounts/{id}/code", s.handleAccountCode)
		api.Post("/accounts/select", s.handleSelectAll)
		api.Post("/accounts/export", s.handleExport)

		api.Post("/mfa/start", s.handleMFAStart)
		api.Get("/mfa/board", s.handleMFABoard)
		api.Get("/mfa/failed", s.handleMFAFailed)
		api.Post("/mfa/retry", s.handleMFARetry)
	})

	r.Handle("/*", s.static())
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// static 优先用磁盘目录，没有就退回内嵌产物。
func (s *Server) static() http.Handler {
	if s.WebDir != "" {
		files := http.FileServer(http.Dir(s.WebDir))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := filepath.Join(s.WebDir, filepath.Clean(r.URL.Path))
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				if strings.HasPrefix(filepath.ToSlash(r.URL.Path), "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(s.WebDir, "index.html"))
		})
	}
	if s.WebFS != nil {
		files := http.FileServer(http.FS(s.WebFS))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clean := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
			if st, err := fs.Stat(s.WebFS, clean); err == nil && !st.IsDir() {
				// 带 hash 的静态资源可长缓存，免每次重下
				if strings.HasPrefix(clean, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			// 前端路由回退；壳文件每次都校验，发版即生效
			data, err := fs.ReadFile(s.WebFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
		})
	}
	return http.NotFoundHandler()
}

type settingsView struct {
	APIKeyMasked string `json:"api_key_masked"`
	ProxyMasked  string `json:"proxy_masked"`
	HasAPIKey    bool   `json:"has_api_key"`
	ProxyHost    string `json:"proxy_host"`
	Region       string `json:"region"`
	Domain       string `json:"domain"`
	DefaultJobs  int    `json:"default_jobs"`
	DefaultWait  int    `json:"default_wait"`
	AddPassword  bool   `json:"add_password"`
	AddMFA       bool   `json:"add_mfa"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsView{
		APIKeyMasked: config.Mask(st.APIKey),
		ProxyMasked:  config.Mask(st.Proxy),
		HasAPIKey:    st.APIKey != "",
		ProxyHost:    hostOf(st.Proxy),
		Region:       st.Region,
		Domain:       st.Domain,
		DefaultJobs:  st.DefaultJobs,
		DefaultWait:  st.DefaultWait,
		AddPassword:  st.AddPassword,
		AddMFA:       st.AddMFA,
	})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		APIKey      string `json:"api_key"`
		APIKeyUnset bool   `json:"api_key_unset"`
		Proxy       string `json:"proxy"`
		Region      string `json:"region"`
		Domain      string `json:"domain"`
		DefaultJobs int    `json:"default_jobs"`
		DefaultWait int    `json:"default_wait"`
		AddPassword bool   `json:"add_password"`
		AddMFA      bool   `json:"add_mfa"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	cur, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	// 空串表示沿用旧值，避免脱敏值被覆盖。
	if in.APIKey != "" && !in.APIKeyUnset {
		cur.APIKey = in.APIKey
	}
	if in.Proxy != "" {
		cur.Proxy = in.Proxy
	}
	if in.Region != "" {
		cur.Region = strings.ToUpper(in.Region)
	}
	// 地区变了就把代理串里的 region 段改过去
	if cur.Region != "" && cur.Proxy != "" {
		cur.Proxy = netx.WithRegion(cur.Proxy, cur.Region)
	}
	if in.Domain != "" {
		cur.Domain = in.Domain
	}
	if in.DefaultJobs > 0 {
		cur.DefaultJobs = in.DefaultJobs
	}
	if in.DefaultWait >= 0 {
		cur.DefaultWait = in.DefaultWait
	}
	cur.AddPassword = in.AddPassword
	cur.AddMFA = in.AddMFA

	if err := s.Store.PutSettings(r.Context(), cur); err != nil {
		s.fail(w, err)
		return
	}
	// 同步落一份到配置文件，便于人工查看与备份。
	if s.Cfg != nil {
		_ = s.Cfg.Update(config.Snapshot{
			APIKey:      cur.APIKey,
			Proxy:       cur.Proxy,
			Domain:      cur.Domain,
			DefaultJobs: cur.DefaultJobs,
			DefaultWait: cur.DefaultWait,
			AddPassword: cur.AddPassword,
			AddMFA:      cur.AddMFA,
		})
	}
	s.handleGetSettings(w, r)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	s.Hub.Serve(w, r)
}

// handleEventHistory 给刷新后的页面补回最近日志。
func (s *Server) handleEventHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Hub.History())
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Count       int    `json:"count"`
		Threads     int    `json:"threads"`
		Delay       int    `json:"delay"`
		Domain      string `json:"domain"`
		SetPassword *bool  `json:"set_password"`
		EnableMFA   *bool  `json:"enable_mfa"`
		AutoEnqueue *bool  `json:"auto_enqueue"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	st, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if st.APIKey == "" || st.Proxy == "" {
		writeErr(w, http.StatusBadRequest, "还没填 API Key 或代理，先去设置页")
		return
	}
	cfg := runner.Config{
		Count:       in.Count,
		Threads:     in.Threads,
		Delay:       time.Duration(in.Delay) * time.Second,
		Domain:      firstNonEmpty(in.Domain, st.Domain),
		SetPassword: pickBool(in.SetPassword, st.AddPassword),
		EnableMFA:   pickBool(in.EnableMFA, st.AddMFA),
		AutoEnqueue: pickBool(in.AutoEnqueue, true),
		APIKey:      st.APIKey,
		Proxy:       st.Proxy,
		Region:      st.Region,
	}
	if cfg.Count < 1 {
		cfg.Count = 1
	}
	if err := s.Runner.Start(cfg); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRunStop(w http.ResponseWriter, r *http.Request) {
	s.Runner.Stop()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRunStat(w http.ResponseWriter, r *http.Request) {
	st := s.Runner.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"running": st.Running,
		"total":   st.Total,
		"done":    st.Done,
		"ok":      st.OK,
		"fail":    st.Fail,
		"rate":    st.Rate(),
	})
}

func (s *Server) handleRunItems(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Runner.Items())
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func pickBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func hostOf(proxy string) string {
	at := -1
	for i := len(proxy) - 1; i >= 0; i-- {
		if proxy[i] == '@' {
			at = i
			break
		}
	}
	if at >= 0 {
		return proxy[at+1:]
	}
	return proxy
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.Log.Error("api", "err", err)
	writeErr(w, http.StatusInternalServerError, err.Error())
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// Ctx 便于测试注入超时。
func Ctx() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
