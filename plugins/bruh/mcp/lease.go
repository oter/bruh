package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var resourceRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type Grant struct {
	Holder  string `json:"holder"`
	Grantor string `json:"grantor"`
	Until   string `json:"until"`
}

type Resource struct {
	Capacity int      `json:"capacity"`
	Patterns []string `json:"patterns,omitempty"`
	Grants   []Grant  `json:"grants"`
}

type LeaseRequest struct {
	Resource  string `json:"resource"`
	Requester string `json:"requester"`
	Grantor   string `json:"grantor"`
	At        string `json:"at"`
}

type LeaseState struct {
	Resources map[string]*Resource `json:"resources"`
	Requests  []LeaseRequest       `json:"requests"`
}

func leaseFile(env Env) (string, error) {
	dir, err := env.Dir("leases")
	return filepath.Join(dir, "state.json"), err
}

func loadLeases(env Env) (*LeaseState, error) {
	st := &LeaseState{Resources: map[string]*Resource{}, Requests: []LeaseRequest{}}
	file, err := leaseFile(env)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, st); err != nil {
			return nil, err
		}
	}
	now := env.Stamp()
	for _, r := range st.Resources {
		r.Grants = slices.DeleteFunc(r.Grants, func(g Grant) bool { return g.Until <= now })
		if r.Grants == nil {
			r.Grants = []Grant{}
		}
	}
	return st, nil
}

// withLeases loads the state under the lock, runs fn, and saves the state when fn succeeds.
func withLeases(env Env, fn func(st *LeaseState) (any, error)) (any, error) {
	var out any
	err := env.WithLock("leases", func() error {
		st, err := loadLeases(env)
		if err != nil {
			return err
		}
		if out, err = fn(st); err != nil {
			return err
		}
		file, err := leaseFile(env)
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(st, "", "  ")
		return atomicWrite(file, data)
	})
	return out, err
}

func resourceOf(st *LeaseState, name string) (*Resource, error) {
	if _, err := checkID(name, resourceRE, "resource"); err != nil {
		return nil, err
	}
	r, ok := st.Resources[name]
	if !ok {
		return nil, fmt.Errorf("unknown resource: %s", name)
	}
	return r, nil
}

