package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// repoConfig is one repository of <data>/repos.json.
type repoConfig struct {
	Repo    string `json:"repo"`
	Host    string `json:"host"`
	APIURL  string `json:"api_url"`
	Project string `json:"project"`
	// ponytail: ignored. repos_set of version 0.11 always wrote merge_method, and
	// DisallowUnknownFields would refuse those files. Drop the field when no such file is left.
	// Agent-derived, needs owner decision (task 19, 2026-10-05).
	OldMergeMethod string `json:"merge_method,omitempty"`
}

type reposConfig struct {
	IntervalSeconds int          `json:"interval_seconds"`
	Repos           []repoConfig `json:"repos"`
}

// gitlabRepoRE is the path of a GitLab project: a group, any subgroups, and the name.
var gitlabRepoRE = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)+$`)

// loadRepos reads <data>/repos.json and sets the defaults.
func loadRepos(dataDir string) (reposConfig, error) {
	return loadReposFile(filepath.Join(dataDir, "repos.json"))
}

// apiURL is the api_url of r with the default of its host and no trailing "/".
func apiURL(r repoConfig) string {
	defaults := map[string]string{"github": "https://api.github.com", "gitlab": "https://gitlab.com/api/v4"}
	return strings.TrimRight(cmp.Or(r.APIURL, defaults[r.Host]), "/")
}

func loadReposFile(file string) (reposConfig, error) {
	var cfg reposConfig
	raw, err := os.ReadFile(file)
	if err != nil {
		return cfg, fmt.Errorf("read the code host configuration: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", file, err)
	}
	cfg.IntervalSeconds = max(cmp.Or(cfg.IntervalSeconds, 60), 10)
	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		if r.Host == "gitlab" {
			if !gitlabRepoRE.MatchString(r.Repo) {
				return cfg, fmt.Errorf("%s: repo must be group/name or group/subgroup/.../name for gitlab: %q", file, r.Repo)
			}
		} else if !repoRE.MatchString(r.Repo) {
			return cfg, fmt.Errorf("%s: repo must be owner/name for github and gitea: %q", file, r.Repo)
		}
		switch r.Host {
		case "github", "gitlab":
		case "gitea":
			if r.APIURL == "" {
				return cfg, fmt.Errorf("%s: %s: api_url is required for gitea", file, r.Repo)
			}
		default:
			return cfg, fmt.Errorf("%s: %s: host must be github, gitlab, or gitea: %q", file, r.Repo, r.Host)
		}
		r.APIURL = apiURL(*r)
		if u, err := url.Parse(r.APIURL); err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return cfg, fmt.Errorf("%s: %s: api_url must be an https URL with no user, query, or fragment: %q", file, r.Repo, r.APIURL)
		}
		// An empty project stays empty: the watcher skips the entry (G2).
		if r.Project != "" && !projectRE.MatchString(r.Project) {
			return cfg, fmt.Errorf("%s: %s: project must be a project key ([a-z0-9-]): %q", file, r.Repo, r.Project)
		}
	}
	return cfg, nil
}

// hostComment is a comment, a review comment, or a review summary.
type hostComment struct {
	ID          int64  `json:"id"`
	Body        string `json:"body"`
	URL         string `json:"html_url"`
	UpdatedAt   string `json:"updated_at"`
	SubmittedAt string `json:"submitted_at"` // reviews
	State       string `json:"state"`        // reviews: APPROVED, CHANGES_REQUESTED, REQUEST_CHANGES, COMMENTED, ...
	PR          int    `json:"-"`            // reviews: the pull request number
	IssueURL    string `json:"issue_url"`
	PullURL     string `json:"pull_request_url"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

// Number is the issue or pull request number of the comment.
func (c hostComment) Number() int {
	if c.PR != 0 {
		return c.PR
	}
	n, _ := strconv.Atoi(path.Base(cmp.Or(c.PullURL, c.IssueURL)))
	return n
}

