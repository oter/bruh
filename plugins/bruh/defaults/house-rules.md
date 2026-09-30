# House rules for review

The clanker gives this text to each clerk, and the clerk passes it to `/bruh:deliver` as `args.house_rules`. Every review agent, the gate check, every refuter, and every fixer gets it in its prompt. These rules come from the principles of the specification (section 2), the `deliver` workflow (section 6.3), and the field lessons of a multi-agent run.

## Scope of a review

1. Diff against the pinned base SHA: `git diff <base SHA> HEAD`. Never diff against `origin/main` or another moving ref. A moving ref shows changes of other work as findings of this task.
2. Review only the change of the task. A problem in code that the change did not touch is not a finding, unless the change makes it worse.
3. Do not flag a deliberate choice. The list of deliberate choices is in the prompt.
4. A finding has a file, a line, and a summary. Findings are deduplicated by `file:line`, so give the line where the problem is, not the start of the file.

## What counts as a finding

1. A test that fails.
2. A skipped test in a required suite. Each gate command of the task is a required suite. Report how many tests ran, passed, failed, and were skipped. An unexpected skip is a failure, not a pass.
3. A claim of the change that the code does not support: a status (merged, deployed, live, down, out of quota) without a source read, a comment or a document that says something the code does not do, a test that does not test what its name says.
4. An invariant of the code that the change breaks.
5. A rule that must never break and that exists only as a sentence in a prompt, a comment, or a note. It needs a mechanical stop: a deny rule, a hook, a sandbox rule, or a check in code.
6. A time that the model wrote. Timestamps come from code (`date -u` or a clock call), never from the model.
7. A handoff or a report that holds only file pointers instead of the items themselves.
8. A message body on a command line. Bodies go through files or the mailbox.
9. A shared resource (a test database, a staging environment, a paid API, a runner) that the change uses without a queue, a gate, or a lease.
10. An edit of a file that is not on the file list of the task, or an edit of lines of a shared instruction file that another task owns.
11. A step that costs money or cannot be undone and that the code retries automatically.
12. A write with a credential whose identity the change does not check before the first write.

## How to work

1. Write a STOP condition for each risky step: the observation that makes you stop instead of continuing. Stop at a conflict. Do not guess.
2. Record each deviation from the plan with its reason.
3. Give risky steps narrow tools. Never retry a failed paid step blindly.
4. Never move, copy, or print a credential. Never switch an account.
5. Do not post outside the project: no comments on pull requests or issues, no chat messages, no emails. Posting is clerk work.
6. Do not push. The clerk pushes.
7. Do not delete anything that is not a temporary file of this run.
8. A refusal (a permission prompt or a classifier refusal) is escalated as a question, never worked around with another form of the same command.
9. Tag a recommendation as a recommendation. Only the owner, or a role that the owner delegated, makes a ruling.

## Refuters

1. Try to refute the finding against the code. Read the lines, run the test, or run the gate command again.
2. Confirm the finding only when you can show it. When you are not sure, the finding is refuted.
3. A refuted finding stays refuted in later rounds.
