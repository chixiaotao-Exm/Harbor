package netx

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeCleansDirty(t *testing.T) {
	cases := map[string]string{
		// 串内空格（本次故障原样）
		"socks5h://123waddsad-region-US-sid-vb5yexrK-t-5: 123wadasd  @us.arxlabs.io:3010": "socks5h://123waddsad-region-US-sid-vb5yexrK-t-5:123wadasd@us.arxlabs.io:3010",
		"  socks5://u:p@h:1  ": "socks5://u:p@h:1",
		"socks5://u:p@h:1\n":   "socks5://u:p@h:1",
		"socks5://u：p＠h：3010":  "socks5://u:p@h:3010",
		"u:p@h:3010":           "socks5h://u:p@h:3010",
		"h:3010":               "http://h:3010",
		"":                     "",
		"   ":                  "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestNormalizeKeepsScheme(t *testing.T) {
	for _, in := range []string{
		"socks5://u:p@h:1", "socks5h://u:p@h:1", "http://h:1", "https://h:1", "socks4://h:1",
	} {
		if got := Normalize(in); got != in {
			t.Errorf("scheme lost: %q -> %q", in, got)
		}
	}
}

func TestRotateChangesSIDEachTime(t *testing.T) {
	base := "socks5://u-sid-AAAAAAAA-t-5:p@h:3010"
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		got := Rotate(base)
		if !strings.HasPrefix(got, "socks5h://") {
			t.Fatalf("socks5 must upgrade to socks5h: %q", got)
		}
		if strings.Contains(got, "sid-AAAAAAAA") {
			t.Fatalf("sid not rotated: %q", got)
		}
		if seen[got] {
			t.Fatalf("duplicate sid at %d: %q", i, got)
		}
		seen[got] = true
	}
}

func TestRotateSessionSegment(t *testing.T) {
	got := Rotate("socks5://u-session-ABCDEFGH:p@h:1")
	if strings.Contains(got, "session-ABCDEFGH") {
		t.Fatalf("session not rotated: %q", got)
	}
	if !strings.Contains(got, "-session-") {
		t.Fatalf("session marker lost: %q", got)
	}
}

func TestRotateWithoutSID(t *testing.T) {
	in := "socks5://u:p@h:3010"
	if Rotate(in) != "socks5h://u:p@h:3010" {
		t.Fatal("plain proxy should just upgrade scheme")
	}
}

func TestHostOf(t *testing.T) {
	if got := HostOf("socks5h://u:p@us.host.example:3010"); got != "us.host.example:3010" {
		t.Fatalf("got %q", got)
	}
}

func TestAcquireRetriesUntilSuccess(t *testing.T) {
	var tries int
	m := &Marker{
		Probe: func(ctx context.Context, proxy string) (string, error) {
			tries++
			if tries < 3 {
				return "", errors.New("boom")
			}
			return "1.2.3.4", nil
		},
		Sleep: func(time.Duration) {},
	}
	got, err := m.Acquire(context.Background(), "socks5://u-sid-AAA:p@h:1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if got.IP != "1.2.3.4" || got.Proxy == "" {
		t.Fatalf("bad exit %+v", got)
	}
	if tries != 3 {
		t.Fatalf("want 3 attempts, got %d", tries)
	}
}

func TestAcquireChangesProxyPerAttempt(t *testing.T) {
	var got []string
	var mu sync.Mutex
	m := &Marker{
		Probe: func(ctx context.Context, proxy string) (string, error) {
			mu.Lock()
			got = append(got, proxy)
			mu.Unlock()
			return "", errors.New("x")
		},
		Sleep: func(time.Duration) {},
	}
	_, err := m.Acquire(context.Background(), "socks5://u-sid-AAA:p@h:1")
	if !errors.Is(err, ErrNoExit) {
		t.Fatalf("want ErrNoExit, got %v", err)
	}
	if len(got) != maxProbeRounds {
		t.Fatalf("want %d attempts, got %d", maxProbeRounds, len(got))
	}
	uniq := map[string]bool{}
	for _, g := range got {
		uniq[g] = true
	}
	if len(uniq) != len(got) {
		t.Fatalf("proxy not rotated between attempts: %v", got)
	}
}

func TestAcquireEmptyProxy(t *testing.T) {
	m := &Marker{Probe: func(context.Context, string) (string, error) { return "x", nil }}
	if _, err := m.Acquire(context.Background(), "  "); !errors.Is(err, ErrNoExit) {
		t.Fatalf("want ErrNoExit, got %v", err)
	}
}

func TestPickIP(t *testing.T) {
	if pickIP([]byte(`{"ip":"9.9.9.9"}`)) != "9.9.9.9" {
		t.Fatal("json ip")
	}
	if pickIP([]byte("8.8.8.8\n")) != "8.8.8.8" {
		t.Fatal("plain ip")
	}
	if pickIP([]byte("noise")) != "" {
		t.Fatal("noise should be empty")
	}
}
