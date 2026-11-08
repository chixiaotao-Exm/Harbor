package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chixi/harbor/internal/events"
	"github.com/chixi/harbor/internal/flow"
	"github.com/chixi/harbor/internal/netx"
	"github.com/chixi/harbor/internal/store"
)

// fakeSource hands out credits and counts concurrency.
type fakeSource struct {
	mu       sync.Mutex
	issued   int
	inFlight int
	maxIn    int
	failAll  bool
	ttl      time.Duration
}

func (f *fakeSource) Acquire(ctx context.Context) (Credit, error) {
	f.mu.Lock()
	if f.failAll {
		f.mu.Unlock()
		return Credit{}, errors.New("no mail")
	}
	f.issued++
	f.inFlight++
	if f.inFlight > f.maxIn {
		f.maxIn = f.inFlight
	}
	n := f.issued
	f.mu.Unlock()

	if f.ttl > 0 {
		time.Sleep(f.ttl)
	}
	return Credit{Email: fmt.Sprintf("u%d@x.com", n), Order: int64(n)}, nil
}

func (f *fakeSource) done() {
	f.mu.Lock()
	f.inFlight--
	f.mu.Unlock()
}

func (f *fakeSource) WaitCode(context.Context, int64, time.Duration) string { return "123456" }
func (f *fakeSource) Done(context.Context, int64)                           { f.done() }
func (f *fakeSource) Cancel(context.Context, int64)                         { f.done() }
func (f *fakeSource) MarkNext(context.Context, int64)                       {}

func newRunner(t *testing.T, src CreditSource) *Runner {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	hub := events.NewHub()
	t.Cleanup(func() {
		ch, cancel := hub.Subscribe()
		cancel()
		_ = ch
	})
	return &Runner{
		Store:  st,
		Hub:    hub,
		Source: src,
		Marker: &netx.Marker{
			Probe: func(context.Context, string) (string, error) { return "1.1.1.1", nil },
			Sleep: func(time.Duration) {},
		},
		NewSession: func(context.Context, string) (Session, error) { return fakeSession{}, nil },
		Register: func(ctx context.Context, s Session, d flow.Deps, req flow.Request) (*flow.Result, error) {
			return &flow.Result{
				Email:       req.Email,
				Password:    "pw",
				Name:        "n",
				Birthdate:   "2000-01-01",
				Token:       "tok",
				HasPassword: true,
			}, nil
		},
	}
}

type fakeSession struct{}

func (fakeSession) Close() {}

func TestStatsRate(t *testing.T) {
	if (Stats{Done: 0}).Rate() != 0 {
		t.Fatal("empty rate should be 0")
	}
	if got := (Stats{Done: 4, OK: 3}).Rate(); got != 75 {
		t.Fatalf("rate %v", got)
	}
}

