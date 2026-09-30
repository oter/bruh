package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// slackAPI calls the Slack Web API with a bot token.
type slackAPI struct {
	base, token string
	hc          *http.Client
}

type slackMessage struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	User     string `json:"user"`
	BotID    string `json:"bot_id"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
}

// errRateLimited carries the Retry-After wait of a 429 answer.
type errRateLimited struct{ wait time.Duration }

func (e errRateLimited) Error() string { return fmt.Sprintf("rate limited; retry after %s", e.wait) }

func (a *slackAPI) call(ctx context.Context, method string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", a.base+method, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		s, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return errRateLimited{time.Duration(max(s, 1)) * time.Second}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	var base struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return fmt.Errorf("%s: %s: %w", method, resp.Status, err)
	}
	if !base.OK {
		return errors.New(method + ": " + base.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (a *slackAPI) post(ctx context.Context, channel, threadTS, text string) (string, error) {
	p := url.Values{"channel": {channel}, "text": {text}}
	if threadTS != "" {
		p.Set("thread_ts", threadTS)
	}
	var out struct {
		TS string `json:"ts"`
	}
	err := a.call(ctx, "chat.postMessage", p, &out)
	return out.TS, err
}

func (a *slackAPI) history(ctx context.Context, channel, oldest string) ([]slackMessage, error) {
	var out struct {
		Messages []slackMessage `json:"messages"`
	}
	err := a.call(ctx, "conversations.history", url.Values{"channel": {channel}, "oldest": {oldest}, "limit": {"100"}}, &out)
	return out.Messages, err
}

func (a *slackAPI) replies(ctx context.Context, channel, ts, oldest string) ([]slackMessage, error) {
	var out struct {
		Messages []slackMessage `json:"messages"`
	}
	err := a.call(ctx, "conversations.replies", url.Values{"channel": {channel}, "ts": {ts}, "oldest": {oldest}, "limit": {"100"}}, &out)
	return out.Messages, err
}

// tsAfter reports whether Slack timestamp a is later than b ("1712345678.123456").
func tsAfter(a, b string) bool {
	x, _ := strconv.ParseFloat(a, 64)
	y, _ := strconv.ParseFloat(b, 64)
	return x > y
}

// Slack escapes these three characters in message text.
var (
	slackEscape   = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	slackUnescape = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">")
)
