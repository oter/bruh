# Simplicity guide

Source: the idea comes from the ponytail plugin. The principles are YAGNI (<https://martinfowler.com/bliki/Yagni.html>) and KISS (<https://en.wikipedia.org/wiki/KISS_principle>). This guide is part of bruh. It works with or without ponytail installed.

The simplicity reviewer of `/bruh:deliver` and the `simplicity` lens of `/bruh:review-and-fix` and `/bruh:review-only` use this guide. An implementer can use it too.

## The ladder

Read the task and the code that the change touches first. Then go down the ladder and stop at the first step that solves the problem:

1. Is the code necessary? When no part of the task needs it, do not write it.
2. Does the codebase have it already? Use the helper, the type, or the pattern that is there.
3. Does the standard library have it? Use the standard library.
4. Does the platform have it? Use a feature of the language, the database, the browser, or the operating system.
5. Does an installed dependency have it? Use that dependency. Do not add a dependency for a few lines of code.
6. Can one line do it? Write one line.
7. Only then, write the minimum code that works.

## Rules

1. Understand the problem before you make the diff small. A small change in the wrong place is a second bug.
2. Add no abstraction that the task did not ask for: for example, an interface that only one type implements, a factory that makes only one kind of object, or an option for a constant.
3. Add no scaffolding for a later need. The later change can add it.
4. Prefer to delete code. Prefer plain code to clever code.
5. Touch as few files as possible.
6. Of two diffs that work, take the shorter one. Of two options of the same size, take the one that is correct at the edge cases.

## A finding

A finding has a file, a line, and a summary. The summary names the problem and ends with a concrete simpler replacement. Example: `src/load.go:12: a Loader interface with one implementation; call readConfig directly and delete the interface.`

Finding kinds:

- an abstraction that the task did not ask for
- code for a speculative need
- a re-implemented standard library function, or a helper that the codebase has already
- a new dependency that a few lines can replace
- scaffolding for later
- a diff that is longer than the problem needs

## What is not a finding

A simplicity finding never overrides the acceptance criteria or a deliberate choice. Do not report:

- something that the task or the acceptance criteria ask for
- a deliberate choice of the task
- input validation at a trust boundary
- error handling that prevents data loss
- a security check
- accessibility basics
- the test that the task needs
