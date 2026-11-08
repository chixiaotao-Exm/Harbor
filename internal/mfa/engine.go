// Package mfa 跑双因子绑定的异步任务。
//
// 阶段：Q1 校验登录态 →（失败）Q2 账号密码登录 → Q3 登记 → Q4 激活 → Q5 固化。
// 每步先自动重试 3 次（退避 3/6/12 秒），重试会换出口；超限进失败页。
package mfa

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chixi/harbor/internal/events"
	"github.com/chixi/harbor/internal/flow"
	"github.com/chixi/harbor/internal/netx"
	"github.com/chixi/harbor/internal/store"
)

// 阶段编号。
const (
	Q1 = 1
	Q2 = 2
	Q3 = 3
	Q4 = 4
	Q5 = 5
)

// 失败原因分类。
const (
	NeedPasswordLogin = "need_password_login"
	StageFailed       = "stage_failed"
)

var (
	// 重试次数与退避。
	MaxAttempts = 3
	Backoff     = []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second}
)

// Worker 绑一份账号，供测试替换。
type Worker interface {
	ProbeToken(ctx context.Context, token string) bool
	LoginWithPassword(ctx context.Context, email, password string) (string, error)
	Enroll(ctx context.Context, token string) (secret, sessionID string, err error)
	Activate(ctx context.Context, token, sessionID, code string) error
	Decide(ctx context.Context, token string) error
	RenewExit(ctx context.Context) error
}

// Engine 驱动队列。
type Engine struct {
	Store  *store.Store
	Hub    *events.Hub
	New    func(ctx context.Context, proxy string) (Worker, error)
	Marker *netx.Marker
	Proxy  string

	mu      sync.Mutex
	board   map[int]map[int64]string // q -> id -> email
	failed  map[int64]string         // id -> reason
	running bool
	pending int
	wg      sync.WaitGroup
}

// Board 返回各列正在处理的账号。
func (e *Engine) Board() map[int][]BoardRow {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[int][]BoardRow{}
	for q := Q1; q <= Q5; q++ {
		rows := make([]BoardRow, 0, len(e.board[q]))
		for id, email := range e.board[q] {
			rows = append(rows, BoardRow{ID: id, Email: email, Q: q})
		}
		out[q] = rows
	}
	return out
}

// BoardRow 是看板一行。
type BoardRow struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Q     int    `json:"q"`
}

// Failed 返回失败页内容。
func (e *Engine) Failed() []FailedRow {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]FailedRow, 0, len(e.failed))
	for id, reason := range e.failed {
		out = append(out, FailedRow{ID: id, Reason: reason})
	}
	return out
}

// FailedRow 是失败页一行。
type FailedRow struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
	Times  int    `json:"times"`
}

// Running 是否在跑。
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

func (e *Engine) init() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.board == nil {
		e.board = map[int]map[int64]string{}
		for q := Q1; q <= Q5; q++ {
			e.board[q] = map[int64]string{}
		}
	}
	if e.failed == nil {
		e.failed = map[int64]string{}
	}
}

// move 把账号从任何旧列移到目标列。
func (e *Engine) move(id int64, _ int, to int, email string) {
	e.mu.Lock()
	for q := Q1; q <= Q5; q++ {
		delete(e.board[q], id)
	}
	e.board[to][id] = email
	e.mu.Unlock()
	e.publishBoard()
}

func (e *Engine) clear(id int64) {
	e.mu.Lock()
	for q := Q1; q <= Q5; q++ {
		delete(e.board[q], id)
	}
	e.mu.Unlock()
	e.publishBoard()
}

func (e *Engine) publishBoard() {
	if e.Hub != nil {
		e.Hub.Publish(events.Event{Kind: events.KindBoard, Data: e.Board()})
	}
}

func (e *Engine) logf(f string, a ...any) {
	if e.Hub != nil {
		e.Hub.Logf(f, a...)
	}
}

