package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	qidRE = regexp.MustCompile(`^Q-\d+$`)
	// verdictRE is the permission reply format of the channels reference: the ID alphabet is a to z without l.
	verdictRE = regexp.MustCompile(`(?i)^\s*(y|yes|n|no)\s+([a-km-z]{5})\s*$`)
)

func orEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

const threadLife = 7 * 24 * time.Hour

type thread struct {
	QuestionID string `json:"question_id,omitempty"` // a question thread
	RequestID  string `json:"request_id,omitempty"`  // a permission prompt thread
	Last       string `json:"last"`                  // ts of the last message read
	Opened     int64  `json:"opened"`                // Unix seconds
	// Done stops the polling of a thread: after the first message of the owner in it, or after the
	// verdict of its permission prompt. The reply tool opens it again.
	Done bool `json:"done,omitempty"`
}

// A thread with no question and no permission prompt (a message of the owner, a reply of bigm)
// is polled for one day only.
const plainThreadLife = 24 * time.Hour

type state struct {
	Oldest  string             `json:"oldest"` // ts of the last top-level message read
	Threads map[string]*thread `json:"threads"`
}

type server struct {
	cfg   config
	api   *slackAPI
	mu    sync.Mutex // guards out and st
	out   *json.Encoder
	st    state
	now   func() time.Time
	pause time.Time // no Slack call before this time (rate limit)
}

func newServer(cfg config, api *slackAPI, out io.Writer) *server {
	return &server{cfg: cfg, api: api, out: json.NewEncoder(out), st: state{Threads: map[string]*thread{}}, now: time.Now}
}

func (s *server) send(msg any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.out.Encode(msg)
}

func (s *server) notify(method string, params any) {
	s.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (s *server) stateFile() string {
	return filepath.Join(s.cfg.DataDir, "channels", "slack", "state.json")
}

func (s *server) load() error {
	raw, err := os.ReadFile(s.stateFile())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &s.st); err != nil {
		return err
	}
	if s.st.Threads == nil {
		s.st.Threads = map[string]*thread{}
	}
	return nil
}

