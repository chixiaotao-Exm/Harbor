package events

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPublishReachesSubscribers(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case e := <-ch:
			if e.Kind != KindLog || e.Text == "" {
				t.Errorf("bad event %+v", e)
			}
		case <-time.After(time.Second):
			t.Error("no event")
		}
	}()
	h.Logf("hello %d", 1)
	wg.Wait()
}

func TestSubscribeCancel(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe()
	cancel()
	h.mu.RLock()
	n := len(h.subs)
	h.mu.RUnlock()
	if n != 0 {
		t.Fatalf("subscriber not removed: %d", n)
	}
}

func TestHistoryKeepsRecent(t *testing.T) {
	h := NewHub()
	for i := 0; i < 10; i++ {
		h.Logf("line %d", i)
	}
	hist := h.History()
	if len(hist) != 10 {
		t.Fatalf("want 10 kept, got %d", len(hist))
	}
	if hist[0].Text != "line 0" || hist[9].Text != "line 9" {
		t.Fatalf("order broken: %q .. %q", hist[0].Text, hist[9].Text)
	}
}

func TestHistoryCapped(t *testing.T) {
	h := NewHub()
	for i := 0; i < h.histN+50; i++ {
		h.Logf("line %d", i)
	}
	hist := h.History()
	if len(hist) != h.histN {
		t.Fatalf("history not capped: %d", len(hist))
	}
	// 保留的应是最后一批
	if hist[len(hist)-1].Text != "line 449" {
		t.Fatalf("newest not last: %q", hist[len(hist)-1].Text)
	}
}

func TestReplayToNewSubscriber(t *testing.T) {
	h := NewHub()
	h.Logf("before connect")
	h.Logf("also before")

	srv := httptest.NewServer(http.HandlerFunc(h.Serve))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	var got strings.Builder
	buf := make([]byte, 256)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && got.Len() < 80 {
		n, err := resp.Body.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			break
		}
	}
	s := got.String()
	if !strings.Contains(s, "before connect") || !strings.Contains(s, "also before") {
		t.Fatalf("history not replayed: %q", s)
	}
}

func TestPublishNoSubscribers(t *testing.T) {
	h := NewHub()
	h.Logf("nobody listens") // 不应阻塞或 panic
}

func TestSlowSubscriberDropsOldest(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	defer cancel()

	for i := 0; i < h.buf+50; i++ {
		h.Logf("line %d", i)
	}
	// 只保证不阻塞，且通道里仍有内容
	select {
	case <-ch:
	default:
		t.Fatal("channel should hold buffered events")
	}
}

func TestServeStreams(t *testing.T) {
	h := NewHub()
	srv := httptest.NewServer(http.HandlerFunc(h.Serve))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "event-stream") {
		t.Fatalf("bad content type %q", ct)
	}

	// 等订阅真正建立后再发，避免竞态。
	deadline := time.Now().Add(2 * time.Second)
	for len(h.subs) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	h.Logf("streamed line")

	var got strings.Builder
	buf := make([]byte, 256)
	for !strings.Contains(got.String(), "streamed line") {
		n, err := resp.Body.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if !strings.Contains(got.String(), "streamed line") {
		t.Fatalf("payload missing: %q", got.String())
	}
}
