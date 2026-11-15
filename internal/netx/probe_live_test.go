package netx

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestProbeRegion(t *testing.T) {
	p := os.Getenv("HARBOR_PROXY")
	if p == "" {
		t.Skip("need HARBOR_PROXY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ip, err := Probe(ctx, p)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("出口 IP %s (%s)", ip, p[len(p)-30:])
}
