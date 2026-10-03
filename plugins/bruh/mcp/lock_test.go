package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWithLockExcludesAndSurvivesADeadHolder(t *testing.T) {
	env := testEnv(t, "")
	locks, _ := env.Dir("locks")
	// A holder that died left its lock file; the kernel released its flock.
	if err := os.WriteFile(filepath.Join(locks, "x.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	inside, most := 0, 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			err := env.WithLock("x", func() error {
				mu.Lock()
				inside++
				most = max(most, inside)
				mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if most != 1 {
		t.Fatalf("%d holders at once", most)
	}
}

func TestQuestionOpenNeverOverwrites(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	dir, _ := env.Dir("questions")
	// A crash left Q-a-testhost-1.json without moving next.
	if err := os.WriteFile(filepath.Join(dir, "Q-a-testhost-1.json"), []byte(`{"id":"Q-a-testhost-1","subject":"first"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	q, err := openQ(t, env, "second")
	if err != nil {
		t.Fatal(err)
	}
	if q["id"] != "Q-a-testhost-2" {
		t.Fatalf("id = %v", q["id"])
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "Q-a-testhost-1.json")); !strings.Contains(string(b), "first") {
		t.Fatalf("Q-a-testhost-1 was overwritten: %s", b)
	}
}

func TestSessionLaunchOneAtATime(t *testing.T) {
	env := testEnv(t, "bigm")
	writeRoleSettings(t, env, "clanker-a")
	dir := t.TempDir()
	agents := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(agents, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The fake becomes a live session named clanker-a when it starts one.
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = agents ]; then cat '%[1]s'; exit 0; fi
sleep 0.2
echo '[{"id":"ab12","kind":"background","name":"clanker-a","pid":1,"startedAt":1}]' > '%[1]s'
echo "backgrounded · ab12 · clanker-a"
`, agents)
	env.ClaudeBin = filepath.Join(dir, "claude")
	if err := os.WriteFile(env.ClaudeBin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, err := call(t, env, "session_launch", map[string]any{"agent": "clanker", "role_key": "clanker-a", "cwd": cwd})
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		})
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if !strings.Contains(err.Error(), "live session") {
			t.Fatal(err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d sessions started: %v", ok, errs)
	}
}
