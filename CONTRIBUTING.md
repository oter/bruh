# Contributing

Thank you for your help. Read the [specification](docs/spec.md) before you change behavior. A change of behavior also changes the specification.

## Checks to run locally

Run all checks before you open a pull request. CI runs the same checks on macOS and Linux.

```bash
# Go: the MCP server
cd plugins/bruh/mcp && test -z "$(gofmt -l .)" && go vet ./... && go test -race ./... && cd -

# Shell: the test library and the dry runs of the smoke test and the load test
sh tests/test.sh

# Shell: lint every script
docker run --rm -v "$PWD:/mnt" -w /mnt koalaman/shellcheck:stable $(git ls-files '*.sh')

# Markdown
npx -y markdownlint-cli2 '**/*.md' '#node_modules'

# Plugin and marketplace
claude plugin validate ./plugins/bruh
claude plugin validate .
```

The MCP server uses the Go standard library only. Do not add a module to `go.mod`.

Before a release, run the smoke test of [tests/smoke/README.md](tests/smoke/README.md). The load test of [tests/load/README.md](tests/load/README.md) takes one hour. Run it when you change the transport.

## Documentation language

Write all documentation in ASD-STE100 Simplified Technical English. This includes the README, the specification, `SKILL.md` files, agent files, defaults, templates, and plan files.

- Use short sentences and the active voice.
- Use one word for one meaning.
- Do not use contractions, slang, or emoji.

Code comments use plain professional English.

## Commits and pull requests

- Write the commit subject in plain English, in the imperative mood, with no prefix such as `feat:`. Example: `Add the lease guard hook`.
- Put the documentation and the code of one change in the same pull request. Do not open a documentation pull request and a separate code pull request.
- Do not commit binaries or build output.
- This repository is public. Do not commit personal names, email addresses, host names, private repository names, or machine paths such as a home folder. Use placeholders such as `owner/repo` and `example.com`.
- Give the test counts in the pull request: how many tests ran, passed, failed, and were skipped.

## Report a vulnerability

See [SECURITY.md](SECURITY.md).