type hostPull struct {
	Number    int    `json:"number"`
	State     string `json:"state"`
	Merged    bool   `json:"merged"`
	MergedAt  string `json:"merged_at"`
	MergeSHA  string `json:"merge_commit_sha"`
	URL       string `json:"html_url"`
	UpdatedAt string `json:"updated_at"`
	Head      struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

// codeHost is the part of a code host API that the watcher uses.
type codeHost interface {
	Branches(ctx context.Context) (map[string]string, error)                 // branch name -> head SHA
	IssueComments(ctx context.Context, since string) ([]hostComment, error)  // issue and pull request comments
	ReviewComments(ctx context.Context, since string) ([]hostComment, error) // line comments (GitHub; Gitea returns none)
	Reviews(ctx context.Context, since string) ([]hostComment, error)        // review summaries of open pull requests
	MergedPulls(ctx context.Context) ([]hostPull, error)
	Checks(ctx context.Context, sha string) (string, error) // success, pending, failure, or none
	LastCall() string
}

// pullChecker reads the checks of the head of a pull request: the head SHA, the Checks value
// (pending until each check is done), and the summary. Only GitHub has it.
type pullChecker interface {
	PullChecks(ctx context.Context, n int) (sha, state, summary string, err error)
}

// httpError is a code host answer that is not 2xx.
type httpError struct {
	Code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

// rest is a small JSON client for one repository.
type rest struct {
	base, auth string
	hc         *http.Client
	last       string
}

func (c *rest) LastCall() string { return c.last }

func (c *rest) do(ctx context.Context, method, p string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	u := c.base + p
	c.last = method + " " + u
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.auth != "" {
		req.Header.Set("Authorization", c.auth)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return &httpError{resp.StatusCode, fmt.Sprintf("%s: %s: %s", c.last, resp.Status, bytes.TrimSpace(data[:min(len(data), 300)]))}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// reviewsSince reads the reviews of the open pull requests updated at or after since.
// ponytail: the 30 most recently updated open pull requests only.
func reviewsSince(ctx context.Context, c *rest, list, page, since string) ([]hostComment, error) {
	var ps []hostPull
	if err := c.do(ctx, "GET", list, nil, &ps); err != nil {
		return nil, err
	}
	t := parseTime(since)
	var out []hostComment
	for _, p := range ps {
		if parseTime(p.UpdatedAt).Before(t) {
			continue
		}
		var rs []hostComment
		if err := c.do(ctx, "GET", fmt.Sprintf("/pulls/%d/reviews%s", p.Number, page), nil, &rs); err != nil {
			return nil, err
		}
		for _, r := range rs {
			if r.SubmittedAt == "" {
				continue // a pending review of its author
			}
			r.UpdatedAt, r.PR = r.SubmittedAt, p.Number
			out = append(out, r)
		}
	}
	return out, nil
}

func mergedOnly(pulls []hostPull) []hostPull {
	var out []hostPull
	for _, p := range pulls {
		if p.Merged || p.MergedAt != "" {
			out = append(out, p)
		}
	}
	return out
}

type github struct{ rest }

func (g *github) Branches(ctx context.Context) (map[string]string, error) {
	var bs []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	// ponytail: the first 100 branches only; add pagination when a watched repository has more.
	if err := g.do(ctx, "GET", "/branches?per_page=100", nil, &bs); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, b := range bs {
		out[b.Name] = b.Commit.SHA
	}
	return out, nil
}

func (g *github) IssueComments(ctx context.Context, since string) ([]hostComment, error) {
	var cs []hostComment
	err := g.do(ctx, "GET", "/issues/comments?per_page=100&sort=updated&direction=asc&since="+url.QueryEscape(since), nil, &cs)
	return cs, err
}

func (g *github) ReviewComments(ctx context.Context, since string) ([]hostComment, error) {
	var cs []hostComment
	err := g.do(ctx, "GET", "/pulls/comments?per_page=100&sort=updated&direction=asc&since="+url.QueryEscape(since), nil, &cs)
	return cs, err
}

func (g *github) Reviews(ctx context.Context, since string) ([]hostComment, error) {
	return reviewsSince(ctx, &g.rest, "/pulls?state=open&sort=updated&direction=desc&per_page=30", "?per_page=100", since)
}

func (g *github) MergedPulls(ctx context.Context) ([]hostPull, error) {
	var ps []hostPull
	err := g.do(ctx, "GET", "/pulls?state=closed&sort=updated&direction=desc&per_page=30", nil, &ps)
	return mergedOnly(ps), err
}

// Checks combines the commit statuses and the check runs. The combined status is "pending"
// when no status exists, so it counts only when total_count is above 0.
func (g *github) Checks(ctx context.Context, sha string) (string, error) {
	state, _, _, err := g.checks(ctx, sha)
	return state, err
}

// PullChecks reads the head SHA of pull request n and its checks. The state is pending while a
// check still runs, also after another check failed, so the caller sees one result when all are
// done. A number that is no pull request (404, an issue) has the state none.
func (g *github) PullChecks(ctx context.Context, n int) (sha, state, summary string, err error) {
	var p hostPull
	if err := g.do(ctx, "GET", fmt.Sprintf("/pulls/%d", n), nil, &p); err != nil {
		if he, ok := errors.AsType[*httpError](err); ok && he.Code == http.StatusNotFound {
			return "", "none", "", nil
		}
		return "", "", "", err
	}
	if p.Head.SHA == "" {
		return "", "", "", fmt.Errorf("%s: no head sha", g.last)
	}
	state, summary, done, err := g.checks(ctx, p.Head.SHA)
	if !done && state == "failure" {
		state = "pending"
	}
	return p.Head.SHA, state, summary, err
}

// checks reads the combined status and the check runs of sha. state is the Checks value. summary
// counts the statuses and the check runs by their exact state, conclusion, or else status, with
// the keys sorted: "12 checks: 1 failure, 11 success". done is false while one of them runs.
func (g *github) checks(ctx context.Context, sha string) (state, summary string, done bool, err error) {
	var st struct {
		State      string `json:"state"`
		TotalCount int    `json:"total_count"`
		Statuses   []struct {
			State string `json:"state"`
		} `json:"statuses"`
	}
	if err := g.do(ctx, "GET", "/commits/"+url.PathEscape(sha)+"/status", nil, &st); err != nil {
		return "", "", false, err
	}
	type checkRun struct {
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
	}
	var all []checkRun
	total := 0
	for page := 1; ; page++ {
		var runs struct {
			TotalCount int        `json:"total_count"`
			CheckRuns  []checkRun `json:"check_runs"`
		}
		if err := g.do(ctx, "GET", fmt.Sprintf("/commits/%s/check-runs?per_page=100&page=%d", url.PathEscape(sha), page), nil, &runs); err != nil {
			return "", "", false, err
		}
		all, total = append(all, runs.CheckRuns...), runs.TotalCount
		if len(all) >= total || len(runs.CheckRuns) == 0 || page == 50 {
			break
		}
	}
	counts := map[string]int{}
	for _, s := range st.Statuses {
		counts[s.State]++
	}
	failed := st.TotalCount > 0 && st.State == "failure"
	pending := st.TotalCount > 0 && st.State == "pending"
	if len(all) < total {
		pending = true // runs that could not be read are never green
	}
	for _, r := range all {
		switch {
		case r.Status != "completed" || r.Conclusion == nil:
			pending = true
			counts[r.Status]++
		default:
			counts[*r.Conclusion]++
			failed = failed || *r.Conclusion != "success" && *r.Conclusion != "neutral" && *r.Conclusion != "skipped"
		}
	}
	n, parts := 0, []string{}
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		n += counts[k]
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
	}
	summary = fmt.Sprintf("%d checks", n)
	if n > 0 {
		summary += ": " + strings.Join(parts, ", ")
	}
	switch {
	case st.TotalCount == 0 && len(all) == 0 && total == 0:
		state = "none"
	case failed:
		state = "failure"
	case pending:
		state = "pending"
	default:
		state = "success"
	}
	return state, summary, !pending, nil
}

type gitea struct{ rest }

func (g *gitea) Branches(ctx context.Context) (map[string]string, error) {
	var bs []struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	// ponytail: the first page only (limit 50 is the usual server maximum); add pagination when needed.
	if err := g.do(ctx, "GET", "/branches?limit=50", nil, &bs); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, b := range bs {
		out[b.Name] = b.Commit.ID
	}
	return out, nil
}

func (g *gitea) IssueComments(ctx context.Context, since string) ([]hostComment, error) {
	var cs []hostComment
	err := g.do(ctx, "GET", "/issues/comments?limit=50&since="+url.QueryEscape(since), nil, &cs)
	return cs, err
}

func (g *gitea) ReviewComments(context.Context, string) ([]hostComment, error) { return nil, nil }

func (g *gitea) Reviews(ctx context.Context, since string) ([]hostComment, error) {
	return reviewsSince(ctx, &g.rest, "/pulls?state=open&sort=recentupdate&limit=30", "?limit=50", since)
}

func (g *gitea) MergedPulls(ctx context.Context) ([]hostPull, error) {
	var ps []hostPull
	err := g.do(ctx, "GET", "/pulls?state=closed&sort=recentupdate&limit=30", nil, &ps)
	return mergedOnly(ps), err
}

func (g *gitea) Checks(ctx context.Context, sha string) (string, error) {
	var st struct {
		State      string `json:"state"`
		TotalCount int    `json:"total_count"`
	}
	if err := g.do(ctx, "GET", "/commits/"+url.PathEscape(sha)+"/status", nil, &st); err != nil {
		return "", err
	}
	switch {
	case st.TotalCount == 0:
		return "none", nil
	case st.State == "success":
		return "success", nil
	case st.State == "pending":
		return "pending", nil
	}
	return "failure", nil // failure, error, and warning: never merge on them
}

// ghBin is the gh binary of the token fallback; tests replace it.
var ghBin = "gh"

// glabBin is the glab binary of the GitLab client and of the scan; tests replace it.
var glabBin = "glab"

// hostHTTP is the HTTP client of the code host clients; tests replace it.
var hostHTTP = &http.Client{Timeout: 30 * time.Second}

// envHostKey turns a host name into the suffix of a per-host variable: GIT_EXAMPLE_COM.
func envHostKey(host string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, host)
}

// hostToken returns the token for the api_url of a repository, and sends a token only to the
// host it belongs to. GitHub: GITHUB_TOKEN, or `gh auth token --hostname`, only for
// api.github.com or a host listed in BRUH_GITHUB_HOSTS (GitHub Enterprise). Gitea: only
// BRUH_GITEA_TOKEN_<HOST> of that host. Any other api_url gets no token.
func hostToken(r repoConfig) string {
	u, err := url.Parse(r.APIURL)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	switch r.Host {
	case "gitea":
		return os.Getenv("BRUH_GITEA_TOKEN_" + envHostKey(host))
	case "github":
		ghHost := "github.com"
		if host != "api.github.com" {
			listed := strings.FieldsFunc(strings.ToLower(os.Getenv("BRUH_GITHUB_HOSTS")), func(c rune) bool { return c == ',' || c == ' ' })
			if !slices.Contains(listed, host) {
				return ""
			}
			ghHost = host
		}
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			return t
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, ghBin, "auth", "token", "--hostname", ghHost).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return ""
}

// newHost builds the client of one repository.
func newHost(r repoConfig) (codeHost, error) {
	c := rest{base: r.APIURL + "/repos/" + r.Repo, hc: hostHTTP}
	tok := hostToken(r)
	switch r.Host {
	case "github":
		if tok != "" {
			c.auth = "Bearer " + tok
		}
		return &github{c}, nil
	case "gitea":
		if tok != "" {
			c.auth = "token " + tok
		}
		return &gitea{c}, nil
	case "gitlab":
		u, err := url.Parse(r.APIURL)
		if err != nil {
			return nil, err
		}
		return &gitlab{host: u.Hostname(), enc: url.PathEscape(r.Repo)}, nil
	}
	return nil, errors.New("unknown host: " + r.Host)
}

func reposTools() []Tool {
	str := stringSchema()
	return []Tool{{
		Name:        "repos_set",
		Description: "Add, replace, or remove (remove: true) one repository in <data>/repos.json, the code host configuration of the watcher. Only bigm. project is the project key of the clanker of the repository ([a-z0-9-]) and is required except with remove: true. A repo that is already configured with another api_url must be removed first. api_url must be https. Tokens are never stored: GitHub uses GITHUB_TOKEN or gh only for api.github.com and the hosts of BRUH_GITHUB_HOSTS; Gitea uses BRUH_GITEA_TOKEN_<HOST>; GitLab uses `glab api --hostname`, and no token passes through bruh.",
		InputSchema: objectSchema(map[string]any{
			"repo": str, "host": map[string]any{"type": "string", "enum": []string{"github", "gitlab", "gitea"}}, "api_url": str,
			"project": str, "remove": map[string]any{"type": "boolean"},
			"interval_seconds": map[string]any{"type": "integer", "minimum": 10},
		}, "repo", "project"),
		Handler: func(c *Call, raw json.RawMessage) (any, error) {
			me, err := c.Env.Caller()
			if err != nil {
				return nil, err
			}
			if me != "bigm" {
				return nil, errors.New("only bigm writes the code host configuration")
			}
			var a struct {
				repoConfig
				Remove          bool `json:"remove"`
				IntervalSeconds int  `json:"interval_seconds"`
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, err
			}
			var out any
			err = c.Env.WithLock("repos", func() error {
				dir, err := c.Env.Dir()
				if err != nil {
					return err
				}
				file := filepath.Join(dir, "repos.json")
				var cfg reposConfig
				if b, err := os.ReadFile(file); err == nil {
					if err := json.Unmarshal(b, &cfg); err != nil {
						return fmt.Errorf("%s: %w", file, err)
					}
				} else if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
				if a.IntervalSeconds != 0 {
					cfg.IntervalSeconds = a.IntervalSeconds
				}
				if !a.Remove {
					if !projectRE.MatchString(a.Project) {
						return fmt.Errorf("%s: %s: project is required and must be a project key ([a-z0-9-]); call repos_set with project", file, a.Repo)
					}
					// ponytail: repos.json is keyed by repo only, so one path on two hosts is refused (G3); key by api_url and repo if both are needed.
					if i := slices.IndexFunc(cfg.Repos, func(r repoConfig) bool { return r.Repo == a.Repo }); i >= 0 && apiURL(cfg.Repos[i]) != apiURL(a.repoConfig) {
						return fmt.Errorf("%s is already configured for %s; remove it first", a.Repo, apiURL(cfg.Repos[i]))
					}
				}
				cfg.Repos = slices.DeleteFunc(cfg.Repos, func(r repoConfig) bool { return r.Repo == a.Repo })
				if !a.Remove {
					cfg.Repos = append(cfg.Repos, a.repoConfig)
				}
				data, _ := json.MarshalIndent(cfg, "", "  ")
				tmp := filepath.Join(dir, "repos.json.check")
				if err := os.WriteFile(tmp, data, 0o600); err != nil {
					return err
				}
				defer os.Remove(tmp)
				if _, err := loadReposFile(tmp); err != nil {
					return err
				}
				out = cfg
				return atomicWrite(file, data)
			})
			return out, err
		},
	}}
}
