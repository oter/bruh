package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Monitors of spec 9.5. A monitor subscribes one role key to one external state (a source).
// Plugin code stores the monitors in <data>/monitors.json; the poller (watch.go) polls each
// source key once for all its subscribers and keeps the cursor in <data>/watch/state.json.

// monSource is the closed schema of a source. Host and APIURL are copied from repos.json.
type monSource struct {
	Kind    string          `json:"kind"` // codehost, command, or mcp
	Repo    string          `json:"repo,omitempty"`
	Ref     string          `json:"ref,omitempty"`
	Number  int             `json:"number,omitempty"`
	Host    string          `json:"host,omitempty"`
	APIURL  string          `json:"api_url,omitempty"`
	Argv    []string        `json:"argv,omitempty"`
	Server  string          `json:"server,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"`
	Items   *string         `json:"items,omitempty"` // JSON pointers (RFC 6901); "" is the root
	ID      *string         `json:"id,omitempty"`
	Version *string         `json:"version,omitempty"`
	Title   *string         `json:"title,omitempty"`
}

type monitor struct {
	ID         string    `json:"id"`
	Subscriber string    `json:"subscriber"`
	Project    string    `json:"project"`
	Reason     string    `json:"reason"`
	Started    string    `json:"started"`
	Until      string    `json:"until"` // empty for a standing monitor
	Source     monSource `json:"source"`
}

const (
	commandTimeout = 30 * time.Second
	commandMaxOut  = 1 << 20
)

// key is the source key. Monitors with the same key share one poll and one cursor. The
// codehost key is the state key of the watcher, so ref and number are filters, not key parts.
func (m monitor) key() string {
	s := m.Source
	switch s.Kind {
	case "codehost":
		return s.Host + ":" + s.Repo
	case "mcp":
		return "mcp:" + m.ID // each session has its own MCP login: no shared poll
	}
	raw, _ := json.Marshal(struct {
		Argv                      []string
		Items, ID, Version, Title *string
	}{s.Argv, s.Items, s.ID, s.Version, s.Title})
	sum := sha256.Sum256(raw)
	return "command:" + hex.EncodeToString(sum[:])[:16]
}

func (m monitor) expired(now time.Time) bool {
	if m.Until == "" {
		return false
	}
	t, err := time.Parse(stampLayout, m.Until)
	return err != nil || !now.Before(t)
}

// wants reports whether the event goes to this monitor: a codehost monitor with ref or number
// gets only the events of that ref or number, and each error and expired event.
func (m monitor) wants(ev watchEvent) bool {
	s := m.Source
	if s.Kind != "codehost" || (s.Ref == "" && s.Number == 0) || ev.Type == "error" || ev.Type == "expired" {
		return true
	}
	return (s.Ref != "" && ev.Ref == s.Ref) || (s.Number != 0 && ev.Number == s.Number)
}

// standingMonitors are the codehost monitors of repos.json (M8): one for each entry with a
// project, for the clanker of that project. Code derives them on each read and never stores them.
func standingMonitors(cfg reposConfig) []monitor {
	var out []monitor
	for _, r := range cfg.Repos {
		if r.Project == "" {
			continue
		}
		out = append(out, monitor{
			ID: "standing:" + r.Host + ":" + r.Repo, Subscriber: "clanker-" + r.Project, Project: r.Project,
			Reason: "standing monitor of repos.json", Source: monSource{Kind: "codehost", Repo: r.Repo, Host: r.Host, APIURL: r.APIURL},
		})
	}
	return out
}

func monitorsFile(env Env) (string, error) {
	dir, err := env.Dir()
	return filepath.Join(dir, "monitors.json"), err
}

