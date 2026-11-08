package mfa

import (
	"context"
	"errors"
	"testing"
)

// Q1 通过、Q3 首次 401，之后密码登录成功且登记成功。
func TestQ3UnauthorizedTriggersPasswordReauth(t *testing.T) {
	w := &fakeWorker{loginOK: true}
	var enrollCalls int
	e := newEngine(t, w)
	id := insert(t, e, "reauth@x.com", "tok", true)

	// 第一次登记失败，第二次成功
	w.enrollHook = func() error {
		enrollCalls++
		if enrollCalls == 1 {
			return errors.New("登记 HTTP 401: re-authenticate")
		}
		return nil
	}

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	acc, _ := e.Store.GetAccount(context.Background(), id)
	if acc.MFAState != "ok" {
		t.Fatalf("should recover after reauth, got %q (%v)", acc.MFAState, e.Failed())
	}
	if acc.Token != "fresh-token" {
		t.Fatalf("token not refreshed: %q", acc.Token)
	}
	found := false
	for _, s := range w.seq {
		if s == "q2" {
			found = true
		}
	}
	if !found {
		t.Fatal("password login never ran")
	}
}

// 无密码时 Q3 401 直接失败。
func TestQ3UnauthorizedNoPasswordFails(t *testing.T) {
	w := &fakeWorker{loginOK: true}
	w.enrollHook = func() error { return errors.New("401") }
	e := newEngine(t, w)
	id := insert(t, e, "nopw@x.com", "tok", false)

	_, _ = e.Start(context.Background(), []int64{id}, "socks5://u:p@h:1")
	e.Wait()

	acc, _ := e.Store.GetAccount(context.Background(), id)
	if acc.MFAState != "fail" {
		t.Fatalf("want fail, got %q", acc.MFAState)
	}
}
