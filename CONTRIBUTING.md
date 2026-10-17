# Contributing

Thank you for your help. Read the [specification](docs/spec.md) before you change behavior. A change of behavior also changes the specification.

## Checks to run locally

Run all checks before you open a pull request. CI runs shellcheck, Markdown lint, the link check, and `claude plugin validate` on Linux. CodeQL (the GitHub default setup) scans the JavaScript and workflow code of each push and pull request. The repository has no tests (owner decision 2026-10-17).

```bash
# Shell: lint every script
docker run --rm -v "$PWD:/mnt" -w /mnt koalaman/shellcheck:stable $(git ls-files '*.sh')

# Markdown
npx -y markdownlint-cli2 '**/*.md' '#node_modules'

# Plugin and marketplace
claude plugin validate ./plugins/bruh
claude plugin validate .
```

## Documentation language

Write all documentation in ASD-STE100 Simplified Technical English. This includes the README, the specification, `SKILL.md` files, defaults, templates, and plan files.

- Use short sentences and the active voice.
- Use one word for one meaning.
- Do not use contractions, slang, or emoji.

Code comments use plain professional English.

## Commits and pull requests

- Write the commit subject in plain English, in the imperative mood, with no prefix such as `feat:`. Example: `Add the simplicity lens`.
- Put the documentation and the code of one change in the same pull request. Do not open a documentation pull request and a separate code pull request.
- Do not commit binaries or build output.
- This repository is public. Do not commit personal names, email addresses, host names, private repository names, or machine paths such as a home folder. Use placeholders such as `owner/repo` and `example.com`.

## Report a vulnerability

See [SECURITY.md](SECURITY.md).
