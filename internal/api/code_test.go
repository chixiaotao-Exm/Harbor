package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/chixi/harbor/internal/store"
)

func codeReq(t *testing.T, s *Server, id string) (int, string) {
	t.Helper()
	rec := &recorder{}
	req := mustReq("GET", "/api/accounts/"+id+"/code")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(contextWithRoute(req, rctx))
	s.handleAccountCode(rec, req)
	return rec.code, rec.body
}

func TestCodeReturnsSixDigits(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := Ctx()
	defer cancel()
	id, err := s.Store.InsertAccount(ctx, store.Account{
		Email: "a@x.com", TotpSecret: "JBSWY3DPEHPK3PXP", HasPassword: true, MFAState: "ok",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	code, body := codeReq(t, s, itoa(id))
	if code != 200 {
		t.Fatalf("code %d body %s", code, body)
	}
	var out struct {
		Code string `json:"code"`
		Left int    `json:"left"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(out.Code) != 6 {
		t.Fatalf("want 6 digits, got %q", out.Code)
	}
	for _, c := range out.Code {
		if c < '0' || c > '9' {
			t.Fatalf("non digit in %q", out.Code)
		}
	}
	if out.Left < 1 || out.Left > 30 {
		t.Fatalf("left out of range: %d", out.Left)
	}
}

func TestCodeChangesOverTime(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := Ctx()
	defer cancel()
	id, _ := s.Store.InsertAccount(ctx, store.Account{
		Email: "b@x.com", TotpSecret: "JBSWY3DPEHPK3PXP", HasPassword: true,
	})
	// 换一个时间点算，应得到不同码
	orig := totpCode
	defer func() { totpCode = orig }()

	totpCode = func(string) (string, error) { return "111111", nil }
	if _, body := codeReq(t, s, itoa(id)); !strings.Contains(body, "111111") {
		t.Fatalf("stub not used: %s", body)
	}
	totpCode = func(string) (string, error) { return "222222", nil }
	if _, body := codeReq(t, s, itoa(id)); !strings.Contains(body, "222222") {
		t.Fatalf("stub not used twice: %s", body)
	}
}

func TestCodeNoSecret(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := Ctx()
	defer cancel()
	id, _ := s.Store.InsertAccount(ctx, store.Account{Email: "c@x.com", HasPassword: true})
	code, body := codeReq(t, s, itoa(id))
	if code != 400 || !strings.Contains(body, "还没绑") {
		t.Fatalf("want 400, got %d %s", code, body)
	}
}

func TestCodeUnknownAccount(t *testing.T) {
	s := newTestServer(t)
	code, _ := codeReq(t, s, "9999")
	if code != 404 {
		t.Fatalf("want 404, got %d", code)
	}
}

func TestCodeBadID(t *testing.T) {
	s := newTestServer(t)
	code, _ := codeReq(t, s, "abc")
	if code != 400 {
		t.Fatalf("want 400, got %d", code)
	}
}
