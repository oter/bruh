package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Question struct {
	ID       string   `json:"id"`
	Priority string   `json:"priority"`
	Subject  string   `json:"subject"`
	Body     string   `json:"body"`
	Blocks   string   `json:"blocks"`
	Asker    string   `json:"asker"`
	OpenedAt string   `json:"opened_at"`
	Options  []Option `json:"options,omitempty"`
	Replaces string   `json:"replaces,omitempty"`
}

// Option is one fixed answer of a P1 question (spec 14.2).
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// optionsBody checks the options of question_open and returns body, then, only with options, a
// blank line and one line "OPTION <n>: <label> | <description>" for each option, from 1.
func optionsBody(body string, opts []Option) (string, error) {
	if len(opts) == 0 {
		return body, nil
	}
	if len(opts) < 2 || len(opts) > 4 {
		return "", errors.New("options must have 2 to 4 items")
	}
	lines := []string{body, ""}
	for i, o := range opts {
		if n := utf8.RuneCountInString(o.Label); n < 1 || n > 60 || strings.ContainsAny(o.Label, "|\r\n") {
			return "", fmt.Errorf("invalid label of option %d: %q (one line, 1 to 60 characters, no |)", i+1, o.Label)
		}
		if n := utf8.RuneCountInString(o.Description); n < 1 || n > 200 || strings.ContainsAny(o.Description, "\r\n") {
			return "", fmt.Errorf("invalid description of option %d: %q (one line, 1 to 200 characters)", i+1, o.Description)
		}
		lines = append(lines, fmt.Sprintf("OPTION %d: %s | %s", i+1, o.Label, o.Description))
	}
	return strings.Join(lines, "\n"), nil
}

