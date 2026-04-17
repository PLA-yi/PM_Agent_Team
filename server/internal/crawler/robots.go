package crawler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// robotsCache 极简 robots.txt 解析（按 User-Agent: * 段 + Disallow/Allow 前缀匹配）
//
// 不实现：crawl-delay 个体、wildcard 模式、sitemap。
// 这是 80/20 取舍 —— 80% 站点用基础 Disallow 规则就够；高级特性进 v0.6。
type robotsCache struct {
	ua    string
	mu    sync.RWMutex
	hosts map[string]*robotsRules
}

type robotsRules struct {
	disallowPrefixes []string
	allowPrefixes    []string
}

func newRobotsCache(ua string) *robotsCache {
	return &robotsCache{
		ua:    ua,
		hosts: make(map[string]*robotsRules),
	}
}

// Allowed 检查 URL 是否被 robots.txt 允许
func (rc *robotsCache) Allowed(ctx context.Context, u *url.URL, hc *http.Client) (bool, error) {
	host := u.Host
	rc.mu.RLock()
	rules, ok := rc.hosts[host]
	rc.mu.RUnlock()
	if !ok {
		// 拉 robots.txt
		var err error
		rules, err = rc.fetchRobots(ctx, u, hc)
		if err != nil {
			// 拿不到默认 allow（不阻塞调用方）
			rules = &robotsRules{}
		}
		rc.mu.Lock()
		rc.hosts[host] = rules
		rc.mu.Unlock()
	}
	return matchRobots(rules, u.Path), nil
}

func (rc *robotsCache) fetchRobots(ctx context.Context, u *url.URL, hc *http.Client) (*robotsRules, error) {
	robotsURL := u.Scheme + "://" + u.Host + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", rc.ua)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &robotsRules{}, nil // 没 robots.txt = 全允许
	}
	body, _ := io.ReadAll(resp.Body)
	return parseRobots(string(body)), nil
}

// parseRobots 取 User-agent: * 段下的 Disallow/Allow 规则
func parseRobots(content string) *robotsRules {
	rules := &robotsRules{}
	var inStarSection bool
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		// 去掉注释
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		key = strings.ToLower(key)
		switch key {
		case "user-agent":
			inStarSection = (strings.TrimSpace(val) == "*")
		case "disallow":
			if inStarSection && val != "" {
				rules.disallowPrefixes = append(rules.disallowPrefixes, val)
			}
		case "allow":
			if inStarSection && val != "" {
				rules.allowPrefixes = append(rules.allowPrefixes, val)
			}
		}
	}
	return rules
}

func splitKV(s string) (k, v string, ok bool) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
}

// matchRobots 给定路径，先看 allow 优先，再看 disallow
func matchRobots(rules *robotsRules, path string) bool {
	if path == "" {
		path = "/"
	}
	for _, allow := range rules.allowPrefixes {
		if strings.HasPrefix(path, allow) {
			return true
		}
	}
	for _, dis := range rules.disallowPrefixes {
		if dis == "/" {
			return false
		}
		if strings.HasPrefix(path, dis) {
			return false
		}
	}
	return true
}
