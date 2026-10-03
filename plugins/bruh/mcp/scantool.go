package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// learnScanInput is the closed input of learn_scan.
type learnScanInput struct {
	Ledger      string            `json:"ledger"`
	Root        string            `json:"root"`
	Depth       int               `json:"depth"`
	Exclude     []string          `json:"exclude"`
	HostAliases map[string]string `json:"host_aliases"`
	HostKinds   map[string]string `json:"host_kinds"`
}

// scanRepo is one repository of the output of learn_scan: the record of the index without
// state, with its group, its SSH host alias, and its activity.
type scanRepo struct {
	Path          string         `json:"path"`
	Remotes       []remote       `json:"remotes"`
	Remote        string         `json:"remote"`
	Host          hostValue      `json:"host"`
	Kind          string         `json:"kind"`
	APIURL        string         `json:"api_url"`
	HostPath      string         `json:"host_path"`
	DefaultBranch string         `json:"default_branch"`
	Group         string         `json:"group"`
	Alias         string         `json:"alias"`
	Activity      activityResult `json:"activity"`
}

// learnScanOutput is the result of learn_scan.
type learnScanOutput struct {
	Repos              []scanRepo        `json:"repos"`
	Projects           []projectProposal `json:"projects"`
	HostAliases        map[string]string `json:"host_aliases"`
	HostsWithoutKind   []string          `json:"hosts_without_kind"`
	AliasesWithoutHost []string          `json:"aliases_without_host"`
}

// learnScan finds the repositories under the root and reads their git facts and their
// activity. It refuses a caller with a role key, and it writes nothing.
func learnScan(env Env, in learnScanInput) (learnScanOutput, error) {
	if env.RoleKey != "" {
		return learnScanOutput{}, fmt.Errorf("learn_scan runs only in the init skill of the owner, in a session with no role key (BRUH_ROLE_KEY is %q); run /bruh:init with claude --setting-sources user in the ledger folder, or from a folder outside the ledger", env.RoleKey)
	}
	in.Root = cmp.Or(in.Root, filepath.Join(env.Home, "workspace"))
	in.Depth = cmp.Or(in.Depth, 4)
	if in.Exclude == nil {
		in.Exclude = []string{"archive"}
	}
	if err := in.check(); err != nil {
		return learnScanOutput{}, err
	}

	dir := os.TempDir() // the working folder of each CLI, outside the repositories
	if info, err := os.Stat(env.DataDir); err == nil && info.IsDir() {
		dir = env.DataDir
	}
	found := walkRepos(in.Root, in.Depth, in.Exclude, in.Ledger)
	logins := loginHosts(dir)
	fc := newFactCtx(in.Root, dir, in.HostAliases, in.HostKinds)
	fc.logins, fc.ssh = logins, true
	out := learnScanOutput{
		Repos:              make([]scanRepo, len(found)),
		Projects:           proposeProjects(found),
		HostAliases:        map[string]string{},
		HostsWithoutKind:   []string{},
		AliasesWithoutHost: []string{},
	}
	if out.Projects == nil {
		out.Projects = []projectProposal{}
	}
	records := make([]indexRepo, len(found))
	for i, rd := range found {
		r, alias, sshHost, err := fc.facts(rd.Path)
		if err != nil {
			return learnScanOutput{}, fmt.Errorf("repository %s: %w", rd.Path, err)
		}
		records[i] = r
		out.Repos[i] = scanRepo{
			Path: r.Path, Remotes: r.Remotes, Remote: r.Remote, Host: r.Host, Kind: r.Kind,
			APIURL: r.APIURL, HostPath: r.HostPath, DefaultBranch: r.DefaultBranch,
			Group: rd.Group, Alias: alias,
		}
		if sshHost != "" {
			out.HostAliases[alias] = sshHost
		}
		switch {
		case r.Host.Value == nil && alias != "":
			out.AliasesWithoutHost = append(out.AliasesWithoutHost, alias)
		case r.Host.Value != nil && r.Kind == "unknown":
			out.HostsWithoutKind = append(out.HostsWithoutKind, *r.Host.Value)
		}
	}
	for i, a := range activity(records, logins, dir) { // always (G60)
		out.Repos[i].Activity = a
	}
	slices.Sort(out.HostsWithoutKind)
	out.HostsWithoutKind = slices.Compact(out.HostsWithoutKind)
	slices.Sort(out.AliasesWithoutHost)
	out.AliasesWithoutHost = slices.Compact(out.AliasesWithoutHost)
	return out, nil
}

// check checks the input of learn_scan after the defaults are set. It runs nothing.
func (in learnScanInput) check() error {
	var errs []error
	check := func(ok bool, format string, args ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf(format, args...))
		}
	}
	check(filepath.IsAbs(in.Ledger), "ledger must be an absolute path: %q", in.Ledger)
	check(filepath.IsAbs(in.Root), "root must be an absolute path: %q", in.Root)
	check(in.Depth >= 1 && in.Depth <= 8, "depth must be 1 to 8: %d", in.Depth)
	for _, alias := range slices.Sorted(maps.Keys(in.HostAliases)) {
		h := in.HostAliases[alias]
		check(hostNameRE.MatchString(alias), "host_aliases: alias %q does not match %s", alias, hostNameRE)
		check(hostNameRE.MatchString(h), "host_aliases: host %q of alias %q does not match %s", h, alias, hostNameRE)
	}
	for _, h := range slices.Sorted(maps.Keys(in.HostKinds)) {
		k := in.HostKinds[h]
		check(hostNameRE.MatchString(h), "host_kinds: host %q does not match %s", h, hostNameRE)
		check(k == "github" || k == "gitlab" || k == "gitea", "host_kinds: kind %q of host %q is not github, gitlab, or gitea", k, h)
	}
	return errors.Join(errs...)
}

// learnScanTool is the MCP tool learn_scan.
func learnScanTool() Tool {
	str := stringSchema()
	return Tool{
		Name:        "learn_scan",
		Description: "Scan the root for git repositories and read their git facts (remotes, host, kind, API root, default branch) and their activity on the code host, for the init skill. Runs only in a session with no role key. Reads only the folder tree and the .git folders; runs no command of a repository; writes nothing.",
		InputSchema: objectSchema(map[string]any{
			"ledger":       str,
			"root":         str,
			"depth":        map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
			"exclude":      map[string]any{"type": "array", "items": str},
			"host_aliases": map[string]any{"type": "object", "additionalProperties": str},
			"host_kinds":   map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string", "enum": []string{"github", "gitlab", "gitea"}}},
		}, "ledger"),
		Handler: func(c *Call, raw json.RawMessage) (any, error) {
			if len(bytes.TrimSpace(raw)) == 0 {
				raw = json.RawMessage(`{}`)
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields() // the input is closed
			var in learnScanInput
			if err := dec.Decode(&in); err != nil {
				return nil, fmt.Errorf("learn_scan input: %w", err)
			}
			return learnScan(c.Env, in)
		},
	}
}
