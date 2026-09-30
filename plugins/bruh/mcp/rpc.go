package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"sync"
)

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(c *Call, args json.RawMessage) (any, error)
}

// Call is the context of one tool call.
type Call struct {
	Env      Env
	progress func(n int, msg string)
}

// Progress sends a progress notification when the request asked for it.
func (c *Call) Progress(n int, msg string) {
	if c.progress != nil {
		c.progress(n, msg)
	}
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}

type Server struct {
	name, version string
	tools         []Tool
	byName        map[string]Tool
	mu            sync.Mutex
	enc           *json.Encoder
}

func NewServer(name, version string, tools []Tool) *Server {
	s := &Server{name: name, version: version, tools: tools, byName: map[string]Tool{}}
	for _, t := range tools {
		s.byName[t.Name] = t
	}
	return s
}

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (s *Server) send(msg any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(msg)
}

func (s *Server) result(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) fail(id json.RawMessage, code int, msg string) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func textResult(text string, isError bool) map[string]any {
	r := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	if isError {
		r["isError"] = true
	}
	return r
}

// Serve reads one JSON-RPC message for each line and answers each request in its own goroutine.
func (s *Server) Serve(env Env, in io.Reader, out io.Writer) error {
	s.enc = json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var wg sync.WaitGroup
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.fail(json.RawMessage("null"), -32700, "parse error")
			continue
		}
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue // a notification from the client
		}
		wg.Go(func() { s.handle(env, req) })
	}
	wg.Wait()
	return sc.Err()
}

func (s *Server) handle(env Env, req request) {
	switch req.Method {
	case "initialize":
		p, _ := decode[struct {
			ProtocolVersion string `json:"protocolVersion"`
		}](req.Params)
		s.result(req.ID, map[string]any{
			"protocolVersion": cmp.Or(p.ProtocolVersion, "2025-06-18"),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		})
	case "ping":
		s.result(req.ID, map[string]any{})
	case "tools/list":
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema})
		}
		s.result(req.ID, map[string]any{"tools": list})
	case "tools/call":
		p, err := decode[struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Meta      struct {
				ProgressToken any `json:"progressToken"`
			} `json:"_meta"`
		}](req.Params)
		if err != nil {
			s.fail(req.ID, -32602, err.Error())
			return
		}
		tool, ok := s.byName[p.Name]
		if !ok {
			s.fail(req.ID, -32602, "unknown tool: "+p.Name)
			return
		}
		c := &Call{Env: env}
		if tok := p.Meta.ProgressToken; tok != nil {
			c.progress = func(n int, msg string) {
				s.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress",
					"params": map[string]any{"progressToken": tok, "progress": n, "message": msg}})
			}
		}
		out, err := tool.Handler(c, p.Arguments)
		if err != nil {
			s.result(req.ID, textResult(err.Error(), true))
			return
		}
		if text, ok := out.(string); ok {
			s.result(req.ID, textResult(text, false))
			return
		}
		data, err := json.Marshal(out)
		if err != nil {
			s.result(req.ID, textResult(err.Error(), true))
			return
		}
		s.result(req.ID, textResult(string(data), false))
	default:
		s.fail(req.ID, -32601, "method not found: "+req.Method)
	}
}
