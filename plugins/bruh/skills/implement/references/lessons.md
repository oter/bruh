# Lessons from the field runs

The lessons come from the field runs of the owner: large runs with many tickets and several merge requests, and smaller runs. This file records what went wrong and what the workflows and scripts now do about it. The examples are invented; the technical lessons are real.

## Workflow mechanics

- **A resume replays stale merge steps.** `resumeFromRunId` returns the cached result of each unchanged agent. A merge agent whose lanes are already applied and cleaned is not cached, because its prompt contains the list of the tickets that passed, and that list changes. It runs again against lane folders that do not exist, and it fails, and the run stops there. After a pause, relaunch as a new run with only the remaining waves in `args`.
- **The prompts of the implementers change with the lane path**, so a wave that ran in part has no cache to recover. `lane.sh start` keeps a lane of the same repository, commit, and run with its work, and makes the lane again for another one, so a stale lane of another feature never lands in the tree.
- **Spend limits stop the reviewers first**, when the reviewer model is the expensive one. Keep review prompts short and at low effort. The fix cap of three stops a bad ticket before it uses ten rounds.
- **The deduplication key `file:line` is sufficient.** Two reviewers that flag the same line give one finding with both problems appended.
- **Refuters remove about one finding in five**, mostly style rules that did not apply and deliberate spec choices reported as bugs. Tell the refuters what the spec chose on purpose (for example a fixed cache lifetime, one retry for each call to an external service, no pagination on an admin list), so that they do not argue it again.
- **Fix in batches by area, one batch at a time.** Parallel fixers in one shared tree collide. Batches of five to twenty-five findings worked. The gate runs after all of them.
- **A dead agent is not a verdict.** An agent that returns nothing (for example at a usage limit) stops the run with a `FAILED:` line. A dead refuter never counts as a refutation, and a run with a dead agent never ends `done`.
- **The Workflow tool refuses a `scriptPath` outside the working folder.** The workflows of this plugin run by name (`/bruh:<name>`), so this does not apply to them.
- **A paused run resumes cleanly before any merge step.** Stop the task, and relaunch with `resumeFromRunId`. The journal replays the finished agents.

## Git and lanes

- **`git apply --3way` needs a clean index.** The shared tree is dirty by design, so `lane.sh` uses a plain `git apply`.
- **The automatic merge of a rebase drops things silently.** In files that both sides edited, a rebase lost imports, helper functions, and the callers of a renamed helper. Run the vet command (which compiles the tests) after each rebase, not only the build.
- **Generate generated code again; do not resolve it.** Take the merged spec and run the generator.
- **Parallel test writers collide on package-level names.** In one wave, several files declared the same constants twice. The gate finds it in seconds, and the fix is to delete the copies. One shared test helper file removes the cause.
- **Worktrees for other repositories** keep the uncommitted work of the owner safe. Create them with absolute paths, or they land inside the repository.
- **A lane needs its own index.** A linked worktree has a `.git` pointer file, and a copied pointer shares the real index. Then the deleted files of a ticket never land, and the index of the shared worktree comes back dirty. `lane.sh` copies with `--exclude /.git`, runs `git init` in the copy with the objects of ROOT as alternates and the HEAD of ROOT, and diffs the staged index against the recorded baseline tree.
- **An exclude that is not anchored removes too much.** An exclude `.bin/` also drops `node_modules/.bin`, and then the tools of a Node lane (`tsc`, `eslint`, `prettier`) are missing. `lane.sh` anchors each path of `EXCLUDES` at the top of the tree.
- **A Node lane copies `node_modules`.** A copy of several hundred megabytes takes some seconds for each lane. That is acceptable for a few lanes. Do not exclude it.

## Tests and CI

