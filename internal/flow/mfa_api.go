package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	http "github.com/bogdanfinn/fhttp"
)

// PasswordLogin 用账号密码换新登录态，供异步任务在登录态失效时兜底。
func PasswordLogin(ctx context.Context, s *Session, email, password string) (string, error) {
	_, raw, err := s.do(ctx, http.MethodGet, home+"/api/auth/csrf", nil,
		map[string]string{"referer": home + "/"})
	if err != nil {
		return "", err
	}
	var csrf struct {
		Token string `json:"csrfToken"`
	}
	if json.Unmarshal(raw, &csrf) != nil || csrf.Token == "" {
		return "", errors.New("取 csrf 失败")
	}

	u := fmt.Sprintf("%s/api/auth/signin/openai?prompt=login&screen_hint=login"+
		"&login_hint=%s", home, url.QueryEscape(email))
	form := "callbackUrl=" + url.QueryEscape(home+"/") + "&csrfToken=" + csrf.Token + "&json=true"
	resp, raw, err := s.do(ctx, http.MethodPost, u, strings.NewReader(form), map[string]string{
		"accept":         "*/*",
		"content-type":   "application/x-www-form-urlencoded;charset=UTF-8",
		"origin":         home,
		"referer":        home + "/",
		"sec-fetch-dest": "empty",
		"sec-fetch-mode": "cors",
		"sec-fetch-site": "same-origin",
	})
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("登录请求 HTTP %d", resp.StatusCode)
	}
	var signed struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &signed) != nil || signed.URL == "" {
		return "", errors.New("登录返回缺少跳转地址")
	}

	if _, _, err := s.do(ctx, http.MethodGet, signed.URL, nil, map[string]string{
		"accept": "text/html",
	}); err != nil {
		return "", err
	}

	// 密码页
	h, err := s.authHdr(ctx, "password_verify", authBase+"/log-in/password", nil)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"password": password})
	resp, raw, err = s.do(ctx, http.MethodPost, authBase+"/api/accounts/password/verify",
		strings.NewReader(string(body)), h)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("密码校验 HTTP %d: %s", resp.StatusCode, snippet(raw))
	}
	var out struct {
		Continue string `json:"continue_url"`
	}
	_ = json.Unmarshal(raw, &out)
	if out.Continue == "" {
		return "", errors.New("密码校验后没有跳转地址")
	}
	token, err := s.Callback(ctx, out.Continue)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", errors.New("登录后没拿到登录态")
	}
	return token, nil
}

// EnrollTOTP 只做登记，返回密钥与登记号。
func EnrollTOTP(ctx context.Context, s *Session, token string) (string, string, error) {
	if token == "" {
		return "", "", errors.New("没有登录态")
	}
	code, raw, err := s.backend(ctx, "/backend-api/accounts/mfa/enroll",
		map[string]string{"factor_type": "totp", "source": "settings"}, token)
	if err != nil {
		return "", "", err
	}
	if code != 200 {
		return "", "", fmt.Errorf("登记 HTTP %d: %s", code, snippet(raw))
	}
	var out struct {
		Secret    string `json:"secret"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Secret == "" {
		return "", "", errors.New("登记返回缺少密钥")
	}
	return out.Secret, out.SessionID, nil
}

// ActivateTOTP 提交动态码激活。
func ActivateTOTP(ctx context.Context, s *Session, token, sessionID, code string) error {
	if token == "" {
		return errors.New("没有登录态")
	}
	httpCode, raw, err := s.backend(ctx, "/backend-api/accounts/mfa/user/activate_enrollment",
		map[string]string{
			"factor_type": "totp",
			"session_id":  sessionID,
			"code":        code,
			"source":      "settings",
		}, token)
	if err != nil {
		return err
	}
	if httpCode != 200 {
		return fmt.Errorf("激活 HTTP %d: %s", httpCode, snippet(raw))
	}
	return nil
}

// FactorDecision 收尾确认。
func FactorDecision(ctx context.Context, s *Session, token string) error {
	if token == "" {
		return errors.New("没有登录态")
	}
	code, raw, err := s.backend(ctx, "/backend-api/accounts/mfa/factor_decision",
		map[string]string{"entrypoint": "post_totp"}, token)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("确认 HTTP %d: %s", code, snippet(raw))
	}
	return nil
}
