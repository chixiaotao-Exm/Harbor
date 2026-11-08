package runner

import (
	"context"
	"testing"
	"time"
)

func TestDebugItem(t *testing.T) {
	src := &fakeSource{ttl: 5 * time.Millisecond}
	r := newRunner(t, src)
	_ = r.Start(Config{Count: 1, Threads: 1, Proxy: "socks5://u:p@h:1"})
	r.Wait()
	for _, it := range r.Items() {
		t.Logf("%+v", it)
	}
	_ = context.Background()
}
