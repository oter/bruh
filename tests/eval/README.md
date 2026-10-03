# Learner eval

The learner eval checks the `bruh:learner` agent against fixed fixtures (spec section 20). The rule of the owner says that meaning needs a model with its own eval set. This folder is that eval set.

## What the eval checks

Each case of `learner/` has fixture repositories and the expected output of the learner. The eval checks only the `LINK` and `DOC` lines of the output. The rule of spec 20 applies:

- Each expected `LINK` and `DOC` line is in the output.
- An extra `DOC` line is allowed.
- A `LINK` line that is not expected fails the case.

The driver compares whole lines. It ignores all other lines of the output.

The eval does not check the `PURPOSE` line. The owner checks the purpose at the confirm step of `/bruh:init`.

## Run

Run the driver from the root of the bruh checkout:

```bash
sh tests/eval/run.sh
```

The driver starts a real Claude Code session for each case, so it uses your Claude account and your usage limits. It loads the learner from `plugins/bruh` of the checkout with `--plugin-dir`. Thus the eval tests the code of the branch, and not the installed plugin.

For each case, the driver does these steps:

1. It copies `root/` of the case to a new temporary folder.
2. It replaces `<root>` in `input.txt` with that folder.
3. It runs `claude -p --agent bruh:learner` with that input in that folder.
4. It compares the output with `expected.txt`, and prints `PASS <case>` or `FAIL <case>` with the difference.

At the end, it prints the summary line `learner eval: <passed> of <cases>`. It exits with 1 when a case failed. An error of `claude` or a timeout is a failure of that case.

Options:

| Option | Meaning |
|---|---|
| `--dry-run` | Print the command of each case. Start no session. |
| `--compare <expected> <output>` | Compare one output file with one expected file, and exit with the result. The shell tests use it. |

The environment variable `EVAL_TIMEOUT` sets the time limit of each case in seconds. The default is 600.

## When to run

Run the eval before each release. Put the summary line `learner eval: <passed> of <cases>` into the release notes.

The eval does not run in CI, because it starts a model session. CI runs only `tests/test.sh`, which checks the dry run and the compare rule with no model call.

## Add a case

1. Make a folder `learner/<case>/`.
2. In `root/`, make two or more project folders with plain files. Do not add a `.git` folder.
3. In `input.txt`, write the prompt input of the learner (spec 8.5). Write the root as `<root>`. Use the labels of the other cases.
4. In `expected.txt`, write one line for each expected link or doc, in the form `LINK <key>: <repo>/<file>:<line>` or `DOC: <repo>/<path>`. Write no other line.
5. Make sure that each expected link has exactly one evidence line in the fixture. No other file and no other line can show that use.
6. Use neutral names only, such as `shop`, `shop-app`, `auth`, and `group-a`. Name no real host or company. Use `example.com` for a URL.
