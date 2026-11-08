// Package runner 是主任务队列：并发槽位、间隔、停止与统计。
package runner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chixi/harbor/internal/events"
	"github.com/chixi/harbor/internal/flow"
	"github.com/chixi/harbor/internal/mailbox"
	"github.com/chixi/harbor/internal/netx"
	"github.com/chixi/harbor/internal/store"
)

// Config 是一次运行参数。
type Config struct {
	Count       int
	Threads     int
	Delay       time.Duration
	Domain      string
	SetPassword bool
	EnableMFA   bool
	AutoEnqueue bool
	APIKey      string
	Proxy       string
	Region      string
}

// Stats 是实时统计快照。
type Stats struct {
	Running bool `json:"running"`
	Total   int  `json:"total"`
	Done    int  `json:"done"`
	OK      int  `json:"ok"`
	Fail    int  `json:"fail"`
}

// Rate 是成功率，0-100。
func (s Stats) Rate() float64 {
	if s.Done == 0 {
		return 0
	}
	return float64(s.OK) * 100 / float64(s.Done)
}

// Item 是工作明细行。
type Item struct {
	Email string `json:"email"`
	IP    string `json:"ip"`
	Stage string `json:"stage"`
	State string `json:"state"`
	Note  string `json:"note"`
}

// Credit 是取号结果。
type Credit struct {
	Email string
	Order int64
}

// CreditSource 可替换，便于测试。
type CreditSource interface {
	Acquire(ctx context.Context) (Credit, error)
	WaitCode(ctx context.Context, order int64, timeout time.Duration) string
	Done(ctx context.Context, order int64)
	Cancel(ctx context.Context, order int64)
	MarkNext(ctx context.Context, order int64)
}

// MailboxSource 用真实接码平台。
type MailboxSource struct{ C *mailbox.Client }

// Acquire 取一个号。
func (m *MailboxSource) Acquire(ctx context.Context) (Credit, error) {
	email, order, err := m.C.Acquire(ctx)
	return Credit{Email: email, Order: order}, err
}

// WaitCode 等验证码。
func (m *MailboxSource) WaitCode(ctx context.Context, order int64, timeout time.Duration) string {
	return m.C.WaitCode(ctx, order, timeout, nil)
}

// Done 标记完成。
func (m *MailboxSource) Done(ctx context.Context, order int64) { m.C.Status(ctx, order, 3) }

// Cancel 释放号码。
func (m *MailboxSource) Cancel(ctx context.Context, order int64) { m.C.Status(ctx, order, 2) }

// MarkNext 等下一条验证码。
func (m *MailboxSource) MarkNext(ctx context.Context, order int64) { m.C.Status(ctx, order, 5) }

// Runner 跑一批任务。
type Runner struct {
	Store  *store.Store
	Hub    *events.Hub
	Source CreditSource
	Marker *netx.Marker
	Paths  func(email string) (proxy, ip string)

	// NewSession / Register 可替换，便于不碰网络地测试。
	NewSession func(ctx context.Context, proxy string) (Session, error)
	Register   func(ctx context.Context, s Session, d flow.Deps, req flow.Request) (*flow.Result, error)

	// Enqueue 把新号投进异步队列，由外部注入。
	Enqueue func(ctx context.Context, ids []int64, proxy string) (int, error)

	mu          sync.Mutex
	stats       Stats
	items       []Item
	accounts    []string
	stop        bool
	runID       int64
	started     time.Time
	consecutive int
	wg          sync.WaitGroup
	done        chan struct{}
}

// Session 抽象出测试需要的会话能力。
type Session interface {
	Close()
}

func (r *Runner) newSession(ctx context.Context, proxy string) (Session, error) {
	if r.NewSession != nil {
		return r.NewSession(ctx, proxy)
	}
	return flow.New(ctx, proxy)
}

func (r *Runner) register(ctx context.Context, s Session, d flow.Deps, req flow.Request) (*flow.Result, error) {
	if r.Register != nil {
		return r.Register(ctx, s, d, req)
	}
	fs, ok := s.(*flow.Session)
	if !ok {
		return nil, errors.New("unexpected session type")
	}
	return flow.Register(ctx, fs, d, req)
}

// Stats 返回当前统计。
func (r *Runner) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// Items 返回明细副本。
func (r *Runner) Items() []Item {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Item, len(r.items))
	copy(out, r.items)
	return out
}

// Running 是否在跑。
func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats.Running
}

// Stop 停止补位，在跑的跑完。
func (r *Runner) Stop() {
	r.mu.Lock()
	r.stop = true
	r.mu.Unlock()
	rHub(r).Logf("收到停止指令，正在跑的会用完手里的号再收工")
}

func rHub(r *Runner) *events.Hub { return r.Hub }

