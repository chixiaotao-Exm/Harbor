package api

import "testing"

const sessionFixture = `{
  "accessToken":"eyJhbGciOiJSUzI1NiIs.abc.def",
  "expires":"2027-02-12T23:58:45.409Z",
  "account":{"id":"b9829606","planType":"free","structure":"personal"},
  "user":{"id":"user-1","email":"a@b.com","name":"zoe"},
  "statsigContext":{"country":"ID","region":"Jakarta","ip":"103.121.169.200","deviceId":"dev-1"}
}`

func TestParseSession(t *testing.T) {
	s, err := parseSession([]byte(sessionFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Plan != "free" || s.Country != "ID" || s.Region != "Jakarta" {
		t.Fatalf("bad summary: %+v", s)
	}
	if s.IP != "103.121.169.200" || s.DeviceID != "dev-1" || s.Email != "a@b.com" {
		t.Fatalf("bad meta: %+v", s)
	}
	if s.Expires != "2027-02-12T23:58:45.409Z" {
		t.Fatalf("expires: %s", s.Expires)
	}
}

func TestParseSessionEmpty(t *testing.T) {
	// 没登录时接口返回 {}，要当成失败而不是空结果
	if _, err := parseSession([]byte("{}")); err == nil {
		t.Fatal("空会话应当报错")
	}
	if _, err := parseSession([]byte("oops")); err == nil {
		t.Fatal("非 JSON 应当报错")
	}
}

func TestParseSessionPartial(t *testing.T) {
	// 只有 user 没有 accessToken 也算拿到了会话
	s, err := parseSession([]byte(`{"user":{"email":"x@y.com"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Email != "x@y.com" || s.Country != "" {
		t.Fatalf("bad: %+v", s)
	}
}
