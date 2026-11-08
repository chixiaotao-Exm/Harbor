// Package events 提供进程内事件总线与 SSE 输出。
package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Kind 是事件类别。
type Kind string

const (
	// KindLog 是给人看的日志行。
	KindLog Kind = "log"
	// KindStat 是统计快照。
	KindStat Kind = "stat"
	// KindItem 是工作明细变化。
	KindItem Kind = "item"
	// KindBoard 是异步看板变化。
	KindBoard Kind = "board"
)

// Event 是一条事件。
type Event struct {
	Kind Kind   `json:"kind"`
	At   string `json:"at"`
	Text string `json:"text,omitempty"`
	Data any    `json:"data,omitempty"`
}

// Hub 是订阅中心，同时保留最近一段历史供刷新后回放。
type Hub struct {
	mu    sync.RWMutex
	subs  map[chan Event]struct{}
	buf   int
	hist  []Event
	histN int
}

// NewHub 造一个带历史的总线。
func NewHub() *Hub {
	return &Hub{subs: map[chan Event]struct{}{}, buf: 256, histN: 400}
}

// Publish 向所有订阅者广播并记入历史，缓冲满则丢弃最旧。
func (h *Hub) Publish(e Event) {
	if e.At == "" {
		e.At = time.Now().Format("15:04:05")
	}
	h.mu.Lock()
	h.hist = append(h.hist, e)
	if len(h.hist) > h.histN {
		h.hist = h.hist[len(h.hist)-h.histN:]
	}
	subs := make([]chan Event, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- e:
			default:
			}
		}
	}
}

// History 返回历史副本（旧在前）。
func (h *Hub) History() []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Event, len(h.hist))
	copy(out, h.hist)
	return out
}

// Logf 发一条日志。
func (h *Hub) Logf(format string, args ...any) {
	h.Publish(Event{Kind: KindLog, Text: fmt.Sprintf(format, args...)})
}

// Subscribe 注册订阅，返回通道与取消函数。
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, h.buf)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Serve 把总线接到一个 SSE 响应上，直到客户端断开。
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, cancel := h.Subscribe()
	defer cancel()

	enc := json.NewEncoder(w)

	// 先把历史灌给新连接，刷新后不丢。
	for _, e := range h.History() {
		fmt.Fprint(w, "data: ")
		if err := enc.Encode(e); err != nil {
			return
		}
		fmt.Fprint(w, "\n")
	}
	flusher.Flush()

	beat := time.NewTicker(15 * time.Second)
	defer beat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, "data: ")
			if err := enc.Encode(e); err != nil {
				return
			}
			fmt.Fprint(w, "\n")
			flusher.Flush()
		}
	}
}
