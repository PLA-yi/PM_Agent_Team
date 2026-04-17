package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeClient 用于测试 — 可控制返回的 usage / err
type fakeClient struct {
	resp     *Response
	err      error
	calls    int
	failN    int // 前 N 次返回 err，之后返回 resp
}

func (f *fakeClient) IsMock() bool { return true }
func (f *fakeClient) Complete(_ context.Context, _ Request) (*Response, error) {
	f.calls++
	if f.calls <= f.failN {
		return nil, f.err
	}
	return f.resp, nil
}

func TestRecorder_BasicAccumulation(t *testing.T) {
	r := NewRecorder(0) // no budget
	r.Record(CallRecord{TaskID: "t1", Agent: "planner", Model: "claude-sonnet-4-5", PromptTokens: 100, OutputTokens: 200, TotalTokens: 300, CostUSD: 0.0033, ElapsedMs: 1200})
	r.Record(CallRecord{TaskID: "t1", Agent: "writer", Model: "claude-sonnet-4-5", PromptTokens: 500, OutputTokens: 1000, TotalTokens: 1500, CostUSD: 0.0165, ElapsedMs: 4500})

	u := r.Get("t1")
	if u.Calls != 2 {
		t.Errorf("calls want 2 got %d", u.Calls)
	}
	if u.TotalTokens != 1800 {
		t.Errorf("tokens want 1800 got %d", u.TotalTokens)
	}
	if u.ByAgent["planner"].Calls != 1 || u.ByAgent["writer"].Calls != 1 {
		t.Errorf("by agent: %+v", u.ByAgent)
	}
	if u.ByModel["claude-sonnet-4-5"].Calls != 2 {
		t.Errorf("by model: %+v", u.ByModel)
	}
}

func TestRecorder_BudgetExceeded(t *testing.T) {
	r := NewRecorder(0.01) // very low budget
	ok1 := r.Record(CallRecord{TaskID: "t2", CostUSD: 0.005})
	if !ok1 {
		t.Fatal("first call should pass budget")
	}
	ok2 := r.Record(CallRecord{TaskID: "t2", CostUSD: 0.020}) // accumulated exceeds 0.01
	if ok2 {
		t.Fatal("second call should exceed budget")
	}
	u := r.Get("t2")
	if !u.BudgetExceeded {
		t.Errorf("BudgetExceeded should be true")
	}
}

func TestEstimateCost(t *testing.T) {
	c := EstimateCost("claude-sonnet-4-5", 1000, 500)
	want := 0.003 + 0.015*0.5 // 0.003 + 0.0075 = 0.0105
	if c < want-0.0001 || c > want+0.0001 {
		t.Errorf("cost want %.4f got %.4f", want, c)
	}
	// unknown model → 0
	if EstimateCost("nonexistent-model", 1000, 1000) != 0 {
		t.Errorf("unknown should be 0")
	}
}

func TestMetered_RecordsTaskAndAgent(t *testing.T) {
	r := NewRecorder(0)
	inner := &fakeClient{resp: &Response{Usage: Usage{PromptTokens: 100, CompletionTokens: 200, TotalTokens: 300}}}
	m := NewMetered(inner, r, "test-provider")

	ctx := WithTask(context.Background(), "task-X")
	ctx = WithAgent(ctx, "planner")
	_, err := m.Complete(ctx, Request{Model: "claude-sonnet-4-5"})
	if err != nil {
		t.Fatal(err)
	}
	u := r.Get("task-X")
	if u == nil {
		t.Fatal("no record")
	}
	if u.ByAgent["planner"].Calls != 1 {
		t.Errorf("planner not attributed")
	}
	if u.CostUSD <= 0 {
		t.Errorf("cost should be > 0 (got %v)", u.CostUSD)
	}
}

func TestMetered_BudgetExceededReturnsErr(t *testing.T) {
	r := NewRecorder(0.0001) // basically immediately exceeded
	inner := &fakeClient{resp: &Response{Usage: Usage{PromptTokens: 1000, CompletionTokens: 1000, TotalTokens: 2000}}}
	m := NewMetered(inner, r, "test")
	ctx := WithTask(context.Background(), "task-budget")
	_, err := m.Complete(ctx, Request{Model: "claude-sonnet-4-5"})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("want ErrBudgetExceeded, got %v", err)
	}
}

func TestRetry_TransientThenSuccess(t *testing.T) {
	inner := &fakeClient{
		resp:  &Response{Usage: Usage{TotalTokens: 100}},
		err:   errors.New("HTTP 503 service unavailable"),
		failN: 2, // 前 2 次失败，第 3 次成功
	}
	r := NewRetry(inner, 3, 10*time.Millisecond)
	resp, err := r.Complete(context.Background(), Request{})
	if err != nil {
		t.Fatalf("retry should succeed eventually, got %v", err)
	}
	if resp == nil {
		t.Fatal("nil resp")
	}
	if inner.calls != 3 {
		t.Errorf("want 3 attempts, got %d", inner.calls)
	}
}

func TestRetry_PermanentFailFast(t *testing.T) {
	inner := &fakeClient{
		err:   errors.New("400 invalid_request_error"),
		failN: 99,
	}
	r := NewRetry(inner, 5, 10*time.Millisecond)
	_, err := r.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("should fail")
	}
	if inner.calls != 1 {
		t.Errorf("permanent error should fail fast (1 attempt), got %d", inner.calls)
	}
}

func TestRetry_BudgetNotRetried(t *testing.T) {
	inner := &fakeClient{err: ErrBudgetExceeded, failN: 99}
	r := NewRetry(inner, 5, 10*time.Millisecond)
	_, err := r.Complete(context.Background(), Request{})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("budget error should pass through, got %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("budget should not retry (1 call), got %d", inner.calls)
	}
}

func TestIsTransient(t *testing.T) {
	cases := map[string]bool{
		"HTTP 429 too many requests":         true,
		"503 service unavailable":            true,
		"connection reset by peer":           true,
		"context deadline exceeded":          true,
		"400 invalid_request_error":          false,
		"401 Unauthorized":                   false,
		"parse error: invalid json":          false,
	}
	for msg, want := range cases {
		got := isTransient(errors.New(msg))
		if got != want {
			t.Errorf("isTransient(%q) want %v got %v", msg, want, got)
		}
	}
}
