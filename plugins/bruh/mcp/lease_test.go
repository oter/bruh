package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func mustErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want error %q, got %v", want, err)
	}
}

func TestLeaseHierarchy(t *testing.T) {
	bigm := testEnv(t, "bigm")
	ca, cb := as(bigm, "clanker-a"), as(bigm, "clanker-b")
	g := func(env Env, to string) error {
		_, err := call(t, env, "lease_grant", map[string]any{"resource": "staging", "to": to, "minutes": 30})
		return err
	}
	_, err := call(t, ca, "lease_define", map[string]any{"resource": "staging", "capacity": 1})
	mustErr(t, err, "only bigm")
	if _, err := call(t, bigm, "lease_define", map[string]any{"resource": "staging", "capacity": 1}); err != nil {
		t.Fatal(err)
	}
	mustErr(t, g(ca, "clerk-a-1"), "holds no grant")
	if err := g(bigm, "clanker-a"); err != nil {
		t.Fatal(err)
	}
	mustErr(t, g(bigm, "clanker-b"), "capacity")
	mustErr(t, g(ca, "clerk-b-1"), "own clerks")
	if err := g(ca, "clerk-a-1"); err != nil {
		t.Fatal(err)
	}
	mustErr(t, g(ca, "clerk-a-2"), "one sub-grant")
	_, err = call(t, cb, "lease_release", map[string]any{"resource": "staging", "holder": "clanker-a"})
	mustErr(t, err, "holder or its grantor")
	if _, err := call(t, bigm, "lease_release", map[string]any{"resource": "staging", "holder": "clanker-a"}); err != nil {
		t.Fatal(err)
	}
	state, _ := call(t, bigm, "lease_list", map[string]any{})
	grants := state.(map[string]any)["resources"].(map[string]any)["staging"].(map[string]any)["grants"].([]any)
	if len(grants) != 0 {
		t.Fatalf("grants = %v", grants)
	}
}

func TestLeaseParallelGrantsKeepCount(t *testing.T) {
	bigm := testEnv(t, "bigm")
	if _, err := call(t, bigm, "lease_define", map[string]any{"resource": "runner", "capacity": 10}); err != nil {
		t.Fatal(err)
	}
	var ok atomic.Int64
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := call(t, bigm, "lease_grant", map[string]any{"resource": "runner", "to": fmt.Sprintf("clanker-p%d", i), "minutes": 5}); err == nil {
				ok.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if ok.Load() != 10 {
		t.Fatalf("granted %d, want 10", ok.Load())
	}
	data, err := os.ReadFile(filepath.Join(bigm.DataDir, "leases", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Resources map[string]struct {
			Grants []any `json:"grants"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if n := len(st.Resources["runner"].Grants); n != 10 {
		t.Fatalf("state has %d grants", n)
	}
}

func TestLeaseGrantUsesParent(t *testing.T) {
	bigm := testEnv(t, "bigm")
	my := as(bigm, "clanker-my")
	g := func(env Env, to string) error {
		_, err := call(t, env, "lease_grant", map[string]any{"resource": "staging", "to": to, "minutes": 30})
		return err
	}
	if _, err := call(t, bigm, "lease_define", map[string]any{"resource": "staging", "capacity": 2}); err != nil {
		t.Fatal(err)
	}
	if err := g(bigm, "clanker-my"); err != nil {
		t.Fatal(err)
	}
	mustErr(t, g(my, "clerk-my-app-t1"), "own clerks")
	mustErr(t, g(bigm, "clerk-my-t1"), "only to clankers")
	mustErr(t, g(bigm, "clerk-ledger"), "only to clankers")
	if err := g(my, "clerk-my-t1"); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseRequestStoresGrantor(t *testing.T) {
	bigm := testEnv(t, "bigm")
	if _, err := call(t, bigm, "lease_define", map[string]any{"resource": "db", "capacity": 1}); err != nil {
		t.Fatal(err)
	}
	_, err := call(t, bigm, "lease_request", map[string]any{"resource": "db"})
	mustErr(t, err, "no grantor")
	if _, err := call(t, as(bigm, "clerk-my-app-t1"), "lease_request", map[string]any{"resource": "db"}); err != nil {
		t.Fatal(err)
	}
	st, _ := call(t, bigm, "lease_list", map[string]any{})
	req := st.(map[string]any)["requests"].([]any)[0].(map[string]any)
	if req["grantor"] != "clanker-my-app" || req["requester"] != "clerk-my-app-t1" {
		t.Fatalf("request = %v", req)
	}
}
