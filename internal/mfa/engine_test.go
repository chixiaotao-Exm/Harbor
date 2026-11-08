package mfa

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chixi/harbor/internal/events"
	"github.com/chixi/harbor/internal/flow"
	"github.com/chixi/harbor/internal/netx"
	"github.com/chixi/harbor/internal/store"
)

// fakeWorker 记录调用并可按阶段失败。
type fakeWorker struct {
	mu         sync.Mutex
	badToken   bool
	loginOK    bool
	enrollErr  error
	actErr     error
	decideErr  error
	seq        []string
	enrollHook func() error
}

func (f *fakeWorker) note(s string) {
	f.mu.Lock()
	f.seq = append(f.seq, s)
	f.mu.Unlock()
}

func (f *fakeWorker) ProbeToken(context.Context, string) bool {
	f.note("q1")
	return !f.badToken
}

func (f *fakeWorker) LoginWithPassword(context.Context, string, string) (string, error) {
	f.note("q2")
	if !f.loginOK {
		return "", errors.New("bad password")
	}
	return "fresh-token", nil
}

func (f *fakeWorker) Enroll(context.Context, string) (string, string, error) {
	f.note("q3")
	if f.enrollHook != nil {
		if err := f.enrollHook(); err != nil {
			return "", "", err
		}
	}
	if f.enrollErr != nil {
		return "", "", f.enrollErr
	}
	return "JBSWY3DPEHPK3PXP", "sid", nil
}

func (f *fakeWorker) Activate(context.Context, string, string, string) error {
	f.note("q4")
	return f.actErr
}

func (f *fakeWorker) Decide(context.Context, string) error {
	f.note("q5")
	return f.decideErr
}

func (f *fakeWorker) RenewExit(context.Context) error { return nil }

func newEngine(t *testing.T, w Worker) *Engine {
	t.Helper()
	SetSleep(func(ctx context.Context, d time.Duration) {})
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	return &Engine{
		Store: st,
		Hub:   events.NewHub(),
		New:   func(context.Context, string) (Worker, error) { return w, nil },
	}
}

