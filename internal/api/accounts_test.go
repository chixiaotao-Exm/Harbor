package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/chixi/harbor/internal/store"
)

func seed(t *testing.T, s *Server, n int, withSecret bool) {
	t.Helper()
	prefix := "p"
	if withSecret {
		prefix = "m"
	}
	ctx, cancel := Ctx()
	defer cancel()
	for i := 0; i < n; i++ {
		acc := store.Account{
			Email:       fmt.Sprintf("%s%03d@x.com", prefix, i),
			Password:    fmt.Sprintf("pw%03d", i),
			HasPassword: true,
			MFAState:    "none",
		}
		if withSecret {
			acc.TotpSecret = fmt.Sprintf("SECRET%03d", i)
			acc.MFAState = "ok"
		}
		if _, err := s.Store.InsertAccount(ctx, acc); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func doGet(t *testing.T, s *Server, path string) (int, string) {
	t.Helper()
	rec := &recorder{}
	s.handleListAccounts(rec, mustReq("GET", path))
	return rec.code, rec.body
}

func TestListPagingBounds(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 25, false)

	code, body := doGet(t, s, "/api/accounts?kind=plain&page=1&size=10")
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	var out struct {
		Items []accountView `json:"items"`
		Total int           `json:"total"`
		Pages int           `json:"pages"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(out.Items) != 10 || out.Total != 25 || out.Pages != 3 {
		t.Fatalf("bad paging: %+v", out)
	}
}

func TestListEmptyPage(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 3, false)
	code, body := doGet(t, s, "/api/accounts?kind=plain&page=99&size=10")
	if code != 200 {
		t.Fatalf("code %d", code)
	}
	var out struct {
		Items []accountView `json:"items"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if len(out.Items) != 0 {
		t.Fatalf("expected empty page, got %d", len(out.Items))
	}
}

func TestListSizeClamped(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 5, false)
	_, body := doGet(t, s, "/api/accounts?kind=plain&size=9999")
	var out struct {
		Size int `json:"size"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.Size != 100 {
		t.Fatalf("size not clamped: %d", out.Size)
	}
	_, body = doGet(t, s, "/api/accounts?kind=plain&size=0")
	_ = json.Unmarshal([]byte(body), &out)
	if out.Size != 20 {
		t.Fatalf("default size wrong: %d", out.Size)
	}
}

func TestTabsDoNotMix(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 4, false) // plain
	seed(t, s, 7, true)  // mfa

	_, body := doGet(t, s, "/api/accounts?kind=plain&size=100")
	var plain struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal([]byte(body), &plain)
	if plain.Total != 4 {
		t.Fatalf("plain total %d", plain.Total)
	}

	_, body = doGet(t, s, "/api/accounts?kind=mfa&size=100")
	var mfa struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal([]byte(body), &mfa)
	if mfa.Total != 7 {
		t.Fatalf("mfa total %d", mfa.Total)
	}
}

func TestSearchFiltersWithinTab(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 3, false)
	ctx, cancel := Ctx()
	defer cancel()
	_, _ = s.Store.InsertAccount(ctx, store.Account{Email: "special@x.com", HasPassword: true})

	_, body := doGet(t, s, "/api/accounts?kind=plain&q=special")
	var out struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if out.Total != 1 {
		t.Fatalf("search total %d", out.Total)
	}
}

func TestExportMFAFormat(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 3, true)

	rec := &recorder{}
	req := mustReq("POST", "/api/accounts/export")
	req.Body = nopCloser(`{"kind":"mfa","all":true}`)
	s.handleExport(rec, req)

	lines := strings.Split(strings.TrimSpace(rec.body), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %q", len(lines), rec.body)
	}
	for _, l := range lines {
		parts := strings.Split(l, "----")
		if len(parts) != 3 {
			t.Fatalf("mfa line must have 3 parts: %q", l)
		}
		if !strings.HasPrefix(parts[2], "SECRET") {
			t.Fatalf("secret missing: %q", l)
		}
	}
}

func TestExportPlainFormat(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 2, false)

	rec := &recorder{}
	req := mustReq("POST", "/api/accounts/export")
	req.Body = nopCloser(`{"kind":"plain","all":true}`)
	s.handleExport(rec, req)

	lines := strings.Split(strings.TrimSpace(rec.body), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	for _, l := range lines {
		if parts := strings.Split(l, "----"); len(parts) != 2 {
			t.Fatalf("plain line must have 2 parts: %q", l)
		}
	}
}

func TestExportByIDs(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 5, false)
	ctx, cancel := Ctx()
	defer cancel()
	ids, _ := s.Store.AllIDs(ctx, "plain", "")
	pick := ids[:2]

	bodyJSON := fmt.Sprintf(`{"kind":"plain","ids":[%d,%d]}`, pick[0], pick[1])
	rec := &recorder{}
	req := mustReq("POST", "/api/accounts/export")
	req.Body = nopCloser(bodyJSON)
	s.handleExport(rec, req)

	if got := len(strings.Split(strings.TrimSpace(rec.body), "\n")); got != 2 {
		t.Fatalf("want 2 lines, got %d", got)
	}
}

func TestExportHeaderFilename(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 1, true)
	rec := &recorder{}
	req := mustReq("POST", "/api/accounts/export")
	req.Body = nopCloser(`{"kind":"mfa","all":true}`)
	s.handleExport(rec, req)
	if got := rec.hdr.Get("Content-Disposition"); !strings.Contains(got, "accounts-mfa.txt") {
		t.Fatalf("bad filename header: %q", got)
	}
}

func TestSelectAllByTab(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 3, false)
	seed(t, s, 4, true)

	rec := &recorder{}
	req := mustReq("POST", "/api/accounts/select")
	req.Body = nopCloser(`{"kind":"mfa"}`)
	s.handleSelectAll(rec, req)

	var out struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal([]byte(rec.body), &out)
	if out.Total != 4 {
		t.Fatalf("select-all should only cover mfa tab, got %d", out.Total)
	}
}

func TestAccountStats(t *testing.T) {
	s := newTestServer(t)
	seed(t, s, 2, false)
	seed(t, s, 5, true)
	rec := &recorder{}
	s.handleAccountStats(rec, mustReq("GET", "/api/accounts/stats"))
	var out map[string]int
	_ = json.Unmarshal([]byte(rec.body), &out)
	if out["plain"] != 2 || out["mfa"] != 5 {
		t.Fatalf("bad counts: %v", out)
	}
}

func TestExportInjectionSafe(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := s.Store.InsertAccount(ctx, store.Account{
		Email: "a----b@x.com", Password: "p\nw", HasPassword: true,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	rec := &recorder{}
	req := mustReq("POST", "/api/accounts/export")
	req.Body = nopCloser(`{"kind":"plain","all":true}`)
	s.handleExport(rec, req)
	if !strings.Contains(rec.body, "a----b@x.com----p\nw") {
		t.Fatalf("injection broke line: %q", rec.body)
	}
}

const nopCloserSrc = ""

func nopCloser(body string) *readCloser { return &readCloser{s: body} }
