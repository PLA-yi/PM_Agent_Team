package stream

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPublishSubscribeRoundTrip(t *testing.T) {
	b := NewBus()
	id := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, unsub := b.Subscribe(ctx, id, 0)
	defer unsub()

	go b.Publish(id, "planner", "start", map[string]string{"hello": "world"})

	select {
	case ev := <-ch:
		if ev.Agent != "planner" || ev.Step != "start" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestHistoryReplay(t *testing.T) {
	b := NewBus()
	id := uuid.New()
	for i := 0; i < 5; i++ {
		b.Publish(id, "search", "tool_call", map[string]int{"i": i})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	ch, unsub := b.Subscribe(ctx, id, 0)
	defer unsub()
	count := 0
	deadline := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case <-ch:
			count++
			if count == 5 {
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	if count != 5 {
		t.Fatalf("want 5 replayed events, got %d", count)
	}
}
