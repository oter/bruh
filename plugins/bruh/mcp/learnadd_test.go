package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// learnAdd calls learn_add as role with args and returns the written paths.
func (f *refreshFixture) learnAdd(t *testing.T, role string, args map[string]any) ([]string, error) {
	t.Helper()
	out, err := call(t, as(f.env, role), "learn_add", args)
	if err != nil {
		return nil, err
	}
	var r struct {
		Written []string `json:"written"`
	}
	decodeStrict(t, []byte(jsonText(t, out)), &r)
	return r.Written, nil
}

// readTree returns learn/tree.json of the fixture.
func (f *refreshFixture) readTree(t *testing.T) treeFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.ledger, "learn", "tree.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tree treeFile
	decodeStrict(t, data, &tree)
	return tree
}

func TestLearnAddAddsProject(t *testing.T) {
	f := newRefreshFixture(t)
	f.writeRepo(t, "gamma", refreshURL("gamma"))
	f.writeRepo(t, "grp/delta", refreshURL("grp/delta"))
	// A project file that exists keeps its bytes: learn_add writes it only when it is missing.
	delta := filepath.Join(f.ledger, "projects", "delta.md")
	writeRefreshFile(t, delta, "# delta\n\n| a |\n|---|\n| row |\n")

	written, err := f.learnAdd(t, "bigm", map[string]any{"repos": []string{"gamma"}})
	if err != nil {
		t.Fatalf("learn_add gamma: %v", err)
	}
	if want := []string{"learn/projects/gamma.json", "learn/tree.json", "projects/gamma.md"}; !slices.Equal(written, want) {
		t.Errorf("learn_add gamma written = %q, want %q", written, want)
	}
	written, err = f.learnAdd(t, "bigm", map[string]any{"repos": []string{"grp/delta"}})
	if err != nil {
		t.Fatalf("learn_add grp/delta: %v", err)
	}
	if want := []string{"learn/projects/delta.json", "learn/tree.json"}; !slices.Equal(written, want) {
		t.Errorf("learn_add grp/delta written = %q, want %q", written, want)
	}

	for key, rel := range map[string]string{"gamma": "gamma", "delta": "grp/delta"} {
		data, err := os.ReadFile(filepath.Join(f.ledger, "learn", "projects", key+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var got projectFile
		decodeStrict(t, data, &got)
		// The defaults of init with no fills: no purpose, no links, no docs.
		want := projectFile{Key: key, Main: rel, Repos: []indexRepo{refreshRepo(rel)}, Links: []link{}, Docs: []doc{}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("project %s = %s, want %s", key, jsonText(t, got), jsonText(t, want))
		}
	}
	tree := f.readTree(t)
	want := []treeProject{{Key: "delta", Group: "grp"}, {Key: "gamma"}, {Key: f.key}}
	slices.SortFunc(want, func(a, b treeProject) int { return strings.Compare(a.Key, b.Key) })
	if !reflect.DeepEqual(tree.Projects, want) || tree.Root != f.root || tree.Depth != 2 {
		t.Errorf("tree = %s, want root %q, depth 2, projects %s", jsonText(t, tree), f.root, jsonText(t, want))
	}
	md, err := os.ReadFile(filepath.Join(f.ledger, "projects", "gamma.md"))
	if err != nil {
		t.Fatal(err)
	}
	if want := projectTemplate(t, f.env, "gamma"); string(md) != want {
		t.Errorf("projects/gamma.md = %q, want %q", md, want)
	}
	if md, _ := os.ReadFile(delta); string(md) != "# delta\n\n| a |\n|---|\n| row |\n" {
		t.Errorf("projects/delta.md = %q, want the stored bytes", md)
	}
}

func TestLearnAddRefusesNonBigm(t *testing.T) {
	tests := []struct{ role, want string }{
		{"", "BRUH_ROLE_KEY is not set"},
		{"clanker-shop", "only bigm calls learn_add"},
		{"clerk-shop-x", "only bigm calls learn_add"},
		{"clerk-ledger", "only bigm calls learn_add"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			f := newRefreshFixture(t)
			f.writeRepo(t, "gamma", refreshURL("gamma"))
			before := f.snapshot(t)
			written, err := f.learnAdd(t, tt.role, map[string]any{"repos": []string{"gamma"}})
			if err == nil || err.Error() != tt.want {
				t.Errorf("learn_add as %q = %q, %v, want error %q", tt.role, written, err, tt.want)
			}
			f.checkSnapshot(t, before)
		})
	}
}

func TestLearnAddKeepsExistingProject(t *testing.T) {
	tests := []struct {
		name string
		args func(f *refreshFixture) map[string]any
		want string // a part of the error
	}{
		{"existing key", func(f *refreshFixture) map[string]any {
			return map[string]any{"repos": []string{"gamma"}, "key": f.key}
		}, "already in learn/tree.json"},
		{"repository of another project", func(*refreshFixture) map[string]any {
			return map[string]any{"repos": []string{"alpha"}, "key": "other"}
		}, `repository "alpha" is already in project`},
		{"repository of another project, default key", func(*refreshFixture) map[string]any {
			return map[string]any{"repos": []string{"gamma", "beta"}, "main": "gamma"}
		}, `repository "beta" is already in project`},
		{"no repository", func(*refreshFixture) map[string]any {
			return map[string]any{"repos": []string{}}
		}, "repos"},
		{"no repository folder", func(*refreshFixture) map[string]any {
			return map[string]any{"repos": []string{"nope"}}
		}, "not a folder under root"},
		{"two repositories, no main", func(*refreshFixture) map[string]any {
			return map[string]any{"repos": []string{"gamma", "grp/delta"}, "key": "gd"}
		}, "is not one of its repos"},
		{"unknown field", func(*refreshFixture) map[string]any {
			return map[string]any{"repos": []string{"gamma"}, "purpose": "x"}
		}, "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRefreshFixture(t)
			f.writeRepo(t, "gamma", refreshURL("gamma"))
			f.writeRepo(t, "grp/delta", refreshURL("grp/delta"))
			before := f.snapshot(t)
			written, err := f.learnAdd(t, "bigm", tt.args(f))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("learn_add = %q, %v, want an error with %q", written, err, tt.want)
			}
			f.checkSnapshot(t, before)
		})
	}

	t.Run("an add keeps the stored project", func(t *testing.T) {
		f := newRefreshFixture(t)
		f.writeRepo(t, "gamma", refreshURL("gamma"))
		writeRefreshFile(t, filepath.Join(f.ledger, "projects", f.key+".md"), "# stored\n")
		before := f.snapshot(t)
		if _, err := f.learnAdd(t, "bigm", map[string]any{"repos": []string{"gamma"}}); err != nil {
			t.Fatalf("learn_add: %v", err)
		}
		after := f.snapshot(t)
		for _, rel := range []string{f.rel(), "projects/" + f.key + ".md"} {
			p := filepath.Join(f.ledger, filepath.FromSlash(rel))
			if after[p] != before[p] {
				t.Errorf("%s changed:\nbefore %q\nafter  %q", rel, before[p], after[p])
			}
		}
	})
}
