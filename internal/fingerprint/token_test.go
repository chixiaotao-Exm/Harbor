package fingerprint

import (
	"encoding/json"
	"testing"
)

func TestBuildSolvesAndFillsAnswer(t *testing.T) {
	p := New()
	srv := Server{Token: "srv-token"}
	srv.Proof.Required = true
	srv.Proof.Seed = "0.4242"
	srv.Proof.Difficulty = "0a1b2c"

	tok, err := Build(p, srv, "email_otp_validate")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if tok.C != "srv-token" || tok.ID != p.DeviceID || tok.Flow != "email_otp_validate" {
		t.Fatalf("bad token meta: %+v", tok)
	}

	arr, err := Decode(tok.P)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	ans, ok := arr[3].(float64)
	if !ok || ans == 0 {
		t.Fatalf("answer not filled: %v", arr[3])
	}
	h := itoa(int(ans))
	_ = h

	// t 字段必须是同数组的明文 base64
	back, err := Decode(prefix + tok.T + suffix)
	if err != nil {
		t.Fatalf("t not decodable: %v", err)
	}
	if back[4] != p.UA {
		t.Fatal("t payload mismatch")
	}
}

func TestBuildWithoutPoW(t *testing.T) {
	p := New()
	srv := Server{Token: "tok"}
	tok, err := Build(p, srv, "email_otp_send")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	arr, err := Decode(tok.P)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if arr[3].(float64) != 0 {
		t.Fatalf("answer should stay 0 without pow, got %v", arr[3])
	}
}

func TestTokensDiffer(t *testing.T) {
	p := New()
	srv := Server{Token: "tok"}
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		tok, err := Build(p, srv, "x")
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if seen[tok.P] {
			t.Fatal("payload repeated")
		}
		seen[tok.P] = true
	}
}

func TestTokenMarshal(t *testing.T) {
	tok := Token{P: "p", T: "t", C: "c", ID: "i", Flow: "f"}
	s, err := tok.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]string
	if err := json.Unmarshal([]byte(s), &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["flow"] != "f" || back["c"] != "c" {
		t.Fatalf("bad json: %s", s)
	}
}

func TestSOHeader(t *testing.T) {
	srv := Server{Token: "tok"}
	if h, _ := SOHeader(srv, "dev", "f"); h != "" {
		t.Fatal("empty collector should give empty header")
	}
	srv.SO.CollectorDX = "PBp5xxx"
	h, err := SOHeader(srv, "dev", "f")
	if err != nil || h == "" {
		t.Fatalf("expected header, got %q err %v", h, err)
	}
	var back map[string]string
	if err := json.Unmarshal([]byte(h), &back); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if back["so"] != "PBp5xxx" {
		t.Fatal("collector lost")
	}
}

func TestChallengeBody(t *testing.T) {
	p := New()
	body, err := ChallengeBody(p)
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	var back map[string]string
	if err := json.Unmarshal([]byte(body), &back); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if back["id"] != p.DeviceID {
		t.Fatal("device id missing")
	}
	if _, err := Decode(prefix + back["p"] + suffix); err != nil {
		t.Fatalf("p not decodable: %v", err)
	}
}
