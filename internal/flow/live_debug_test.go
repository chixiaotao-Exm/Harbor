//go:build live

package flow

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"

	"github.com/chixi/harbor/internal/mailbox"
	"github.com/chixi/harbor/internal/netx"
)

func TestLiveDebug(t *testing.T) {
	key := os.Getenv("HARBOR_KEY")
	proxy := os.Getenv("HARBOR_PROXY")
	if key == "" || proxy == "" {
		t.Skip("need env")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	m := netx.NewMarker(func(s string) { t.Log(s) })
	exit, err := m.Acquire(ctx, proxy)
	if err != nil {
		t.Fatal(err)
	}
	mb := mailbox.New(key, "icloud.com", "")
	email, order, err := mb.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer mb.Status(ctx, order, 3)
	t.Logf("email=%s", email)

	s, _ := New(ctx, exit.Proxy)
	defer s.Close()

	if err := s.Bootstrap(ctx, email); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Log("bootstrap ok")
	dumpCookies(t, s)

	h, err := s.authHdr(ctx, "email_otp_send", authBase+"/email-verification", nil)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	resp, raw, err := s.do(ctx, http.MethodPost,
		authBase+"/api/accounts/passwordless/send-otp", strings.NewReader("{}"), h)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Logf("send-otp status=%d body=%s", resp.StatusCode, snippet(raw))
	dumpCookies(t, s)
}

func dumpCookies(t *testing.T, s *Session) {
	cl, ok := s.Client.(interface {
		GetCookies(u *url.URL) []*http.Cookie
	})
	if !ok {
		t.Log("client has no cookie api")
		return
	}
	for _, host := range []string{"https://chatgpt.com/", "https://auth.openai.com/"} {
		u, err := url.Parse(host)
		if err != nil {
			continue
		}
		var names []string
		for _, c := range cl.GetCookies(u) {
			names = append(names, c.Name)
		}
		t.Logf("cookies for %s: %v", host, names)
	}
}
