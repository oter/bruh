package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	for {
		state, err := h.Checks(ctx, sha)
		if err != nil {
			return done("skipped", "checks read failed: "+err.Error(), "error")
		}
		if state == "success" {
			break
		}
		if state != "pending" {
			return done("skipped", "checks_"+state, state)
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
// each. It stops at a merge it cannot confirm, because the state of the repository is then not
// known. It returns true when every pull request is merged.
func mergeTrain(ctx context.Context, env Env, h codeHost, r repoConfig, numbers []int, wait, every time.Duration, out io.Writer) bool {
	all := true
	for _, n := range numbers {
		res := trainOne(ctx, env, h, r, n, wait, every)
		line, _ := json.Marshal(res)
		fmt.Fprintf(out, "%s\n", line)
		all = all && res.Result == "merged"
		if res.Result == "not_confirmed" || ctx.Err() != nil {
			return false
		}
	}
	return all
}
