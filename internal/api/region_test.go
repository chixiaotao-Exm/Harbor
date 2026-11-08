package api

import (
	"strings"
	"testing"

	"github.com/chixi/harbor/internal/store"
)

func TestRegionRewriteProxy(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := Ctx()
	defer cancel()
	_ = s.Store.PutSettings(ctx, store.Settings{
		APIKey: "k", Proxy: "socks5://u-region-US-sid-ABC-t-5:p@h:3010",
		Domain: "icloud.com", Region: "US", DefaultJobs: 1, DefaultWait: 5,
		AddPassword: true, AddMFA: true,
	})

	rec := &recorder{}
	req := mustReq("PUT", "/api/settings")
	req.Body = nopCloser(`{"region":"JP","add_password":true,"add_mfa":true,"default_jobs":1,"default_wait":5}`)
	s.handlePutSettings(rec, req)
	if rec.code != 200 {
		t.Fatalf("code %d %s", rec.code, rec.body)
	}

	got, _ := s.Store.GetSettings(ctx)
	if !strings.Contains(got.Proxy, "region-JP") {
		t.Fatalf("proxy not rewritten: %q", got.Proxy)
	}
	if strings.Contains(got.Proxy, "region-US") {
		t.Fatalf("old region kept: %q", got.Proxy)
	}
	if got.Region != "JP" {
		t.Fatalf("region %q", got.Region)
	}
}
