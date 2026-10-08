package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func readJSON(t *testing.T, rel string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
}

func TestMarketplaceListsBruh(t *testing.T) {
	var m struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../../../.claude-plugin/marketplace.json", &m)
	if m.Name != "oter" || len(m.Plugins) != 1 || m.Plugins[0].Name != "bruh" || m.Plugins[0].Source != "./plugins/bruh" {
		t.Fatalf("marketplace = %+v", m)
	}
}

func TestPluginManifest(t *testing.T) {
	var p struct {
		Name    string `json:"name"`
		License string `json:"license"`
	}
	readJSON(t, "../.claude-plugin/plugin.json", &p)
	if p.Name != "bruh" || p.License != "Apache-2.0" {
		t.Fatalf("plugin = %+v", p)
	}
}

func TestVersionsMatch(t *testing.T) {
	var p struct {
		Version string `json:"version"`
	}
	readJSON(t, "../.claude-plugin/plugin.json", &p)
	versions := []string{p.Version}
	for file, pattern := range map[string]string{
		"main.go":                     `NewServer\("bruh", "([^"]+)"`,
		"../channels/slack/server.go": `"bruh-slack", "version": "([^"]+)"`,
	} {
		data, err := os.ReadFile(filepath.FromSlash(file))
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(pattern).FindAllSubmatch(data, -1)
		if len(m) != 1 {
			t.Fatalf("%s: %d matches of %s, want 1", file, len(m), pattern)
		}
		versions = append(versions, string(m[0][1]))
	}
	if versions[0] == "" || versions[1] != versions[0] || versions[2] != versions[0] {
		t.Fatalf("versions differ: %v", versions)
	}
}

func TestMCPConfig(t *testing.T) {
	var c struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Timeout int               `json:"timeout"`
		} `json:"mcpServers"`
	}
	readJSON(t, "../.mcp.json", &c)
	s, ok := c.MCPServers["bruh"]
	if !ok {
		t.Fatal("no bruh server")
	}
	if s.Command != "go" || !slices.Equal(s.Args, []string{"run", "-C", "${CLAUDE_PLUGIN_ROOT}/mcp", "."}) {
		t.Fatalf("server = %+v", s)
	}
	if s.Env["BRUH_DATA"] != "${CLAUDE_PLUGIN_DATA}" || s.Env["BRUH_PLUGIN_ROOT"] != "${CLAUDE_PLUGIN_ROOT}" || s.Env["GOTOOLCHAIN"] != "local" {
		t.Fatalf("env = %v", s.Env)
	}
	if s.Timeout != 86400000 {
		t.Fatalf("timeout = %d", s.Timeout)
	}
}

func TestUserConfig(t *testing.T) {
	var p struct {
		UserConfig map[string]map[string]any `json:"userConfig"`
	}
	readJSON(t, "../.claude-plugin/plugin.json", &p)
	want := map[string]string{"user_name": "string", "handoff_percent": "number", "max_busy_clerks": "number"}
	for k, typ := range want {
		o := p.UserConfig[k]
		if o["type"] != typ || o["title"] == nil || o["description"] == nil {
			t.Errorf("%s = %v", k, o)
		}
	}
	if p.UserConfig["handoff_percent"]["default"] != 50.0 || p.UserConfig["max_busy_clerks"]["default"] != 8.0 {
		t.Fatalf("defaults = %v", p.UserConfig)
	}
	keys := slices.Sorted(maps.Keys(p.UserConfig))
	wantKeys := []string{"board_design", "handoff_percent", "max_busy_clerks", "orca_local", "slack_bot_token", "slack_channel_id", "slack_owner_user_id", "user_name"}
	if !slices.Equal(keys, wantKeys) {
		t.Errorf("userConfig keys = %v, want %v", keys, wantKeys)
	}
	for k, o := range p.UserConfig {
		if title, _ := o["title"].(string); !strings.HasPrefix(title, "bruh: ") {
			t.Errorf("userConfig[%q].title = %q, want prefix %q", k, title, "bruh: ")
		}
	}
}

func TestInitSkill(t *testing.T) {
	data, err := os.ReadFile("../skills/init/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "---\nname: init\ndescription: ") || !strings.Contains(s, "\ndisable-model-invocation: true\n---\n") {
		t.Fatal("bad frontmatter")
	}
	for _, w := range []string{
		"bruh_info", "learn_scan", "bruh:learner", "PURPOSE:", "LINK ", "DOC:", "HOST ", "at most 8",
		"init_plan", "init_apply", "AskUserQuestion", "(current)", "change projects", "change settings",
		"learn again", "split", "join", "accept all", "one by one", "show all", "ledger_table", "outside_diff",
		"fills", "host_aliases", "now", "at the first work", "/exit", "--answers", "BRUH_INIT_", "--setting-sources user",
	} {
		if !strings.Contains(s, w) {
			t.Errorf("SKILL.md has no %q", w)
		}
	}
	for _, w := range []string{"pluginConfigs", "LEARN ", "relearn", "gates", "learn_set"} {
		if strings.Contains(s, w) {
			t.Errorf("SKILL.md has %q, want none", w)
		}
	}
}

func TestSlackChannelWiring(t *testing.T) {
	var c struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	readJSON(t, "../.mcp.json", &c)
	s := c.MCPServers["slack"]
	if s.Command != "go" || !slices.Equal(s.Args, []string{"run", "-C", "${CLAUDE_PLUGIN_ROOT}/channels/slack", "."}) ||
		s.Env["SLACK_BOT_TOKEN"] != "${user_config.slack_bot_token}" || s.Env["GOTOOLCHAIN"] != "local" || s.Env["BRUH_DATA"] != "${CLAUDE_PLUGIN_DATA}" {
		t.Fatalf("slack server = %+v", s)
	}
	var p struct {
		UserConfig map[string]map[string]any `json:"userConfig"`
		Channels   []map[string]any          `json:"channels"`
	}
	readJSON(t, "../.claude-plugin/plugin.json", &p)
	if len(p.Channels) != 1 || p.Channels[0]["server"] != "slack" || p.UserConfig["slack_bot_token"]["sensitive"] != true {
		t.Fatalf("plugin = %+v", p)
	}
	if _, err := os.Stat("../channels/slack/go.mod"); err != nil {
		t.Fatal(err)
	}
}