// Start 开跑。已开跑返回错误。
func (r *Runner) Start(cfg Config) error {
	r.mu.Lock()
	if r.stats.Running {
		r.mu.Unlock()
		return errors.New("已有任务在跑")
	}
	if cfg.Count < 1 {
		cfg.Count = 1
	}
	if cfg.Threads < 1 {
		cfg.Threads = 1
	}
	if cfg.Threads > 8 {
		cfg.Threads = 8
	}
	r.stats = Stats{Running: true, Total: cfg.Count}
	r.items = make([]Item, 0, cfg.Count)
	r.stop = false
	r.started = time.Now()
	r.done = make(chan struct{})
	r.mu.Unlock()

	runID, err := r.openRun(cfg)
	if err != nil {
		r.finish()
		return err
	}
	r.mu.Lock()
	r.runID = runID
	r.mu.Unlock()

	r.Hub.Logf("开工，共 %d 单，并发 %d，间隔 %s", cfg.Count, cfg.Threads, cfg.Delay)
	r.publishStats()

	pending := make(chan int, cfg.Count)
	for i := 0; i < cfg.Count; i++ {
		pending <- i
	}
	close(pending)

	for w := 0; w < cfg.Threads; w++ {
		r.wg.Add(1)
		go r.slot(cfg, pending)
	}
	go func() {
		r.wg.Wait()
		r.finish()
		r.Hub.Logf("收工：成功 %d，失败 %d", r.Stats().OK, r.Stats().Fail)
		close(r.done)
	}()
	return nil
}

// Wait 等这一批彻底跑完（含收尾），主要给测试用。
func (r *Runner) Wait() {
	r.mu.Lock()
	d := r.done
	r.mu.Unlock()
	if d != nil {
		<-d
	}
}

func (r *Runner) slot(cfg Config, pending <-chan int) {
	defer r.wg.Done()
	for idx := range pending {
		if r.isStopped() {
			return
		}
		if !r.runOne(cfg, idx) {
			return // 连续取号失败，已暂停
		}
		if cfg.Delay > 0 && !r.isStopped() {
			time.Sleep(cfg.Delay)
		}
	}
}

func (r *Runner) isStopped() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stop
}

func (r *Runner) setStage(idx int, email, stage, state, note string) {
	r.setStageWithIP(idx, email, "", stage, state, note)
}

// setStageWithIP 同上，附带把出口 IP 记到明细上。
func (r *Runner) setStageWithIP(idx int, email, ip, stage, state, note string) {
	r.mu.Lock()
	for i := range r.items {
		if r.items[i].Email == email && email != "" {
			r.items[i].Stage, r.items[i].State, r.items[i].Note = stage, state, note
			if ip != "" {
				r.items[i].IP = ip
			}
			r.mu.Unlock()
			r.Hub.Publish(events.Event{Kind: events.KindItem, Data: r.items[i]})
			return
		}
	}
	r.items = append(r.items, Item{Email: email, IP: ip, Stage: stage, State: state, Note: note})
	row := r.items[len(r.items)-1]
	r.mu.Unlock()
	r.Hub.Publish(events.Event{Kind: events.KindItem, Data: row})
}

func (r *Runner) markDone(ok bool) {
	r.mu.Lock()
	r.stats.Done++
	if ok {
		r.stats.OK++
	} else {
		r.stats.Fail++
	}
	r.mu.Unlock()
	r.publishStats()
}

func (r *Runner) publishStats() {
	r.Hub.Publish(events.Event{Kind: events.KindStat, Data: r.Stats()})
}

func (r *Runner) finish() {
	r.mu.Lock()
	r.stats.Running = false
	runID := r.runID
	ok, fail := r.stats.OK, r.stats.Fail
	r.mu.Unlock()
	if runID > 0 {
		_ = r.Store.CloseRun(context.Background(), runID, ok, fail)
	}
	r.publishStats()
}

