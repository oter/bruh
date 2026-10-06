package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnswerWaitGetsAnswerWithProgress(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	var wait func(*Call, json.RawMessage) (any, error)
	for _, tool := range AllTools() {
		if tool.Name == "answer_wait" {
			wait = tool.Handler
		}
	}
	if wait == nil {
		t.Fatal("no answer_wait tool")
	}
	var mu sync.Mutex
	progress := 0
	c := &Call{Env: env, progress: func(int, string) { mu.Lock(); progress++; mu.Unlock() }}
	done := make(chan any, 1)
	go func() {
		out, err := wait(c, json.RawMessage(`{"question_id":"Q-a-testhost-7","deadline_seconds":5}`))
		if err != nil {
			t.Error(err)
		}
		done <- out
	}()
	time.Sleep(150 * time.Millisecond)
	if _, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-a-testhost-7", "text": "Use option B. Owner, 2026-09-30."}); err != nil {
		t.Fatal(err)
	}
	out := (<-done).(map[string]string)
	if out["status"] != "answered" || out["text"] != "Use option B. Owner, 2026-09-30." {
		t.Fatalf("out = %v", out)
	}
	// The answer file of a clerk has no subject and no asker (spec A9.1).
	for _, k := range []string{"subject", "asker"} {
		if _, ok := out[k]; ok {
			t.Errorf("answer_wait of a clerk file returned key %q: %v", k, out)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if progress < 2 {
		t.Fatalf("progress = %d", progress)
	}
}

func TestAnswerWaitPendingAtDeadline(t *testing.T) {
	out, err := call(t, testEnv(t, "clerk-a-1"), "answer_wait", map[string]any{"question_id": "Q-a-testhost-8", "deadline_seconds": 0.2})
	if err != nil || out.(map[string]any)["status"] != "pending" {
		t.Fatalf("out = %v, %v", out, err)
	}
}

func TestAnswerWaitZeroDeadlineChecksOnce(t *testing.T) {
	env := testEnv(t, "bigm")
	args := map[string]any{"question_id": "Q-shop-testhost-4", "deadline_seconds": 0}
	start := time.Now()
	out, err := call(t, env, "answer_wait", args)
	if err != nil || out.(map[string]any)["status"] != "pending" {
		t.Fatalf("answer_wait with no file = %v, %v, want pending", out, err)
	}
	if d := time.Since(start); d >= time.Second {
		t.Errorf("answer_wait with deadline 0 took %v, want less than 1s", d)
	}
	// bigm writes each answer that it sends with subject and asker (spec 204, A9.1).
	subject := "subject of " + t.Name()
	if _, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-shop-testhost-4", "text": "answer of " + t.Name(), "subject": subject, "asker": "clanker-shop"}); err != nil {
		t.Fatalf("answer_write: %v", err)
	}
	out, err = call(t, env, "answer_wait", args)
	if err != nil {
		t.Fatalf("answer_wait: %v", err)
	}
	if got := out.(map[string]any); got["status"] != "answered" || got["subject"] != subject || got["asker"] != "clanker-shop" {
		t.Errorf("answer_wait after answer_write = %v, want answered with subject %q and asker clanker-shop", got, subject)
	}
}

func TestAnswerWriteRefusesBadInput(t *testing.T) {
	for _, tc := range []struct {
		name, role string
		args       map[string]any
		wantErr    string
	}{
		// ../Q-1 leaves the answers folder; Q-7 is the old form, refused since spec 5 (D2).
		{"ID leaves the answers folder", "clerk-a-1", map[string]any{"question_id": "../Q-1"}, "invalid question ID"},
		{"ID of the old form", "clerk-a-1", map[string]any{"question_id": "Q-7"}, "invalid question ID"},
		// watcher is no role key; Clanker-A has capitals.
		{"asker is no role key", "bigm", map[string]any{"question_id": "Q-shop-testhost-5", "asker": "watcher"}, ""},
		{"asker has capitals", "bigm", map[string]any{"question_id": "Q-shop-testhost-5", "asker": "Clanker-A"}, ""},
		{"subject has a newline", "bigm", map[string]any{"question_id": "Q-shop-testhost-6", "subject": "subject of\n" + t.Name()}, ""},
		{"subject is longer than 200", "bigm", map[string]any{"question_id": "Q-shop-testhost-6", "subject": strings.Repeat("s", 201)}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testEnv(t, tc.role)
			tc.args["text"] = "answer of " + t.Name()
			_, err := call(t, env, "answer_write", tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("answer_write %v: err = %v, want an error with %q", tc.args, err, tc.wantErr)
			}
			if err := filepath.WalkDir(env.DataDir, func(p string, d fs.DirEntry, err error) error {
				if err == nil && strings.HasSuffix(p, ".answer") {
					t.Errorf("answer_write %v wrote %s, want no file", tc.args, p)
				}
				return err
			}); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
		})
	}
}

func TestAnswerWriteClearsLinkedHold(t *testing.T) {
	env := testEnv(t, "bigm")
	writeHold(t, env.DataDir, "H-1", "clerk-a-1", "Q-a-testhost-1", map[string]string{"command": "x"})
	writeHold(t, env.DataDir, "H-2", "clerk-a-1", "Q-a-testhost-2", map[string]string{"command": "y"})
	writeHold(t, env.DataDir, "H-3", "clerk-a-1", "", map[string]string{"command": "z"})
	if _, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-a-testhost-1", "text": "run it. Owner, 2026-10-04."}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "holds", "H-1.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("os.Stat(H-1.json) after answer_write(Q-a-testhost-1) = %v, want fs.ErrNotExist", err)
	}
	for _, id := range []string{"H-2", "H-3"} {
		if _, err := os.Stat(filepath.Join(env.DataDir, "holds", id+".json")); err != nil {
			t.Errorf("os.Stat(%s.json) after answer_write(Q-a-testhost-1) = %v, want nil", id, err)
		}
	}
}

// fixedStamp is the Stamp of the clock of fixedNow.
const fixedStamp = "2026-10-06T16:30:00.000Z"

// fixedNow returns env with a clock fixed at fixedStamp.
func fixedNow(env Env) Env {
	env.Now = func() time.Time { return time.Date(2026, 10, 6, 16, 30, 0, 0, time.UTC) }
	return env
}

// Task 29: answer_write replaces {now} in the text with the time that it stores, so the caller
// passes no time and runs no date -u.
func TestAnswerWriteFillsNow(t *testing.T) {
	env := fixedNow(testEnv(t, "bigm"))
	out, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-shop-testhost-9", "text": "owner, {now}, terminal: yes"})
	if err != nil {
		t.Fatalf("answer_write: %v", err)
	}
	if at := out.(map[string]any)["at"]; at != fixedStamp {
		t.Fatalf("answer_write at = %v, want %s", at, fixedStamp)
	}
	got, err := call(t, env, "answer_wait", map[string]any{"question_id": "Q-shop-testhost-9", "deadline_seconds": 0})
	if err != nil {
		t.Fatalf("answer_wait: %v", err)
	}
	if text, want := got.(map[string]any)["text"], "owner, "+fixedStamp+", terminal: yes"; text != want {
		t.Errorf("answer_wait text = %q, want %q", text, want)
	}
}
