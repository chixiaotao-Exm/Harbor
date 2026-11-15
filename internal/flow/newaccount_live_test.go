//go:build live

package flow

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"

	"github.com/chixi/harbor/internal/mailbox"
)

// 注册一个印尼号，用它自己的会话看定价接口是否给出额外优惠。
//
//	HARBOR_KEY=... HARBOR_PROXY=socks5://...-region-ID-... \
//	  go test -tags=live -run TestLiveNewAccountPromo -v ./internal/flow/
func TestLiveNewAccountPromo(t *testing.T) {
	key := os.Getenv("HARBOR_KEY")
	proxy := os.Getenv("HARBOR_PROXY")
	cc := os.Getenv("HARBOR_CC")
	if cc == "" {
		cc = "ID"
	}
	if key == "" || proxy == "" {
		t.Skip("need HARBOR_KEY / HARBOR_PROXY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	mb := mailbox.New(key, "icloud.com", "")
	email, order, err := mb.Acquire(ctx)
	if err != nil {
		t.Fatalf("取号: %v", err)
	}
	defer mb.Status(ctx, order, 3)
	t.Logf("邮箱 %s", email)

	s, err := New(ctx, proxy)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer s.Close()

	d := Deps{
		WaitCode: func(ctx context.Context, id int64, timeout time.Duration) string {
			return mb.WaitCode(ctx, id, timeout, func() { t.Log("等码…") })
		},
		MarkNext: func(ctx context.Context, id int64) { mb.Status(ctx, id, 5) },
		Log:      t.Logf,
	}
	res, err := Register(ctx, s, d, Request{Email: email, OrderID: order})
	if err != nil {
		t.Fatalf("注册: %v", err)
	}
	t.Logf("注册成功 token 长度 %d", len(res.Token))

	// 1) 账号自己的 session
	_, sess, err := s.do(ctx, http.MethodGet, home+"/api/auth/session", nil,
		map[string]string{"accept": "application/json", "referer": home + "/"})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	_ = os.WriteFile("_newsession.json", sess, 0o644)
	t.Logf("session 长度 %d", len(sess))

	// 2) 同一会话下取该国定价
	resp, price, err := s.do(ctx, http.MethodGet,
		"https://chatgpt.com/backend-api/checkout_pricing_config/configs/"+cc, nil,
		map[string]string{"accept": "*/*", "oai-language": "zh-CN", "oai-device-id": s.Device})
	if err != nil {
		t.Fatalf("pricing: %v", err)
	}
	t.Logf("定价 HTTP %d", resp.StatusCode)

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
	t.Logf("账号 %s 国家=%s 货币=%s", email, w.Country, w.CC.SymbolCode)
	for k, v := range w.CC.Promos {
		t.Logf("  promo %s enabled=%v amount=%v", k, v.Enabled, v.Amount)
	}
}
