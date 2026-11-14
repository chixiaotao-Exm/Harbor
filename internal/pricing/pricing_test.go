package pricing

import (
	"context"
	"encoding/json"
	"testing"
)

const fixture = `{
  "country_code":"ID",
  "currency_config":{
    "symbol":"Rp","symbol_code":"IDR",
    "plus":{"month":{"amount":349000.0,"tax":"exclusive"},"year":{"amount":290833.33}},
    "go":{"month":{"amount":80000.0}},
    "pro":{"month":{"amount":3000000.0}},
    "business":{"month":{"amount":400000.0},"year":{"amount":320000.0}},
    "promos":{
      "business_free_trial":{"enabled":true,"type":"value","amount":0.0},
      "plus_intro_offer_3m":{"enabled":false},
      "plus_intro_offer_12m":{"enabled":true,"type":"value","amount":10.0}
    }
  }
}`

func TestParse(t *testing.T) {
	p, err := parse("ID", []byte(fixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Country != "ID" || p.Currency != "IDR" || p.Symbol != "Rp" {
		t.Fatalf("bad meta: %+v", p)
	}
	if got := p.Plans["plus"].Month; got != 349000 {
		t.Fatalf("plus month = %v", got)
	}
	if got := p.Plans["plus"].Year; got != 290833.33 {
		t.Fatalf("plus year = %v", got)
	}
	if _, ok := p.Plans["business"]; !ok {
		t.Fatal("business missing")
	}
	if len(p.Promos) != 3 {
		t.Fatalf("promos = %d", len(p.Promos))
	}
	// 排序后应稳定
	if p.Promos[0].Name != "business_free_trial" {
		t.Fatalf("promos not sorted: %+v", p.Promos)
	}
}

func TestParseBadJSON(t *testing.T) {
	if _, err := parse("US", []byte("not json")); err == nil {
		t.Fatal("should fail on bad json")
	}
}

func TestGetRejectsBadCodes(t *testing.T) {
	c := New("")
	for _, cc := range []string{"", "  ", "ZZ", "RAND"} {
		if _, err := c.Get(context.Background(), cc); err == nil {
			t.Fatalf("%q should be rejected", cc)
		}
	}
}

func TestScanKeepsOrder(t *testing.T) {
	// 用假取数避免联网：直接验证扫描结果长度与顺序占位
	c := New("")
	got := c.Scan(context.Background(), []string{"ZZ", "XX"}, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if got[0].Country != "ZZ" || got[1].Country != "XX" {
		t.Fatalf("order broken: %+v", got)
	}
	if got[0].Error == "" {
		t.Fatal("invalid code should carry an error")
	}
}

func TestDeviceIDStable(t *testing.T) {
	a, b := deviceID(), deviceID()
	if a != b || len(a) != 36 {
		t.Fatalf("device id unstable: %q %q", a, b)
	}
	if _, err := json.Marshal(a); err != nil {
		t.Fatal(err)
	}
}
