package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// gitlabWaitStatuses are the values of detailed_merge_status of GitLab that the merge train waits
// on, as on GitHub unknown (gap G8 of the v0.6 spec): mergeable merges, and each other value skips.
var gitlabWaitStatuses = []string{"checking", "unchecked", "preparing", "ci_still_running", "approvals_syncing"}

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
			// The host also knows the required checks and reviews: mergeable_state of GitHub, and
			// detailed_merge_status of GitLab (G8).
			q, err := h.Pull(ctx, n)
			if err != nil {
				return done("skipped", "read failed: "+err.Error(), "error")
			}
			switch {
			case q.Head.SHA != sha:
				return done("skipped", "head_moved", "head: "+q.Head.SHA)
			case q.DetailedMergeStatus == "mergeable":
				break checks
			case q.DetailedMergeStatus != "":
				if v := q.DetailedMergeStatus; !slices.Contains(gitlabWaitStatuses, v) {
					return done("skipped", "detailed_merge_status_"+v, "detailed_merge_status: "+v)
				}
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
// ANSWER Q-<id>: merge <repo>#<pr>[,#<pr>...] approved
var approvalRE = regexp.MustCompile(`^ANSWER (` + qidPattern + `): merge ([A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)+)#(\d+(?:,#\d+)*) approved$`)

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

// mergeGate is a speed bump (principle 2): it refuses the merge train unless the repository has a
// project (G2), the caller is the merger clerk clerk-<project>-merge of that project, the account
// of an SSH host alias is confirmed (checkAliasIdentity), and either grants.md of the ledger has a
// row for this repository and this key, or the mailbox of the caller holds an approval of bigm to
// the question answer that names this repository and each of numbers. A session with Bash can
// still merge by other means.
func mergeGate(env Env, r repoConfig, answer string, numbers []int) error {
	if r.Project == "" {
		return fmt.Errorf("%s: no project; bigm calls repos_set with project", r.Repo)
	}
	k, err := ParseRoleKey(env.RoleKey)
	if err != nil || k.Role != "clerk" || k.Task != "merge" || k.Project != r.Project {
		return fmt.Errorf("merge-train runs only in the merger clerk clerk-%s-merge (BRUH_ROLE_KEY is %q)", r.Project, env.RoleKey)
	}
	if err := checkAliasIdentity(env, r); err != nil {
		return err
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
	ledger, err := ledgerPath(env)
	if err != nil {
		return fmt.Errorf("pass --answer Q-<id>, or fix this: %w", err)
	}
	grants, err := os.ReadFile(filepath.Join(ledger, "grants.md"))
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(string(grants), "\n") {
		cells := tableCells(line)
		if len(cells) >= 2 && cells[0] == r.Repo && cells[1] == env.RoleKey {
			return nil
		}
	}
	return fmt.Errorf("grants.md has no merge grant for %s and %s; ask for a P1 answer and pass --answer Q-<id>", r.Repo, env.RoleKey)
}

// checkAliasIdentity refuses a repository whose chosen remote in <ledger>/learn/projects/<project>.json
// uses an SSH host alias (G10: the host of its URL differs from host.value, or host.value is null)
// until a data row of the table under "## Identities" of <ledger>/projects/<project>.md names the
// alias in its first cell and an account in its second (spec 8.5). No ledger path, no index file,
// no repository with host_path r.Repo, or no alias: no check.
func checkAliasIdentity(env Env, r repoConfig) error {
	ledger, err := ledgerPath(env)
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(ledger, "learn", "projects", r.Project+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var pf projectFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return fmt.Errorf("learn/projects/%s.json: %w", r.Project, err)
	}
	i := slices.IndexFunc(pf.Repos, func(x indexRepo) bool { return x.HostPath == r.Repo })
	if i < 0 {
		return nil
	}
	repo := pf.Repos[i]
	j := slices.IndexFunc(repo.Remotes, func(x remote) bool { return x.Name == repo.Remote })
	if j < 0 {
		return nil
	}
	_, alias, _, _ := splitRemoteURL(repo.Remotes[j].URL)
	if alias == "" || repo.Host.Value != nil && strings.EqualFold(alias, *repo.Host.Value) {
		return nil
	}
	md, _ := os.ReadFile(filepath.Join(ledger, "projects", r.Project+".md")) // no file: no row
	in := false
	for line := range strings.SplitSeq(string(md), "\n") {
		if strings.HasPrefix(line, "#") {
			in = strings.TrimSpace(line) == "## Identities"
			continue
		}
		if cells := tableCells(line); in && len(cells) >= 2 && cells[0] == alias && cells[1] != "" {
			return nil
		}
	}
	return fmt.Errorf(`the remote of %s uses the SSH host alias %s, so its account is not checked; the owner confirms the account in "Identities" of projects/%s.md (a row for %s)`,
		r.Repo, alias, r.Project, alias)
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
