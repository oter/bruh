package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// monFixture is a data folder with a ledger (grants.md and mode.md), a fake CLI that prints
// the file out and appends one line to the file calls at each run, and a clock that the test moves.
type monFixture struct {
	env    Env
	ledger string
	cli    string
	out    string
	calls  string
	now    *time.Time
	print  bytes.Buffer
}

const monGrantHeader = "| Argv prefix (JSON array) | Conditions | Owner words | Date (UTC) | Question ID |\n|---|---|---|---|---|\n"

func newMonFixture(t *testing.T, mode string) *monFixture {
	t.Helper()
	f := &monFixture{env: testEnv(t, "bigm"), ledger: t.TempDir()}
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	f.now = &now
	f.env.Now = func() time.Time { return *f.now }
	dir := t.TempDir()
	f.cli, f.out, f.calls = filepath.Join(dir, "tracker-cli"), filepath.Join(dir, "out.json"), filepath.Join(dir, "calls")
	writeFile(t, f.cli, fmt.Sprintf("#!/bin/sh\necho \"$*\" >> %s\ncat %s\n", shq(f.calls), shq(f.out)))
	if err := os.Chmod(f.cli, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(initConfig{LedgerPath: f.ledger})
	writeFile(t, filepath.Join(f.env.DataDir, "init", "config.json"), string(cfg))
	writeFile(t, filepath.Join(f.ledger, "mode.md"), "# Mode\n\nmode: human\n"+mode)
	f.grants(t, fmt.Sprintf(`| ["%s","list"] | read only | "watch the tracker" | 2026-10-04T11:00:00Z | Q-x-h-1 |`, f.cli))
	f.items(t, `{"data": {"list": []}}`)
	return f
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *monFixture) grants(t *testing.T, rows ...string) {
	writeFile(t, filepath.Join(f.ledger, "grants.md"), "# Grants\n\n## Merge grants\n\n| Repository | Merger role key |\n|---|---|\n\n## Command grants\n\n"+monGrantHeader+strings.Join(rows, "\n")+"\n")
}

func (f *monFixture) items(t *testing.T, doc string) { writeFile(t, f.out, doc) }

func (f *monFixture) cliCalls(t *testing.T) int {
	b, _ := os.ReadFile(f.calls)
	return strings.Count(string(b), "\n")
}

// local makes key a local role: a role settings file on this machine.
func (f *monFixture) local(t *testing.T, key string) {
	writeFile(t, filepath.Join(f.env.DataDir, "roles", key+".json"), "{}")
}

// source is a command source with nested pointers; the id key "a/b" needs the escape ~1.
func (f *monFixture) source(argv ...string) map[string]any {
	return map[string]any{"kind": "command", "argv": append([]string{f.cli}, argv...),
		"items": "/data/list", "id": "/a~1b", "version": "/meta/rev", "title": "/fields/name"}
}

func (f *monFixture) start(t *testing.T, key string, args map[string]any) map[string]any {
	t.Helper()
	got, err := call(t, as(f.env, key), "monitor_start", args)
	if err != nil {
		t.Fatalf("monitor_start as %s: %v", key, err)
	}
	return got.(map[string]any)
}

// loop runs one poller loop.
func (f *monFixture) loop(t *testing.T) {
	t.Helper()
	if err := pollOnce(t.Context(), f.env, &watcher{env: f.env, out: &f.print}); err != nil {
		t.Fatal(err)
	}
}

// mail reads and archives the mailbox of key.
func (f *monFixture) mail(t *testing.T, key string) []Message {
	t.Helper()
	got, err := call(t, as(f.env, key), "mail_read", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	var msgs []Message
	if err := json.Unmarshal(raw, &msgs); err != nil {
		t.Fatal(err)
	}
	return msgs
}

// eventOf decodes the event of a mail body or a report line.
func eventOf(t *testing.T, line string) (ReportLine, watchEvent) {
	t.Helper()
	var l ReportLine
	var ev watchEvent
	if err := json.Unmarshal([]byte(line), &l); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(l.Event, &ev); err != nil {
		t.Fatal(err)
	}
	return l, ev
}

func item(id, rev, name string) string {
	return fmt.Sprintf(`{"a/b": %q, "meta": {"rev": %s}, "fields": {"name": %q}}`, id, rev, name)
}

func TestMonitorSharedPoll(t *testing.T) {
	f := newMonFixture(t, "")
	f.local(t, "clerk-shop-t1")
	f.local(t, "clerk-shop-t2")
	f.items(t, `{"data": {"list": [`+item("1", "1", "qq zz")+`]}}`)
	m1 := f.start(t, "clerk-shop-t1", map[string]any{"source": f.source("list", "--json"), "reason": "wait for item changes"})
	m2 := f.start(t, "clerk-shop-t2", map[string]any{"source": f.source("list", "--json"), "reason": "same source"})
	if m1["shared"] != false || m2["shared"] != true || m1["key"] != m2["key"] || !strings.HasPrefix(m1["key"].(string), "command:") {
		t.Fatalf("starts = %v, %v; want the same command key, shared only for the second", m1, m2)
	}
	if n := f.cliCalls(t); n != 1 {
		t.Fatalf("%d CLI calls after two starts, want 1 baseline", n)
	}
	f.loop(t) // not due: the baseline is less than 60 seconds old
	*f.now = f.now.Add(61 * time.Second)
	f.items(t, `{"data": {"list": [`+item("1", "1", "qq zz")+`, `+item("2", "1", "xx")+`]}}`)
	f.loop(t)
	if n := f.cliCalls(t); n != 2 {
		t.Fatalf("%d CLI calls, want 1 baseline and 1 poll for two subscribers", n)
	}
	for key, id := range map[string]any{"clerk-shop-t1": m1["id"], "clerk-shop-t2": m2["id"]} {
		msgs := f.mail(t, key)
		if len(msgs) != 1 || msgs[0].Header != "DONE: event shop: new xx" || msgs[0].From != "bigm" {
			t.Fatalf("mail of %s = %+v", key, msgs)
		}
		if _, ev := eventOf(t, msgs[0].Body); ev.Monitor != id || ev.Type != "new" || ev.Item != "2" || ev.Key != m1["key"] {
			t.Errorf("event of %s = %+v, want monitor %v", key, ev, id)
		}
	}
	if f.print.Len() != 0 {
		t.Errorf("printed %q; local subscribers get mail only", f.print.String())
	}
	report, _ := os.ReadFile(filepath.Join(f.env.DataDir, "reports", "clanker-shop.jsonl"))
	if n := strings.Count(string(report), "\n"); n != 1 {
		t.Errorf("report file has %d lines, want one line for the project", n)
	}
}

func TestMonitorBaselineThenChanges(t *testing.T) {
	f := newMonFixture(t, "")
	f.local(t, "clanker-shop")
	f.items(t, `{"data": {"list": [`+item("1", "1", "qq zz")+`, `+item("2", `"r1"`, "")+`]}}`)
	m := f.start(t, "clanker-shop", map[string]any{"source": f.source("list"), "reason": "wait"})
	src := m["source"].(map[string]any)
	if src["call"] != commandCall([]string{f.cli, "list"}) || src["value"] != "2 items" || src["at"] != "2026-10-04T12:00:00.000Z" {
		t.Fatalf("baseline source read = %v", src)
	}
	*f.now = f.now.Add(2 * time.Minute)
	f.loop(t)
	if msgs := f.mail(t, "clanker-shop"); len(msgs) != 0 {
		t.Fatalf("no change, but mail = %+v", msgs)
	}
	*f.now = f.now.Add(2 * time.Minute)
	f.items(t, `{"data": {"list": [`+item("1", "2", "qq zz")+`, `+item("3", "1", "")+`]}}`)
	f.loop(t)
	var got []string
	for _, msg := range f.mail(t, "clanker-shop") {
		_, ev := eventOf(t, msg.Body)
		got = append(got, ev.Type+" "+ev.Item+" "+ev.Version+" | "+msg.Header)
	}
	want := []string{"changed 1 2 | DONE: event shop: changed qq zz", "new 3 1 | DONE: event shop: new 3", "gone 2  | DONE: event shop: gone 2"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("events =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// A duplicate ID is an error event, once for the same error text.
	*f.now = f.now.Add(2 * time.Minute)
	f.items(t, `{"data": {"list": [`+item("1", "2", "a")+`, `+item("1", "3", "b")+`]}}`)
	f.loop(t)
	*f.now = f.now.Add(2 * time.Minute)
	f.loop(t)
	msgs := f.mail(t, "clanker-shop")
	if len(msgs) != 1 || !strings.Contains(msgs[0].Header, `two items have the id "1"`) {
		t.Errorf("mail after a duplicate ID = %+v, want one error", msgs)
	}
	if !strings.Contains(f.print.String(), `"type":"error"`) {
		t.Errorf("an error event is printed for bigm: %q", f.print.String())
	}
}

func TestMonitorExpiry(t *testing.T) {
	f := newMonFixture(t, "monitor_default_hours: 2\nmonitor_max_hours: 10\n")
	f.local(t, "clanker-shop")
	if _, err := call(t, as(f.env, "clanker-shop"), "monitor_start", map[string]any{"source": f.source("list"), "reason": "r", "hours": 11}); err == nil || !strings.Contains(err.Error(), "monitor_max_hours (10)") {
		t.Fatalf("hours over monitor_max_hours: err = %v", err)
	}
	m := f.start(t, "clanker-shop", map[string]any{"source": f.source("list"), "reason": "until test"})
	if m["until"] != "2026-10-04T14:00:00.000Z" {
		t.Fatalf("until = %v, want now + monitor_default_hours", m["until"])
	}
	*f.now = f.now.Add(2 * time.Hour)
	f.loop(t)
	msgs := f.mail(t, "clanker-shop")
	if len(msgs) != 1 {
		t.Fatalf("mail = %+v, want one expired event", msgs)
	}
	if _, ev := eventOf(t, msgs[0].Body); ev.Type != "expired" || ev.Cause != "until" || ev.Monitor != m["id"] {
		t.Fatalf("event = %+v", ev)
	}
	got, _ := call(t, f.env, "monitor_list", map[string]any{})
	if l := got.(map[string]any)["monitors"].([]any); len(l) != 0 {
		t.Errorf("monitor_list after the expiry = %v", l)
	}
	calls := f.cliCalls(t)
	*f.now = f.now.Add(time.Hour)
	f.loop(t)
	if msgs := f.mail(t, "clanker-shop"); len(msgs) != 0 || f.cliCalls(t) != calls {
		t.Errorf("after the expiry: mail = %+v, %d new CLI calls", msgs, f.cliCalls(t)-calls)
	}
}

func TestMonitorCommandGrant(t *testing.T) {
	if !granted([][]string{{"tracker-cli", "list"}}, []string{"tracker-cli", "list", "--json"}) ||
		granted([][]string{{"tracker-cli", "list"}}, []string{"tracker-cli", "listx"}) ||
		granted([][]string{{"tracker-cli", "list"}}, []string{"tracker-cli"}) || granted(nil, []string{"tracker-cli"}) {
		t.Fatal("granted is not the exact element prefix")
	}
	f := newMonFixture(t, "")
	f.local(t, "clanker-shop")
	start := func(argv ...string) error {
		_, err := call(t, as(f.env, "clanker-shop"), "monitor_start", map[string]any{"source": f.source(argv...), "reason": "r"})
		return err
	}
	for _, argv := range [][]string{{"listx"}, {}, {"lis"}} {
		mustErr(t, start(argv...), "no command grant")
	}
	if err := start("list", "--json"); err != nil {
		t.Fatal(err)
	}
	if n := f.cliCalls(t); n != 1 {
		t.Fatalf("%d CLI calls; a refused source must not run", n)
	}
	// A grant that bigm deletes stops the monitor at the next loop.
	f.grants(t)
	f.loop(t)
	msgs := f.mail(t, "clanker-shop")
	if len(msgs) != 1 {
		t.Fatalf("mail = %+v", msgs)
	}
	if _, ev := eventOf(t, msgs[0].Body); ev.Type != "expired" || ev.Cause != "grant" {
		t.Errorf("event = %+v, want expired with the cause grant", ev)
	}
	mustErr(t, start("list"), "no command grant")
	// No ledger: nothing is granted.
	if err := os.Remove(filepath.Join(f.env.DataDir, "init", "config.json")); err != nil {
		t.Fatal(err)
	}
	f.grants(t, fmt.Sprintf(`| ["%s"] | | | | |`, f.cli))
	mustErr(t, start("list"), "no command grant")
}

func TestMonitorMailboxDelivery(t *testing.T) {
	f := newMonFixture(t, "")
	f.local(t, "clerk-shop-t1")
	long := strings.Repeat("word ", 60) + "\nsecond line\x07"
	f.items(t, `{"data": {"list": []}}`)
	f.start(t, "clerk-shop-t1", map[string]any{"source": f.source("list"), "reason": "r"})
	f.start(t, "clanker-shop", map[string]any{"source": f.source("list"), "reason": "remote subscriber"}) // no roles file: remote
	*f.now = f.now.Add(time.Minute)
	doc, _ := json.Marshal(map[string]any{"data": map[string]any{"list": []any{map[string]any{"a/b": 7, "meta": map[string]any{"rev": 1}, "fields": map[string]any{"name": long}}}}})
	f.items(t, string(doc))
	f.loop(t)
	msgs := f.mail(t, "clerk-shop-t1")
	if len(msgs) != 1 || !headerRE.MatchString(msgs[0].Header) || !strings.HasPrefix(msgs[0].Header, "DONE: event shop: new word word") || len([]rune(msgs[0].Header)) != len("DONE: ")+200 {
		t.Fatalf("mail = %+v", msgs)
	}
	report, _ := os.ReadFile(filepath.Join(f.env.DataDir, "reports", "clanker-shop.jsonl"))
	if string(report) != msgs[0].Body+"\n" {
		t.Errorf("report file = %q, want the mail body %q", report, msgs[0].Body)
	}
	if msgs := f.mail(t, "clanker-shop"); len(msgs) != 0 {
		t.Errorf("a remote subscriber got mail: %+v", msgs)
	}
	_, ev := eventOf(t, strings.TrimSpace(f.print.String()))
	if ev.Type != "new" || ev.Item != "7" || !strings.HasPrefix(ev.Subject, "word") {
		t.Errorf("printed event for the relay of bigm = %+v", ev)
	}
}

func TestPollerLock(t *testing.T) {
	env := testEnv(t, "bigm")
	release, ok, err := lockPoller(env)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := runWatch(ctx, env, &bytes.Buffer{}, false); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("a second poller did not return at once")
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "watch", "poller_at")); err == nil {
		t.Fatal("a second poller polled")
	}
	release()
	if _, ok, _ := lockPoller(env); !ok {
		t.Fatal("the lock stays after the release")
	}
}

func TestMonitorReport(t *testing.T) {
	f := newMonFixture(t, "")
	f.local(t, "clerk-shop-t1")
	src := map[string]any{"kind": "mcp", "server": "tracker", "tool": "list_items", "args": map[string]any{"q": "mine"},
		"items": "", "id": "/id", "version": "/state"}
	m := f.start(t, "clerk-shop-t1", map[string]any{"source": src, "reason": "poll the tracker MCP"})
	if m["key"] != "mcp:"+m["id"].(string) || m["shared"] != false {
		t.Fatalf("start = %v", m)
	}
	report := func(key string, result any) (map[string]any, error) {
		got, err := call(t, as(f.env, key), "monitor_report", map[string]any{"id": m["id"], "result": result})
		if err != nil {
			return nil, err
		}
		return got.(map[string]any), nil
	}
	got, err := report("clerk-shop-t1", `[{"id": "a", "state": "open"}, {"id": "b", "state": "open"}]`) // the text of a tool result
	if err != nil || got["baseline"] != true || got["items"] != 2.0 {
		t.Fatalf("first report = %v, %v", got, err)
	}
	got, err = report("clerk-shop-t1", []any{map[string]any{"id": "a", "state": "done"}})
	if err != nil {
		t.Fatal(err)
	}
	evs := got["events"].([]any)
	if len(evs) != 2 || evs[0].(map[string]any)["type"] != "changed" || evs[1].(map[string]any)["type"] != "gone" {
		t.Fatalf("events = %v", evs)
	}
	if msgs := f.mail(t, "clerk-shop-t1"); len(msgs) != 0 {
		t.Errorf("the subscriber has the events in the result, but got mail: %+v", msgs)
	}
	if b, _ := os.ReadFile(filepath.Join(f.env.DataDir, "reports", "clanker-shop.jsonl")); strings.Count(string(b), "\n") != 2 {
		t.Errorf("report file = %q, want 2 lines", b)
	}
	_, err = report("clanker-shop", []any{})
	mustErr(t, err, "only the subscriber")
	// The poller never polls an mcp source and keeps its cursor.
	f.loop(t)
	if got, err = report("clerk-shop-t1", []any{map[string]any{"id": "a", "state": "done"}}); err != nil || len(got["events"].([]any)) != 0 {
		t.Errorf("report after a poller loop = %v, %v; want no events", got, err)
	}
	*f.now = f.now.Add(25 * time.Hour)
	_, err = report("clerk-shop-t1", []any{})
	mustErr(t, err, "past its until")
}

func TestMonitorStandingAndCodehostFilter(t *testing.T) {
	fg, r := newFakeForge(t, "github")
	r.Project = "repo"
	f := newMonFixture(t, "")
	raw, _ := json.Marshal(reposConfig{Repos: []repoConfig{r}})
	writeFile(t, filepath.Join(f.env.DataDir, "repos.json"), string(raw))
	f.local(t, "clanker-repo")
	f.local(t, "clerk-repo-t1")
	fg.branches["main"] = "a1"
	f.loop(t) // the standing baseline
	got, err := call(t, f.env, "monitor_list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	list := got.(map[string]any)
	l := list["monitors"].([]any)
	if list["poller_down"] != false || len(l) != 1 {
		t.Fatalf("monitor_list = %v", list)
	}
	st := l[0].(map[string]any)
	if st["id"] != "standing:github:owner/repo" || st["subscriber"] != "clanker-repo" || st["until"] != "standing" || st["key"] != "github:owner/repo" {
		t.Fatalf("standing monitor = %v", st)
	}
	_, err = call(t, as(f.env, "bigm"), "monitor_stop", map[string]any{"id": st["id"]})
	mustErr(t, err, "remove: true")

	_, err = call(t, as(f.env, "clerk-repo-t1"), "monitor_start", map[string]any{"source": map[string]any{"kind": "codehost", "repo": "owner/other"}, "reason": "r"})
	mustErr(t, err, "repos_set")
	_, err = call(t, as(f.env, "clerk-repo-t1"), "monitor_start", map[string]any{"source": map[string]any{"kind": "codehost", "repo": "owner/repo", "host": "github"}, "reason": "r"})
	mustErr(t, err, "only the keys of its kind")
	_, err = call(t, as(f.env, "clerk-repo-t1"), "monitor_start", map[string]any{"source": map[string]any{"kind": "http", "url": "https://example.com"}, "reason": "r"})
	mustErr(t, err, "unknown field")
	m := f.start(t, "clerk-repo-t1", map[string]any{"source": map[string]any{"kind": "codehost", "repo": "owner/repo", "ref": "fix"}, "reason": "checks of my push"})
	if m["shared"] != true || m["key"] != "github:owner/repo" {
		t.Fatalf("start = %v", m)
	}
	fg.branches["main"] = "a2"
	fg.branches["fix"] = "f1"
	f.loop(t)
	refs := func(key string) (out []string) {
		for _, msg := range f.mail(t, key) {
			_, ev := eventOf(t, msg.Body)
			out = append(out, ev.Ref)
		}
		return out
	}
	if got := refs("clerk-repo-t1"); strings.Join(got, ",") != "fix" {
		t.Errorf("events of the clerk with ref fix = %v", got)
	}
	if got := refs("clanker-repo"); strings.Join(got, ",") != "fix,main" {
		t.Errorf("events of the standing monitor = %v", got)
	}
	// The subscriber stops its monitor; another clerk cannot.
	_, err = call(t, as(f.env, "clerk-repo-t2"), "monitor_stop", map[string]any{"id": m["id"]})
	mustErr(t, err, "only clerk-repo-t1")
	if _, err := call(t, as(f.env, "clanker-repo"), "monitor_stop", map[string]any{"id": m["id"]}); err != nil {
		t.Fatalf("the parent stops the monitor of its clerk: %v", err)
	}
}
