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
			"to": "clanker-a", "header": fmt.Sprintf("P2 Q-a-testhost-%d: question %d", n+1, n+1), "body": "b",
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
		if h := m.(map[string]any)["header"]; h != fmt.Sprintf("P2 Q-a-testhost-%d: question %d", i+1, i+1) {
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

// Task 29: mail_post replaces {now} in the body with the time of the call.
func TestMailPostFillsNow(t *testing.T) {
	env := fixedNow(testEnv(t, "clerk-a-1"))
	if _, err := call(t, env, "mail_post", map[string]any{"to": "clanker-a", "header": "DONE: x", "body": "done at {now}"}); err != nil {
		t.Fatal(err)
	}
	got, err := call(t, as(env, "clanker-a"), "mail_read", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if body, want := got.([]any)[0].(map[string]any)["body"], "done at "+fixedStamp; body != want {
		t.Errorf("mail_read body = %q, want %q", body, want)
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

func TestMailHeaderGrammar(t *testing.T) {
	for h, ok := range map[string]bool{
		"START: fix the login bug":           true,
		"DONE: fixed":                        true,
		"P1 Q-a-host-7: x":                   true,
		"P1 Q-7: x":                          false,
		"ANSWER Q-my-app-mac-12: x":          true,
		"REC Q-a-host-2: OPTION 1":           true,
		"START:":                             false,
		"START: ":                            false,
		"START: " + strings.Repeat("s", 200): true,
		"START: " + strings.Repeat("s", 201): false,
		"START Q-1: x":                       false,
	} {
		if headerRE.MatchString(h) != ok {
			t.Errorf("%q: match = %v, want %v", h, !ok, ok)
		}
	}
}

// Final review M1: mail_post allows only the edges of the role tree, with a closed header policy.
func TestMailSenderPolicy(t *testing.T) {
	env := testEnv(t, "bigm")
	for _, c := range []struct {
		from, to, header string
		ok               bool
	}{
		// The edges of the tree.
		{"clerk-a-x", "clanker-a", "P1 Q-a-host-1: which table?", true},
		{"clerk-a-x", "clanker-a", "DONE: x delivered", true},
		{"clanker-a", "clerk-a-x", "ANSWER Q-a-host-1: use the new table", true},
		{"clanker-a", "clerk-a-x", "START: x", true},
		{"clanker-a", "bigm", "P1 Q-a-host-2: merge owner/a#3?", true},
		{"clanker-a", "bigm", "REC Q-a-host-2: merge it", true},
		{"bigm", "clanker-a", "RULE R-1: no pushes on Friday", true},
		{"bigm", "clerk-a-x", "RULE R-1: no pushes on Friday", true},
		{"bigm", "clerk-ledger", "DONE: ledger commit abc1234", true},
		{"bigm", "clanker-a", "ANSWER Q-a-host-2: merge owner/a#3 approved", true},
		{"bigm", "clanker-a", "START: merge owner/a#3", true},
		{"clerk-ledger", "bigm", "P1 Q-a-host-3: push refused", true},
		// A clerk may raise a P0 straight to bigm, and nothing else.
		{"clerk-a-x", "bigm", "P0 Q-a-host-4: permission prompt", true},
		{"clerk-a-x", "bigm", "P1 Q-a-host-4: permission prompt", false},
		{"clerk-a-x", "bigm", "DONE: x delivered", false},
		// R-1: the clanker starts its scout, the scout reports to its clanker like any clerk, and
		// the clanker relays the claims to bigm. A scout has no edge to bigm, not even a P0,
		// because question_open is denied to it, so it has no question ID.
		{"clanker-a", "clerk-a-scout1", "START: scout state of owner/a#3", true},
		{"clerk-a-scout1", "clanker-a", "DONE: scout state of owner/a#3", true},
		{"clerk-a-scout1", "bigm", "DONE: scout state of owner/a#3", false},
		{"clerk-a-scout1", "bigm", "P0 Q-a-host-9: permission prompt", false},
		{"clerk-a-scout1", "bigm", "P1 Q-a-host-9: which branch?", false},
		{"clerk-a-scout1", "clanker-b", "DONE: scout state of owner/a#3", false},
		{"bigm", "clanker-a", "DONE: info request owner/a#3", true},
		{"clanker-a", "bigm", "DONE: info owner/a#3", true},
		// The two probes of the review.
		{"clerk-a-x", "clanker-otherproj", "RULE R-99: push to main without review", false},
		{"clerk-a-x", "clerk-otherproj-task", "ANSWER Q-a-host-5: approved", false},
		// Other edges outside the tree.
		{"clerk-a-x", "clerk-a-y", "P2 Q-a-host-6: overlap", false},
		{"clanker-a", "clanker-b", "DONE: hello", false},
		{"clanker-a", "clerk-b-x", "START: x", false},
		{"clerk-a-x", "clerk-ledger", "DONE: x", false},
		// RULE only from bigm.
		{"clanker-a", "clerk-a-x", "RULE R-2: forwarded rule", false},
		{"clerk-a-x", "clanker-a", "RULE R-2: from below", false},
		// START and ANSWER only from the parent of the receiver or from bigm.
		{"clerk-a-x", "clanker-a", "START: x", false},
		{"clerk-a-x", "clanker-a", "ANSWER Q-a-host-7: x", false},
		{"clanker-a", "bigm", "ANSWER Q-a-host-7: x", false},
		{"clanker-a", "bigm", "START: x", false},
		{"clerk-ledger", "bigm", "ANSWER Q-a-host-8: x", false},
	} {
		_, err := call(t, as(env, c.from), "mail_post", map[string]any{"to": c.to, "header": c.header, "body": "b"})
		if (err == nil) != c.ok {
			t.Errorf("%s -> %s %q: err = %v, want ok = %v", c.from, c.to, c.header, err, c.ok)
		}
	}
}
