package netx

import "testing"

func TestRegionsHaveBothNames(t *testing.T) {
	list := Regions()
	if len(list) < 150 {
		t.Fatalf("region table too small: %d", len(list))
	}
	seen := map[string]bool{}
	for _, r := range list {
		if r.ZH == "" || r.EN == "" {
			t.Fatalf("region %s missing a name: %+v", r.Code, r)
		}
		if seen[r.Code] {
			t.Fatalf("duplicate code %s", r.Code)
		}
		seen[r.Code] = true
	}
}

func TestRegionTableOrdering(t *testing.T) {
	list := Regions()
	if list[0].Code != RandCode {
		t.Fatalf("first entry should be random, got %s", list[0].Code)
	}
	// 常见的几个必须靠前，便于选择
	head := map[string]bool{}
	for _, r := range list[:9] {
		head[r.Code] = true
	}
	for _, code := range []string{"US", "DE", "SG", "FR", "GB", "JP", "TH", "VN"} {
		if !head[code] {
			t.Errorf("%s should be in the first screen", code)
		}
	}
}

func TestFindRegion(t *testing.T) {
	r, ok := FindRegion("us")
	if !ok || r.Code != "US" || r.ZH != "美国" || r.EN != "United States" {
		t.Fatalf("bad lookup: %+v ok=%v", r, ok)
	}
	if _, ok := FindRegion("ZZ"); ok {
		t.Fatal("unknown code should miss")
	}
	if _, ok := FindRegion(""); ok {
		t.Fatal("empty code should miss")
	}
	if !ValidRegion("RAND") || !ValidRegion("JP") || ValidRegion("XX") {
		t.Fatal("ValidRegion wrong")
	}
}

func TestWithRegionRandStripsSegment(t *testing.T) {
	in := "socks5://u-region-US-sid-ABC:p@h:3010"
	got := WithRegion(in, RandCode)
	if got != "socks5://u-sid-ABC:p@h:3010" {
		t.Fatalf("random should strip region: %q", got)
	}
	// 没有 region 段时保持原样
	plain := "socks5://u-sid-ABC:p@h:3010"
	if WithRegion(plain, RandCode) != plain {
		t.Fatalf("plain proxy changed: %q", WithRegion(plain, RandCode))
	}
}

func TestWithRegionAllCodes(t *testing.T) {
	in := "socks5://u-region-US-sid-ABC:p@h:3010"
	for _, r := range Regions() {
		if r.Code == RandCode {
			continue
		}
		got := WithRegion(in, r.Code)
		if want := "socks5://u-region-" + r.Code + "-sid-ABC:p@h:3010"; got != want {
			t.Fatalf("%s: got %q want %q", r.Code, got, want)
		}
	}
}
