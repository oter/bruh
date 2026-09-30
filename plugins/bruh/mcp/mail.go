package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var headerRE = regexp.MustCompile(`^(P[012] Q-\d+|ANSWER Q-\d+|REC Q-\d+|RULE R-\d+|DONE|START): \S.{0,199}$`)

var mailSeq atomic.Int64

type Message struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Header string `json:"header"`
	Body   string `json:"body"`
	At     string `json:"at"`
}

func mailTools() []Tool {
	return []Tool{
		{
			Name:        "mail_post",
			Description: "Put a message in the durable mailbox of a role. Send only the header line as the SendMessage nudge.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"to":     map[string]any{"type": "string", "description": "Role key of the receiver"},
					"header": map[string]any{"type": "string", "description": `One header line, for example "P1 Q-7: merge the login fix?"`},
					"body":   map[string]any{"type": "string"},
				},
				"required": []string{"to", "header", "body"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				from, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct{ To, Header, Body string }](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkKey(a.To, "role key"); err != nil {
					return nil, err
				}
				if !headerRE.MatchString(a.Header) {
					return nil, fmt.Errorf("invalid header: %q", a.Header)
				}
				box, err := c.Env.Dir("mail", a.To)
				if err != nil {
					return nil, err
				}
				msg := Message{
					ID:     fmt.Sprintf("%015d-%06d-%s", time.Now().UnixMilli(), mailSeq.Add(1), strings.ToLower(rand.Text()[:8])),
					From:   from,
					To:     a.To,
					Header: a.Header,
					Body:   a.Body,
					At:     c.Env.Stamp(),
				}
				data, _ := json.Marshal(msg)
				if err := atomicWrite(filepath.Join(box, msg.ID+".json"), data); err != nil {
					return nil, err
				}
				return map[string]string{"id": msg.ID, "at": msg.At}, nil
			},
		},
		{
			Name:        "mail_read",
			Description: "Read and archive all new messages for the calling role, oldest first.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				box, err := c.Env.Dir("mail", me)
				if err != nil {
					return nil, err
				}
				readDir, err := c.Env.Dir("mail", me, "read")
				if err != nil {
					return nil, err
				}
				entries, err := os.ReadDir(box) // sorted by file name
				if err != nil {
					return nil, err
				}
				msgs := []Message{}
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
						continue
					}
					data, err := os.ReadFile(filepath.Join(box, e.Name()))
					if err != nil {
						return nil, err
					}
					var m Message
					if err := json.Unmarshal(data, &m); err != nil {
						return nil, errors.Join(fmt.Errorf("bad message %s", e.Name()), err)
					}
					if err := os.Rename(filepath.Join(box, e.Name()), filepath.Join(readDir, e.Name())); err != nil {
						return nil, err
					}
					msgs = append(msgs, m)
				}
				return msgs, nil
			},
		},
	}
}
