# Example args

The values below are invented examples with the shape of a large run. Change the paths and the slices. Keep the shape. `/work/app` is an example repository root.

## /bruh:tickets

```json
{
  "root": "/work/app",
  "spec": "/work/app/.scratch/feature/spec.md",
  "issues": "/work/app/.scratch/feature/issues",
  "guides": "/work/app/.scratch/feature/guides",
  "rules": "<the text of references/ticket-template.md, then the rules of the project>",
  "ground": ["/work/app/cmd/server/main.go", "/work/app/.gitlab-ci.yml"],
  "round_cap": 5
}
```

## /bruh:implement-tickets

```json
{
  "root": "/work/app",
  "spec": "/work/app/.scratch/feature/spec.md",
  "guides": "/work/app/.scratch/feature/guides",
  "issues": "/work/app/.scratch/feature/issues",
  "lane": "<plugin root>/scripts/lane.sh",
  "gates": ["test -z \"$(gofmt -l .)\"", "go vet ./...", "go build ./...", "golangci-lint run", "go mod tidy && git diff --exit-code go.mod go.sum", "go test -race -timeout 30m ./...", "task coverage-check"],
  "test_gates": ["go test -race -timeout 30m ./..."],
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

A large diff (about 20,000 lines) can use ten lenses in two file slices, plus the two `simplicity` lenses: A (entities, storage, the clients of external services) and B (controllers, the server, handlers, the main entry point). No reviewer gets the full diff.

| key | files | guides and focus |
|---|---|---|
| `modern-go-a`, `modern-go-b` | Go files that are not tests, in slice A and in slice B | Modern Go guidelines: each idiom up to the Go version of the module; cite the guideline ID |
| `google-style-a`, `google-style-b` | Go files that are not tests, in slice A and in slice B | Google Go style guide: names, error strings, doc comments, wrapping, context first |
| `tests-unit` | `_test.go` files outside the integration folder | Go wiki TestComments; mocks so loose that they prove nothing; dependence on the wall clock; expensive hashes in loops |
| `tests-integration` | the integration `_test.go` files | TestComments and the testcontainers guides; order dependence in a shared emulator; duplicate helpers; a black-box test for each route and status of the spec |
| `spec-and-config` | the OpenAPI file, the database indexes and rules, the env example, the configuration loader | Spectral OpenAPI rules; an index for each composite query; env names that agree with the spec and the main entry point |
| `security` | the code that handles identities, sessions, and secrets | OWASP cheat sheets and ASVS chapters that match the code; no secrets in logs; a security scheme on each secured route; each finding cites the requirement |
| `correctness-a` | the controllers, handlers, server, and main entry point | trace each request to its response: status and envelope against the spec, the map from sentinel errors to statuses, context copies, nil dereferences, time handling, wiring; each finding needs an input and its wrong output |
| `correctness-b` | the storage and entities, against the index file | transactions around read-modify-write steps, deterministic and random IDs, not-found sentinels that do not leak driver codes |
| `simplicity` | the files of the diff; for a large diff, one lens for each slice (`simplicity-a`, `simplicity-b`) | `references/simplicity.md`: the ladder and the rules; each finding names a concrete simpler replacement; what the task or a deliberate choice asks for is not a finding |

A lens in `args.lenses`:

```json
{"key": "security", "prompt": "Review the files under internal/session/ and internal/secrets/ with guides/owasp-session.md and guides/asvs.md. Cite the requirement for each finding."}
```

`deliberate`: `["a fixed cache lifetime of 60 seconds", "one retry for each call to an external service", "no pagination on the admin list"]`.

The other keys of `/bruh:review-and-fix`:

```json
{
  "root": "/work/app",
  "base": "<the pinned base SHA, 40 hex>",
  "head": "<git rev-parse HEAD of root, 40 hex>",
  "spec": "/work/app/.scratch/feature/spec.md",
  "guides": "/work/app/.scratch/feature/guides",
  "gates": ["go vet ./...", "go test -race -timeout 30m ./..."],
  "test_gates": ["go test -race -timeout 30m ./..."],
  "deadline_seconds": 3600,
  "house_rules": "<the text of house-rules.md>",
  "round_cap": 2
}
```

For `/bruh:review-only`, `root` is a detached worktree at the head of the pull request, `base` is its base SHA, and `head` is its head SHA. It has no `gates` and no `round_cap`.

Without `head`, the workflow takes HEAD of `root` and checks that the tree is clean. Add `answers` only on a relaunch after `status: question`.
