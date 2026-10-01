# Ticket template and rules

Pass the text of this file, plus the rules of the project, to `/bruh:tickets` as `args.rules`.

One Markdown file for each ticket in `<issues>/`, with the name `<lane>-<NN>-<slug>.md`. `<lane>` groups the tickets that land in one merge request (for example `mr1`, `mr2`). `NN` is the dependency order in the lane, with two digits, blockers first. The file name without `.md` is the ticket ID of `/bruh:implement-tickets` and of `lane.sh`.

## Template: these sections, in this order

```markdown
# <lane>-<NN>: <Title>

**Kind:** test | implementation | other
**Lane:** <lane>
**Repo:** <name> (`<absolute path of the repository root>`)
**Blocked by:** comma-separated ticket IDs, or "None (can start immediately)"
**Files:**
<absolute path, one on each line, each file that the agent will create or edit>
**Parallel-safe:** yes | no
**Status:** ready-for-agent

## What to build

Plain words: what exists after this ticket that did not exist before. For a function:
the exact signature in a code block, the package path, and the behavior, with the error
cases. For a test ticket: the stub body panics (for example `panic("not implemented")`),
and the list of the cases that the test must cover.

## Acceptance criteria

- [ ] observable when you run a command or read a file
- [ ] ...
- [ ] the exact verify commands that the reviewer runs, always last

## Review guides

The rows of `<guides>/INDEX.md` whose globs match the Files of this ticket.
```

## Rules

- **Size**: function-sized. One function, one type with its methods, one configuration file, one path of an API spec, one infrastructure resource group, or one decision entry. A new agent with no memory of the design conversation completes it in one session. No vertical slices.
- **TDD pairs**: each function that is not trivial gets two tickets. First the test ticket: it writes the test against the agreed signature, and a stub in the real file whose body panics, so the package compiles and the test fails red. Second the implementation ticket, blocked by the test ticket: it replaces the stub so the test passes, without a change to the test. Trivial one-line functions, generated code, configuration, YAML, and docs are `Kind: other`, with no pair.
- **Integration tests** over the real server get their own test tickets for each endpoint, blocked by the implementation ticket of the endpoint. Tell them to use the shared test helper file, and to derive the data of each test (emails, IDs) from the name of the test, so that tests in one package cannot collide.
- **Parallel-safe: no** when Files includes the module file or its checksum file, generated code, the task runner file, CI configuration, `.gitignore`, the mock generator configuration, the root spec file, the code generator configuration, the main entry point, or any file that another ticket of the same lane also lists. Both halves of a TDD pair share a file, so both are `no`.
- **Blocking edges**: no cycles. A ticket that edits a file that another ticket creates is blocked by it. No edge that does not gate.
- **Coverage**: each sentence of the normative sections of the spec maps to at least one ticket. No item of the out-of-scope list of the spec is in a ticket.
- **No commits** in a ticket, no infrastructure apply, and nothing that touches the signing flow of the owner.
- **INDEX.md**: for each lane, a table of ID, title, kind, blocked by, parallel-safe, and files. Then the execution waves. A wave is the set of tickets that are not blocked, that are parallel-safe, and whose files do not overlap. A ticket that is not parallel-safe is a wave of one.

## The checklist of the breakdown reviewer

1. Template compliance, with the verify command last.
2. Size: split a ticket that is larger than one unit; merge a ticket that you cannot verify alone.
3. TDD pairs are there, and the signatures of each pair are identical byte for byte.
4. Blocking edges are acyclic and necessary.
5. Files are complete; parallel safety agrees with the shared-file rule; the waves agree.
6. Each edited (not created) path exists on disk now (`ls`).
7. Spec coverage, sentence by sentence; a gap is reported as `COVERAGE`.
8. No contradiction with the spec.
9. The Review guides rows agree with the map of INDEX.md.