func questionTools() []Tool {
	return []Tool{
		{
			Name:        "question_open",
			Description: "Open a question and get its ID, header line, and body. With 2 to 4 fixed answers, pass options; send the returned body, which has the OPTION lines. Send the header as the SendMessage nudge, then wait with answer_wait.",
			InputSchema: objectSchema(map[string]any{
				"priority": map[string]any{"type": "string", "enum": []string{"P0", "P1", "P2"}},
				"subject":  map[string]any{"type": "string", "description": "One line, at most 200 characters"},
				"body":     stringSchema(),
				"blocks":   map[string]any{"type": "string", "description": "The work that waits for the answer"},
				"options": map[string]any{
					"type":     "array",
					"items":    objectSchema(map[string]any{"label": stringSchema(), "description": stringSchema()}, "label", "description"),
					"minItems": 2,
					"maxItems": 4,
				},
				"hold":     map[string]any{"type": "string", "description": "The hold ID of a refusal (spec 15.1): makes the question P0 and adds the COMMAND and CATEGORY lines from the hold record. It also replaces your open P0 with the same COMMAND and CATEGORY lines"},
				"replaces": map[string]any{"type": "string", "description": "The ID of an older question of yours that this one replaces (a reask): the answer of this one also closes it"},
			}, "priority", "subject", "body", "blocks"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Priority, Subject, Body, Blocks, Hold, Replaces string
					Options                                         []Option
				}](raw)
				if err != nil {
					return nil, err
				}
				holdLines := ""
				if a.Hold != "" {
					h, err := readHold(c.Env, a.Hold, me)
					if err != nil {
						return nil, err
					}
					holdLines = h.lines()
					a.Priority = "P0"
					a.Body = strings.TrimRight(a.Body, "\n") + "\n\n" + holdLines
				}
				if !slices.Contains([]string{"P0", "P1", "P2"}, a.Priority) {
					return nil, fmt.Errorf("invalid priority: %q", a.Priority)
				}
				if strings.TrimSpace(a.Body) == "" || strings.TrimSpace(a.Blocks) == "" {
					return nil, errors.New("body and blocks must not be empty")
				}
				body, err := optionsBody(a.Body, a.Options)
				if err != nil {
					return nil, err
				}
				var q Question
				open := func() error {
					return c.Env.WithLock("questions", func() error {
						dir, err := c.Env.Dir("questions")
						if err != nil {
							return err
						}
						n := 1
						if data, err := os.ReadFile(filepath.Join(dir, "next")); err == nil {
							if n, err = strconv.Atoi(strings.TrimSpace(string(data))); err != nil || n < 1 {
								return fmt.Errorf("bad questions/next: %q", data)
							}
						} else if !errors.Is(err, fs.ErrNotExist) {
							return err
						}
						q = Question{Priority: a.Priority, Subject: a.Subject, Body: a.Body, Blocks: a.Blocks, Asker: me, OpenedAt: c.Env.Stamp(), Options: a.Options, Replaces: a.Replaces}
						if a.Replaces != "" {
							if _, err := checkID(a.Replaces, qidRE, "replaces"); err != nil {
								return err
							}
							var old Question
							data, err := os.ReadFile(filepath.Join(dir, a.Replaces+".json"))
							if err == nil {
								err = json.Unmarshal(data, &old)
							}
							if err != nil || old.Asker != me {
								return fmt.Errorf("replaces %s: no question of %s", a.Replaces, me)
							}
						} else if holdLines != "" {
							q.Replaces = repeatedHold(c.Env, dir, me, holdLines)
						}
						if !headerRE.MatchString(q.Priority + " " + questionID(me, c.Env.Host, n) + ": " + q.Subject) {
							return fmt.Errorf("invalid subject: %q (one line, 1 to 200 characters)", a.Subject)
						}
						// Create the file first and never over an existing one: after a crash between the
						// two writes, the number in next is used already, and the loop skips it.
						for ; ; n++ {
							q.ID = questionID(me, c.Env.Host, n)
							data, _ := json.MarshalIndent(q, "", "  ")
							f, err := os.OpenFile(filepath.Join(dir, q.ID+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
							if errors.Is(err, fs.ErrExist) {
								continue
							}
							if err != nil {
								return err
							}
							_, werr := f.Write(data)
							if err := errors.Join(werr, f.Close()); err != nil {
								return err
							}
							break
						}
						return atomicWrite(filepath.Join(dir, "next"), []byte(strconv.Itoa(n+1)))
					})
				}
				if a.Hold == "" {
					err = open()
				} else {
					// One P0 for each hold: check again and link the question under the lock.
					err = c.Env.WithLock("holds", func() error {
						h, err := readHold(c.Env, a.Hold, me)
						if err != nil {
							return err
						}
						if err := open(); err != nil {
							return err
						}
						h.m["question_id"] = q.ID
						data, _ := json.Marshal(h.m)
						return atomicWrite(h.file, data)
					})
				}
				if err != nil {
					return nil, err
				}
				return map[string]string{"id": q.ID, "header": q.header(), "body": body}, nil
			},
		},
		{
			Name:        "question_list",
			Description: "List the open P0 and P1 questions, oldest first: each question file with no answer file in any role folder. A question that a newer answered question replaces is closed",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				open := []map[string]string{}
				qs := readQuestions(filepath.Join(c.Env.DataDir, "questions"))
				slices.SortStableFunc(qs, func(a, b Question) int { return cmp.Compare(a.OpenedAt, b.OpenedAt) })
				for _, q := range qs {
					if (q.Priority == "P0" || q.Priority == "P1") && !answered(c.Env, q.ID) {
						open = append(open, map[string]string{"id": q.ID, "priority": q.Priority, "subject": q.Subject, "asker": q.Asker, "opened_at": q.OpenedAt})
					}
				}
				return map[string]any{"questions": open}, nil
			},
		},
		{
			Name:        "bruh_info",
			Description: "Return the plugin root, the data folder, the role key of this session, and the plugin version. Use the plugin root to find scripts/.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				root, err := filepath.Abs(c.Env.PluginRoot)
				if err != nil {
					return nil, err
				}
				data := c.Env.DataDir
				if data != "" {
					if data, err = filepath.Abs(data); err != nil {
						return nil, err
					}
				}
				var p struct {
					Version string `json:"version"`
				}
				if b, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json")); err == nil {
					_ = json.Unmarshal(b, &p)
				}
				return map[string]string{"plugin_root": root, "data_dir": data, "role_key": c.Env.RoleKey, "version": p.Version}, nil
			},
		},
	}
}

