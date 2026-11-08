package api

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/chixi/harbor/internal/config"
	"github.com/chixi/harbor/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return &Server{Store: st, Log: slog.New(slog.DiscardHandler)}
}

func TestSettingsMasking(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := Ctx()
	defer cancel()

	const key = "TESTKEY-abcd-1234-wxyz-5678"
	err := s.Store.PutSettings(ctx, store.Settings{
		APIKey: key, Proxy: "socks5://u:p@h:3010", Domain: "icloud.com",
		DefaultJobs: 2, DefaultWait: 5, AddPassword: true, AddMFA: true,
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	got, _ := s.Store.GetSettings(ctx)
	if got.APIKey != key {
		t.Fatalf("key should be stored whole, got %q", got.APIKey)
	}
	if v := config.Mask(got.APIKey); v == got.APIKey {
		t.Fatalf("mask must differ from plain: %q", v)
	}
	if v := config.Mask(got.APIKey); v != "TEST…5678" {
		t.Fatalf("unexpected mask %q", v)
	}
}

func TestSettingsViewShape(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := Ctx()
	defer cancel()
	_ = s.Store.PutSettings(ctx, store.Settings{
		APIKey: "abcdefgh12345678", Proxy: "socks5h://u:p@1.2.3.4:1080",
		Domain: "gmail.com", DefaultJobs: 1, DefaultWait: 5,
	})

	got, err := s.Store.GetSettings(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	view := settingsView{
		APIKeyMasked: config.Mask(got.APIKey),
		ProxyMasked:  config.Mask(got.Proxy),
		HasAPIKey:    got.APIKey != "",
		ProxyHost:    hostOf(got.Proxy),
		Domain:       got.Domain,
	}
	b, _ := json.Marshal(view)
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["api_key_masked"] == "abcdefgh12345678" {
		t.Fatal("api_key_masked leaked full key")
	}
	if back["proxy_host"] != "1.2.3.4:1080" {
		t.Fatalf("bad host %v", back["proxy_host"])
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"socks5://u:p@h:3010":       "h:3010",
		"socks5h://u-sid-x:p@a.b:1": "a.b:1",
		"http://h:8080":             "http://h:8080",
		"":                          "",
	}
	for in, want := range cases {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q)=%q want %q", in, got, want)
		}
	}
}

func TestStaticFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/index.html", []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t)
	s.WebDir = dir
	h := s.static()

	rec := &recorder{}
	req := mustReq("GET", "/some/deep/route")
	h.ServeHTTP(rec, req)
	if rec.code != 200 || rec.body == "" {
		t.Fatalf("fallback should serve index, got %d", rec.code)
	}
}
