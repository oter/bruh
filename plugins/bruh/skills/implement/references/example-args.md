# Example args

The values below have the shape of the first full run. Change the paths and the slices. Keep the shape. `/work/app` is an example repository root.

## /bruh:tickets

```json
{
  "root": "/work/app",
  "spec": "/work/app/.scratch/auth/spec.md",
  "issues": "/work/app/.scratch/auth/issues",
  "guides": "/work/app/.scratch/auth/guides",
  "rules": "<the text of references/ticket-template.md, then the rules of the project>",
  "ground": ["/work/app/cmd/server/main.go", "/work/app/.gitlab-ci.yml"],
  "round_cap": 5
}
```

## /bruh:implement-tickets

```json
{
  "root": "/work/app",
  "spec": "/work/app/.scratch/auth/spec.md",
  "guides": "/work/app/.scratch/auth/guides",
  "issues": "/work/app/.scratch/auth/issues",
  "lane": "<plugin root>/scripts/lane.sh",
  "gates": ["test -z \"$(gofmt -l .)\"", "go vet ./...", "go build ./...", "golangci-lint run", "go mod tidy && git diff --exit-code go.mod go.sum", "go test -race -timeout 30m ./...", "task coverage-check"],
  "fix_cap": 3,
  "waves": [
    ["mr1-01-test-parse-config.md"],
    ["mr1-02-impl-parse-config.md"],
    ["mr1-03-server-struct.md", "mr1-04-env-example.md"],
    ["mr1-05-integration-get-items.md", "mr1-06-integration-post-items.md", "mr1-07-integration-delete-item.md"]
  ]
}
```

In a skill, `<plugin root>` is `${CLAUDE_PLUGIN_ROOT}`. In a role session, the clerk gets it from `bruh_info`. Build `waves` from `issues/INDEX.md` with a short script. After a pause, pass only the waves whose tickets are not `Status: done`. Relaunch as a new run; do not resume.

## /bruh:review-and-fix and /bruh:review-only lenses

The first run used ten lenses for a diff of 21,000 lines, in two file slices: A (entities, storage, the email client) and B (controllers, the server, handlers, the main entry point). No reviewer got the full diff.

| key | files | guides and focus |
|---|---|---|
| `modern-go-a`, `modern-go-b` | Go files that are not tests, in slice A and in slice B | Modern Go guidelines: each idiom up to the Go version of the module; cite the guideline ID |
| `google-style-a`, `google-style-b` | Go files that are not tests, in slice A and in slice B | Google Go style guide: names, error strings, doc comments, wrapping, context first |
| `tests-unit` | `_test.go` files outside the integration folder | Go wiki TestComments; mocks so loose that they prove nothing; dependence on the wall clock; expensive hashes in loops |
| `tests-integration` | the integration `_test.go` files | TestComments and the testcontainers guides; order dependence in a shared emulator; duplicate helpers; a black-box test for each route and status of the spec |
| `spec-and-config` | the OpenAPI file, the database indexes and rules, the env example, the configuration loader | Spectral OpenAPI rules; an index for each composite query; expiry fields typed and declared once; env names that agree with the spec and the main entry point |
| `security` | the auth entities, controllers, stores, middleware, and handlers | OWASP cheat sheets and ASVS chapters: token entropy, hash-only storage, constant-time comparisons, atomic counters, account enumeration (timing too), no secrets in logs, invalidation on logout and password change, single use under concurrency, a security scheme on each secured route |
| `correctness-a` | the controllers, handlers, server, and main entry point | trace each request to its response: status and envelope against the spec, the map from sentinel errors to statuses, context copies, nil dereferences, time handling, wiring; each finding needs an input and its wrong output |
| `correctness-b` | the storage and entities, against the index file | transactions around reservations and counters, expiry fields in each write, deterministic and random IDs, not-found sentinels that do not leak driver codes |

A lens in `args.lenses`:

```json
{"key": "security", "prompt": "Review the auth entities, controllers, stores, middleware, and handlers with guides/owasp-authentication.md, guides/owasp-session.md, and guides/asvs-v6-v7.md. Check token entropy, hash-only storage, constant-time comparisons, atomic counters, and account enumeration, timing too."}
```

`deliberate`: `["fixed session lifetime", "no password complexity rules", "72-byte password cap", "6-digit one-time code with 3 attempts", "one generic 401 message"]`.

The other keys of `/bruh:review-and-fix`:

```json
{
  "root": "/work/app",
  "base": "<the pinned base SHA, 40 hex>",
  "head": "<git rev-parse HEAD of root, 40 hex>",
  "spec": "/work/app/.scratch/auth/spec.md",
  "guides": "/work/app/.scratch/auth/guides",
  "gates": ["go vet ./...", "go test -race -timeout 30m ./..."],
  "house_rules": "<the text of house-rules.md>",
  "round_cap": 2
}
```

For `/bruh:review-only`, `root` is a detached worktree at the head of the pull request, `base` is its base SHA, and `head` is its head SHA. It has no `gates` and no `round_cap`.

The outcome of the first run: 73 raw findings, 61 unique, 49 confirmed (6 bugs, 11 security, 21 guideline, 11 nits), six fix batches, and one gate failure (a load-dependent emulator lock timeout).
