# Ledger

This repository is the ledger of bruh. It describes all other repositories. Keep it private.

## Who writes it

- bigm writes the Markdown files. It commits after each change.
- Plugin code writes the files of `learn/`: `init_apply` at init, and `learn_refresh` at each sweep of bigm.
- The init skill writes `.claude/settings.json`.
- bigm is the only role that commits. It also commits the files of `learn/` and `.claude/settings.json`.
- The ledger clerk `clerk-ledger` pushes after each commit of bigm.
- No other role edits a file of this repository. A clanker and a clerk send reports through the bruh MCP server, and bigm moves the facts into these files.

## Layout

| File | Contents |
|---|---|
| `README.md` | This file |
| `mode.md` | `human` or `autonomous`, the date and the reason of the last change, and the runtime settings |
| `priorities.md` | P-level definitions, delegated P1 classes, and the never-without-the-owner list |
| `rules.md` | Standing owner rules, word for word, with dates and sources |
| `grants.md` | Post grants and merge grants |
| `questions.md` | Open P1 questions, oldest first, with age and what they block |
| `owed.md` | Items owed to the owner, and asks of the owner |
| `leases.md` | The lease table of the clankers |
| `projects/<key>.md` | One file for each selected project, made from `projects/_template.md` |
| `learn/tree.json` | The hierarchy of the projects and the scan settings. Plugin code writes it. |
| `learn/projects/<key>.json` | The index of a project: the purpose, the repositories, the links to other projects, and the doc pointers. Plugin code writes it. |
| `.claude/settings.json` | The start settings of bigm: the agent `bruh:bigm`, the role key, the env values, and the deny rules. The init skill writes it. |

## Rules for each row

- A status (merged, deployed, live, down, out of quota) carries its source read: the command or the API call, the value it returned, and a UTC time. A status without a source read is "unverified".
- Each time comes from `date -u +%Y-%m-%dT%H:%M:%SZ` or from the bruh MCP server, never from the model.
- A dispatch is a row. A report is an update. An owner action is a row too.

## Current state only

Each Markdown file of this ledger shows only open or live items. Git is the history.

- When an item closes, bigm deletes its row in the commit that closes it. The subject of that commit is `close <kind>: <subject>`. So `git log --grep` finds each closed item.
- `<kind>` is `task`, `merge`, `live`, `question`, `owed`, `decision`, `lease`, `waiting`, `session`, `rule`, or `grant`.
- These items close: a task that is done, a merge or a deployment that bigm showed in a reply to a message of the owner, a question that has an answer, an item of `owed.md` that the owner got, a decision that a newer decision replaces, a lease that ended, an item of "Waiting on others" that arrived, and the session of a retired role.

## How init writes this ledger

Each change of init is in the diff that init shows before it applies.

### First run

The init skill writes each file of this folder that does not exist yet in the ledger, from this template. It makes these changes, and no others:

1. In `mode.md`, it replaces the value of each `key: value` line with the init answer of the same key (`mode`, `p1_batch_minutes`, `p1_batch_size`, `review_round_cap`). It sets `changed` to the UTC time of the init and `reason` to `init`.
2. It replaces `priorities.md` with the default `priorities.md` of the plugin.

### Later run

For each file of this folder that exists already in the ledger, the init skill writes this text:

- `README.md` and `projects/_template.md`: the template. These files hold no ledger data.
- `priorities.md`: the existing file. A placeholder file (a file with no line `## Never without the owner`) gets the default `priorities.md` of the plugin.
- `rules.md`: the template text before the line `## Rules`, then the existing file from its line `## Rules` to the end.
- Each other file (`mode.md`, `questions.md`, `owed.md`, `leases.md`, `grants.md`): the template text. Each `key: value` line gets the value of the same key in the existing file. A key that the existing file does not have keeps the template value. After each table of the template (found by its header line) come the data rows of the table with the same header line in the existing file. A table of the existing file whose header line is not in the template goes at the end, unchanged.
- Init does not change an existing `projects/<key>.md` file.

### Index and project files

When the init answers have `projects`, the init skill does these steps:

1. It writes `learn/tree.json`, and `learn/projects/<key>.json` for each selected project. It keeps the stored JSON of a project that the answers do not change.
2. It writes `projects/<key>.md` from `projects/_template.md` for each selected project that has no project file.
3. For each removed project, it deletes `learn/projects/<key>.json`. It deletes `projects/<key>.md` only when no table of the file has a data row. Else it keeps the file and lists it in `kept`.

When the answers have no `projects`, init changes no file of `learn/` and no project file. The answer `projects: []` removes each project.

### Start settings of bigm

The init skill also writes `.claude/settings.json`, which is not a file of this folder. When that file exists, init changes only the start settings of bigm in it, and keeps each other key.
