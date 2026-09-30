package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var qidRE = regexp.MustCompile(`^Q-\d+$`)

type answer struct {
	Text string `json:"text"`
	At   string `json:"at"`
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

func answerTools() []Tool {
	return []Tool{
		{
			Name:        "answer_write",
			Description: "Write the answer to a question of a workflow agent of this clerk.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question_id": map[string]any{"type": "string"},
					"text":        map[string]any{"type": "string"},
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
				}](raw)
				if err != nil {
					return nil, err
				}
				file, err := answerFile(c.Env, a.QuestionID)
				if err != nil {
					return nil, err
				}
				at := c.Env.Stamp()
				data, _ := json.Marshal(answer{Text: a.Text, At: at})
				return map[string]string{"at": at}, atomicWrite(file, data)
			},
		},
		{
			Name:        "answer_wait",
			Description: `Wait for the answer to a question. Returns {"status":"answered","text"} or {"status":"pending"} at the deadline.`,
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
						return map[string]string{"status": "answered", "text": ans.Text, "at": ans.At}, nil
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