// loadMonitors reads the stored monitors. A missing file is no monitors.
func loadMonitors(env Env) ([]monitor, error) {
	file, err := monitorsFile(env)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f struct {
		Monitors []monitor `json:"monitors"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return f.Monitors, nil
}

// saveMonitors writes the stored monitors. The caller holds the lock "monitors".
func saveMonitors(env Env, ms []monitor) error {
	file, err := monitorsFile(env)
	if err != nil {
		return err
	}
	if ms == nil {
		ms = []monitor{}
	}
	raw, _ := json.MarshalIndent(map[string][]monitor{"monitors": ms}, "", "  ")
	return atomicWrite(file, raw)
}

func watchFile(env Env) (string, error) {
	dir, err := env.Dir("watch")
	return filepath.Join(dir, "state.json"), err
}

// updateWatchState runs fn on the cursors under the lock "watch" and writes them. The poller,
// monitor_start, and monitor_report write the same file, so each write reloads it first.
func updateWatchState(env Env, fn func(map[string]*repoState) error) error {
	file, err := watchFile(env)
	if err != nil {
		return err
	}
	return env.WithLock("watch", func() error {
		state, err := loadWatchState(file)
		if err != nil {
			return err
		}
		if err := fn(state); err != nil {
			return err
		}
		raw, _ := json.MarshalIndent(state, "", "  ")
		return atomicWrite(file, raw)
	})
}

// modeInt reads the line "<key>: <n>" of mode.md of the ledger, or def when the ledger, the
// line, or a positive number is missing.
func modeInt(env Env, key string, def int) int {
	ledger, err := ledgerPath(env)
	if err != nil {
		return def
	}
	raw, err := os.ReadFile(filepath.Join(ledger, "mode.md"))
	if err != nil {
		return def
	}
	for line := range strings.Lines(string(raw)) {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), key+": "); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				return n
			}
			return def
		}
	}
	return def
}

// readCommandGrants returns the argv prefixes of the section "## Command grants" of grants.md
// (M6): the first cell of each row is a JSON array of strings. A row that does not parse never
// matches. No ledger or no grants.md grants nothing.
func readCommandGrants(env Env) ([][]string, error) {
	ledger, err := ledgerPath(env)
	if errors.Is(err, errNoLedger) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(ledger, "grants.md"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var grants [][]string
	in := false
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(line, "#") {
			in = strings.TrimSpace(line) == "## Command grants"
			continue
		}
		cells := tableCells(line)
		var p []string
		if in && len(cells) > 0 && json.Unmarshal([]byte(cells[0]), &p) == nil && len(p) > 0 {
			grants = append(grants, p)
		}
	}
	return grants, nil
}

// granted reports whether argv starts with a granted prefix, element by element, byte for byte.
func granted(grants [][]string, argv []string) bool {
	for _, p := range grants {
		if len(argv) >= len(p) && slices.Equal(argv[:len(p)], p) {
			return true
		}
	}
	return false
}

// capWriter keeps at most max bytes. A strict writer fails past max, so the command stops.
type capWriter struct {
	b      bytes.Buffer
	max    int
	strict bool
	over   bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	if room := w.max - w.b.Len(); len(p) > room {
		w.b.Write(p[:max(room, 0)])
		w.over = true
		if w.strict {
			return 0, errors.New("output limit")
		}
		return len(p), nil
	}
	return w.b.Write(p)
}

// runCommand runs argv with no shell, with a time limit and an output limit.
func runCommand(ctx context.Context, argv []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	out, errOut := &capWriter{max: commandMaxOut, strict: true}, &capWriter{max: 300}
	cmd.Stdout, cmd.Stderr, cmd.WaitDelay = out, errOut, time.Second
	err := cmd.Run()
	switch {
	case out.over:
		return nil, errors.New("output over 1 MiB")
	case ctx.Err() != nil:
		return nil, fmt.Errorf("no exit after %s", commandTimeout)
	case err != nil:
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(errOut.b.String()))
	}
	return out.b.Bytes(), nil
}

func commandCall(argv []string) string {
	raw, _ := json.Marshal(argv)
	return "argv " + string(raw)
}

// jsonPointer resolves an RFC 6901 pointer in a decoded JSON value.
func jsonPointer(doc any, ptr string) (any, error) {
	if ptr == "" {
		return doc, nil
	}
	if !strings.HasPrefix(ptr, "/") {
		return nil, fmt.Errorf("pointer %q does not start with /", ptr)
	}
	for tok := range strings.SplitSeq(ptr[1:], "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch v := doc.(type) {
		case map[string]any:
			x, ok := v[tok]
			if !ok {
				return nil, fmt.Errorf("pointer %q: no key %q", ptr, tok)
			}
			doc = x
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(v) || strconv.Itoa(i) != tok {
				return nil, fmt.Errorf("pointer %q: no index %q", ptr, tok)
			}
			doc = v[i]
		default:
			return nil, fmt.Errorf("pointer %q: %q is not in an object or an array", ptr, tok)
		}
	}
	return doc, nil
}

// valueString is a string as it is, null as "", and each other value as compact JSON.
func valueString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

type sourceItem struct{ Version, Title string }

// sourceItems applies the pointers of src to one JSON document. Only structure decides:
// the item IDs and versions, never the meaning of a text. A document that is a JSON string
// (the text of an MCP tool result) is decoded once more.
func sourceItems(raw []byte, src monSource) (map[string]sourceItem, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("the output is not JSON: %w", err)
	}
	if s, ok := doc.(string); ok {
		dec = json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			return nil, fmt.Errorf("the text is not JSON: %w", err)
		}
	}
	list, err := jsonPointer(doc, *src.Items)
	if err != nil {
		return nil, err
	}
	arr, ok := list.([]any)
	if !ok {
		return nil, fmt.Errorf("items %q is not an array", *src.Items)
	}
	items := map[string]sourceItem{}
	for i, x := range arr {
		idv, err := jsonPointer(x, *src.ID)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		id := valueString(idv)
		if id == "" {
			return nil, fmt.Errorf("item %d has an empty id", i)
		}
		if _, dup := items[id]; dup {
			return nil, fmt.Errorf("two items have the id %q", id)
		}
		vv, err := jsonPointer(x, *src.Version)
		if err != nil {
			return nil, fmt.Errorf("item %q: %w", id, err)
		}
		it := sourceItem{Version: valueString(vv)}
		if src.Title != nil {
			if t, err := jsonPointer(x, *src.Title); err == nil {
				it.Title = valueString(t)
			}
		}
		items[id] = it
	}
	return items, nil
}

// applyItems compares the items of one read with the cursor and emits new, changed, and gone.
// The first read is the baseline and emits nothing.
func (w *watcher) applyItems(st *repoState, items map[string]sourceItem, call string) error {
	st.LastCall, st.LastValue, st.LastAt = call, fmt.Sprintf("%d items", len(items)), w.env.Stamp()
	if st.Items == nil {
		st.Items = map[string]string{}
		for id, it := range items {
			st.Items[id] = it.Version
		}
		return nil
	}
	for _, id := range slices.Sorted(maps.Keys(items)) {
		it := items[id]
		prev, known := st.Items[id]
		typ := "new"
		if known {
			if prev == it.Version {
				continue
			}
			typ = "changed"
		}
		subject := cmp.Or(it.Title, id)
		if err := w.emit(watchEvent{Type: typ, Subject: subject, Item: id, Version: it.Version}, typ+" "+subject, call, it.Version); err != nil {
			return err
		}
		st.Items[id] = it.Version
	}
	for _, id := range slices.Sorted(maps.Keys(st.Items)) {
		if _, ok := items[id]; ok {
			continue
		}
		if err := w.emit(watchEvent{Type: "gone", Subject: id, Item: id}, "gone "+id, call, "absent"); err != nil {
			return err
		}
		delete(st.Items, id)
	}
	return nil
}

// eventHeader is the mail header of an event, cleaned of control characters and cut so that
// headerRE accepts it.
func eventHeader(project, text string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(text, "?"))
	rest := []rune("event " + project + ": " + strings.Join(strings.Fields(s), " "))
	return "DONE: " + strings.TrimSpace(string(rest[:min(len(rest), 200)]))
}

// isLocal reports whether key runs on this machine: bigm, or a role with a role settings file.
// A remote clanker writes its role settings on its own machine.
func isLocal(env Env, key string) bool {
	if key == "bigm" {
		return true
	}
	_, err := os.Stat(filepath.Join(env.DataDir, "roles", key+".json"))
	return err == nil
}

func validPointer(p *string) bool { return p != nil && (*p == "" || strings.HasPrefix(*p, "/")) }

// decodeSource decodes a source with a closed schema: an unknown key, or a key of another
// kind, is an error.
func decodeSource(raw json.RawMessage) (monSource, error) {
	var s monSource
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("source: %w", err)
	}
	pointers := s.Items != nil || s.ID != nil || s.Version != nil || s.Title != nil
	codehost := s.Repo != "" || s.Ref != "" || s.Number != 0
	mcp := s.Server != "" || s.Tool != "" || s.Args != nil
	bad := s.Host != "" || s.APIURL != ""
	switch s.Kind {
	case "codehost":
		bad = bad || s.Repo == "" || s.Number < 0 || len(s.Argv) > 0 || mcp || pointers
	case "command":
		bad = bad || len(s.Argv) == 0 || s.Argv[0] == "" || codehost || mcp
	case "mcp":
		bad = bad || s.Server == "" || s.Tool == "" || len(s.Argv) > 0 || codehost || (s.Args != nil && !bytes.HasPrefix(bytes.TrimSpace(s.Args), []byte("{")))
	default:
		return s, fmt.Errorf("source: kind must be codehost, command, or mcp, not %q", s.Kind)
	}
	if bad {
		return s, fmt.Errorf("source: a %s source has only the keys of its kind: codehost {repo, ref, number}, command {argv, items, id, version, title}, mcp {server, tool, args, items, id, version, title}", s.Kind)
	}
	if s.Kind != "codehost" && (!validPointer(s.Items) || !validPointer(s.ID) || !validPointer(s.Version) || (s.Title != nil && !validPointer(s.Title))) {
		return s, errors.New(`source: items, id, and version are required JSON pointers ("" or starting with /), and title is optional`)
	}
	return s, nil
}

func monitorTools() []Tool {
	str := stringSchema()
	return []Tool{
		{
			Name:        "monitor_start",
			Description: "Start a monitor on an external state that your next step waits on (spec 9.5). source: {kind: codehost, repo, ref?, number?} for a repository of repos_set; {kind: command, argv, items, id, version, title?} for a read-only CLI that prints JSON and has a command grant in grants.md; {kind: mcp, server, tool, args?, items, id, version, title?} for a tool that you poll yourself with a CronCreate task and monitor_report. items, id, version, and title are JSON pointers. hours defaults to monitor_default_hours of mode.md. The first poll is the baseline; events reach your mailbox as DONE: event <project>: <subject>.",
			InputSchema: objectSchema(map[string]any{
				"source": map[string]any{"type": "object"}, "project": str, "reason": str,
				"hours": map[string]any{"type": "integer", "minimum": 1},
			}, "source", "reason"),
			Handler: monitorStart,
		},
		{
			Name:        "monitor_stop",
			Description: "Stop a monitor. The subscriber, its parent, or bigm. A standing monitor of repos.json ends only when bigm calls repos_set with remove: true.",
			InputSchema: objectSchema(map[string]any{"id": str}, "id"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct{ ID string }](raw)
				if err != nil {
					return nil, err
				}
				if strings.HasPrefix(a.ID, "standing:") {
					return nil, errors.New("a standing monitor ends when bigm calls repos_set with remove: true")
				}
				var key string
				err = c.Env.WithLock("monitors", func() error {
					all, err := loadMonitors(c.Env)
					if err != nil {
						return err
					}
					i := slices.IndexFunc(all, func(m monitor) bool { return m.ID == a.ID })
					if i < 0 {
						return fmt.Errorf("no monitor %q", a.ID)
					}
					sub, _ := ParseRoleKey(all[i].Subscriber)
					if me != all[i].Subscriber && me != sub.Parent() && me != "bigm" {
						return fmt.Errorf("only %s, its parent, or bigm stops monitor %s", all[i].Subscriber, a.ID)
					}
					key = all[i].key()
					return saveMonitors(c.Env, slices.Delete(all, i, i+1))
				})
				if err != nil {
					return nil, err
				}
				return map[string]string{"stopped": a.ID, "key": key}, nil
			},
		},
		{
			Name:        "monitor_list",
			Description: "List the active monitors of this machine (the live source; monitors.md of the ledger is a copy) and the poller state: poller_at is the time of the last poll loop, and poller_down is true when it is missing or older than 3 poll intervals.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				cfg, err := loadReposOrEmpty(c.Env.DataDir)
				if err != nil {
					return nil, err
				}
				stored, err := loadMonitors(c.Env)
				if err != nil {
					return nil, err
				}
				now := c.Env.Now()
				list := []map[string]any{}
				for _, m := range append(standingMonitors(cfg), stored...) {
					if m.expired(now) {
						continue
					}
					list = append(list, map[string]any{"id": m.ID, "subscriber": m.Subscriber, "project": m.Project, "kind": m.Source.Kind,
						"key": m.key(), "reason": m.Reason, "until": cmp.Or(m.Until, "standing")})
				}
				at, down := pollerState(c.Env, cfg, now)
				return map[string]any{"poller_at": at, "poller_down": down, "monitors": list}, nil
			},
		},
		{
			Name:        "monitor_report",
			Description: "Report one result of the MCP tool of your mcp monitor, as it is (M5). Call it from the CronCreate task of the monitor. The first call is the baseline; later calls return the events new, changed, and gone, which also go to the report file of the project.",
			InputSchema: objectSchema(map[string]any{"id": str, "result": map[string]any{}}, "id", "result"),
			Handler:     monitorReport,
		},
	}
}

// pollerState reads watch/poller_at, the time of the last poll loop. The poller is down when
// the file is missing or older than 3 poll intervals. monitor_list and monitor_start share it.
func pollerState(env Env, cfg reposConfig, now time.Time) (at string, down bool) {
	raw, _ := os.ReadFile(filepath.Join(env.DataDir, "watch", "poller_at"))
	at = strings.TrimSpace(string(raw))
	t, err := time.Parse(stampLayout, at)
	return at, err != nil || now.Sub(t) > 3*time.Duration(cfg.IntervalSeconds)*time.Second
}

func monitorStart(c *Call, raw json.RawMessage) (any, error) {
	env := c.Env
	me, err := env.Caller()
	if err != nil {
		return nil, err
	}
	a, err := decode[struct {
		Source          json.RawMessage
		Project, Reason string
		Hours           int
	}](raw)
	if err != nil {
		return nil, err
	}
	src, err := decodeSource(a.Source)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(a.Reason)
	if reason == "" || strings.ContainsAny(reason, "\r\n") || len([]rune(reason)) > 200 {
		return nil, errors.New("reason is one line of 1 to 200 characters")
	}
	k, _ := ParseRoleKey(me)
	project := cmp.Or(a.Project, k.Project)
	if me != "bigm" && project != k.Project {
		return nil, fmt.Errorf("%s starts monitors only for its own project %q", me, k.Project)
	}
	cfg, err := loadReposOrEmpty(env.DataDir)
	if err != nil {
		return nil, err
	}
	// The poller polls codehost and command sources; the role itself polls an mcp source.
	if at, down := pollerState(env, cfg, env.Now()); down && src.Kind != "mcp" {
		return nil, fmt.Errorf("the poller is down (poller_at %q); the poller runs only in the session of bigm on this machine", at)
	}
	var repo repoConfig
	if src.Kind == "codehost" {
		i := slices.IndexFunc(cfg.Repos, func(r repoConfig) bool { return r.Repo == src.Repo })
		if i < 0 || cfg.Repos[i].Project == "" {
			return nil, fmt.Errorf("%s is not in repos.json with a project; bigm calls repos_set", src.Repo)
		}
		repo = cfg.Repos[i]
		if project != "" && project != repo.Project {
			return nil, fmt.Errorf("%s belongs to project %s, not %s", src.Repo, repo.Project, project)
		}
		project, src.Host, src.APIURL = repo.Project, repo.Host, repo.APIURL
	}
	if !projectRE.MatchString(project) {
		return nil, errors.New("project is required: bigm passes the project of the work")
	}
	hours, maxHours := cmp.Or(a.Hours, modeInt(env, "monitor_default_hours", 24)), modeInt(env, "monitor_max_hours", 168)
	if hours < 1 || hours > maxHours {
		return nil, fmt.Errorf("hours must be 1 to monitor_max_hours (%d), not %d", maxHours, hours)
	}
	if src.Kind == "command" {
		grants, err := readCommandGrants(env)
		if err != nil {
			return nil, err
		}
		if !granted(grants, src.Argv) {
			return nil, fmt.Errorf("grants.md has no command grant for %s; the owner grants a command, and bigm records it under \"Command grants\"", commandCall(src.Argv))
		}
	}
	now := env.Now()
	m := monitor{ID: "m-" + strings.ToLower(rand.Text()[:8]), Subscriber: me, Project: project, Reason: reason,
		Started: env.Stamp(), Until: now.Add(time.Duration(hours) * time.Hour).UTC().Format(stampLayout), Source: src}
	key := m.key()

	// The baseline: the stored last read of a key that has a cursor, else one poll now.
	file, err := watchFile(env)
	if err != nil {
		return nil, err
	}
	state, err := loadWatchState(file)
	if err != nil {
		return nil, err
	}
	var read Source
	var baseline *repoState
	if st := state[key]; st != nil {
		read = Source{Call: cmp.Or(st.LastCall, "watch/state.json"), Value: cmp.Or(st.LastValue, "cursor of "+key), At: cmp.Or(st.LastAt, env.Stamp())}
	} else {
		baseline = &repoState{}
		switch src.Kind {
		case "codehost":
			h, err := newHost(repo)
			if err != nil {
				return nil, err
			}
			if err := (&watcher{env: env, out: io.Discard}).pollRepo(context.Background(), repo, h, baseline); err != nil {
				return nil, fmt.Errorf("baseline of %s: %w", key, err)
			}
			baseline.LastCall, baseline.LastValue, baseline.LastAt = h.LastCall(), fmt.Sprintf("%d branches", len(baseline.Branches)), env.Stamp()
		case "command":
			out, err := runCommand(context.Background(), src.Argv)
			if err != nil {
				return nil, fmt.Errorf("baseline of %s: %w", commandCall(src.Argv), err)
			}
			items, err := sourceItems(out, src)
			if err != nil {
				return nil, fmt.Errorf("baseline of %s: %w", commandCall(src.Argv), err)
			}
			if err := (&watcher{env: env, out: io.Discard}).applyItems(baseline, items, commandCall(src.Argv)); err != nil {
				return nil, err
			}
		case "mcp":
			baseline = nil // the first monitor_report is the baseline
			read = Source{Call: "monitor_start", Value: "no baseline yet: the first monitor_report is the baseline", At: env.Stamp()}
		}
		if baseline != nil {
			read = Source{Call: baseline.LastCall, Value: baseline.LastValue, At: baseline.LastAt}
		}
	}

	shared := false
	err = env.WithLock("monitors", func() error {
		all, err := loadMonitors(env)
		if err != nil {
			return err
		}
		keys := map[string]bool{}
		for _, x := range append(standingMonitors(cfg), all...) {
			if !x.expired(now) {
				keys[x.key()] = true
			}
		}
		shared = keys[key]
		if maxActive := modeInt(env, "monitor_max_active", 20); !shared && len(keys) >= maxActive {
			return fmt.Errorf("monitor_max_active (%d) source keys are active; send a P2 to your parent", maxActive)
		}
		return saveMonitors(env, append(all, m))
	})
	if err != nil {
		return nil, err
	}
	// The monitor is stored first, so the poller does not prune this cursor.
	if baseline != nil {
		err := updateWatchState(env, func(state map[string]*repoState) error {
			if state[key] == nil {
				state[key] = baseline
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"id": m.ID, "key": key, "until": m.Until, "shared": shared, "source": read}, nil
}

func monitorReport(c *Call, raw json.RawMessage) (any, error) {
	env := c.Env
	me, err := env.Caller()
	if err != nil {
		return nil, err
	}
	a, err := decode[struct {
		ID     string
		Result json.RawMessage
	}](raw)
	if err != nil {
		return nil, err
	}
	all, err := loadMonitors(env)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(all, func(m monitor) bool { return m.ID == a.ID })
	if i < 0 {
		return nil, fmt.Errorf("no monitor %q; it was stopped or expired", a.ID)
	}
	m := all[i]
	switch {
	case m.Subscriber != me:
		return nil, fmt.Errorf("only the subscriber %s reports for monitor %s", m.Subscriber, m.ID)
	case m.Source.Kind != "mcp":
		return nil, fmt.Errorf("monitor %s has the kind %s; the poller polls it", m.ID, m.Source.Kind)
	case m.expired(env.Now()):
		return nil, fmt.Errorf("monitor %s is past its until; start a new one if the work still waits", m.ID)
	}
	items, err := sourceItems(a.Result, m.Source)
	if err != nil {
		return nil, err
	}
	w := &watcher{env: env, out: io.Discard, quiet: true, subs: []monitor{m}, key: m.key()}
	baseline := false
	err = updateWatchState(env, func(state map[string]*repoState) error {
		st := state[w.key]
		if st == nil {
			st = &repoState{}
			state[w.key] = st
		}
		baseline = st.Items == nil
		return w.applyItems(st, items, "mcp "+m.Source.Server+" "+m.Source.Tool)
	})
	if err != nil {
		return nil, err
	}
	if baseline {
		return map[string]any{"baseline": true, "items": len(items)}, nil
	}
	if w.sent == nil {
		w.sent = []watchEvent{}
	}
	return map[string]any{"baseline": false, "events": w.sent}, nil
}
