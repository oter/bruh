package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"time"
)

// gitlab is the GitLab client of one repository. It calls glab api --hostname <host> <path>, so
// glab keeps the login of the owner and no token passes through bruh.
type gitlab struct {
	host string // the host of api_url, for --hostname
	enc  string // the repository path, URL-encoded
	last string
}

// glMergeRequest is the part of a GitLab merge request that hostPull needs.
type glMergeRequest struct {
	IID                 int    `json:"iid"`
	State               string `json:"state"`
	Draft               bool   `json:"draft"`
	SHA                 string `json:"sha"`
	MergeCommitSHA      string `json:"merge_commit_sha"`
	SquashCommitSHA     string `json:"squash_commit_sha"`
	WebURL              string `json:"web_url"`
	UpdatedAt           string `json:"updated_at"`
	MergedAt            string `json:"merged_at"`
	DetailedMergeStatus string `json:"detailed_merge_status"`
}

func (m glMergeRequest) pull() hostPull {
	p := hostPull{
		Number:              m.IID,
		State:               m.State,
		Merged:              m.State == "merged",
		Draft:               m.Draft,
		MergeSHA:            cmp.Or(m.MergeCommitSHA, m.SquashCommitSHA),
		URL:                 m.WebURL,
		UpdatedAt:           m.UpdatedAt,
		MergedAt:            m.MergedAt,
		DetailedMergeStatus: m.DetailedMergeStatus,
	}
	if m.State == "opened" {
		p.State = "open"
	}
	p.Head.SHA = m.SHA
	return p
}

// api runs glab api for projects/<enc>/<p> with the extra arguments and stdin, outside the
// repositories, and decodes stdout into out when out is not nil.
func (g *gitlab) api(ctx context.Context, p string, stdin io.Reader, out any, extra ...string) error {
	p = "projects/" + g.enc + "/" + p
	g.last = "glab api --hostname " + g.host + " " + p
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, glabBin, append([]string{"api", "--hostname", g.host, p}, extra...)...)
	cmd.Dir = os.TempDir()
	cmd.Stdin = stdin
	data, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = ee.Stderr
		}
		msg := g.last
		if s := bytes.TrimSpace(stderr[:min(len(stderr), 300)]); len(s) > 0 {
			msg += ": " + string(s)
		}
		return fmt.Errorf("%s: %w", msg, err)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: decode: %w", g.last, err)
	}
	return nil
}

func (g *gitlab) Branches(ctx context.Context) (map[string]string, error) {
	var bs []struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	// ponytail: the first 100 branches only; add pagination when a watched repository has more.
	if err := g.api(ctx, "repository/branches?per_page=100", nil, &bs); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, b := range bs {
		out[b.Name] = b.Commit.ID
	}
	return out, nil
}

// IssueComments reads the notes of the open merge requests updated at or after since, without
// the system notes.
func (g *gitlab) IssueComments(ctx context.Context, since string) ([]hostComment, error) {
	var mrs []glMergeRequest
	// ponytail: the 30 most recently updated open merge requests and their first 100 notes only.
	if err := g.api(ctx, "merge_requests?state=opened&order_by=updated_at&sort=desc&per_page=30", nil, &mrs); err != nil {
		return nil, err
	}
	t := parseTime(since)
	var out []hostComment
	for _, m := range mrs {
		if parseTime(m.UpdatedAt).Before(t) {
			continue
		}
		var notes []struct {
			ID        int64  `json:"id"`
			Body      string `json:"body"`
			System    bool   `json:"system"`
			UpdatedAt string `json:"updated_at"`
			Author    struct {
				Username string `json:"username"`
			} `json:"author"`
		}
		if err := g.api(ctx, fmt.Sprintf("merge_requests/%d/notes?sort=asc&order_by=updated_at&per_page=100", m.IID), nil, &notes); err != nil {
			return nil, err
		}
		for _, n := range notes {
			if n.System {
				continue
			}
			c := hostComment{ID: n.ID, Body: n.Body, UpdatedAt: n.UpdatedAt, PR: m.IID, URL: fmt.Sprintf("%s#note_%d", m.WebURL, n.ID)}
			c.User.Login = n.Author.Username
			out = append(out, c)
		}
	}
	return out, nil
}

// ReviewComments returns none: the line comments of GitLab are notes, read by IssueComments.
func (g *gitlab) ReviewComments(ctx context.Context, since string) ([]hostComment, error) {
	return nil, nil
}

// Reviews returns none: GitLab gives no review events.
func (g *gitlab) Reviews(ctx context.Context, since string) ([]hostComment, error) {
	return nil, nil
}

func (g *gitlab) MergedPulls(ctx context.Context) ([]hostPull, error) {
	var mrs []glMergeRequest
	// ponytail: the 30 most recently updated merged merge requests only.
	if err := g.api(ctx, "merge_requests?state=merged&order_by=updated_at&sort=desc&per_page=30", nil, &mrs); err != nil {
		return nil, err
	}
	out := make([]hostPull, 0, len(mrs))
	for _, m := range mrs {
		out = append(out, m.pull())
	}
	return out, nil
}

func (g *gitlab) Pull(ctx context.Context, n int) (hostPull, error) {
	var m glMergeRequest
	if err := g.api(ctx, fmt.Sprintf("merge_requests/%d", n), nil, &m); err != nil {
		return hostPull{}, err
	}
	return m.pull(), nil
}

// Checks maps the state of the newest pipeline of sha: success is green, failed is red, and
// each other state is pending.
func (g *gitlab) Checks(ctx context.Context, sha string) (string, error) {
	var ps []struct {
		Status string `json:"status"`
	}
	if err := g.api(ctx, "pipelines?sha="+url.QueryEscape(sha)+"&order_by=id&sort=desc&per_page=1", nil, &ps); err != nil {
		return "", err
	}
	if len(ps) == 0 {
		return "none", nil
	}
	switch ps[0].Status {
	case "success":
		return "success", nil
	case "failed":
		return "failure", nil
	}
	return "pending", nil
}

func (g *gitlab) Merge(ctx context.Context, n int, sha, method string) error {
	body, err := json.Marshal(struct {
		SHA    string `json:"sha"`
		Squash bool   `json:"squash,omitempty"`
	}{sha, method == "squash"})
	if err != nil {
		return err
	}
	return g.api(ctx, fmt.Sprintf("merge_requests/%d/merge", n), bytes.NewReader(body), nil,
		"-X", "PUT", "-H", "Content-Type: application/json", "--input", "-")
}

func (g *gitlab) LastCall() string {
	return g.last
}
