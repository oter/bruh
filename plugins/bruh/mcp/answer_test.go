package main

import (
	"encoding/json"
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
		out, err := wait(c, json.RawMessage(`{"question_id":"Q-7","deadline_seconds":5}`))
		if err != nil {
			t.Error(err)
		}
		done <- out
	}()
	time.Sleep(150 * time.Millisecond)
	if _, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-7", "text": "Use option B. Owner, 2026-09-30."}); err != nil {
		t.Fatal(err)
	}
	out := (<-done).(map[string]string)
	if out["status"] != "answered" || out["text"] != "Use option B. Owner, 2026-09-30." {
		t.Fatalf("out = %v", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if progress < 2 {
		t.Fatalf("progress = %d", progress)
	}
}

func TestAnswerWaitPendingAtDeadline(t *testing.T) {
	out, err := call(t, testEnv(t, "clerk-a-1"), "answer_wait", map[string]any{"question_id": "Q-8", "deadline_seconds": 0.2})
	if err != nil || out.(map[string]any)["status"] != "pending" {
		t.Fatalf("out = %v, %v", out, err)
	}
}

func TestAnswerRefusesBadID(t *testing.T) {
	_, err := call(t, testEnv(t, "clerk-a-1"), "answer_write", map[string]any{"question_id": "../Q-1", "text": "x"})
	if err == nil || !strings.Contains(err.Error(), "invalid question ID") {
		t.Fatalf("err = %v", err)
	}
}