- **The default Go test timeout of ten minutes can be too short** for an integration suite with an expensive password hash under the race detector. One hash can take seconds on a shared runner under the race detector, and a test that calls it in a loop multiplies that. Set a longer `-timeout` in the task runner, and make the hash cost a variable that the test packages set to the minimum cost in `TestMain`. Then the full suite runs in seconds, not minutes.
- **Compute a dummy hash lazily**, so that the timing equalization uses the cost that is in effect. A literal hash at the production cost undoes the test setting.
- **Use `-coverpkg=<module path>/...`, never `./...`.** In CI, the module cache can be under the project folder, and `./...` adds each dependency to the denominator. Then the coverage number falls to a small part of its true value.
- **Remove generated code from the coverage profile** (for example `*.gen.go` and `/mocks/`), or you measure the output of the generator.
- **A silent emulator skip measures nothing.** A `TestMain` that returns 0 when Docker is not there hides a full package in CI. Fail when `CI` is set.
- **Emulator transactions time out under load.** Many goroutines on one document across several parallel emulator containers gave `Aborted` after the retry budget of the client. Tests that race callers retry on `Aborted`, and reset each per-run result at the top of the transaction function, because the function runs again.
- **A CI component include can reset your image.** A lint component that includes the base Go component again with the default inputs wins as the last include. Set `image:` at job level. Better: one shared builder image for all Go jobs.
- **A shared builder image in a private registry needs access.** The consuming group must be on the job-token allowlist of the source project, or each job gets 403 on the pull.
- **BuildKit attestations can break registry pushes.** Since BuildKit 0.32, some registries refuse the attestation manifests: use `--provenance=false --sbom=false`, and annotate `manifest:`, not `index:` (a single-platform build has no index when the attestations are off).
- **The gate text must match the stack.** For a Node repository, the gates are the commands of the repository (for example `npm run verify`, and Playwright with a throwaway config). "The race detector and Docker" in a gate prompt sends a low-effort agent to look for things that do not exist. The workflows of this plugin take the gates as `args.gates`.
- **Check a tool before a prompt names it.** Do not put a version manager command into a prompt unless the tool exists. If the tool on PATH already has the right version, say that.
- **A snapshot update changes only the snapshots over budget.** When many of the listed goldens do not change, that is correct, and no ticket is missing.

## Design problems that the review of one ticket cannot see

The multi-lens review found these only because each reviewer saw a full layer:

- A counter read in one store call and written in another (for example an attempt counter): lost updates under concurrency. Each became one transactional store method that takes a mutate function.
- An expiry field of the database that was also the business expiry: the database could delete a record that was still in use. A separate purge field fixed it.
- Placeholder configuration values that nothing checked: the start-up now refuses them.
- The order of side effects: a notification went out before the record that it describes was durable. Now it is the last step and best effort.
- Timing that tells an attacker which records exist: early exits skipped an expensive comparison.

Plan one such pass for each merge request of this size.

## Posts on the pull request

- **glab rejects bracketed field names in a JSON body.** `-f 'position[new_line]=42'` is an error, not a nested field. `post-findings.sh` builds each payload whole with `jq` and sends it with `--input`. The text of a finding never goes through the shell.
- **A rejected inline position becomes one general comment.** GitLab answers 400 and GitHub answers 422 when the position is not in the diff. The script then posts the same body as a general comment that names `file:line`.
- **A workflow cannot write files, and an agent in a workflow must not post.** An agent that posted from inside a workflow refused twice, because it saw the latest chat message of the owner. The workflows now return the result, and a session posts it with `post-findings.sh` after the yes of the owner or under a post grant.

## The house rules are review lenses

A clean review passed one literal value pasted into several resources, and copies of one resource. Since then, each review carries the house rules in its lens prompts, not only in the guides. Examples of such rules:

- No hard-coded values: a value lives in one place (a local, a variable, or an attribute of another resource). A literal repeated across resources, or a pasted resource where a loop fits, is a finding.
- The languages that the house rules allow for each part of the repository.
- Images at the current stable tag, read from the registry. New databases on the current major version.
- Comments only for what the code cannot say.

## Rule zero held

In a small run, the orchestrator wrote only the spec, the rules, a lane wrapper, and edits of the guide index. Each edit of the repository came from an implementer agent and passed a reviewer with the guides. A guide can also change a spec detail: in one run, the documentation of a UI library said that a link must not be rendered as a button, and the spec changed before the tickets were written. Read the guides before you freeze the spec.
