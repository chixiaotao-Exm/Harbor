package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/chixi/harbor/internal/flow"
	"github.com/chixi/harbor/internal/netx"
)

// sessionSummary 是从 /api/auth/session 里挑出来给界面看的字段。
type sessionSummary struct {
	Plan     string `json:"plan"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	IP       string `json:"ip"`
	Expires  string `json:"expires"`
	Email    string `json:"email"`
	DeviceID string `json:"device_id"`
}

// sessionRow 是单个账号的刷新结果。
type sessionRow struct {
	ID      int64           `json:"id"`
	Email   string          `json:"email"`
	OK      bool            `json:"ok"`
	Summary *sessionSummary `json:"summary,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// handleAccountSession 用账号自己的密码+双因子登录，取 /api/auth/session。
//
// 拿到的是账号真实数据：套餐、国家/地区、真实出口 IP、令牌与到期时间。
func (s *Server) handleAccountSession(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs     []int64 `json:"ids"`
		All     bool    `json:"all"`
		Workers int     `json:"workers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	ids := in.IDs
	if in.All || len(ids) == 0 {
		ids = nil
		for _, kind := range []string{"mfa", "plain"} {
			part, err := s.Store.AllIDs(r.Context(), kind, "")
			if err != nil {
				s.fail(w, err)
				return
			}
			ids = append(ids, part...)
		}
	}
	if in.Workers < 1 || in.Workers > 4 {
		in.Workers = 2
	}

	out := make([]sessionRow, len(ids))
	sem := make(chan struct{}, in.Workers)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = s.refreshSession(r.Context(), id)
		}(i, id)
	}
	wg.Wait()

	ok := 0
	for _, row := range out {
		if row.OK {
			ok++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": ok, "failed": len(out) - ok, "rows": out,
	})
}

// refreshSession 对单个账号执行一次会话刷新。
func (s *Server) refreshSession(ctx context.Context, id int64) sessionRow {
	res := sessionRow{ID: id}
	acc, err := s.Store.GetAccount(ctx, id)
	if err != nil {
		res.Error = "账号不存在"
		return res
	}
	res.Email = acc.Email
	if acc.Password == "" {
		res.Error = "没有密码"
		return res
	}

	proxy := acc.Proxy
	if proxy == "" {
		if st, e := s.Store.GetSettings(ctx); e == nil {
			proxy = netx.WithRegion(st.Proxy, st.Region)
		}
	}

	sess, err := flow.New(ctx, proxy)
	if err != nil {
		res.Error = "代理不可用: " + err.Error()
		return res
	}
	defer sess.Close()

	token, err := flow.PasswordLoginTOTP(ctx, sess, acc.Email, acc.Password, acc.TotpSecret)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	code, raw, err := sess.Fetch(ctx, "https://chatgpt.com/api/auth/session")
	if err != nil {
		res.Error = "取会话失败: " + err.Error()
		return res
	}
	if code != 200 {
		res.Error = "取会话 HTTP " + strconv.Itoa(code)
		return res
	}
	sum, err := parseSession(raw)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	_ = s.Store.SetAccountSession(ctx, id, string(raw), time.Now().Unix())
	if sum.Country != "" {
		_ = s.Store.SetAccountCountry(ctx, id, sum.Country)
	}
	if token != "" {
		_ = s.Store.SetAccountToken(ctx, id, token)
	}

	res.OK = true
	res.Summary = sum
	return res
}

func parseSession(raw []byte) (*sessionSummary, error) {
	var d struct {
		AccessToken string `json:"accessToken"`
		Expires     string `json:"expires"`
		Account     struct {
			PlanType string `json:"planType"`
		} `json:"account"`
		User struct {
			Email string `json:"email"`
		} `json:"user"`
		Statsig struct {
			Country  string `json:"country"`
			Region   string `json:"region"`
			IP       string `json:"ip"`
			DeviceID string `json:"deviceId"`
		} `json:"statsigContext"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, errors.New("会话不是合法 JSON")
	}
	if d.AccessToken == "" && d.User.Email == "" {
		return nil, errors.New("会话为空，可能没登录成功")
	}
	return &sessionSummary{
		Plan: d.Account.PlanType, Country: d.Statsig.Country, Region: d.Statsig.Region,
		IP: d.Statsig.IP, Expires: d.Expires, Email: d.User.Email, DeviceID: d.Statsig.DeviceID,
	}, nil
}
