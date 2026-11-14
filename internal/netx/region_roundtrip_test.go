package netx

import "testing"

func TestRegionRoundTrip(t *testing.T) {
	base := "socks5://user-region-US-sid-ABC-t-5:p@h:3010"

	// 选中国码 -> 换成该码
	if got := WithRegion(base, "JP"); got != "socks5://user-region-JP-sid-ABC-t-5:p@h:3010" {
		t.Fatalf("to JP: %q", got)
	}

	// 选随机 -> 去掉 region 段
	randed := WithRegion(base, RandCode)
	if randed != "socks5://user-sid-ABC-t-5:p@h:3010" {
		t.Fatalf("to RAND: %q", randed)
	}

	// 随机之后再选国家 -> region 段要能插回去
	back := WithRegion(randed, "DE")
	if back != "socks5://user-region-DE-sid-ABC-t-5:p@h:3010" {
		t.Fatalf("RAND then DE: %q", back)
	}
}

func TestRegionInsertWithSessionMarker(t *testing.T) {
	in := "socks5://user-session-ABC:p@h:1"
	got := WithRegion(in, "SG")
	if got != "socks5://user-region-SG-session-ABC:p@h:1" {
		t.Fatalf("session insert: %q", got)
	}
}

func TestRegionNoMarkerLeavesProxy(t *testing.T) {
	in := "socks5://user:p@h:1"
	if got := WithRegion(in, "US"); got != in {
		t.Fatalf("should stay untouched: %q", got)
	}
	if got := WithRegion(in, RandCode); got != in {
		t.Fatalf("rand on plain should stay: %q", got)
	}
}