// Start 把一批账号投入队列。
//
// 队列已在跑时允许追加，不报错（开户后自动投递需要）。
func (e *Engine) Start(ctx context.Context, ids []int64, proxy string) (int, error) {
	e.init()
	e.mu.Lock()
	alreadyRunning := e.running
	if !alreadyRunning {
		e.running = true
		e.failed = map[int64]string{}
	}
	e.mu.Unlock()

	if proxy != "" {
		e.mu.Lock()
		e.Proxy = proxy
		e.mu.Unlock()
	}
	queued := 0
	for _, id := range ids {
		acc, err := e.Store.GetAccount(ctx, id)
		if err != nil {
			continue
		}
		if acc.TotpSecret != "" {
			continue
		}
		_ = e.Store.SetMFAState(ctx, id, "queued", "")
		e.wg.Add(1)
		e.mu.Lock()
		e.pending++
		e.mu.Unlock()
		queued++
		go func(id int64, email string) {
			defer func() {
				e.mu.Lock()
				e.pending--
				left := e.pending
				if left == 0 {
					e.running = false
				}
				e.mu.Unlock()
				if left == 0 {
					e.publishBoard()
					e.logf("异步队列收工")
				}
				e.wg.Done()
			}()
			e.runOne(context.Background(), id, email)
		}(id, acc.Email)
	}
	// 队列本来就空着又有新活，才需要等它排空
	if !alreadyRunning && queued > 0 {
		go func() {
			e.wg.Wait()
		}()
	}
	return queued, nil
}

// Wait 等队列跑空，测试用。
func (e *Engine) Wait() { e.wg.Wait() }

func (e *Engine) runOne(ctx context.Context, id int64, email string) {
	acc, err := e.Store.GetAccount(ctx, id)
	if err != nil {
		return
	}
	if acc.TotpSecret != "" {
		return
	}

	// Q1 校验登录态
	if e.stepWithRetry(ctx, id, email, Q1, func(w Worker) error {
		if w.ProbeToken(ctx, acc.Token) {
			return nil
		}
		return errBadToken
	}) {
		// Q1 通过，跳到 Q3
	} else if !e.refreshToken(ctx, &acc, id, email, NeedPasswordLogin) {
		e.fail(ctx, id, email, Q1, NeedPasswordLogin, "登录态失效且无法用密码换新，需要人工补密码")
		return
	}

	// Q3 登记
	var secret, sessionID string
	enroll := func() error {
		s, sid, err := func() (string, string, error) {
			w, werr := e.worker(ctx)
			if werr != nil {
				return "", "", werr
			}
			return w.Enroll(ctx, acc.Token)
		}()
		if err != nil {
			return err
		}
		secret, sessionID = s, sid
		return nil
	}
	// Q3 登记。401 表示「必须重新认证」，直接换新 token 再来，不必干重试。
	if !e.stepWithRetry(ctx, id, email, Q3, func(Worker) error { return enroll() }, isAuthErr) {
		e.logf("%s 登记被要求重新认证，改用密码登录后重试", email)
		if !e.refreshToken(ctx, &acc, id, email, StageFailed) {
			e.fail(ctx, id, email, Q3, NeedPasswordLogin, "登记被拒且无法重新认证，需要人工处理")
			return
		}
		if !e.stepWithRetry(ctx, id, email, Q3, func(Worker) error { return enroll() }) {
			e.fail(ctx, id, email, Q3, StageFailed, "双因子登记失败")
			return
		}
	}

	// Q4 激活
	if !e.stepWithRetry(ctx, id, email, Q4, func(w Worker) error {
		code, err := flow.TOTPCode(secret)
		if err != nil {
			return err
		}
		return w.Activate(ctx, acc.Token, sessionID, code)
	}) {
		e.fail(ctx, id, email, Q4, StageFailed, "动态码激活失败")
		return
	}

	// Q5 固化
	if !e.stepWithRetry(ctx, id, email, Q5, func(w Worker) error {
		return w.Decide(ctx, acc.Token)
	}) {
		e.fail(ctx, id, email, Q5, StageFailed, "收尾确认失败")
		return
	}

	_ = e.Store.SetMFAState(ctx, id, "ok", "")
	_ = e.Store.SetSecret(ctx, id, secret)
	e.clear(id)
	e.logf("%s 双因子已绑定", email)
}

var errBadToken = errors.New("登录态无效")

// isAuthErr 判断是否「需要重新认证」，这类错重试无用，应换 token。
func isAuthErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "401") || strings.Contains(msg, "re-authenticate")
}

