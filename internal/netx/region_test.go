package netx

import "testing"

func TestWithRegion(t *testing.T) {
	in := "socks5://u-region-US-sid-ABC-t-5:p@h:3010"
	if got := WithRegion(in, "jp"); got != "socks5://u-region-JP-sid-ABC-t-5:p@h:3010" {
		t.Fatalf("got %q", got)
	}
	if got := WithRegion(in, ""); got != in {
		t.Fatalf("empty should keep: %q", got)
	}
	if got := WithRegion(in, "ZZ"); got != in {
		t.Fatalf("unknown should keep: %q", got)
	}
	if RegionOf(in) != "US" {
		t.Fatalf("RegionOf %q", RegionOf(in))
	}
	if RegionOf("socks5://u:p@h:1") != "" {
		t.Fatal("no region should be empty")
	}
	if !ValidRegion("us") || ValidRegion("zz") {
		t.Fatal("ValidRegion wrong")
	}
}
