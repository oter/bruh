# Bug reports of bruh

Follow these steps when you talk to the owner and you find a defect of bruh itself, or you get a notice `DONE: bruh defect: <subject>`. The sessions that talk to the owner are bigm and the session of a bruh skill that the owner runs (`/bruh:init`, or `/bruh:implement` in a manual session). A clanker or a clerk sends a notice to its parent, as its agent text says, and files nothing.

## What counts

A defect of bruh is a result of the plugin that breaks the specification, the decision log, its own text, or an answer of the owner. The plugin is its MCP tools, skills, agent texts, scripts, workflows, hooks, and default files. A defect in a project of the owner is not a defect of bruh.

## Steps

1. Read the value `repository` of `<plugin_root>/.claude-plugin/plugin.json`. The report goes to this repository.
2. Search its open and closed issues for the same defect: `gh issue list --repo <repository> --state all --search '<words>'`. For a duplicate, tell the owner its link, and stop.
3. Write the draft with the fields of the bug form of the repository:
   - the title;
   - the Claude Code version (`claude --version`), the bruh version (`version` of `bruh_info`), the Go version (`go version`), and the operating system;
   - the role where the defect occurs: bigm, clanker, clerk, workflow, init skill, hook or status line tap, MCP server, or other;
   - the steps to reproduce, the expected result with the section of the specification when you know it, the actual result, and the evidence.

   Add the cause with `file:line` when you know it, the options when the fix needs a decision of the owner, and a workaround.
4. Replace each private name in the draft with a placeholder, for example `owner/repo`: a token, a personal name, a host name, a private repository, a client, a local path, or an account. The repository is public.
5. When the defect is a vulnerability (it gives an attacker access, a credential, or an action without the owner), make a private report. Give the owner the draft and the link `<repository>/security/advisories/new` of `SECURITY.md`. The owner files it. Stop.
6. Offer the owner the bug report: show the final draft, and ask one question with the answers yes and no. The yes covers the draft that you showed. A change of the draft needs a new yes.
7. After a yes, file the draft as you showed it. The body goes through standard input, so you write no file. Its last line is the post marker of spec 9.2: `<!-- bruh:<role key> -->`, or `<!-- bruh:owner -->` in a session with no role key. In the title, write each `'` as `'\''`.

   ```bash
   gh issue create --repo <repository> --title '<title>' --label bug --body-file - <<'EOF'
   <the draft>

   <!-- bruh:<role key> -->
   EOF
   ```

8. Tell the owner the link of the issue.