// runOne 跑一单。返回 false 表示连续取号失败、应暂停整批。
func (r *Runner) runOne(cfg Config, idx int) bool {
	ctx := context.Background()

	// 1. 先探活定出口
	exit, err := r.Marker.Acquire(ctx, netx.WithRegion(cfg.Proxy, cfg.Region))
	if err != nil {
		r.Hub.Logf("第 %d 单：出口拿不到 —— %v", idx+1, err)
		r.setStage(idx, "", "等待出口", "fail", err.Error())
		r.markDone(false)
		return true
	}

	// 2. 用这个出口取号
	src := r.Source
	if src == nil {
		src = &MailboxSource{C: mailbox.New(cfg.APIKey, cfg.Domain, "")}
	}
	credit, err := src.Acquire(ctx)
	if err != nil {
		r.Hub.Logf("第 %d 单：取号失败 —— %v", idx+1, err)
		r.setStage(idx, "", "取号", "fail", err.Error())
		r.markDone(false)
		r.noteRefund()
		return true
	}
	r.Hub.Logf("第 %d 单：拿到 %s（出口 %s）", idx+1, credit.Email, exit.IP)
	r.setStageWithIP(idx, credit.Email, exit.IP, "取号完成", "running", "")
	defer src.Done(ctx, credit.Order)

	// 3. 走流程
	sess, err := r.newSession(ctx, exit.Proxy)
	if err != nil {
		src.Cancel(ctx, credit.Order)
		r.Hub.Logf("%s 建会话失败 —— %v", credit.Email, err)
		r.setStage(idx, credit.Email, "建会话", "fail", err.Error())
		r.markDone(false)
		return true
	}
	defer sess.Close()

	deps := flow.Deps{
		WaitCode: func(ctx context.Context, order int64, timeout time.Duration) string {
			return src.WaitCode(ctx, order, timeout)
		},
		MarkNext: func(ctx context.Context, order int64) { src.MarkNext(ctx, order) },
		Log:      func(f string, a ...any) { r.Hub.Logf(f, a...) },
	}

	res, err := r.register(ctx, sess, deps, flow.Request{
		Email: credit.Email, OrderID: credit.Order,
		SetPassword: cfg.SetPassword, EnableMFA: false,
	})
	if err != nil {
		if errors.Is(err, flow.ErrBlocked) {
			// 被拦：换出口重来一次
			r.Hub.Logf("%s 被拦，换出口重试一次", credit.Email)
			if newExit, e2 := r.Marker.Acquire(ctx, netx.WithRegion(cfg.Proxy, cfg.Region)); e2 == nil {
				if s2, e3 := r.newSession(ctx, newExit.Proxy); e3 == nil {
					res, err = r.register(ctx, s2, deps, flow.Request{
						Email: credit.Email, OrderID: credit.Order, SetPassword: cfg.SetPassword,
					})
					s2.Close()
					if e3 == nil && err == nil {
						exit = newExit
					}
				}
			}
		}
		if err != nil {
			src.Cancel(ctx, credit.Order)
			r.Hub.Logf("%s 失败 —— %v", credit.Email, err)
			r.setStage(idx, credit.Email, "失败", "fail", err.Error())
			r.markDone(false)
			return true
		}
	}

	acc := store.Account{
		Email:       res.Email,
		Password:    res.Password,
		TotpSecret:  res.Secret,
		Name:        res.Name,
		Birthdate:   res.Birthdate,
		Token:       res.Token,
		Proxy:       exit.Proxy,
		IP:          exit.IP,
		HasPassword: res.HasPassword,
		MFAState:    mfaStateOf(res.Secret),
	}
	newID, err := r.Store.InsertAccount(ctx, acc)
	if err != nil {
		r.Hub.Logf("%s 入库失败 —— %v", credit.Email, err)
		r.setStage(idx, credit.Email, "入库", "fail", err.Error())
		r.markDone(false)
		return true
	}

	r.LogAccount(res.Email)
	r.setStage(idx, credit.Email, "完成", "ok", "")
	r.Hub.Logf("%s 开户成功", credit.Email)

	// 开户成功且没绑双因子时，按开关立即投递
	if cfg.AutoEnqueue && res.Secret == "" && res.HasPassword && newID > 0 {
		r.autoEnqueue(ctx, newID, res.Email, cfg.Proxy)
	}

	r.markDone(true)
	return true
}

// autoEnqueue 把刚开的号直接投进异步队列。
func (r *Runner) autoEnqueue(ctx context.Context, id int64, email, proxy string) {
	if r.Enqueue == nil {
		return
	}
	n, err := r.Enqueue(ctx, []int64{id}, proxy)
	if err != nil {
		r.Hub.Logf("%s 投递异步队列失败 —— %v", email, err)
		return
	}
	if n > 0 {
		r.Hub.Logf("%s 已自动投递到异步队列", email)
	}
}

func mfaStateOf(secret string) string {
	if secret != "" {
		return "ok"
	}
	return "queued"
}

// LogAccount 通知外部有新账号（异步队列用）。
func (r *Runner) LogAccount(email string) {
	r.mu.Lock()
	r.accounts = append(r.accounts, email)
	r.mu.Unlock()
}

func (r *Runner) openRun(cfg Config) (int64, error) {
	return r.Store.OpenRun(context.Background(), cfg.Count)
}

func (r *Runner) noteRefund() {
	r.mu.Lock()
	r.consecutive++
	n := r.consecutive
	r.mu.Unlock()
	if n >= 5 && !r.isStopped() {
		r.Stop()
		r.Hub.Logf("连续 %d 次取号失败，先暂停整批，检查 Key 或域名", n)
	}
}
