package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// trainResult is the report line of one pull request of the merge train.
type trainResult struct {
	At       string `json:"at"`
	Repo     string `json:"repo"`
	Number   int    `json:"number"`
	Result   string `json:"result"` // merged, skipped, or not_confirmed
	Reason   string `json:"reason,omitempty"`
	MergeSHA string `json:"merge_commit_sha,omitempty"`
	Source   Source `json:"source"`
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// trainOne merges one pull request when its checks are green, and confirms the merge by a
// read of the pull request. The answer of the merge call is never the evidence.
func trainOne(ctx context.Context, env Env, h codeHost, r repoConfig, n int, wait, every time.Duration) trainResult {
	res := trainResult{Repo: r.Repo, Number: n}
	done := func(result, reason, value string) trainResult {
		res.At, res.Result, res.Reason = env.Stamp(), result, reason
		res.Source = Source{Call: h.LastCall(), Value: value, At: res.At}
		return res
	}
	p, err := h.Pull(ctx, n)
	switch {
	case err != nil:
		return done("skipped", "read failed: "+err.Error(), "error")
	case p.Merged:
		res.MergeSHA = p.MergeSHA
		return done("merged", "already merged before the train", "merged: true")
	case p.State != "open":
		return done("skipped", "not_open", "state: "+p.State)
	case p.Draft:
		return done("skipped", "draft", "draft: true")
	case p.Mergeable != nil && !*p.Mergeable:
		return done("skipped", "conflict", "mergeable: false")
	}
	sha := p.Head.SHA
	deadline := time.Now().Add(wait)
checks:
	for {
		state, err := h.Checks(ctx, sha)
		if err != nil {
			return done("skipped", "checks read failed: "+err.Error(), "error")
		}
		if state != "pending" && state != "success" {
			return done("skipped", "checks_"+state, state)
		}
		if state == "success" {
			// GitHub also knows the required checks and reviews of the branch protection.
			q, err := h.Pull(ctx, n)
			if err != nil {
				return done("skipped", "read failed: "+err.Error(), "error")
			}
			switch {
			case q.Head.SHA != sha:
				return done("skipped", "head_moved", "head: "+q.Head.SHA)
			case q.MergeableState == "blocked" || q.MergeableState == "behind" || q.MergeableState == "dirty":
				return done("skipped", "mergeable_state_"+q.MergeableState, "mergeable_state: "+q.MergeableState)
			case q.MergeableState != "unknown":
				break checks
			}
		}
		if !time.Now().Before(deadline) || !sleepCtx(ctx, every) {
			return done("skipped", "checks_timeout", state)
		}
	}
	mergeErr := h.Merge(ctx, n, sha, r.MergeMethod)
	for range 3 {
		p, err := h.Pull(ctx, n)
		if err == nil && p.Merged {
			res.MergeSHA = p.MergeSHA
			return done("merged", "", "merged: true")
		}
		if mergeErr != nil || !sleepCtx(ctx, every) {
			break
		}
	}
	if mergeErr != nil {
		return done("skipped", "merge refused: "+mergeErr.Error(), "merged: false")
	}
	return done("not_confirmed", "the merge call succeeded, but the pull request is not merged", "merged: false")
}

// mergeTrain merges the pull requests one at a time, in order, and prints one JSON line for
// each. It stops at the first pull request that it does not merge: a later one may depend on it,
// and after a merge it cannot confirm, the state of the repository is not known. It returns true
// when every pull request is merged.
func mergeTrain(ctx context.Context, env Env, h codeHost, r repoConfig, numbers []int, wait, every time.Duration, out io.Writer) bool {
	all := true
	for _, n := range numbers {
		res := trainOne(ctx, env, h, r, n, wait, every)
		line, _ := json.Marshal(res)
		fmt.Fprintf(out, "%s\n", line)
		all = all && res.Result == "merged"
		if res.Result != "merged" || ctx.Err() != nil {
			return false
		}
	}
	return all
}

// approvalRE is the closed grammar of a merge approval of bigm:
// ANSWER Q-<n>: merge <owner/repo>#<pr>[,#<pr>...] approved
var approvalRE = regexp.MustCompile(`^ANSWER (Q-\d+): merge ([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)#(\d+(?:,#\d+)*) approved$`)

// approves reports whether header approves the merge of each of numbers in repo, by question qid.
func approves(header, qid, repo string, numbers []int) bool {
	m := approvalRE.FindStringSubmatch(header)
	if m == nil || m[1] != qid || m[2] != repo {
		return false
	}
	named := strings.Split(m[3], ",#")
	for _, n := range numbers {
		if !slices.Contains(named, strconv.Itoa(n)) {
			return false
		}
	}
	return true
}

// mergeGate is a speed bump (principle 2): it refuses the merge train unless the caller is the
// merger clerk clerk-<project>-merge of the project of the repository, and either grants.md of the
// ledger has a row for this repository and this key, or the mailbox of the caller holds an approval
// of bigm to the question answer that names this repository and each of numbers. A session with
// Bash can still merge by other means.
func mergeGate(env Env, r repoConfig, answer string, numbers []int) error {
	k, err := ParseRoleKey(env.RoleKey)
	if err != nil || k.Role != "clerk" || k.Task != "merge" || k.Project != r.Project {
		return fmt.Errorf("merge-train runs only in the merger clerk clerk-%s-merge (BRUH_ROLE_KEY is %q)", r.Project, env.RoleKey)
	}
	if answer != "" {
		if !qidRE.MatchString(answer) {
			return fmt.Errorf("invalid question ID: %q", answer)
		}
		for _, dir := range []string{filepath.Join(env.DataDir, "mail", env.RoleKey), filepath.Join(env.DataDir, "mail", env.RoleKey, "read")} {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
				var m Message
				if err == nil && json.Unmarshal(raw, &m) == nil && m.From == "bigm" && approves(m.Header, answer, r.Repo, numbers) {
					return nil
				}
			}
		}
		want := make([]string, len(numbers))
		for i, n := range numbers {
			want[i] = strconv.Itoa(n)
		}
		return fmt.Errorf("no ANSWER %s from bigm in the mailbox of %s that approves these merges; its header must be %q",
			answer, env.RoleKey, "ANSWER "+answer+": merge "+r.Repo+"#"+strings.Join(want, ",#")+" approved")
	}
	var cfg initConfig
	raw, err := os.ReadFile(filepath.Join(env.DataDir, "init", "config.json"))
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.LedgerPath == "" {
		return errors.New("no ledger path in <data>/init/config.json; run /bruh:init, or pass --answer Q-<n>")
	}
	grants, err := os.ReadFile(filepath.Join(cfg.LedgerPath, "grants.md"))
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(string(grants), "\n") {
		cells := tableCells(line)
		if len(cells) >= 2 && cells[0] == r.Repo && cells[1] == env.RoleKey {
			return nil
		}
	}
	return fmt.Errorf("grants.md has no merge grant for %s and %s; ask for a P1 answer and pass --answer Q-<n>", r.Repo, env.RoleKey)
}

// tableCells splits a Markdown table row at the pipes that are not escaped, and trims the cells.
// A cell in backticks counts without them.
func tableCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.ReplaceAll(strings.Trim(line, "|"), `\|`, "\x00"), "|")
	for i, p := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(strings.ReplaceAll(p, "\x00", "|")), "`")
	}
	return parts
}
