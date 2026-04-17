package llm

import (
	"context"
	"errors"
	"strings"
	"time"
)

// RetryClient 给底层 client 加 transient-error retry
//
// 区分 transient vs permanent：
//   transient: HTTP 429/500/502/503/504, "rate", "timeout", "EOF", "connection reset"
//   permanent: 4xx (except 429), "invalid_request", parse error → fail fast
//
// budget 超限不重试（直接传出）
type RetryClient struct {
	Inner       Client
	MaxAttempts int           // 总共尝试次数（含首次）, 默认 3
	BaseBackoff time.Duration // 退避起点, 默认 500ms
}

func NewRetry(inner Client, maxAttempts int, base time.Duration) *RetryClient {
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	return &RetryClient{Inner: inner, MaxAttempts: maxAttempts, BaseBackoff: base}
}

func (r *RetryClient) IsMock() bool { return r.Inner.IsMock() }

func (r *RetryClient) Complete(ctx context.Context, req Request) (*Response, error) {
	var lastErr error
	for attempt := 0; attempt < r.MaxAttempts; attempt++ {
		if attempt > 0 {
			// 退避：500ms / 1s / 2s （指数）
			backoff := r.BaseBackoff * (1 << attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		resp, err := r.Inner.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// budget 超限不重试
		if errors.Is(err, ErrBudgetExceeded) {
			return resp, err
		}
		// permanent 错误不重试
		if !isTransient(err) {
			return resp, err
		}
		// transient → 继续 retry
	}
	return nil, lastErr
}

// isTransient 判断错误是否值得重试
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	transientHints := []string{
		"429", "500", "502", "503", "504",
		"rate", "timeout", "deadline", "eof",
		"connection reset", "connection refused",
		"temporary", "try again", "overloaded",
	}
	for _, h := range transientHints {
		if strings.Contains(s, h) {
			return true
		}
	}
	return false
}
