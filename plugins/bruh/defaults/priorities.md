# Priorities

This file is data. The init skill copies it into the ledger as `priorities.md`. The owner can change it at any time. bigm gives its text to each clanker, and a clanker reads it again before it routes a question. The P-level definitions are owner decisions of 2026-09-27.

Each question has one P-level. The asker writes the P-level that it thinks is correct. The clanker decides the final P-level.

## P0

Work is blocked, and only the owner can unblock it. bigm shows a P0 at once: at the end of its next reply and through the channel (R-16: what waits on the owner comes last; owner answer Q-bruh-maksyms-macbook-pro-188, 2026-10-08). A P0 never waits for a batch.

Examples:

- Security.
- Data loss.
- Money.
- An item of the section "Never without the owner".
- A permission block.
- A classifier refusal. The P0 carries the exact command and the refusal category, and gives two options: the owner runs the command, or the owner adds a scoped allow rule.
- A session that waits on a prompt. The P0 carries the command `claude attach <id>`.
- A session that failed two times.
- A model change.

## P1

An owner decision. bigm queues P1 questions and shows them as a batch. The batch interval and the batch size are in `mode.md`.

Examples:

- An open question.
- An "X versus Y" choice.
- A premise that changed under an owner decision.
- A scope change.
- A merge, except under a merge grant in `grants.md`.
- A post of a review result on a pull request, except under a post grant in `grants.md`.
- The choice of an account after a usage limit.

A clanker answers only the P1 classes of the next section. For another P1 question, a clanker can add a recommendation with a `REC` header, but it does not answer.

## P2

The clanker answers from the project context: the code, the docs, the ADRs, and the ledger. The clanker logs each answer in its report file.

## Delegated P1 classes

A clanker can answer a P1 question of these classes, and it logs each answer in its report file. An item of the section "Never without the owner" is never a delegated class. bigm writes a class here when the owner tells it, one item for each class. With no items, a clanker answers no P1 question.

## Never without the owner

These items are a hard stop in human mode and in autonomous mode. No role acts on one of them without an answer of the owner. A merge grant or a post grant in `grants.md` is an answer that the owner gave in advance. In autonomous mode, bigm sends a P0 for these items through the channel and continues other work.

- An irreversible or outward-facing action: publish, deploy, delete, send, merge (except under a merge grant), post a review result on a pull request (except under a post grant, section 6.4). A post grant, like a merge grant, is an answer that the owner gives in advance for one repository and one role key.
- Deletes and cleanup of anything that is not a temporary file of the session.
- A model change.
- A write under the personal identity of the owner, or with a credential whose identity nobody checked. The identity goes into the project file.
- Moving or copying a credential, or switching an account. A read-only token read inside a process is allowed.
- Scope growth past the task.

## Deny rules

The items above that match an exact command pattern are deny rules in the role settings files (`defaults/role-settings.json`), not in user settings. So the manual sessions of the owner are not blocked. A deny rule matches only the command text, so it is a speed bump, not a stop. The other items stay P0.

- `Bash(docker volume rm:*)`
- `Bash(docker volume prune:*)`
- `Bash(git push --force:*)`
- `Bash(git push -f:*)`
