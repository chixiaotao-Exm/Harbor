package fingerprint

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPayloadRoundTrip(t *testing.T) {
	p := New()
	seed := byte(137)
	payload, err := Encode(p.Arr(), seed)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.HasPrefix(payload, prefix) || !strings.HasSuffix(payload, suffix) {
		t.Fatalf("bad framing: %s", payload)
	}
	back, err := Decode(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(back) != 25 {
		t.Fatalf("want 25 fields, got %d", len(back))
	}
	if back[4] != p.UA {
		t.Fatalf("ua mismatch: %v", back[4])
	}
	if back[14] != p.DeviceID {
		t.Fatalf("device id mismatch: %v", back[14])
	}
}

func TestSeedIsSingleByte(t *testing.T) {
	p := New()
	for seed := 0; seed < 256; seed++ {
		payload, err := Encode(p.Arr(), byte(seed))
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		arr, err := Decode(payload)
		if err != nil {
			t.Fatalf("seed %d decode: %v", seed, err)
		}
		if len(arr) != 25 {
			t.Fatalf("seed %d lost fields", seed)
		}
	}
}

func TestSolveMeetsDifficulty(t *testing.T) {
	p := New()
	for i := 0; i < 200; i++ {
		seed := fmt.Sprintf("%.16f", randFloat(0, 1))
		diff := fmt.Sprintf("%06x", randInt(0x100000))
		arr, err := Solve(p.Arr(), seed, diff)
		if err != nil {
			t.Fatalf("solve with diff %s: %v", diff, err)
		}
		ans, ok := arr[3].(int)
		if !ok {
			t.Fatalf("answer not int: %T", arr[3])
		}
		h := fmt.Sprintf("%08x", fnvMurmur([]byte(seed+itoa(ans))))
		if h[:len(diff)] > diff {
			t.Fatalf("answer %d hash %s does not meet %s", ans, h, diff)
		}
	}
}

func TestSolveKeepsOtherFields(t *testing.T) {
	p := New()
	arr := p.Arr()
	out, err := Solve(arr, "0.5", "ff")
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if arr[3].(int) != 0 {
		t.Fatal("input array must not be mutated")
	}
	if out[4] != arr[4] || out[14] != arr[14] {
		t.Fatal("solve changed other fields")
	}
}

func TestFieldDomains(t *testing.T) {
	for i := 0; i < 100; i++ {
		p := New()
		if p.Width < 1280 || p.Width > 2560 {
			t.Fatalf("width out of range: %d", p.Width)
		}
		if p.Height < 720 || p.Height > 1440 {
			t.Fatalf("height out of range: %d", p.Height)
		}
		if !strings.Contains(p.UA, "Chrome/") || !strings.Contains(p.UA, "Windows NT") {
			t.Fatalf("bad ua: %s", p.UA)
		}
		if p.Lang == "" || strings.Contains(p.Lang, ",") {
			t.Fatalf("bad lang: %q", p.Lang)
		}
		if len(p.DeviceID) != 36 {
			t.Fatalf("bad device id: %s", p.DeviceID)
		}
		for _, k := range []string{"ai", "cache", "dataBuild", "solana", "installTrigger"} {
			if _, ok := p.Flags[k]; !ok {
				t.Fatalf("missing flag %s", k)
			}
		}
	}
}

func TestProfilesDiffer(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p := New()
		if seen[p.DeviceID] {
			t.Fatal("device id repeated")
		}
		seen[p.DeviceID] = true
	}
}

func TestHeadersCarryProfile(t *testing.T) {
	p := New()
	h := p.Headers()
	if h["User-Agent"] != p.UA {
		t.Fatal("ua not propagated")
	}
	if h["Accept-Language"] != p.Languages {
		t.Fatal("languages not propagated")
	}
	if !strings.Contains(h["sec-ch-ua"], "Chromium") {
		t.Fatal("sec-ch-ua malformed")
	}
}

func TestEncodePlainIsBase64JSON(t *testing.T) {
	p := New()
	s, err := EncodePlain(p.Arr())
	if err != nil {
		t.Fatalf("encode plain: %v", err)
	}
	raw, err := decoder(s)
	if err != nil {
		t.Fatalf("decode plain: %v", err)
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil {
		t.Fatalf("plain is not json: %v", err)
	}
	if len(arr) != 25 {
		t.Fatalf("want 25, got %d", len(arr))
	}
}
