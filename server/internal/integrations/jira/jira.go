// Package jira REST API client（Cloud / Server 通用）。
//
// 设置：Atlassian → Settings → API tokens → Create
// .env: JIRA_BASE_URL=https://yourorg.atlassian.net
//       JIRA_EMAIL=you@company.com
//       JIRA_API_TOKEN=...
package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL  string
	Email    string
	APIToken string
	HTTP     *http.Client
}

func New(baseURL, email, apiToken string) *Client {
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Email:    email,
		APIToken: apiToken,
		HTTP:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) IsConfigured() bool {
	return c.BaseURL != "" && c.Email != "" && c.APIToken != ""
}

func (c *Client) authHeader() string {
	cred := c.Email + ":" + c.APIToken
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(cred))
}

// CreateIssueRequest 创建 issue
// projectKey 例如 "PMHIVE"; issueType 例如 "Task"
type CreateIssueRequest struct {
	ProjectKey  string
	IssueType   string
	Summary     string
	Description string
}

type CreateIssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`  // e.g. "PMHIVE-42"
	Self string `json:"self"`
}

// Jira API V3 issue payload
type jiraIssueFields struct {
	Project   map[string]string  `json:"project"`
	IssueType map[string]string  `json:"issuetype"`
	Summary   string             `json:"summary"`
	Description map[string]any   `json:"description"` // ADF 格式
}

type jiraIssueBody struct {
	Fields jiraIssueFields `json:"fields"`
}

func (c *Client) CreateIssue(ctx context.Context, req CreateIssueRequest) (*CreateIssueResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("jira: not configured (need JIRA_BASE_URL / JIRA_EMAIL / JIRA_API_TOKEN)")
	}
	// Description 用 ADF (Atlassian Document Format) — 这里只发纯文本段落
	descADF := map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": req.Description},
				},
			},
		},
	}
	body := jiraIssueBody{
		Fields: jiraIssueFields{
			Project:     map[string]string{"key": req.ProjectKey},
			IssueType:   map[string]string{"name": req.IssueType},
			Summary:     req.Summary,
			Description: descADF,
		},
	}
	buf, _ := json.Marshal(body)
	endpoint := c.BaseURL + "/rest/api/3/issue"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", c.authHeader())

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("jira http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jira %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	var out CreateIssueResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("jira decode: %w", err)
	}
	return &out, nil
}

// AddComment 在 issue 上加评论（纯文本）
func (c *Client) AddComment(ctx context.Context, issueKey, text string) error {
	if !c.IsConfigured() {
		return fmt.Errorf("jira: not configured")
	}
	body := map[string]any{
		"body": map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []any{map[string]any{"type": "paragraph", "content": []any{
				map[string]any{"type": "text", "text": text},
			}}},
		},
	}
	buf, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", c.BaseURL, issueKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("jira http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("jira %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