func objectSchema(props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func stringSchema() map[string]any { return map[string]any{"type": "string"} }

func leaseTools() []Tool {
	return []Tool{
		{
			Name:        "lease_define",
			Description: "Define a shared resource, its capacity, and the command prefixes that use it (stored for the record; no hook enforces them). Only bigm.",
			InputSchema: objectSchema(map[string]any{
				"resource": stringSchema(),
				"capacity": map[string]any{"type": "integer", "minimum": 1},
				"patterns": map[string]any{"type": "array", "items": stringSchema(), "description": `Exact command prefixes, for example ["psql -h test-db", "make integration"]`},
			}, "resource", "capacity"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				if me != "bigm" {
					return nil, errors.New("only bigm defines resources")
				}
				a, err := decode[struct {
					Resource string   `json:"resource"`
					Capacity int      `json:"capacity"`
					Patterns []string `json:"patterns"`
				}](raw)
				if err != nil {
					return nil, err
				}
				for _, p := range a.Patterns {
					if strings.TrimSpace(p) != p || p == "" || len(p) > 200 || strings.ContainsAny(p, "\n\r\t") {
						return nil, fmt.Errorf("invalid pattern: %q (one line, 1 to 200 characters, no white space at the start or the end)", p)
					}
				}
				if _, err := checkID(a.Resource, resourceRE, "resource"); err != nil {
					return nil, err
				}
				if a.Capacity < 1 {
					return nil, errors.New("capacity must be 1 or more")
				}
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					r := &Resource{Capacity: a.Capacity, Patterns: a.Patterns, Grants: []Grant{}}
					if old, ok := st.Resources[a.Resource]; ok {
						r.Grants = old.Grants
						if a.Patterns == nil {
							r.Patterns = old.Patterns
						}
					}
					st.Resources[a.Resource] = r
					return map[string]any{"resource": a.Resource, "capacity": r.Capacity, "patterns": r.Patterns}, nil
				})
			},
		},
		{
			Name:        "lease_request",
			Description: "Ask your grantor for a lease on a resource. A clanker asks bigm; a clerk asks its clanker.",
			InputSchema: objectSchema(map[string]any{"resource": stringSchema()}, "resource"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Resource string `json:"resource"`
				}](raw)
				if err != nil {
					return nil, err
				}
				k, _ := ParseRoleKey(me)
				grantor := k.Parent()
				if grantor == "" {
					return nil, errors.New("bigm has no grantor; bigm defines resources with lease_define")
				}
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					if _, err := resourceOf(st, a.Resource); err != nil {
						return nil, err
					}
					at := c.Env.Stamp()
					st.Requests = append(st.Requests, LeaseRequest{Resource: a.Resource, Requester: me, Grantor: grantor, At: at})
					return map[string]string{"at": at}, nil
				})
			},
		},
		{
			Name:        "lease_grant",
			Description: "Grant a lease. bigm grants to clankers; a clanker grants one sub-lease at a time to its own clerks.",
			InputSchema: objectSchema(map[string]any{"resource": stringSchema(), "to": stringSchema(), "minutes": map[string]any{"type": "number", "minimum": 1}}, "resource", "to", "minutes"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Resource string  `json:"resource"`
					To       string  `json:"to"`
					Minutes  float64 `json:"minutes"`
				}](raw)
				if err != nil {
					return nil, err
				}
				to, err := ParseRoleKey(a.To)
				if err != nil {
					return nil, err
				}
				caller, _ := ParseRoleKey(me)
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					r, err := resourceOf(st, a.Resource)
					if err != nil {
						return nil, err
					}
					until := c.Env.Now().Add(time.Duration(a.Minutes * float64(time.Minute))).UTC().Format(stampLayout)
					switch caller.Role {
					case "bigm":
						if to.Role != "clanker" {
							return nil, errors.New("bigm grants only to clankers")
						}
						active := 0
						for _, g := range r.Grants {
							if g.Grantor == "bigm" {
								active++
							}
						}
						if active >= r.Capacity {
							return nil, fmt.Errorf("resource %s is at capacity %d", a.Resource, r.Capacity)
						}
					case "clanker":
						if to.Role != "clerk" || to.Parent() != me {
							return nil, errors.New("a clanker grants only to its own clerks")
						}
						i := slices.IndexFunc(r.Grants, func(g Grant) bool { return g.Holder == me && g.Grantor == "bigm" })
						if i < 0 {
							return nil, fmt.Errorf("%s holds no grant on %s", me, a.Resource)
						}
						if slices.ContainsFunc(r.Grants, func(g Grant) bool { return g.Grantor == me }) {
							return nil, errors.New("a clanker has at most one sub-grant for each resource")
						}
						until = min(until, r.Grants[i].Until)
					default:
						return nil, fmt.Errorf("%s cannot grant leases", me)
					}
					r.Grants = append(r.Grants, Grant{Holder: a.To, Grantor: me, Until: until})
					st.Requests = slices.DeleteFunc(st.Requests, func(q LeaseRequest) bool {
						return q.Resource == a.Resource && q.Requester == a.To
					})
					return map[string]string{"resource": a.Resource, "holder": a.To, "until": until}, nil
				})
			},
		},
		{
			Name:        "lease_release",
			Description: "Release a lease. The holder or its grantor can release it. Releasing a clanker grant releases its sub-grants.",
			InputSchema: objectSchema(map[string]any{"resource": stringSchema(), "holder": stringSchema()}, "resource", "holder"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Resource string `json:"resource"`
					Holder   string `json:"holder"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkKey(a.Holder, "role key"); err != nil {
					return nil, err
				}
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					r, err := resourceOf(st, a.Resource)
					if err != nil {
						return nil, err
					}
					i := slices.IndexFunc(r.Grants, func(g Grant) bool { return g.Holder == a.Holder })
					if i < 0 {
						return map[string]int{"released": 0}, nil
					}
					if me != a.Holder && me != r.Grants[i].Grantor {
						return nil, errors.New("only the holder or its grantor can release a lease")
					}
					before := len(r.Grants)
					r.Grants = slices.DeleteFunc(r.Grants, func(g Grant) bool { return g.Holder == a.Holder || g.Grantor == a.Holder })
					return map[string]int{"released": before - len(r.Grants)}, nil
				})
			},
		},
		{
			Name:        "lease_list",
			Description: "List resources, active grants, and open requests.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				return loadLeases(c.Env)
			},
		},
	}
}
