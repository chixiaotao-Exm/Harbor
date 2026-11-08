package flow

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

type fakeResp struct {
	status int
	body   string
}

// fakeClient replays responses by URL fragment.
type fakeClient struct {
	routes map[string]fakeResp
	seen   []string
}

func (f *fakeClient) Do(req *http.Request) (*http.Response, error) {
	key := req.URL.Host + req.URL.Path
	f.seen = append(f.seen, key)
	// 首页是 host+"/"，单独兜底，避免被前缀匹配吃掉。
	if req.URL.Path == "/" && req.URL.Host != "" {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("<html>home</html>")), Header: http.Header{}}, nil
	}
	for frag, r := range f.routes {
		if strings.Contains(key, frag) {
			return &http.Response{
				StatusCode: r.status,
				Body:       io.NopCloser(strings.NewReader(r.body)),
				Header:     http.Header{},
			}, nil
		}
	}
	return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

func newFake(routes map[string]fakeResp) *fakeClient { return &fakeClient{routes: routes} }

func newSession(c *fakeClient) *Session {
	s, _ := New(context.Background(), "")
	s.Client = c
	return s
}

func baseRoutes() map[string]fakeResp {
	return map[string]fakeResp{
		"chatgpt.com/api/auth/csrf":                          {200, `{"csrfToken":"csrf-1"}`},
		"chatgpt.com/api/auth/signin/openai":                 {200, `{"url":"https://auth.openai.com/oauth/authorize?x=1"}`},
		"auth.openai.com/oauth/authorize":                    {200, "<html>verify</html>"},
		"sentinel.openai.com/backend-api/sentinel/req":       {200, `{"token":"srv","proofofwork":{"required":false}}`},
		"auth.openai.com/api/accounts/passwordless/send-otp": {200, `{}`},
		"auth.openai.com/api/accounts/email-otp/validate":    {200, `{"page":{"type":"about_you"}}`},
		"auth.openai.com/api/accounts/create_account":        {200, `{"continue_url":"https://chatgpt.com/api/auth/callback/openai?code=1"}`},
		"chatgpt.com/api/auth/callback/openai":               {200, "<html>ok</html>"},
		"chatgpt.com/api/auth/session":                       {200, `{"accessToken":"tok-1"}`},
	}
}

func okDeps() Deps {
	return Deps{
		WaitCode: func(ctx context.Context, id int64, d time.Duration) string { return "123456" },
		Log:      func(string, ...any) {},
	}
}

