// Package slack 通过 Incoming Webhook 推卡片到 Slack 频道。
//
// 设置：Slack workspace → Apps → Incoming Webhooks → Add to Channel → 拷贝 URL
// SLACK_WEBHOOK_URL 写到 .env 即可。
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	WebhookURL string
	HTTP       *http.Client
}

func New(webhookURL string) *Client {
	return &Client{
		WebhookURL: webhookURL,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
	}
}

// IsConfigured 是否有 webhook URL
func (c *Client) IsConfigured() bool { return c.WebhookURL != "" }

// Block Slack Block Kit 元素
type Block struct {
	Type     string                 `json:"type"`
	Text     *BlockText             `json:"text,omitempty"`
	Fields   []BlockText            `json:"fields,omitempty"`
	Elements []map[string]interface{} `json:"elements,omitempty"`
}

type BlockText struct {
	Type string `json:"type"` // "mrkdwn" | "plain_text"
	Text string `json:"text"`
}

type Message struct {
	Text   string  `json:"text,omitempty"` // fallback 简文
	Blocks []Block `json:"blocks,omitempty"`
}

func (c *Client) Post(ctx context.Context, msg Message) error {
	if !c.IsConfigured() {
		return fmt.Errorf("slack: webhook URL not configured (set SLACK_WEBHOOK_URL)")
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("slack http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// PostTaskSummary 标准化任务总结消息
func (c *Client) PostTaskSummary(ctx context.Context, taskTitle, taskInput, reportPreview, reportURL string, score *float64) error {
	header := fmt.Sprintf("*%s*", taskTitle)
	scoreStr := ""
	if score != nil {
		scoreStr = fmt.Sprintf(" · Reviewer: *%.1f/10*", *score)
	}

	blocks := []Block{
		{Type: "section", Text: &BlockText{Type: "mrkdwn", Text: header + scoreStr}},
		{Type: "section", Text: &BlockText{Type: "mrkdwn", Text: ":mag: *Input:*\n" + taskInput}},
		{Type: "section", Text: &BlockText{Type: "mrkdwn", Text: ":memo: *Report preview:*\n" + truncate(reportPreview, 600)}},
	}
	if reportURL != "" {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &BlockText{Type: "mrkdwn", Text: fmt.Sprintf("<%s|View full report →>", reportURL)},
		})
	}
	return c.Post(ctx, Message{Text: header, Blocks: blocks})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
