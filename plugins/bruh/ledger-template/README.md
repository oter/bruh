# Ledger

This repository is the ledger of bruh. It describes all other repositories. Keep it private.

## Who writes it

- bigm is the only writer. It commits after each change.
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
| `projects/<project>.md` | One file for each project, made from `projects/_template.md` |

## Rules for each row

- A status (merged, deployed, live, down, out of quota) carries its source read: the command or the API call, the value it returned, and a UTC time. A status without a source read is "unverified".
- Each time comes from `date -u +%Y-%m-%dT%H:%M:%SZ` or from the bruh MCP server, never from the model.
- A dispatch is a row. A report is an update. An owner action is a row too.

## How the init skill fills this template

The init skill writes each file of this folder that does not exist yet in the ledger repository, and it never changes an existing file. In the files that it writes, it makes these changes, and no others:

1. In `mode.md`, it replaces the value of each `key: value` line with the init answer of the same key (`mode`, `p1_batch_minutes`, `p1_batch_size`, `review_round_cap`). It sets `changed` to the UTC time of the init and `reason` to `init`.
2. It replaces `priorities.md` with the default `priorities.md` of the plugin, and it writes the answer `delegated_p1_classes` into its section "Delegated P1 classes", one item for each class.
3. It adds one row to the table "Merge grants" of `grants.md`, the last table of the file, for each item of the answer `merge_grants`.
