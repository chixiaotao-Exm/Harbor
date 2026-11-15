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

// 验证「登录后 session + 定价接口是否给出账号专属优惠」。
//
//	HARBOR_EMAIL=... HARBOR_PASSWORD=... HARBOR_PROXY=socks5://... \
//	  go test -tags=live -run TestLiveSessionPromo -v ./internal/flow/
func TestLiveSessionPromo(t *testing.T) {
	email := os.Getenv("HARBOR_EMAIL")
	pass := os.Getenv("HARBOR_PASSWORD")
	proxy := os.Getenv("HARBOR_PROXY")
	cc := os.Getenv("HARBOR_CC")
	if cc == "" {
		cc = "ID"
	}
	if email == "" || pass == "" || proxy == "" {
		t.Skip("need HARBOR_EMAIL / HARBOR_PASSWORD / HARBOR_PROXY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	s, err := New(ctx, proxy)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer s.Close()

	tok, err := PasswordLogin(ctx, s, email, pass)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	t.Logf("登录成功，token 长度 %d", len(tok))

	// 1) 带 cookie 取 session
	_, raw, err := s.do(ctx, http.MethodGet, home+"/api/auth/session", nil,
		map[string]string{"accept": "application/json", "referer": home + "/"})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	t.Logf("/api/auth/session 长度 %d：%s", len(raw), snippet(raw))
	_ = os.WriteFile("_session.json", raw, 0o644)

	// 2) 带 cookie 取该国定价
	resp, price, err := s.do(ctx, http.MethodGet,
		"https://chatgpt.com/backend-api/checkout_pricing_config/configs/"+cc, nil,
		map[string]string{"accept": "*/*", "oai-language": "zh-CN",
			"oai-device-id": s.Device})
	if err != nil {
		t.Fatalf("pricing: %v", err)
	}
	t.Logf("定价 HTTP %d 长度 %d", resp.StatusCode, len(price))

	var w struct {
		Country string `json:"country_code"`
		CC      struct {
			SymbolCode string `json:"symbol_code"`
			Promos     map[string]struct {
				Enabled bool    `json:"enabled"`
				Amount  float64 `json:"amount"`
			} `json:"promos"`
		} `json:"currency_config"`
	}
	if err := json.Unmarshal(price, &w); err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Logf("国家=%s 货币=%s", w.Country, w.CC.SymbolCode)
	for k, v := range w.CC.Promos {
		t.Logf("  promo %s enabled=%v amount=%v", k, v.Enabled, v.Amount)
	}
}
