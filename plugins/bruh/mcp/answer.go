package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var qidRE = regexp.MustCompile("^" + qidPattern + "$")

type answer struct {
	Text    string `json:"text"`
	At      string `json:"at"`
	Subject string `json:"subject,omitempty"`
	Asker   string `json:"asker,omitempty"`
}

func answerFile(env Env, qid string) (string, error) {
	me, err := env.Caller()
	if err != nil {
		return "", err
	}
	if _, err := checkID(qid, qidRE, "question ID"); err != nil {
		return "", err
	}
	dir, err := env.Dir("answers", me)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, qid+".answer"), nil
}

// clearHolds removes each hold whose P0 is the question qid: the answer is the decision of the
// owner (spec 15.1).
func clearHolds(env Env, qid string) error {
	return env.WithLock("holds", func() error {
		files, _ := filepath.Glob(filepath.Join(env.DataDir, "holds", "H-*.json")) // only ErrBadPattern; the pattern is constant
		for _, f := range files {
			data, err := os.ReadFile(f)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("read hold %s: %w", f, err)
			}
			var h struct {
				QuestionID string `json:"question_id"`
			}
			if err := json.Unmarshal(data, &h); err != nil {
				return fmt.Errorf("parse hold %s: %w", f, err)
			}
			if h.QuestionID == qid {
				if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
		}
		return nil
	})
}

func answerTools() []Tool {
	return []Tool{
		{
			Name:        "answer_write",
			Description: "Record the answer to a question under the role key of the caller: a clerk for its workflow agents; bigm for each answer that it sends, with subject and asker",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question_id": map[string]any{"type": "string"},
					"text":        map[string]any{"type": "string"},
					"subject":     map[string]any{"type": "string", "description": "One line, at most 200 characters"},
					"asker":       map[string]any{"type": "string", "description": "Role key of the asker"},
				},
				"required": []string{"question_id", "text"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				a, err := decode[struct {
					QuestionID string `json:"question_id"`
					Text       string `json:"text"`
					Subject    string `json:"subject"`
					Asker      string `json:"asker"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if strings.ContainsAny(a.Subject, "\r\n") || utf8.RuneCountInString(a.Subject) > 200 {
					return nil, fmt.Errorf("invalid subject: %q (one line, at most 200 characters)", a.Subject)
				}
				if a.Asker != "" {
					if _, err := checkKey(a.Asker, "asker"); err != nil {
						return nil, err
					}
				}
				file, err := answerFile(c.Env, a.QuestionID)
				if err != nil {
					return nil, err
				}
				at := c.Env.Stamp()
				data, _ := json.Marshal(answer{Text: a.Text, At: at, Subject: a.Subject, Asker: a.Asker})
				if err := atomicWrite(file, data); err != nil {
					return nil, err
				}
				return map[string]string{"at": at}, clearHolds(c.Env, a.QuestionID)
			},
		},
		{
			Name:        "answer_wait",
			Description: `Wait for the answer to a question. Returns {"status":"answered","text","at"} (and subject and asker when the file has them) or {"status":"pending"} at the deadline. With deadline_seconds 0, it checks once: answered or pending`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question_id":      map[string]any{"type": "string"},
					"deadline_seconds": map[string]any{"type": "number"},
				},
				"required": []string{"question_id", "deadline_seconds"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				a, err := decode[struct {
					QuestionID      string  `json:"question_id"`
					DeadlineSeconds float64 `json:"deadline_seconds"`
				}](raw)
				if err != nil {
					return nil, err
				}
				file, err := answerFile(c.Env, a.QuestionID)
				if err != nil {
					return nil, err
				}
				end := time.Now().Add(time.Duration(a.DeadlineSeconds * float64(time.Second)))
				n := 1
				c.Progress(n, "waiting for "+a.QuestionID)
				lastProgress := time.Now()
				for {
					data, err := os.ReadFile(file)
					if err == nil {
						var ans answer
						if err := json.Unmarshal(data, &ans); err != nil {
							return nil, err
						}
						out := map[string]string{"status": "answered", "text": ans.Text, "at": ans.At}
						if ans.Subject != "" {
							out["subject"] = ans.Subject
						}
						if ans.Asker != "" {
							out["asker"] = ans.Asker
						}
						return out, nil
					}
					if !errors.Is(err, fs.ErrNotExist) {
						return nil, err
					}
					if !time.Now().Before(end) {
						return map[string]string{"status": "pending"}, nil
					}
					if time.Since(lastProgress) >= c.Env.ProgressInterval {
						n++
						c.Progress(n, "waiting for "+a.QuestionID)
						lastProgress = time.Now()
					}
					time.Sleep(min(c.Env.PollInterval, max(time.Millisecond, time.Until(end))))
				}
			},
		},
	}
}
