package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestMailKeepsOrderAndArchives(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	for n := range 3 {
		if _, err := call(t, env, "mail_post", map[string]any{
			"to": "clanker-a", "header": fmt.Sprintf("P2 Q-%d: question %d", n+1, n+1), "body": "b",
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := call(t, as(env, "clanker-a"), "mail_read", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	msgs := got.([]any)
	if len(msgs) != 3 {
		t.Fatalf("got %d messages", len(msgs))
	}
	for i, m := range msgs {
		if h := m.(map[string]any)["header"]; h != fmt.Sprintf("P2 Q-%d: question %d", i+1, i+1) {
			t.Fatalf("message %d header %v", i, h)
		}
	}
	if from := msgs[0].(map[string]any)["from"]; from != "clerk-a-1" {
		t.Fatalf("from = %v", from)
	}
	again, _ := call(t, as(env, "clanker-a"), "mail_read", map[string]any{})
	if len(again.([]any)) != 0 {
		t.Fatal("read messages were not archived")
	}
}

func TestMailRefusesBadInput(t *testing.T) {
	env := testEnv(t, "bigm")
	if _, err := call(t, env, "mail_post", map[string]any{"to": "clanker-a", "header": "STATUS: x", "body": ""}); err == nil || !strings.Contains(err.Error(), "invalid header") {
		t.Fatalf("err = %v", err)
	}
	if _, err := call(t, env, "mail_post", map[string]any{"to": "../x", "header": "DONE: x", "body": ""}); err == nil || !strings.Contains(err.Error(), "invalid role key") {
		t.Fatalf("err = %v", err)
	}
}