func TestConcurrencyCapped(t *testing.T) {
	src := &fakeSource{ttl: 20 * time.Millisecond}
	r := newRunner(t, src)
	if err := r.Start(Config{Count: 12, Threads: 3, Proxy: "socks5://u:p@h:1"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	r.Wait()
	s := r.Stats()
	if s.Running {
		t.Fatal("should have stopped")
	}
	if s.Done != 12 || s.OK != 12 {
		t.Fatalf("bad stats %+v", s)
	}
	if src.maxIn > 3 {
		t.Fatalf("concurrency exceeded: %d", src.maxIn)
	}
}

func TestStopOnlyBlocksNewWork(t *testing.T) {
	src := &fakeSource{ttl: 40 * time.Millisecond}
	r := newRunner(t, src)
	// 每次注册都慢一点，保证停止时还有活没跑完。
	r.Register = func(ctx context.Context, s Session, d flow.Deps, req flow.Request) (*flow.Result, error) {
		time.Sleep(30 * time.Millisecond)
		return &flow.Result{Email: req.Email, Token: "tok"}, nil
	}
	if err := r.Start(Config{Count: 40, Threads: 1, Proxy: "socks5://u:p@h:1"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(70 * time.Millisecond)
	r.Stop()
	r.Wait()

	s := r.Stats()
	if s.Running {
		t.Fatal("must not still be running")
	}
	if s.Done >= 40 {
		t.Fatalf("stop should cut the batch short, done=%d", s.Done)
	}
	if s.Done == 0 {
		t.Fatal("in-flight item should still finish")
	}
}

func TestAcquireFailureCountsAsFail(t *testing.T) {
	src := &fakeSource{failAll: true}
	r := newRunner(t, src)
	_ = r.Start(Config{Count: 2, Threads: 1, Proxy: "socks5://u:p@h:1"})
	r.Wait()
	s := r.Stats()
	if s.Fail != 2 || s.OK != 0 {
		t.Fatalf("bad stats %+v", s)
	}
}

func TestConsecutiveFailuresPauseBatch(t *testing.T) {
	src := &fakeSource{failAll: true}
	r := newRunner(t, src)
	_ = r.Start(Config{Count: 20, Threads: 1, Proxy: "socks5://u:p@h:1"})
	r.Wait()
	if got := r.Stats().Done; got >= 20 {
		t.Fatalf("batch should pause after 5 failures, done=%d", got)
	}
}

func TestDoubleStartRejected(t *testing.T) {
	src := &fakeSource{ttl: 30 * time.Millisecond}
	r := newRunner(t, src)
	if err := r.Start(Config{Count: 2, Threads: 1, Proxy: "socks5://u:p@h:1"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := r.Start(Config{Count: 2}); err == nil {
		t.Fatal("second start should fail")
	}
	r.Wait()
}

func TestItemsRecorded(t *testing.T) {
	src := &fakeSource{}
	r := newRunner(t, src)
	r.setStage(0, "a@x.com", "取号完成", "running", "")
	r.setStage(0, "a@x.com", "完成", "ok", "")
	items := r.Items()
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Stage != "完成" || items[0].State != "ok" {
		t.Fatalf("item not updated in place: %+v", items[0])
	}
}

func TestAutoEnqueueOnSuccess(t *testing.T) {
	src := &fakeSource{}
	r := newRunner(t, src)
	var got []int64
	var gotProxy string
	r.Enqueue = func(ctx context.Context, ids []int64, proxy string) (int, error) {
		got = append(got, ids...)
		gotProxy = proxy
		return len(ids), nil
	}
	_ = r.Start(Config{Count: 2, Threads: 1, Proxy: "socks5://u:p@h:1", AutoEnqueue: true})
	r.Wait()

	if len(got) != 2 {
		t.Fatalf("want 2 enqueued, got %v", got)
	}
	if gotProxy == "" {
		t.Fatal("proxy not forwarded")
	}
}

func TestAutoEnqueueOff(t *testing.T) {
	src := &fakeSource{}
	r := newRunner(t, src)
	var n int
	r.Enqueue = func(ctx context.Context, ids []int64, proxy string) (int, error) {
		n += len(ids)
		return len(ids), nil
	}
	_ = r.Start(Config{Count: 2, Threads: 1, Proxy: "socks5://u:p@h:1", AutoEnqueue: false})
	r.Wait()
	if n != 0 {
		t.Fatalf("should not enqueue, got %d", n)
	}
}

func TestAutoEnqueueSkipsWhenSecretPresent(t *testing.T) {
	src := &fakeSource{}
	r := newRunner(t, src)
	// 注册结果自带密钥，说明已绑，不该再投
	r.Register = func(ctx context.Context, s Session, d flow.Deps, req flow.Request) (*flow.Result, error) {
		return &flow.Result{Email: req.Email, Token: "tok", Secret: "SECRET", HasPassword: true}, nil
	}
	var n int
	r.Enqueue = func(ctx context.Context, ids []int64, proxy string) (int, error) {
		n += len(ids)
		return len(ids), nil
	}
	_ = r.Start(Config{Count: 1, Threads: 1, Proxy: "socks5://u:p@h:1", AutoEnqueue: true})
	r.Wait()
	if n != 0 {
		t.Fatalf("already bound should skip enqueue, got %d", n)
	}
}

func TestAutoEnqueueErrorDoesNotFailItem(t *testing.T) {
	src := &fakeSource{}
	r := newRunner(t, src)
	r.Enqueue = func(ctx context.Context, ids []int64, proxy string) (int, error) {
		return 0, errors.New("engine busy")
	}
	_ = r.Start(Config{Count: 1, Threads: 1, Proxy: "socks5://u:p@h:1", AutoEnqueue: true})
	r.Wait()
	if s := r.Stats(); s.OK != 1 || s.Fail != 0 {
		t.Fatalf("enqueue error must not fail the item: %+v", s)
	}
}

func TestStatsPublished(t *testing.T) {
	src := &fakeSource{}
	r := newRunner(t, src)
	ch, cancel := r.Hub.Subscribe()
	defer cancel()

	var n int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for e := range ch {
			if e.Kind == events.KindStat {
				atomic.AddInt32(&n, 1)
			}
		}
	}()
	_ = r.Start(Config{Count: 1, Threads: 1, Proxy: "socks5://u:p@h:1"})
	r.Wait()
	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()
	if atomic.LoadInt32(&n) == 0 {
		t.Fatal("no stat events published")
	}
}
