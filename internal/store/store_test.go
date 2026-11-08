package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

func openTmp(t *testing.T) *Store {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.db")
	s, err := Open(p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.db")
	s1, err := Open(p)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	v1, err := s1.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	s1.Close()

	// 再开一次不应报错，版本不变。
	s2, err := Open(p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	v2, err := s2.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("version 2: %v", err)
	}
	if v1 != v2 || v2 != schemaVersion {
		t.Fatalf("version drift: %d -> %d", v1, v2)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := openTmp(t)
	ctx := context.Background()

	got, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if got.Domain != "icloud.com" || !got.AddPassword || !got.AddMFA {
		t.Fatalf("unexpected defaults: %+v", got)
	}

	in := Settings{APIKey: "k-123", Proxy: "socks5h://u:p@h:1", Domain: "gmail.com",
		DefaultJobs: 3, DefaultWait: 7, AddPassword: false, AddMFA: true}
	if err := s.PutSettings(ctx, in); err != nil {
		t.Fatalf("put: %v", err)
	}
	back, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if back != in {
		t.Fatalf("round trip mismatch: %+v vs %+v", back, in)
	}
}

func TestConcurrentWrites(t *testing.T) {
	s := openTmp(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, err := s.InsertAccount(ctx, Account{
					Email:       emailFor(g, i),
					MFAState:    "none",
					CreatedAt:   now(),
					HasPassword: true,
				})
				if err != nil {
					errs <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent insert: %v", err)
	}

	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1000 {
		t.Fatalf("want 1000 rows, got %d", n)
	}
}

func emailFor(g, i int) string {
	return "u" + string(rune('a'+g)) + "-" + itoa(i) + "@x.com"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
