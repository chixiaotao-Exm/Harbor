package mailbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// 用假 call 替掉网络。
type fakeClient struct {
	*Client
	mu       sync.Mutex
	calls    []string
	inFlight int
	maxPar   int
	fail     error
	noMail   map[string]bool
}

func newFake() *fakeClient {
	return &fakeClient{Client: New("k", "icloud.com", ""), noMail: map[string]bool{}}
}

func (f *fakeClient) acquire(ctx context.Context) (string, int64, error) {
	acquireMu.Lock()
	defer acquireMu.Unlock()

	f.mu.Lock()
	f.inFlight++
	if f.inFlight > f.maxPar {
		f.maxPar = f.inFlight
	}
	f.mu.Unlock()

	time.Sleep(5 * time.Millisecond)
	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()

	if f.fail != nil {
		return "", 0, f.fail
	}
	for _, d := range Domains {
		if f.noMail[d] {
			continue
		}
		f.mu.Lock()
		f.calls = append(f.calls, d)
		f.Client.Domain = d
		f.mu.Unlock()
		return "a@x", 1, nil
	}
	return "", 0, ErrNoMail
}

func TestAcquireSerialUnderConcurrency(t *testing.T) {
	f := newFake()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := f.acquire(context.Background()); err != nil {
				t.Errorf("acquire: %v", err)
			}
		}()
	}
	wg.Wait()
	if f.maxPar != 1 {
		t.Fatalf("acquire must be serial, saw %d concurrent", f.maxPar)
	}
}

func TestAcquireFallsBackWhenNoMail(t *testing.T) {
	f := newFake()
	f.noMail["icloud.com"] = true
	f.noMail["gmail.com"] = true
	email, id, err := f.acquire(context.Background())
	if err != nil {
		t.Fatalf("should fall back to outlook, got %v", err)
	}
	if email == "" || id == 0 {
		t.Fatal("empty result")
	}
	if f.Client.Domain != "outlook.com" {
		t.Fatalf("domain not updated: %s", f.Client.Domain)
	}
}

func TestAcquireAllEmpty(t *testing.T) {
	f := newFake()
	for _, d := range Domains {
		f.noMail[d] = true
	}
	_, _, err := f.acquire(context.Background())
	if !errors.Is(err, ErrNoMail) {
		t.Fatalf("want ErrNoMail, got %v", err)
	}
}

func TestWaitCodeFindsDigits(t *testing.T) {
	c := &Client{}
	// 直接走正则，不碰网络
	if m := codeRe.FindString("code is 123456."); m != "123456" {
		t.Fatalf("got %q", m)
	}
	_ = c
}

func TestWithout(t *testing.T) {
	got := without([]string{"a", "b", "c"}, "b")
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("got %v", got)
	}
}

func TestItoa(t *testing.T) {
	cases := map[int64]string{0: "0", 7: "7", 1234: "1234", -5: "-5"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d)=%q", in, got)
		}
	}
}
