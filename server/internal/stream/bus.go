// Package stream 提供任务级事件总线：Agent 推送进度，HTTP 层订阅推 SSE。
package stream

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event 一条进度事件
type Event struct {
	TaskID    uuid.UUID       `json:"task_id"`
	Seq       int64           `json:"seq"`
	Agent     string          `json:"agent"`
	Step      string          `json:"step"`     // start / tool_call / tool_result / thought / message / done / error
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// Bus 多任务事件总线
type Bus struct {
	mu       sync.RWMutex
	subs     map[uuid.UUID]map[chan Event]struct{}
	history  map[uuid.UUID][]Event // 缓存最近事件，新订阅者可回放
	maxHist  int
	seq      map[uuid.UUID]int64
}

func NewBus() *Bus {
	return &Bus{
		subs:    make(map[uuid.UUID]map[chan Event]struct{}),
		history: make(map[uuid.UUID][]Event),
		seq:     make(map[uuid.UUID]int64),
		maxHist: 500,
	}
}

// Publish 推送一条事件给所有订阅者，同时缓存到 history。
func (b *Bus) Publish(taskID uuid.UUID, agent, step string, payload interface{}) Event {
	raw, _ := json.Marshal(payload)
	b.mu.Lock()
	b.seq[taskID]++
	ev := Event{
		TaskID:    taskID,
		Seq:       b.seq[taskID],
		Agent:     agent,
		Step:      step,
		Payload:   raw,
		CreatedAt: time.Now(),
	}
	hist := b.history[taskID]
	hist = append(hist, ev)
	if len(hist) > b.maxHist {
		hist = hist[len(hist)-b.maxHist:]
	}
	b.history[taskID] = hist

	// 锁内快照订阅者，锁外发送，避免迭代共享 map
	subSnap := make([]chan Event, 0, len(b.subs[taskID]))
	for ch := range b.subs[taskID] {
		subSnap = append(subSnap, ch)
	}
	b.mu.Unlock()

	for _, ch := range subSnap {
		select {
		case ch <- ev:
		default:
			// 慢消费者丢事件，依赖 history 回放兜底
		}
	}
	return ev
}

// Subscribe 订阅任务事件，返回 channel 与 unsubscribe 函数。
// 如果 sinceSeq > 0，会先把 history 中 seq > sinceSeq 的事件回放进 channel。
func (b *Bus) Subscribe(ctx context.Context, taskID uuid.UUID, sinceSeq int64) (<-chan Event, func()) {
	ch := make(chan Event, 64)

	b.mu.Lock()
	if _, ok := b.subs[taskID]; !ok {
		b.subs[taskID] = make(map[chan Event]struct{})
	}
	b.subs[taskID][ch] = struct{}{}
	hist := append([]Event(nil), b.history[taskID]...)
	b.mu.Unlock()

	// 回放
	go func() {
		for _, ev := range hist {
			if ev.Seq <= sinceSeq {
				continue
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	unsub := func() {
		b.mu.Lock()
		if set, ok := b.subs[taskID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.subs, taskID)
			}
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, unsub
}

// History 拿到任务全部缓存事件（用于持久化前的快照）
func (b *Bus) History(taskID uuid.UUID) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Event, len(b.history[taskID]))
	copy(out, b.history[taskID])
	return out
}

// Clear 清理任务的 history（任务完成后可调用）
func (b *Bus) Clear(taskID uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.history, taskID)
	delete(b.seq, taskID)
}
