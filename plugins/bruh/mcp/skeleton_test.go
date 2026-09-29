package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	if m.Name != "bruh" || len(m.Plugins) != 1 || m.Plugins[0].Name != "bruh" || m.Plugins[0].Source != "./plugins/bruh" {
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

func TestLicenseIsApache(t *testing.T) {
	data, err := os.ReadFile("../../../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`Apache License\s+Version 2\.0, January 2004`).Match(data) {
		t.Fatal("LICENSE is not Apache-2.0")
	}
}
