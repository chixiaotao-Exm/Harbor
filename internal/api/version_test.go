package api

import (
	"encoding/json"
	"testing"
)

func TestVersionEndpoint(t *testing.T) {
	s := newTestServer(t)
	s.Version = "v1.2.3"
	s.Commit = "abc1234"
	s.BuildTime = "2026-01-01T00:00:00Z"

	rec := &recorder{}
	s.handleVersion(rec, mustReq("GET", "/api/version"))
	if rec.code != 200 {
		t.Fatalf("code %d", rec.code)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(rec.body), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if out["version"] != "v1.2.3" || out["commit"] != "abc1234" {
		t.Fatalf("bad payload: %v", out)
	}
}

func TestVersionDefaults(t *testing.T) {
	s := newTestServer(t)
	rec := &recorder{}
	s.handleVersion(rec, mustReq("GET", "/api/version"))
	var out map[string]string
	_ = json.Unmarshal([]byte(rec.body), &out)
	if _, ok := out["version"]; !ok {
		t.Fatalf("version field missing: %s", rec.body)
	}
}
