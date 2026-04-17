package crawler

import (
	"context"
	"sync"
	"time"
)

// domainBuckets 简易 per-domain 令牌桶（固定 QPS）
//
// 每个 host 独立 ticker；首次访问时懒初始化。
// 不实现 burst —— 按周期 1/qps 秒发一次令牌就够大多数礼貌爬取场景。
type domainBuckets struct {
	qps  float64
	mu   sync.Mutex
	last map[string]time.Time
}

func newDomainBuckets(qps float64) *domainBuckets {
	if qps <= 0 {
		qps = 1
	}
	return &domainBuckets{
		qps:  qps,
		last: make(map[string]time.Time),
	}
}

// Wait 阻塞直到可以请求 host（受 ctx 取消保护）
func (d *domainBuckets) Wait(ctx context.Context, host string) {
	interval := time.Duration(float64(time.Second) / d.qps)
	d.mu.Lock()
	last, ok := d.last[host]
	now := time.Now()
	var sleep time.Duration
	if ok {
		elapsed := now.Sub(last)
		if elapsed < interval {
			sleep = interval - elapsed
		}
	}
	d.last[host] = now.Add(sleep) // 提前预约
	d.mu.Unlock()

	if sleep > 0 {
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
		}
	}
}