// repeatedHold returns the ID of the newest open P0 of me whose body ends with the same COMMAND
// and CATEGORY lines (an exact match), or "": a repeat of the same refusal replaces it.
func repeatedHold(env Env, dir, me, lines string) string {
	var newest Question
	for _, q := range readQuestions(dir) {
		if q.Asker == me && q.Priority == "P0" && strings.HasSuffix(q.Body, "\n\n"+lines) && !answered(env, q.ID) && q.OpenedAt >= newest.OpenedAt {
			newest = q
		}
	}
	return newest.ID
}

// readQuestions reads each question file of dir; it skips a file that it cannot read or parse.
func readQuestions(dir string) []Question {
	files, _ := filepath.Glob(filepath.Join(dir, "Q-*.json")) // only ErrBadPattern; the pattern is constant
	var qs []Question
	for _, f := range files {
		var q Question
		if data, err := os.ReadFile(f); err == nil && json.Unmarshal(data, &q) == nil && q.ID != "" {
			qs = append(qs, q)
		}
	}
	return qs
}

func (q Question) header() string { return q.Priority + " " + q.ID + ": " + q.Subject }

var holdRE = regexp.MustCompile(`^H-[0-9A-Za-z-]+$`)

// holdRecord is a hold file that scripts/refusal-stop.sh writes after a refusal (spec 15.1).
type holdRecord struct {
	file string
	m    map[string]any
}

// readHold reads the hold id of the caller me that has no question yet.
func readHold(env Env, id, me string) (holdRecord, error) {
	if _, err := checkID(id, holdRE, "hold ID"); err != nil {
		return holdRecord{}, err
	}
	dir, err := env.Dir("holds")
	if err != nil {
		return holdRecord{}, err
	}
	h := holdRecord{file: filepath.Join(dir, id+".json")}
	data, err := os.ReadFile(h.file)
	if err != nil {
		return holdRecord{}, fmt.Errorf("no hold %s: %w", id, err)
	}
	if err := json.Unmarshal(data, &h.m); err != nil {
		return holdRecord{}, fmt.Errorf("bad hold %s: %w", id, err)
	}
	if h.str("role_key") != me {
		return holdRecord{}, fmt.Errorf("hold %s is not a hold of %s", id, me)
	}
	if qid := h.str("question_id"); qid != "" {
		return holdRecord{}, fmt.Errorf("hold %s has the P0 %s already: wait for its answer with answer_wait", id, qid)
	}
	return h, nil
}

func (h holdRecord) str(key string) string { s, _ := h.m[key].(string); return s }

// lines returns the COMMAND and CATEGORY lines of the P0, word for word from the record.
func (h holdRecord) lines() string {
	in, _ := h.m["tool_input"].(map[string]any)
	cmd, ok := in["command"].(string)
	if !ok {
		data, _ := json.Marshal(h.m["tool_input"])
		cmd = string(data)
	}
	return "COMMAND: " + cmd + "\nCATEGORY: " + h.str("denial_source") + " " + h.str("denial_reason")
}

// questionID returns the ID Q-<project>-<host>-<n> of a question of the role key caller:
// clanker-<p> and clerk-<p>-<task> give <p>, bigm gives bigm, and clerk-ledger gives ledger.
func questionID(caller, host string, n int) string {
	k, _ := ParseRoleKey(caller) // Env.Caller has checked the role key already
	return "Q-" + cmp.Or(k.Project, k.Role) + "-" + host + "-" + strconv.Itoa(n)
}
