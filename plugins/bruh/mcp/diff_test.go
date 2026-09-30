package main

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	if d := unifiedDiff("a", "a", "x\n", "x\n"); d != "" {
		t.Fatalf("no change: %q", d)
	}
	got := unifiedDiff("/dev/null", "f", "", "one\ntwo\n")
	if got != "--- /dev/null\n+++ f\n@@ -0,0 +1,2 @@\n+one\n+two\n" {
		t.Fatalf("new file:\n%s", got)
	}
	old := strings.Join([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"}, "\n") + "\n"
	new := strings.Replace(strings.Replace(old, "2\n", "two\n", 1), "14\n", "14\n14b\n", 1)
	want := "--- f\n+++ f\n@@ -1,5 +1,5 @@\n 1\n-2\n+two\n 3\n 4\n 5\n@@ -12,4 +12,5 @@\n 12\n 13\n 14\n+14b\n 15\n"
	if got := unifiedDiff("f", "f", old, new); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// Two changes within six lines of each other share one hunk.
	new = strings.Replace(strings.Replace(old, "2\n", "two\n", 1), "7\n", "seven\n", 1)
	if got := unifiedDiff("f", "f", old, new); strings.Count(got, "@@ -") != 1 {
		t.Fatalf("got:\n%s", got)
	}
}

func TestOrderedJSONRoundTrip(t *testing.T) {
	in := `{"z":1,"a":{"y":[1,2e3,"x && y <b>"],"b":null},"m":true,"n":1.50}`
	v, err := parseOrdered([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"z\": 1,\n  \"a\": {\n    \"y\": [\n      1,\n      2e3,\n      \"x && y <b>\"\n    ],\n    \"b\": null\n  },\n  \"m\": true,\n  \"n\": 1.50\n}\n"
	if got := string(encodeOrdered(v)); got != want {
		t.Fatalf("got:\n%s", got)
	}
	o := v.(*object)
	o.child("a").set("c", "new")
	o.set("z", 2)
	if got := string(encodeOrdered(o)); !strings.HasPrefix(got, "{\n  \"z\": 2,") || !strings.Contains(got, "\"b\": null,\n    \"c\": \"new\"") {
		t.Fatalf("got:\n%s", got)
	}
	for _, bad := range []string{`{"a":1} x`, `{"a":`, ``} {
		if _, err := parseOrdered([]byte(bad)); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}
