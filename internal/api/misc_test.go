package api

import (
	"strings"
	"testing"
)

func TestSettingsRegionPersists(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := Ctx()
	defer cancel()

	rec := &recorder{}
	req := mustReq("PUT", "/api/settings")
	req.Body = nopCloser(`{"region":"JP","add_password":true,"add_mfa":true,"default_jobs":1,"default_wait":5}`)
	s.handlePutSettings(rec, req)
	if rec.code != 200 {
		t.Fatalf("code %d body %s", rec.code, rec.body)
	}
	if !strings.Contains(rec.body, `"region":"JP"`) {
		t.Fatalf("region not echoed: %s", rec.body)
	}

	got, err := s.Store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Region != "JP" {
		t.Fatalf("region not stored: %q", got.Region)
	}
}

func TestRegionsEndpoint(t *testing.T) {
	s := newTestServer(t)
	rec := &recorder{}
	s.handleRegions(rec, mustReq("GET", "/api/regions"))
	if rec.code != 200 || !strings.Contains(rec.body, "US") || !strings.Contains(rec.body, "JP") {
		t.Fatalf("bad regions: %d %s", rec.code, rec.body)
	}
}

func TestBalanceWithoutKey(t *testing.T) {
	s := newTestServer(t)
	rec := &recorder{}
	s.handleBalance(rec, mustReq("GET", "/api/balance"))
	if rec.code != 200 || !strings.Contains(rec.body, "还没填") {
		t.Fatalf("bad: %d %s", rec.code, rec.body)
	}
}
