//go:build live

package flow

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

// 用邮箱+密码+双因子密钥登录，再取 /api/auth/session。
//
//	HARBOR_EMAIL= HARBOR_PASSWORD= HARBOR_SECRET= HARBOR_PROXY= \
//	  go test -tags=live -run TestLiveSessionWithTOTP -v ./internal/flow/
func TestLiveSessionWithTOTP(t *testing.T) {
	email := os.Getenv("HARBOR_EMAIL")
	pass := os.Getenv("HARBOR_PASSWORD")
	secret := os.Getenv("HARBOR_SECRET")
	proxy := os.Getenv("HARBOR_PROXY")
	if email == "" || pass == "" || secret == "" || proxy == "" {
		t.Skip("need HARBOR_EMAIL/PASSWORD/SECRET/PROXY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	s, err := New(ctx, proxy)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer s.Close()

	tok, err := PasswordLoginTOTP(ctx, s, email, pass, secret)
	if err != nil {
		t.Fatalf("登录: %v", err)
	}
	t.Logf("登录成功 token 长度 %d", len(tok))

	_, raw, err := s.do(ctx, http.MethodGet, home+"/api/auth/session", nil,
		map[string]string{"accept": "application/json", "referer": home + "/"})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
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
		t.Fatalf("解析: %v", err)
	}
	t.Logf("邮箱=%s 套餐=%s 到期=%s", d.User.Email, d.Account.PlanType, d.Expires)
	t.Logf("国家=%s 地区=%s IP=%s", d.Statsig.Country, d.Statsig.Region, d.Statsig.IP)
	t.Logf("session 长度 %d，accessToken 与登录 token 一致=%v", len(raw), d.AccessToken == tok)
}