// save writes the state with a temporary file and a rename. The caller holds mu.
func (s *server) save() error {
	file := s.stateFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(s.st, "", "  ")
	tmp, err := os.CreateTemp(filepath.Dir(file), "state.*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(raw)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), file)
}

func (s *server) track(ts, questionID, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Threads[ts] = &thread{QuestionID: questionID, RequestID: requestID, Last: ts, Opened: s.now().Unix()}
	return s.save()
}

// reopen polls a thread again, and keeps its question ID and its read position when it is known.
func (s *server) reopen(ts string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.st.Threads[ts]; t != nil {
		t.Done, t.Opened = false, s.now().Unix()
	} else {
		s.st.Threads[ts] = &thread{Last: ts, Opened: s.now().Unix()}
	}
	return s.save()
}

const instructions = `Slack channel of bruh. Only bigm uses it. Post each P0 or P1 question for the owner with post_question; each question is one Slack thread. ` +
	`The owner answers in the thread. An answer arrives as <channel source="slack" question_id="Q-<n>" thread_ts="..." user_id="..." ts="...">; record it as the owner answer of that question ID. ` +
	`A top-level message of the owner arrives without question_id. Reply in a thread with the reply tool and its thread_ts. Only allowlisted senders reach this session.`

func (s *server) tools() []map[string]any {
	if !s.cfg.Active {
		return []map[string]any{}
	}
	str := map[string]any{"type": "string"}
	obj := func(props map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": req}
	}
	return []map[string]any{
		{"name": "post_question", "description": "Post a P0 or P1 question to the Slack channel of the owner as a new thread. Returns thread_ts.",
			"inputSchema": obj(map[string]any{"question_id": str, "header": str, "body": str}, "question_id", "header", "body")},
		{"name": "reply", "description": "Post a message in a Slack thread of the channel.",
			"inputSchema": obj(map[string]any{"thread_ts": str, "text": str}, "thread_ts", "text")},
	}
}

func (s *server) callTool(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	if !s.cfg.Active {
		return nil, errors.New("the Slack channel is not active: it runs only in bigm with the Slack options set")
	}
	var a struct {
		QuestionID string `json:"question_id"`
		Header     string `json:"header"`
		Body       string `json:"body"`
		ThreadTS   string `json:"thread_ts"`
		Text       string `json:"text"`
	}
	if err := json.Unmarshal(orEmpty(raw), &a); err != nil {
		return nil, err
	}
	switch name {
	case "post_question":
		if !qidRE.MatchString(a.QuestionID) || strings.TrimSpace(a.Header) == "" {
			return nil, fmt.Errorf("post_question needs a question_id Q-<n> and a header")
		}
		text := slackEscape.Replace(a.Header+"\n\n"+a.Body) + "\n\nAnswer in this thread."
		ts, err := s.api.post(ctx, s.cfg.Channel, "", text)
		if err != nil {
			return nil, err
		}
		return map[string]string{"thread_ts": ts}, s.track(ts, a.QuestionID, "")
	case "reply":
		if a.ThreadTS == "" || strings.TrimSpace(a.Text) == "" {
			return nil, errors.New("reply needs thread_ts and text")
		}
		ts, err := s.api.post(ctx, s.cfg.Channel, a.ThreadTS, slackEscape.Replace(a.Text))
		if err != nil {
			return nil, err
		}
		return map[string]string{"ts": ts}, s.reopen(a.ThreadTS) // read the answer of the owner to this reply
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

// relay posts a permission prompt of the session and tracks it as a thread.
func (s *server) relay(ctx context.Context, raw json.RawMessage) error {
	var p struct {
		RequestID    string `json:"request_id"`
		ToolName     string `json:"tool_name"`
		Description  string `json:"description"`
		InputPreview string `json:"input_preview"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	text := slackEscape.Replace(fmt.Sprintf("bigm asks to run %s: %s\n%s\n\nReply \"yes %s\" or \"no %s\".", p.ToolName, p.Description, p.InputPreview, p.RequestID, p.RequestID))
	ts, err := s.api.post(ctx, s.cfg.Channel, "", text)
	if err != nil {
		return err
	}
	return s.track(ts, "", p.RequestID)
}

// inbound handles one message. It drops a message of a bot or of a sender who is not on the
// allowlist, turns a verdict into a permission notification, and forwards the rest. It returns
// dropped, verdict, or forwarded.
func (s *server) inbound(m slackMessage, threadTS, questionID string) int {
	// Replies also sent to the channel and messages with a file are messages of the owner too.
	if m.BotID != "" || !slices.Contains([]string{"", "thread_broadcast", "file_share"}, m.Subtype) || !slices.Contains(s.cfg.Allowed, m.User) {
		return dropped
	}
	text := slackUnescape.Replace(m.Text)
	if v := verdictRE.FindStringSubmatch(text); v != nil {
		behavior := "deny"
		if strings.HasPrefix(strings.ToLower(v[1]), "y") {
			behavior = "allow"
		}
		id := strings.ToLower(v[2])
		s.notify("notifications/claude/channel/permission", map[string]string{"request_id": id, "behavior": behavior})
		s.mu.Lock()
		for _, t := range s.st.Threads {
			if t.RequestID == id {
				t.Done = true
			}
		}
		s.mu.Unlock()
		return verdict
	}
	meta := map[string]string{"thread_ts": threadTS, "ts": m.TS, "user_id": m.User}
	if questionID != "" {
		meta["question_id"] = questionID
	}
	s.notify("notifications/claude/channel", map[string]any{"content": text, "meta": meta})
	return forwarded
}

const (
	dropped = iota
	verdict
	forwarded
)

func sortByTS(ms []slackMessage) {
	slices.SortFunc(ms, func(a, b slackMessage) int {
		x, _ := strconv.ParseFloat(a.TS, 64)
		y, _ := strconv.ParseFloat(b.TS, 64)
		return cmp.Compare(x, y)
	})
}

// pollOnce reads new top-level messages and new replies of each open thread.
func (s *server) pollOnce(ctx context.Context) error {
	if s.now().Before(s.pause) {
		return nil
	}
	err := s.poll(ctx)
	if rl, ok := errors.AsType[errRateLimited](err); ok {
		s.pause = s.now().Add(rl.wait)
	}
	return err
}

func (s *server) poll(ctx context.Context) error {
	s.mu.Lock()
	if s.st.Oldest == "" {
		s.st.Oldest = strconv.FormatInt(s.now().Unix(), 10) + ".000000" // start now; never replay old history
	}
	oldest := s.st.Oldest
	threads := map[string]thread{}
	for ts, t := range s.st.Threads {
		life := threadLife
		if t.QuestionID == "" && t.RequestID == "" {
			life = plainThreadLife
		}
		if s.now().Sub(time.Unix(t.Opened, 0)) > life {
			delete(s.st.Threads, ts)
			continue
		}
		if !t.Done {
			threads[ts] = *t
		}
	}
	s.mu.Unlock()

	top, err := s.api.history(ctx, s.cfg.Channel, oldest)
	if err != nil {
		return err
	}
	sortByTS(top)
	for _, m := range top {
		if !tsAfter(m.TS, oldest) {
			continue
		}
		oldest = m.TS
		thread := m.TS
		if m.ThreadTS != "" && m.ThreadTS != m.TS {
			// A reply also sent to the channel: the thread poll reads it when the thread is open.
			if _, open := threads[m.ThreadTS]; open {
				continue
			}
			thread = m.ThreadTS
		}
		s.mu.Lock()
		questionID := ""
		if t := s.st.Threads[thread]; t != nil {
			questionID = t.QuestionID
		}
		s.mu.Unlock()
		if s.inbound(m, thread, questionID) == forwarded && questionID == "" {
			if err := s.reopen(thread); err != nil { // read the follow-ups of the owner in its thread
				return err
			}
		}
	}
	s.mu.Lock()
	s.st.Oldest = oldest
	s.mu.Unlock()

	// Newest threads first: after a rate limit, the current answers are read first.
	order := slices.Sorted(maps.Keys(threads))
	slices.Reverse(order)
	for _, ts := range order {
		t := threads[ts]
		rs, err := s.api.replies(ctx, s.cfg.Channel, ts, t.Last)
		if err != nil {
			return err
		}
		sortByTS(rs)
		last, answered := t.Last, false
		for _, m := range rs {
			if m.TS == ts || !tsAfter(m.TS, last) {
				continue // the parent message, or read before
			}
			last = m.TS
			answered = s.inbound(m, ts, t.QuestionID) != dropped || answered
		}
		s.mu.Lock()
		if cur := s.st.Threads[ts]; cur != nil {
			cur.Last = last
			cur.Done = cur.Done || answered
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save()
}

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (s *server) result(id json.RawMessage, v any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": v})
}

func text(v string, isError bool) map[string]any {
	r := map[string]any{"content": []map[string]any{{"type": "text", "text": v}}}
	if isError {
		r["isError"] = true
	}
	return r
}

// handle answers one request or acts on one notification.
func (s *server) handle(ctx context.Context, req request) {
	isNote := len(req.ID) == 0 || string(req.ID) == "null"
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(orEmpty(req.Params), &p)
		exp := map[string]any{"claude/channel": map[string]any{}}
		if s.cfg.Active {
			// Relay only when the sender allowlist gates the verdicts.
			exp["claude/channel/permission"] = map[string]any{}
		}
		// Claude Code does not register a channel that negotiates protocol revision 2026-07-28,
		// so the server answers only with a revision that it knows.
		version := "2025-06-18"
		if slices.Contains([]string{"2025-03-26", "2025-06-18", "2025-11-25"}, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		s.result(req.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"experimental": exp, "tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "bruh-slack", "version": "0.1.0-dev"},
			"instructions":    instructions,
		})
	case "ping":
		s.result(req.ID, map[string]any{})
	case "tools/list":
		s.result(req.ID, map[string]any{"tools": s.tools()})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		out, err := s.callTool(ctx, p.Name, p.Arguments)
		if err != nil {
			s.result(req.ID, text(err.Error(), true))
			return
		}
		raw, _ := json.Marshal(out)
		s.result(req.ID, text(string(raw), false))
	case "notifications/claude/channel/permission_request":
		if s.cfg.Active {
			if err := s.relay(ctx, req.Params); err != nil {
				log.Printf("slack: relay: %v", err)
			}
		}
	default:
		if !isNote {
			s.send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "method not found: " + req.Method}})
		}
	}
}

// serve reads one JSON-RPC message for each line. When active, it polls Slack until in closes.
func (s *server) serve(ctx context.Context, in io.Reader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.cfg.Active {
		if err := s.load(); err != nil {
			return err
		}
		go func() {
			for {
				if err := s.pollOnce(ctx); err != nil && ctx.Err() == nil {
					log.Printf("slack: poll: %v", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.cfg.Poll):
				}
			}
		}()
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var wg sync.WaitGroup
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		wg.Go(func() { s.handle(ctx, req) })
	}
	wg.Wait()
	return sc.Err()
}