// refreshToken 用账号密码换一份新登录态并写回；无密码返回 false。
//
// 上游绑定双因子要求「近期认证」，光有可用 token 不够，
// 所以登记被 401 顶回来时也走这里。
func (e *Engine) refreshToken(ctx context.Context, acc *store.Account, id int64, email, failReason string) bool {
	if !acc.HasPassword || acc.Password == "" {
		e.logf("%s 没有密码，无法重新认证", email)
		return false
	}
	var fresh string
	ok := e.stepWithRetry(ctx, id, email, Q2, func(w Worker) error {
		tok, err := w.LoginWithPassword(ctx, email, acc.Password)
		if err != nil {
			return err
		}
		fresh = tok
		return nil
	})
	if !ok {
		e.logf("%s 密码登录失败（%s）", email, failReason)
		return false
	}
	acc.Token = fresh
	_ = e.Store.UpdateToken(ctx, id, fresh)
	return true
}

// stepWithRetry 跑一步，失败按退避重试，重试时换出口。
//
// stopOn 可传入判定函数：命中即不再重试，直接交回上层处理（如 401 需重新认证）。
func (e *Engine) stepWithRetry(ctx context.Context, id int64, email string, q int, fn func(Worker) error, stopOn ...func(error) bool) bool {
	e.move(id, q-1, q, email)
	e.logf("%s 开始 Q%d：%s", email, q, stageName(q))

	shouldStop := func(error) bool { return false }
	if len(stopOn) > 0 && stopOn[0] != nil {
		shouldStop = stopOn[0]
	}

	var last error
	for attempt := 0; attempt < MaxAttempts; attempt++ {
		if attempt > 0 {
			wait := Backoff[min(attempt-1, len(Backoff)-1)]
			e.logf("%s Q%d 第 %d 次重试，等 %s 并换出口", email, q, attempt, wait)
			sleepFn(ctx, wait)
			e.renew(ctx)
		}
		w, err := e.worker(ctx)
		if err != nil {
			last = err
			continue
		}
		err = fn(w)
		if err == nil {
			return true
		}
		last = err
		e.logf("%s Q%d 失败：%v", email, q, err)
		if shouldStop(err) {
			e.logf("%s Q%d 命中不可重试错误，交给上层", email, q)
			break
		}
	}
	_ = last
	return false
}

func (e *Engine) worker(ctx context.Context) (Worker, error) {
	if e.New == nil {
		return nil, errors.New("未配置 worker")
	}
	return e.New(ctx, e.Proxy)
}

func (e *Engine) renew(ctx context.Context) bool {
	if e.Marker == nil || e.Proxy == "" {
		return false
	}
	exit, err := e.Marker.Acquire(ctx, e.Proxy)
	if err != nil {
		return false
	}
	e.mu.Lock()
	e.Proxy = exit.Proxy
	e.mu.Unlock()
	return true
}

func (e *Engine) fail(ctx context.Context, id int64, email string, q int, reason, note string) {
	times, _ := e.Store.BumpAttempts(ctx, id)
	_ = e.Store.SetMFAState(ctx, id, "fail", reason+": "+note)
	e.mu.Lock()
	e.failed[id] = reason + ": " + note
	_ = times
	e.mu.Unlock()
	e.clear(id)
	e.logf("%s 双因子失败（Q%d），已入失败页：%s", email, q, note)
}

// Retry 重新投失败页里的账号。
func (e *Engine) Retry(ctx context.Context, ids []int64) (int, error) {
	if len(ids) == 0 {
		e.mu.Lock()
		for id := range e.failed {
			ids = append(ids, id)
		}
		e.mu.Unlock()
	}
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return 0, errors.New("异步队列还在跑")
	}
	e.mu.Unlock()
	return e.Start(ctx, ids, "")
}

func stageName(q int) string {
	switch q {
	case Q1:
		return "校验登录态"
	case Q2:
		return "账号密码登录"
	case Q3:
		return "提交双因子登记"
	case Q4:
		return "动态码激活"
	case Q5:
		return "收尾确认"
	}
	return fmt.Sprintf("Q%d", q)
}

// sleep 可被测试替换。
var sleepFn = func(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetSleep 供测试替换退避等待。
func SetSleep(fn func(ctx context.Context, d time.Duration)) { sleepFn = fn }
