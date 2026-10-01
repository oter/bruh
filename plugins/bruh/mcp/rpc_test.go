package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

type rpcClient struct {
	in  *io.PipeWriter
	out *bufio.Scanner
}

func startRPC(t *testing.T, env Env, tools []Tool) *rpcClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := NewServer("bruh", "test", tools)
	go func() { _ = srv.Serve(env, inR, outW); outW.Close() }()
	t.Cleanup(func() { inW.Close() })
	return &rpcClient{in: inW, out: bufio.NewScanner(outR)}
}

func (c *rpcClient) send(t *testing.T, msg string) {
	t.Helper()
	if _, err := io.WriteString(c.in, msg+"\n"); err != nil {
		t.Fatal(err)
	}
}

func (c *rpcClient) next(t *testing.T) map[string]any {
	t.Helper()
	if !c.out.Scan() {
		t.Fatal("no response")
	}
	var m map[string]any
	if err := json.Unmarshal(c.out.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInitializeAndList(t *testing.T) {
	c := startRPC(t, testEnv(t, "bigm"), AllTools())
	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	init := c.next(t)
	info := init["result"].(map[string]any)["serverInfo"].(map[string]any)
	if info["name"] != "bruh" {
		t.Fatalf("init = %v", init)
	}
	c.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	list := c.next(t)
	if _, ok := list["result"].(map[string]any)["tools"].([]any); !ok {
		t.Fatalf("list = %v", list)
	}
}

func TestUnknownTool(t *testing.T) {
	c := startRPC(t, testEnv(t, "bigm"), AllTools())
	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"no_such_tool","arguments":{}}}`)
	resp := c.next(t)
	if !strings.Contains(resp["error"].(map[string]any)["message"].(string), "unknown tool") {
		t.Fatalf("resp = %v", resp)
	}
}

func TestToolErrorAndProgress(t *testing.T) {
	tools := []Tool{{
		Name:        "slow",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(c *Call, _ json.RawMessage) (any, error) {
			c.Progress(1, "working")
			return nil, io.ErrUnexpectedEOF
		},
	}}
	c := startRPC(t, testEnv(t, "bigm"), tools)
	c.send(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"slow","arguments":{},"_meta":{"progressToken":"tok"}}}`)
	progress := c.next(t)
	if progress["method"] != "notifications/progress" || progress["params"].(map[string]any)["progressToken"] != "tok" {
		t.Fatalf("progress = %v", progress)
	}
	result := c.next(t)["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("result = %v", result)
	}
}

func TestMissingRoleKeyWritesNothing(t *testing.T) {
	env := testEnv(t, "")
	anySession := []string{"bruh_info", "init_plan", "init_apply", "result_save"}
	for _, tool := range AllTools() {
		_, err := tool.Handler(&Call{Env: env}, json.RawMessage(`{}`))
		if slices.Contains(anySession, tool.Name) {
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "BRUH_ROLE_KEY is not set") {
			t.Errorf("%s: err = %v", tool.Name, err)
		}
	}
	entries, err := os.ReadDir(env.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("data folder has %d entries", len(entries))
	}
}
