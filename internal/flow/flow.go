// Package flow 实现上游交互全流程。
package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"

	"github.com/chixi/harbor/internal/fingerprint"
	"github.com/chixi/harbor/internal/netx"
)

// 上游地址。
const (
	home      = "https://chatgpt.com"
	authBase  = "https://auth.openai.com"
	guardBase = "https://sentinel.openai.com"
	callback  = home + "/api/auth/callback/openai"
)

// ErrBlocked 表示出口被拦，值得换 IP 重试。
var ErrBlocked = errors.New("出口被拦")

// Session 是一条任务的会话，出口固定。
type Session struct {
	Profile fingerprint.Profile
	Client  httpClient
	Device  string
	proxy   string
	token   string
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// New 用给定出口建会话。
func New(ctx context.Context, proxy string) (*Session, error) {
	p := fingerprint.New()
	c, err := netx.NewClient(proxy, p.UA)
	if err != nil {
		return nil, err
	}
	return &Session{Profile: p, Client: c, Device: p.DeviceID, proxy: proxy}, nil
}

// Close 释放底层连接。
func (s *Session) Close() {
	if closer, ok := s.Client.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (s *Session) hdr(extra map[string]string) http.Header {
	h := http.Header{}
	for k, v := range s.Profile.Headers() {
		h.Set(k, v)
	}
	for k, v := range extra {
		h.Set(k, v)
	}
	return h
}

func (s *Session) do(ctx context.Context, method, url string, body io.Reader, extra map[string]string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header = s.hdr(extra)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp, nil, err
	}
	return resp, raw, nil
}

// guard 领一次放行凭据。
func (s *Session) guard(ctx context.Context, kind string) (map[string]string, error) {
	body, err := fingerprint.ChallengeBody(s.Profile)
	if err != nil {
		return nil, err
	}
	resp, raw, err := s.do(ctx, http.MethodPost, guardBase+"/backend-api/sentinel/req",
		strings.NewReader(body), map[string]string{
			"content-type": "text/plain;charset=UTF-8",
			"origin":       guardBase,
			"referer":      guardBase + "/backend-api/sentinel/frame.html?sv=20260810913b",
		})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("放行接口 HTTP %d", resp.StatusCode)
	}
	var srv fingerprint.Server
	if err := json.Unmarshal(raw, &srv); err != nil {
		return nil, err
	}
	tok, err := fingerprint.Build(s.Profile, srv, kind)
	if err != nil {
		return nil, err
	}
	head, err := tok.Marshal()
	if err != nil {
		return nil, err
	}
	out := map[string]string{"openai-sentinel-token": head}
	if so, err := fingerprint.SOHeader(srv, s.Device, kind); err == nil && so != "" {
		out["openai-sentinel-so-token"] = so
	}
	return out, nil
}

func (s *Session) authHdr(ctx context.Context, kind, referer string, extra map[string]string) (map[string]string, error) {
	h := map[string]string{
		"accept":                      "application/json",
		"content-type":                "application/json",
		"origin":                      authBase,
		"referer":                     referer,
		"sec-fetch-dest":              "empty",
		"sec-fetch-mode":              "cors",
		"sec-fetch-site":              "same-origin",
		"x-access-flow-invocation-id": randUUID(),
	}
	g, err := s.guard(ctx, kind)
	if err != nil {
		return h, err
	}
	for k, v := range g {
		h[k] = v
	}
	for k, v := range extra {
		h[k] = v
	}
	return h, nil
}

// Bootstrap 打开验证页。
func (s *Session) Bootstrap(ctx context.Context, email string) error {
	resp, _, err := s.do(ctx, http.MethodGet, home+"/", nil, map[string]string{
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "none",
		"upgrade-insecure-requests": "1",
	})
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("打开首页 HTTP %d", resp.StatusCode)
	}

	_, raw, err := s.do(ctx, http.MethodGet, home+"/api/auth/csrf", nil,
		map[string]string{"referer": home + "/"})
	if err != nil {
		return err
	}
	var csrf struct {
		Token string `json:"csrfToken"`
	}
	if err := json.Unmarshal(raw, &csrf); err != nil || csrf.Token == "" {
		return fmt.Errorf("取 csrf 失败")
	}

	u := fmt.Sprintf("%s/api/auth/signin/openai?prompt=login&screen_hint=signup"+
		"&auth_session_logging_id=%s&login_hint=%s",
		home, randUUID(), url.QueryEscape(email))
	form := "callbackUrl=" + url.QueryEscape(home+"/") + "&csrfToken=" + csrf.Token + "&json=true"
	resp, raw, err = s.do(ctx, http.MethodPost, u, strings.NewReader(form), map[string]string{
		"accept":         "*/*",
		"content-type":   "application/x-www-form-urlencoded;charset=UTF-8",
		"origin":         home,
		"referer":        home + "/",
		"sec-fetch-dest": "empty",
		"sec-fetch-mode": "cors",
		"sec-fetch-site": "same-origin",
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("登录请求 HTTP %d", resp.StatusCode)
	}
	var signed struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &signed); err != nil || signed.URL == "" {
		return fmt.Errorf("登录返回缺少跳转地址")
	}

	resp, _, err = s.do(ctx, http.MethodGet, signed.URL, nil, map[string]string{
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"referer":                   home + "/",
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "cross-site",
		"upgrade-insecure-requests": "1",
	})
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case 200:
		return nil
	case 403, 429:
		return fmt.Errorf("%w: 授权页 HTTP %d", ErrBlocked, resp.StatusCode)
	default:
		return fmt.Errorf("授权页 HTTP %d", resp.StatusCode)
	}
}

