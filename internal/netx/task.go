package netx

import (
	"context"
	"sync"
)

// Task 表示一条任务专用的出口。
//
// 出口在任务开始时定一次，任务内固定；只有重试才换。
type Task struct {
	mu     sync.Mutex
	marker *Marker
	base   string
	cur    Exit
	log    func(string)
}

// NewTask 绑定一条任务的原始代理。
func NewTask(base string, m *Marker, log func(string)) *Task {
	return &Task{marker: m, base: base, log: log}
}

// Ensure 落实出口，可反复调用；已有出口则直接返回。
func (t *Task) Ensure(ctx context.Context) (Exit, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur.Proxy != "" {
		return t.cur, nil
	}
	return t.refreshLocked(ctx)
}

// Renew 强制换一个新出口，用于重试。
func (t *Task) Renew(ctx context.Context) (Exit, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cur = Exit{}
	return t.refreshLocked(ctx)
}

func (t *Task) refreshLocked(ctx context.Context) (Exit, error) {
	e, err := t.marker.Acquire(ctx, t.base)
	if err != nil {
		return Exit{}, err
	}
	t.cur = e
	if t.log != nil {
		t.log("出口就绪 " + e.IP)
	}
	return e, nil
}

// Current 返回当前出口（可能为空）。
func (t *Task) Current() Exit {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cur
}
