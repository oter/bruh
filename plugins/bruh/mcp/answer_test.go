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

func TestAnswerRefusesBadID(t *testing.T) {
	// ../Q-1 leaves the answers folder; Q-7 is the old form, refused since spec 5 (D2).
	for _, id := range []string{"../Q-1", "Q-7"} {
		_, err := call(t, testEnv(t, "clerk-a-1"), "answer_write", map[string]any{"question_id": id, "text": "x"})
		if err == nil || !strings.Contains(err.Error(), "invalid question ID") {
			t.Errorf("answer_write %q: err = %v", id, err)
		}
	}
}

func TestAnswerKeepsSubjectAndAsker(t *testing.T) {
	env := testEnv(t, "bigm")
	text, subject := "answer of "+t.Name(), "subject of "+t.Name()
	if _, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-shop-testhost-3", "text": text, "subject": subject, "asker": "clanker-shop"}); err != nil {
		t.Fatalf("answer_write: %v", err)
	}
	out, err := call(t, env, "answer_wait", map[string]any{"question_id": "Q-shop-testhost-3", "deadline_seconds": 0})
	if err != nil {
		t.Fatalf("answer_wait: %v", err)
	}
	got := out.(map[string]any)
	want := map[string]any{"status": "answered", "text": text, "subject": subject, "asker": "clanker-shop"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("answer_wait %s = %v, want %v (out = %v)", k, got[k], v, got)
		}
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
	if got := out.(map[string]any); got["status"] != "answered" || got["subject"] != subject {
		t.Errorf("answer_wait after answer_write = %v, want answered with subject %q", got, subject)
	}
}

func TestAnswerRefusesBadAsker(t *testing.T) {
	// watcher is no role key; Clanker-A has capitals.
	for _, asker := range []string{"watcher", "Clanker-A"} {
		env := testEnv(t, "bigm")
		_, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-shop-testhost-5", "text": "answer of " + t.Name(), "asker": asker})
		if err == nil {
			t.Errorf("answer_write asker %q: err = nil, want an error", asker)
		}
		file := filepath.Join(env.DataDir, "answers", "bigm", "Q-shop-testhost-5.answer")
		if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("answer_write asker %q: stat %s = %v, want no file", asker, file, err)
		}
	}
}

func TestAnswerRefusesMultilineSubject(t *testing.T) {
	for _, subject := range []string{"subject of\n" + t.Name(), strings.Repeat("s", 201)} {
		_, err := call(t, testEnv(t, "bigm"), "answer_write", map[string]any{"question_id": "Q-shop-testhost-6", "text": "answer of " + t.Name(), "subject": subject})
		if err == nil {
			t.Errorf("answer_write subject %q: err = nil, want an error", subject)
		}
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