// SendOTP 发验证码。
func (s *Session) SendOTP(ctx context.Context) error {
	h, err := s.authHdr(ctx, "email_otp_send", authBase+"/email-verification", nil)
	if err != nil {
		return err
	}
	resp, raw, err := s.do(ctx, http.MethodPost, authBase+"/api/accounts/passwordless/send-otp",
		strings.NewReader("{}"), h)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("发码 HTTP %d: %s", resp.StatusCode, snippet(raw))
	}
	return nil
}

// ValidateOTP 校验验证码，返回页面类型。
func (s *Session) ValidateOTP(ctx context.Context, code string) (string, error) {
	h, err := s.authHdr(ctx, "email_otp_validate", authBase+"/email-verification", nil)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"code": code})
	resp, raw, err := s.do(ctx, http.MethodPost, authBase+"/api/accounts/email-otp/validate",
		strings.NewReader(string(body)), h)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("校验码 HTTP %d: %s", resp.StatusCode, snippet(raw))
	}
	return pageType(raw), nil
}

// CreateAccount 建档，返回回调地址。
func (s *Session) CreateAccount(ctx context.Context, name, birthdate string) (string, error) {
	h, err := s.authHdr(ctx, "oauth_create_account", authBase+"/about-you", nil)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"name": name, "birthdate": birthdate})
	resp, raw, err := s.do(ctx, http.MethodPost, authBase+"/api/accounts/create_account",
		strings.NewReader(string(body)), h)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("建档 HTTP %d: %s", resp.StatusCode, snippet(raw))
	}
	var out struct {
		Continue string `json:"continue_url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Continue == "" {
		return "", fmt.Errorf("建档返回缺少跳转地址")
	}
	return out.Continue, nil
}

// Callback 跟随回调并把登录态取回来。
func (s *Session) Callback(ctx context.Context, u string) (string, error) {
	if _, _, err := s.do(ctx, http.MethodGet, u, nil, map[string]string{
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"referer":                   authBase + "/",
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "cross-site",
		"upgrade-insecure-requests": "1",
	}); err != nil {
		return "", err
	}
	_, raw, err := s.do(ctx, http.MethodGet, home+"/api/auth/session", nil,
		map[string]string{"referer": home + "/"})
	if err != nil {
		return "", err
	}
	var sess struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(raw, &sess)
	return sess.AccessToken, nil
}

// SessionToken 读当前登录态。
func (s *Session) SessionToken(ctx context.Context) (string, error) {
	_, raw, err := s.do(ctx, http.MethodGet, home+"/api/auth/session", nil,
		map[string]string{"referer": home + "/"})
	if err != nil {
		return "", err
	}
	var sess struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(raw, &sess)
	return sess.AccessToken, nil
}

// SetPassword 用第二封验证码写密码。
func (s *Session) SetPassword(ctx context.Context, email, password string, otp func() string) (string, bool) {
	_, raw, err := s.do(ctx, http.MethodGet, home+"/api/auth/csrf", nil,
		map[string]string{"referer": home + "/"})
	if err != nil {
		return "", false
	}
	var csrf struct {
		Token string `json:"csrfToken"`
	}
	if json.Unmarshal(raw, &csrf) != nil || csrf.Token == "" {
		return "", false
	}

	u := fmt.Sprintf("%s/api/auth/signin/openai?prompt=login&screen_hint=login_or_signup"+
		"&reauth=password&max_age=0&login_hint=%s&post_login_add_password=true",
		home, url.QueryEscape(email))
	form := "callbackUrl=" + url.QueryEscape(home+"/settings/security") +
		"&csrfToken=" + csrf.Token + "&json=true"
	resp, raw, err := s.do(ctx, http.MethodPost, u, strings.NewReader(form), map[string]string{
		"accept":         "*/*",
		"content-type":   "application/x-www-form-urlencoded;charset=UTF-8",
		"origin":         home,
		"referer":        home + "/",
		"sec-fetch-dest": "empty",
		"sec-fetch-mode": "cors",
		"sec-fetch-site": "same-origin",
	})
	if err != nil || resp.StatusCode != 200 {
		return "", false
	}
	var signed struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &signed) != nil || signed.URL == "" {
		return "", false
	}
	if _, _, err := s.do(ctx, http.MethodGet, signed.URL, nil,
		map[string]string{"accept": "text/html"}); err != nil {
		return "", false
	}

	if err := s.SendOTP(ctx); err != nil {
		return "", false
	}
	code := otp()
	if code == "" {
		return "", false
	}
	page, err := s.ValidateOTP(ctx, code)
	if err != nil || !strings.Contains(page, "password") {
		return "", false
	}
	h, err := s.authHdr(ctx, "password_add", authBase+"/reset-password/new-password", nil)
	if err != nil {
		return "", false
	}
	body, _ := json.Marshal(map[string]string{"password": password})
	resp, raw, err = s.do(ctx, http.MethodPost, authBase+"/api/accounts/password/add",
		strings.NewReader(string(body)), h)
	if err != nil || resp.StatusCode != 200 {
		return "", false
	}
	var out struct {
		Continue string `json:"continue_url"`
	}
	_ = json.Unmarshal(raw, &out)
	token := ""
	if out.Continue != "" {
		token, _ = s.Callback(ctx, out.Continue)
	}
	return token, true
}

func pageType(raw []byte) string {
	var v struct {
		Page struct {
			Type string `json:"type"`
		} `json:"page"`
	}
	_ = json.Unmarshal(raw, &v)
	return v.Page.Type
}

func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

func randUUID() string {
	return fingerprint.New().DeviceID
}

// 时序常量，测试可覆盖。
var pause = func(d time.Duration) {}
