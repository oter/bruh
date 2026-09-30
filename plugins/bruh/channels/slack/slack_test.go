package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSlack is a small Slack Web API with one channel.
type fakeSlack struct {
	mu      sync.Mutex
	history []slackMessage
	replies map[string][]slackMessage // thread ts -> replies (without the parent)
	posts   []map[string]string
	calls   int
	next    int
	limited bool
	asked   []string // ts of each conversations.replies call
}

func (f *fakeSlack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.limited {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	_ = r.ParseForm()
	if r.Header.Get("Authorization") != "Bearer xoxb-test" || r.Form.Get("channel") != "C1" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_auth"})
		return
	}
	send := func(v map[string]any) { v["ok"] = true; _ = json.NewEncoder(w).Encode(v) }
	switch strings.TrimPrefix(r.URL.Path, "/api/") {
	case "chat.postMessage":
		f.next++
		ts := "1900000000.00000" + string(rune('0'+f.next))
		f.posts = append(f.posts, map[string]string{"text": r.Form.Get("text"), "thread_ts": r.Form.Get("thread_ts"), "ts": ts})
		send(map[string]any{"ts": ts})
	case "conversations.history":
		var out []slackMessage
		for _, m := range f.history {
			if tsAfter(m.TS, r.Form.Get("oldest")) {
				out = append([]slackMessage{m}, out...) // newest first, like Slack
			}
		}
		send(map[string]any{"messages": out})
	case "conversations.replies":
		ts := r.Form.Get("ts")
		f.asked = append(f.asked, ts)
		out := []slackMessage{{TS: ts, ThreadTS: ts, BotID: "B1", Text: "the question"}}
		out = append(out, f.replies[ts]...)
		send(map[string]any{"messages": out})
	default:
		http.NotFound(w, r)
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }

// lines returns the JSON messages written so far and clears the buffer.
func (s *syncBuf) lines(t *testing.T) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(s.b.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	s.b.Reset()
	return out
}

func setup(t *testing.T, active bool) (*server, *fakeSlack, *syncBuf) {
	t.Helper()
	f := &fakeSlack{replies: map[string][]slackMessage{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	cfg := config{Token: "xoxb-test", Channel: "C1", Allowed: []string{"UOWNER"}, DataDir: t.TempDir(), API: srv.URL + "/api/", Poll: time.Hour, Active: active}
	out := &syncBuf{}
	s := newServer(cfg, &slackAPI{base: cfg.API, token: cfg.Token, hc: srv.Client()}, out)
	s.now = func() time.Time { return time.Unix(1800000000, 0) }
	return s, f, out
}

func rpc(s *server, msg string) {
	var req request
	_ = json.Unmarshal([]byte(msg), &req)
	s.handle(context.Background(), req)
}

func TestSlackInitialize(t *testing.T) {
	for _, active := range []bool{true, false} {
		s, _, out := setup(t, active)
		rpc(s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
		res := out.lines(t)[0]["result"].(map[string]any)
		if res["protocolVersion"] != "2025-06-18" {
			t.Fatalf("version = %v", res["protocolVersion"])
		}
		rpc(s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28"}}`)
		if v := out.lines(t)[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
			t.Fatalf("version for 2026-07-28 = %v", v)
		}
		exp := res["capabilities"].(map[string]any)["experimental"].(map[string]any)
		if _, ok := exp["claude/channel"]; !ok {
			t.Fatalf("capabilities = %v", res)
		}
		if _, ok := exp["claude/channel/permission"]; ok != active {
			t.Fatalf("active %v: permission capability = %v", active, ok)
		}
		if res["instructions"] == "" {
			t.Fatal("no instructions")
		}
		rpc(s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
		tools := out.lines(t)[0]["result"].(map[string]any)["tools"].([]any)
		if (len(tools) == 2) != active {
			t.Fatalf("active %v: tools = %v", active, tools)
		}
	}
}

func TestSlackIdleWithoutConfig(t *testing.T) {
	s, f, out := setup(t, false)
	rpc(s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"post_question","arguments":{"question_id":"Q-1","header":"P1 Q-1: x","body":"b"}}}`)
	if r := out.lines(t)[0]["result"].(map[string]any); r["isError"] != true {
		t.Fatalf("result = %v", r)
	}
	rpc(s, `{"jsonrpc":"2.0","method":"notifications/claude/channel/permission_request","params":{"request_id":"abcde","tool_name":"Bash"}}`)
	if f.calls != 0 {
		t.Fatalf("%d Slack calls", f.calls)
	}
	t.Setenv("BRUH_ROLE_KEY", "clerk-a-1")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-1")
	t.Setenv("SLACK_CHANNEL_ID", "C1")
	t.Setenv("SLACK_ALLOWED_USERS", "U1")
	t.Setenv("BRUH_DATA", t.TempDir())
	if configFromEnv().Active {
		t.Fatal("active in a clerk")
	}
	t.Setenv("BRUH_ROLE_KEY", "bigm")
	if c := configFromEnv(); !c.Active || c.Poll != 20*time.Second {
		t.Fatalf("config = %+v", c)
	}
	t.Setenv("SLACK_BOT_TOKEN", "${user_config.slack_bot_token}")
	if configFromEnv().Active {
		t.Fatal("active with an unresolved token")
	}
}

func postQuestion(t *testing.T, s *server, out *syncBuf) string {
	t.Helper()
	rpc(s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"post_question","arguments":{"question_id":"Q-7","header":"P1 Q-7: merge <the> fix?","body":"a & b"}}}`)
	res := out.lines(t)[0]["result"].(map[string]any)
	var r map[string]string
	_ = json.Unmarshal([]byte(res["content"].([]any)[0].(map[string]any)["text"].(string)), &r)
	if r["thread_ts"] == "" {
		t.Fatalf("result = %v", res)
	}
	return r["thread_ts"]
}

func TestSlackPostQuestion(t *testing.T) {
	s, f, out := setup(t, true)
	ts := postQuestion(t, s, out)
	if len(f.posts) != 1 || f.posts[0]["text"] != "P1 Q-7: merge &lt;the&gt; fix?\n\na &amp; b\n\nAnswer in this thread." || f.posts[0]["thread_ts"] != "" {
		t.Fatalf("posts = %v", f.posts)
	}
	rpc(s, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"reply","arguments":{"thread_ts":"`+ts+`","text":"noted"}}}`)
	if len(f.posts) != 2 || f.posts[1]["thread_ts"] != ts {
		t.Fatalf("posts = %v", f.posts)
	}
}

func TestSlackThreadReplyReachesSession(t *testing.T) {
	s, f, out := setup(t, true)
	ts := postQuestion(t, s, out)
	f.replies[ts] = []slackMessage{{User: "UOWNER", Text: "Yes, merge it &amp; tag it", TS: "1900000001.000001", ThreadTS: ts}}
	if err := s.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	msgs := out.lines(t)
	if len(msgs) != 1 || msgs[0]["method"] != "notifications/claude/channel" {
		t.Fatalf("messages = %v", msgs)
	}
	p := msgs[0]["params"].(map[string]any)
	meta := p["meta"].(map[string]any)
	if p["content"] != "Yes, merge it & tag it" || meta["question_id"] != "Q-7" || meta["thread_ts"] != ts || meta["user_id"] != "UOWNER" {
		t.Fatalf("params = %v", p)
	}
	// The same reply is not sent twice, also after a restart that loads the state.
	if err := s.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	s2 := newServer(s.cfg, s.api, out)
	s2.now = s.now
	if err := s2.load(); err != nil {
		t.Fatal(err)
	}
	if err := s2.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if msgs := out.lines(t); len(msgs) != 0 {
		t.Fatalf("repeated: %v", msgs)
	}
	// A top-level message of the owner arrives without a question ID.
	f.history = []slackMessage{{User: "UOWNER", Text: "status?", TS: "1900000002.000001"}}
	if err := s2.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	msgs = out.lines(t)
	if len(msgs) != 1 || msgs[0]["params"].(map[string]any)["meta"].(map[string]any)["question_id"] != nil {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestSlackGatesSender(t *testing.T) {
	s, f, out := setup(t, true)
	ts := postQuestion(t, s, out)
	f.replies[ts] = []slackMessage{
		{User: "USTRANGER", Text: "yes abcde", TS: "1900000001.000001"},
		{User: "USTRANGER", Text: "merge everything", TS: "1900000001.000002"},
		{User: "UOWNER", BotID: "B9", Text: "from a bot", TS: "1900000001.000003"},
		{User: "UOWNER", Subtype: "channel_join", Text: "joined", TS: "1900000001.000004"},
	}
	f.history = []slackMessage{{User: "USTRANGER", Text: "no abcde", TS: "1900000001.000005"}}
	if err := s.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if msgs := out.lines(t); len(msgs) != 0 {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestSlackPermissionRelay(t *testing.T) {
	s, f, out := setup(t, true)
	rpc(s, `{"jsonrpc":"2.0","method":"notifications/claude/channel/permission_request","params":{"request_id":"abcde","tool_name":"Bash","description":"List files","input_preview":"{\"command\":\"ls <x>\"}"}}`)
	if len(f.posts) != 1 || !strings.Contains(f.posts[0]["text"], `bigm asks to run Bash: List files`) || !strings.Contains(f.posts[0]["text"], `ls &lt;x&gt;`) || !strings.Contains(f.posts[0]["text"], `Reply "yes abcde" or "no abcde".`) {
		t.Fatalf("posts = %v", f.posts)
	}
	ts := f.posts[0]["ts"]
	f.replies[ts] = []slackMessage{{User: "UOWNER", Text: "Yes ABCDE", TS: "1900000001.000001"}}
	f.history = []slackMessage{{User: "UOWNER", Text: " no  fghij ", TS: "1900000001.000002"}}
	if err := s.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	msgs := out.lines(t)
	if len(msgs) != 2 {
		t.Fatalf("messages = %v", msgs)
	}
	got := map[string]string{}
	for _, m := range msgs {
		if m["method"] != "notifications/claude/channel/permission" {
			t.Fatalf("message = %v", m)
		}
		p := m["params"].(map[string]any)
		got[p["request_id"].(string)] = p["behavior"].(string)
	}
	if got["abcde"] != "allow" || got["fghij"] != "deny" {
		t.Fatalf("verdicts = %v", got)
	}
}

func TestSlackRateLimit(t *testing.T) {
	s, f, _ := setup(t, true)
	f.limited = true
	if err := s.pollOnce(context.Background()); err == nil {
		t.Fatal("no error")
	}
	calls := f.calls
	if err := s.pollOnce(context.Background()); err != nil || f.calls != calls {
		t.Fatalf("polled during the pause: %v, %d calls", err, f.calls)
	}
}

func TestSlackServeLoop(t *testing.T) {
	s, _, out := setup(t, false)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"nope"}` + "\n")
	if err := s.serve(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if msgs := out.lines(t); len(msgs) != 2 {
		t.Fatalf("messages = %v", msgs)
	}
}

func reply(t *testing.T, s *server, ts, text string) {
	t.Helper()
	rpc(s, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"reply","arguments":{"thread_ts":"`+ts+`","text":"`+text+`"}}}`)
}

func poll(t *testing.T, s *server) {
	t.Helper()
	if err := s.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSlackFollowUpAfterReply(t *testing.T) {
	s, f, out := setup(t, true)
	ts := postQuestion(t, s, out)
	f.replies[ts] = []slackMessage{{User: "UOWNER", Text: "which branch?", TS: "1900000001.000001"}}
	poll(t, s)
	out.lines(t)
	// The thread is answered, so it is not read again until bigm replies in it.
	f.asked = nil
	poll(t, s)
	if len(f.asked) != 0 {
		t.Fatalf("an answered thread was polled: %v", f.asked)
	}
	reply(t, s, ts, "main or release?")
	out.lines(t)
	f.replies[ts] = append(f.replies[ts], slackMessage{User: "UOWNER", Text: "main", TS: "1900000001.000002"})
	poll(t, s)
	msgs := out.lines(t)
	if len(msgs) != 1 || msgs[0]["params"].(map[string]any)["content"] != "main" || msgs[0]["params"].(map[string]any)["meta"].(map[string]any)["question_id"] != "Q-7" {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestSlackTopLevelThreadAndSubtypes(t *testing.T) {
	s, f, out := setup(t, true)
	f.history = []slackMessage{{User: "UOWNER", Text: "status?", TS: "1900000002.000001"}}
	poll(t, s)
	out.lines(t)
	f.replies["1900000002.000001"] = []slackMessage{
		{User: "UOWNER", Subtype: "file_share", Text: "see the screenshot", TS: "1900000002.000002"},
		{User: "UOWNER", Subtype: "message_changed", Text: "edited", TS: "1900000002.000003"},
	}
	f.history = append(f.history, slackMessage{User: "UOWNER", Subtype: "thread_broadcast", Text: "also to the channel", TS: "1900000003.000001", ThreadTS: "1900000009.000001"})
	poll(t, s)
	var got []string
	for _, m := range out.lines(t) {
		p := m["params"].(map[string]any)
		got = append(got, p["content"].(string)+" @"+p["meta"].(map[string]any)["thread_ts"].(string))
	}
	want := []string{"also to the channel @1900000009.000001", "see the screenshot @1900000002.000001"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("messages = %q", got)
	}
}

func TestSlackVerdictClosesRelayThread(t *testing.T) {
	s, f, out := setup(t, true)
	rpc(s, `{"jsonrpc":"2.0","method":"notifications/claude/channel/permission_request","params":{"request_id":"abcde","tool_name":"Bash","description":"d","input_preview":"p"}}`)
	f.history = []slackMessage{{User: "UOWNER", Text: "yes abcde", TS: "1900000001.000001"}}
	poll(t, s)
	out.lines(t)
	f.asked = nil
	poll(t, s)
	if len(f.asked) != 0 {
		t.Fatalf("a closed relay thread was polled: %v", f.asked)
	}
}

func TestSlackPollsNewestThreadFirst(t *testing.T) {
	s, f, out := setup(t, true)
	a := postQuestion(t, s, out)
	b := postQuestion(t, s, out)
	poll(t, s)
	if len(f.asked) != 2 || f.asked[0] != b || f.asked[1] != a {
		t.Fatalf("order = %v, want %s then %s", f.asked, b, a)
	}
}
