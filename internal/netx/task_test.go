package netx

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func testMarker(failTimes int) (*Marker, *int) {
	var n int
	var mu sync.Mutex
	m := &Marker{
		Probe: func(ctx context.Context, proxy string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			n++
			if n <= failTimes {
				return "", errors.New("boom")
			}
			return "10.0.0." + itoa(n), nil
		},
		Sleep: func(time.Duration) {},
	}
	return m, &n
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func TestTaskEnsureStableWithinTask(t *testing.T) {
	m, _ := testMarker(0)
	task := NewTask("socks5://u-sid-AAA:p@h:1", m, nil)
	ctx := context.Background()

	first, err := task.Ensure(ctx)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := task.Ensure(ctx)
		if err != nil {
			t.Fatalf("ensure again: %v", err)
		}
		if again.Proxy != first.Proxy || again.IP != first.IP {
			t.Fatalf("exit changed inside task: %v -> %v", first, again)
		}
	}
}

func TestTaskRenewChangesExit(t *testing.T) {
	m, _ := testMarker(0)
	task := NewTask("socks5://u-sid-AAA:p@h:1", m, nil)
	ctx := context.Background()

	first, err := task.Ensure(ctx)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	next, err := task.Renew(ctx)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if next.Proxy == first.Proxy {
		t.Fatal("renew must change proxy")
	}
	if next.IP == first.IP {
		t.Fatal("renew must change ip")
	}
}

func TestTaskEnsureRetriesProbe(t *testing.T) {
	m, n := testMarker(2)
	task := NewTask("socks5://u-sid-AAA:p@h:1", m, nil)
	got, err := task.Ensure(context.Background())
	if err != nil {
		t.Fatalf("ensure should survive probe failures: %v", err)
	}
	if got.IP == "" {
		t.Fatal("no ip")
	}
	if *n != 3 {
		t.Fatalf("want 3 probe tries, got %d", *n)
	}
}

func TestTaskCurrentEmptyBeforeEnsure(t *testing.T) {
	m, _ := testMarker(0)
	task := NewTask("socks5://u:p@h:1", m, nil)
	if task.Current().Proxy != "" {
		t.Fatal("should be empty before ensure")
	}
}