func insert(t *testing.T, e *Engine, email, token string, withPassword bool) int64 {
	t.Helper()
	pw := ""
	if withPassword {
		pw = "pw"
	}
	id, err := e.Store.InsertAccount(context.Background(), store.Account{
		Email: email, Token: token, Password: pw, HasPassword: withPassword, MFAState: "none",
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return id
}

func TestQ1SuccessJumpsToQ3(t *testing.T) {
	w := &fakeWorker{badToken: false}
	e := newEngine(t, w)
	id := insert(t, e, "a@x.com", "tok", true)

	if _, err := e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	e.Wait()

	acc, _ := e.Store.GetAccount(context.Background(), id)
	if acc.MFAState != "ok" || acc.TotpSecret == "" {
		t.Fatalf("account not finished: %+v", acc)
	}
	for _, s := range w.seq {
		if s == "q2" {
			t.Fatal("Q2 should be skipped when Q1 passes")
		}
	}
}

func TestQ1FailFallsBackToPasswordLogin(t *testing.T) {
	w := &fakeWorker{badToken: true, loginOK: true}
	e := newEngine(t, w)
	id := insert(t, e, "b@x.com", "stale", true)

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	acc, _ := e.Store.GetAccount(context.Background(), id)
	if acc.MFAState != "ok" {
		t.Fatalf("should recover through password login: %+v", acc)
	}
	if acc.Token != "fresh-token" {
		t.Fatalf("token not refreshed: %q", acc.Token)
	}
	found := false
	for _, s := range w.seq {
		if s == "q2" {
			found = true
		}
	}
	if !found {
		t.Fatal("Q2 never ran")
	}
}

func TestQ1FailNoPasswordGoesToFailedPage(t *testing.T) {
	w := &fakeWorker{badToken: true}
	e := newEngine(t, w)
	id := insert(t, e, "c@x.com", "stale", false)

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	acc, _ := e.Store.GetAccount(context.Background(), id)
	if acc.MFAState != "fail" {
		t.Fatalf("want fail state, got %q", acc.MFAState)
	}
	if len(e.Failed()) != 1 {
		t.Fatalf("failed page should list it: %v", e.Failed())
	}
	if reason := e.Failed()[0].Reason; !contains(reason, NeedPasswordLogin) {
		t.Fatalf("reason should flag password login: %q", reason)
	}
}

func TestQ2FailGoesToFailedPage(t *testing.T) {
	w := &fakeWorker{badToken: true, loginOK: false}
	e := newEngine(t, w)
	id := insert(t, e, "d@x.com", "stale", true)

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	if len(e.Failed()) != 1 {
		t.Fatalf("want failed entry, got %v", e.Failed())
	}
}

func TestRetryCountAndBackoff(t *testing.T) {
	var waits []time.Duration
	w := &fakeWorker{badToken: true, loginOK: true, enrollErr: errors.New("boom")}
	e := newEngine(t, w)
	SetSleep(func(ctx context.Context, d time.Duration) { waits = append(waits, d) })
	id := insert(t, e, "e@x.com", "tok", true)
	e.Store.SetSecret(context.Background(), id, "")

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	// Q1 连续失败 3 次（2 次退避），Q1 转密码登录成功后 Q3 又失败 3 次（再 2 次退避），
	// 之后密码登录再失败，合计 3+3 次重试。
	want := []time.Duration{Backoff[0], Backoff[1], Backoff[0], Backoff[1], Backoff[0], Backoff[1]}
	if len(waits) != len(want) {
		t.Fatalf("want %d backoff waits, got %d (%v)", len(want), len(waits), waits)
	}
	for i, w := range waits {
		if w != want[i] {
			t.Fatalf("backoff %d = %s, want %s", i, w, want[i])
		}
	}
}

func TestQ5SuccessMovesIntoMFA(t *testing.T) {
	w := &fakeWorker{}
	e := newEngine(t, w)
	id := insert(t, e, "f@x.com", "tok", true)

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	board := e.Board()
	for q := Q1; q <= Q5; q++ {
		if len(board[q]) != 0 {
			t.Fatalf("q%d should be empty, has %v", q, board[q])
		}
	}
	if len(e.Failed()) != 0 {
		t.Fatalf("nothing should fail: %v", e.Failed())
	}
}

func TestBoardTracksProgress(t *testing.T) {
	w := &fakeWorker{enrollErr: errors.New("hold")}
	e := newEngine(t, w)
	id := insert(t, e, "g@x.com", "tok", true)
	e.Store.SetSecret(context.Background(), id, "")

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	if len(e.Board()[Q3]) != 0 {
		t.Fatal("Q3 should be cleared after final failure")
	}
}

func TestRetryRequeuesFailed(t *testing.T) {
	w := &fakeWorker{badToken: false}
	e := newEngine(t, w)
	id := insert(t, e, "h@x.com", "tok", true)
	w.badToken = true
	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()
	if len(e.Failed()) != 1 {
		t.Fatal("setup: should fail")
	}

	w.badToken = false
	if _, err := e.Retry(context.Background(), nil); err != nil {
		t.Fatalf("retry: %v", err)
	}
	e.Wait()
	if len(e.Failed()) != 0 {
		t.Fatalf("should clear failures: %v", e.Failed())
	}
}

func TestAlreadyBoundSkipped(t *testing.T) {
	w := &fakeWorker{}
	e := newEngine(t, w)
	id := insert(t, e, "i@x.com", "tok", true)
	e.Store.SetSecret(context.Background(), id, "EXIST")

	n, err := e.Start(context.Background(), []int64{id}, "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	e.Wait()
	if n != 0 {
		t.Fatalf("should skip already-bound, queued %d", n)
	}
}

func TestStartWhileRunningAppends(t *testing.T) {
	w := &fakeWorker{}
	e := newEngine(t, w)
	a := insert(t, e, "j@x.com", "tok", true)
	b := insert(t, e, "k@x.com", "tok", true)

	if _, err := e.Start(context.Background(), []int64{a}, "socks5://u:p@h:1"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	n, err := e.Start(context.Background(), []int64{b}, "socks5://u:p@h:1")
	if err != nil {
		t.Fatalf("append should be allowed, got %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 appended, got %d", n)
	}
	e.Wait()

	for _, id := range []int64{a, b} {
		acc, _ := e.Store.GetAccount(context.Background(), id)
		if acc.MFAState != "ok" {
			t.Fatalf("account %d not finished: %s", id, acc.MFAState)
		}
	}
	if e.Running() {
		t.Fatal("should be idle after drain")
	}
}

func TestRunningFlagClearsAfterDrain(t *testing.T) {
	w := &fakeWorker{}
	e := newEngine(t, w)
	id := insert(t, e, "l@x.com", "tok", true)
	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()
	if e.Running() {
		t.Fatal("running flag must clear")
	}
}

func TestStageNames(t *testing.T) {
	want := map[int]string{1: "校验登录态", 2: "账号密码登录", 3: "提交双因子登记", 4: "动态码激活", 5: "收尾确认"}
	for q, s := range want {
		if got := stageName(q); got != s {
			t.Errorf("stageName(%d)=%q want %q", q, got, s)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ = flow.TOTPCode
var _ = netx.NewMarker
