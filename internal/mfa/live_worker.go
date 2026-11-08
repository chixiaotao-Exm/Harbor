package mfa

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chixi/harbor/internal/flow"
	"github.com/chixi/harbor/internal/netx"
)

// LiveWorker 用真实上游实现 Worker，每条任务一个固定出口。
type LiveWorker struct {
	Sess    *flow.Session
	Marker  *netx.Marker
	Proxy   string
	Renewed *string
}

// NewLiveWorker 造真实 worker。
func NewLiveWorker(marker *netx.Marker, proxy string) func(ctx context.Context, p string) (Worker, error) {
	return func(ctx context.Context, p string) (Worker, error) {
		s, err := flow.New(ctx, p)
		if err != nil {
			return nil, err
		}
		w := &LiveWorker{Sess: s, Marker: marker, Proxy: p}
		return w, nil
	}
}

// ProbeToken 校验登录态。
func (w *LiveWorker) ProbeToken(ctx context.Context, token string) bool {
	return flow.ProbeSession(ctx, w.Sess, token)
}

// LoginWithPassword 用账号密码换新登录态。
func (w *LiveWorker) LoginWithPassword(ctx context.Context, email, password string) (string, error) {
	return flow.PasswordLogin(ctx, w.Sess, email, password)
}

// Enroll 提交登记。
func (w *LiveWorker) Enroll(ctx context.Context, token string) (string, string, error) {
	return flow.EnrollTOTP(ctx, w.Sess, token)
}

// Activate 提交动态码。
func (w *LiveWorker) Activate(ctx context.Context, token, sessionID, code string) error {
	return flow.ActivateTOTP(ctx, w.Sess, token, sessionID, code)
}

// Decide 收尾确认。
func (w *LiveWorker) Decide(ctx context.Context, token string) error {
	return flow.FactorDecision(ctx, w.Sess, token)
}

// RenewExit 换出口，供引擎重试时调用。
func (w *LiveWorker) RenewExit(ctx context.Context) error {
	if w.Marker == nil || w.Proxy == "" {
		return nil
	}
	exit, err := w.Marker.Acquire(ctx, w.Proxy)
	if err != nil {
		return err
	}
	w.Proxy = exit.Proxy
	s, err := flow.New(ctx, exit.Proxy)
	if err != nil {
		return err
	}
	w.Sess = s
	return nil
}

// Close 释放连接。
func (w *LiveWorker) Close() {
	if w.Sess != nil {
		w.Sess.Close()
	}
}

var (
	errNoToken = errors.New("没有登录态")
	errNoEmail = errors.New("缺少账号")
)

func need(v, label string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("缺少%s", label)
	}
	return nil
}