func TestBootstrapHappyPath(t *testing.T) {
	c := newFake(baseRoutes())
	s := newSession(c)
	if err := s.Bootstrap(context.Background(), "a@b.com"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	joined := strings.Join(c.seen, ",")
	if !strings.Contains(joined, "signin/openai") || !strings.Contains(joined, "oauth/authorize") {
		t.Fatalf("flow incomplete: %v", c.seen)
	}
}

func TestBootstrapBlockedIsTypedError(t *testing.T) {
	r := baseRoutes()
	r["auth.openai.com/oauth/authorize"] = fakeResp{403, "denied"}
	err := newSession(newFake(r)).Bootstrap(context.Background(), "a@b.com")
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
}

func TestBootstrapMissingURLErrors(t *testing.T) {
	r := baseRoutes()
	r["chatgpt.com/api/auth/signin/openai"] = fakeResp{200, `{}`}
	if err := newSession(newFake(r)).Bootstrap(context.Background(), "a@b.com"); err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestGuardAttachesToken(t *testing.T) {
	c := newFake(map[string]fakeResp{
		"sentinel/req": {200, `{"token":"srv","proofofwork":{"required":false}}`},
	})
	h, err := newSession(c).guard(context.Background(), "email_otp_send")
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	tok := h["openai-sentinel-token"]
	if !strings.Contains(tok, `"c":"srv"`) || !strings.Contains(tok, "gAAAAAB") {
		t.Fatalf("bad token header: %s", tok)
	}
}

func TestGuardSolvesPoW(t *testing.T) {
	c := newFake(map[string]fakeResp{
		"sentinel/req": {200, `{"token":"srv","proofofwork":{"required":true,"seed":"0.5","difficulty":"ff"}}`},
	})
	h, err := newSession(c).guard(context.Background(), "email_otp_validate")
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !strings.Contains(h["openai-sentinel-token"], "gAAAAAB") {
		t.Fatal("token missing")
	}
}

func TestValidateOTPPageType(t *testing.T) {
	r := baseRoutes()
	r["auth.openai.com/api/accounts/email-otp/validate"] = fakeResp{200, `{"page":{"type":"reset_password_new_password"}}`}
	page, err := newSession(newFake(r)).ValidateOTP(context.Background(), "123456")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if page != "reset_password_new_password" {
		t.Fatalf("got %q", page)
	}
}

func TestValidateOTPHTTPError(t *testing.T) {
	r := baseRoutes()
	r["auth.openai.com/api/accounts/email-otp/validate"] = fakeResp{400, "bad code"}
	_, err := newSession(newFake(r)).ValidateOTP(context.Background(), "000000")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("want 400 error, got %v", err)
	}
}

func TestCreateAccountReturnsCallback(t *testing.T) {
	cb, err := newSession(newFake(baseRoutes())).CreateAccount(context.Background(), "n", "2000-01-01")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(cb, "code=1") {
		t.Fatalf("bad callback %q", cb)
	}
}

func TestCallbackReturnsToken(t *testing.T) {
	tok, err := newSession(newFake(baseRoutes())).Callback(context.Background(),
		"https://chatgpt.com/api/auth/callback/openai?code=1")
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if tok != "tok-1" {
		t.Fatalf("got %q", tok)
	}
}

func TestRegisterHappyPathWithoutPassword(t *testing.T) {
	s := newSession(newFake(baseRoutes()))
	res, err := Register(context.Background(), s, okDeps(), Request{Email: "a@b.com", OrderID: 1})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if res.Token != "tok-1" || res.Name == "" || res.Birthdate == "" {
		t.Fatalf("bad result %+v", res)
	}
	if res.HasPassword {
		t.Fatal("password not requested")
	}
}

func TestRegisterFailsOnNoCode(t *testing.T) {
	d := okDeps()
	d.WaitCode = func(context.Context, int64, time.Duration) string { return "" }
	_, err := Register(context.Background(), newSession(newFake(baseRoutes())), d, Request{Email: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "等待验证码邮件") {
		t.Fatalf("want stage error, got %v", err)
	}
}

func TestRegisterFailsAtBootstrap(t *testing.T) {
	r := baseRoutes()
	r["auth.openai.com/oauth/authorize"] = fakeResp{403, ""}
	_, err := Register(context.Background(), newSession(newFake(r)), okDeps(), Request{Email: "a@b.com"})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
	if !strings.Contains(err.Error(), "打开验证页") {
		t.Fatalf("stage missing: %v", err)
	}
}

func TestRegisterPasswordAndMFA(t *testing.T) {
	r := baseRoutes()
	r["auth.openai.com/api/accounts/email-otp/validate"] = fakeResp{200, `{"page":{"type":"reset_password_new_password"}}`}
	r["auth.openai.com/api/accounts/password/add"] = fakeResp{200, `{"continue_url":"https://chatgpt.com/api/auth/callback/openai?code=2"}`}
	r["chatgpt.com/backend-api/accounts/mfa/enroll"] = fakeResp{200, `{"secret":"JBSWY3DPEHPK3PXP","session_id":"sid"}`}
	r["chatgpt.com/backend-api/accounts/mfa/user/activate_enrollment"] = fakeResp{200, `{}`}
	r["chatgpt.com/backend-api/accounts/mfa/factor_decision"] = fakeResp{200, `{}`}

	res, err := Register(context.Background(), newSession(newFake(r)), okDeps(),
		Request{Email: "a@b.com", OrderID: 1, SetPassword: true, EnableMFA: true})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !res.HasPassword || res.Password == "" {
		t.Fatalf("password not set: %+v", res)
	}
	if res.Secret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("secret not captured: %q", res.Secret)
	}
}

func TestRegisterPasswordFailureKeepsAccount(t *testing.T) {
	r := baseRoutes()
	r["auth.openai.com/api/accounts/password/add"] = fakeResp{500, ""}
	res, err := Register(context.Background(), newSession(newFake(r)), okDeps(),
		Request{Email: "a@b.com", OrderID: 1, SetPassword: true})
	if err != nil {
		t.Fatalf("register should still succeed: %v", err)
	}
	if res.HasPassword {
		t.Fatal("password should be reported unset")
	}
	if res.Token == "" {
		t.Fatal("account token lost")
	}
}

func TestNamesAndPasswordsVary(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		n := randomName()
		if seen[n] {
			t.Fatalf("name repeated: %s", n)
		}
		seen[n] = true
		if p := randomPassword(); len(p) != 16 {
			t.Fatalf("password length %d", len(p))
		}
		if b := randomBirthdate(); len(b) != 10 || b[4] != '-' {
			t.Fatalf("bad birthdate %q", b)
		}
	}
}

func TestTOTPCode(t *testing.T) {
	code, err := TOTPCode("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("totp: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("bad code %q", code)
	}
}

func TestProbeSessionNoToken(t *testing.T) {
	if ProbeSession(context.Background(), newSession(newFake(baseRoutes())), "") {
		t.Fatal("empty token should be invalid")
	}
}
