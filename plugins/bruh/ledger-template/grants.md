# Grants

A grant is an answer that the owner gave in advance, for one kind of outward-facing action. Only the owner gives a grant, explicitly. bigm records it here with the words of the owner, the date, and the question ID. Each row is one line of a table. Put new rows at the end of their table. When the owner withdraws a grant, bigm deletes its row in a commit `close grant: ...`.

## Post grants

A post on a code host (a comment on a pull request or a merge request) goes out under an account of the owner. Every post is a P1 to the owner, except under a post grant. A post grant names one poster role key, one host name (for example `gitlab.com`, `gitlab.example.com`, or `github.com`), and one repository. A GitLab repository can have subgroups (`group/subgroup/repo`). `scripts/post-findings.sh` reads only this section: in a role session, it posts without an approval `ANSWER` of bigm only when a row here names `BRUH_ROLE_KEY` in the first column, the host name of the post (`--hostname`) in the second column, and the repository in the third column.

| Poster role key | Host | Repository | Conditions | Owner words | Date (UTC) | Question ID |
|---|---|---|---|---|---|---|

## Command grants

A monitor with a `command` source (spec 9.5) runs a program outside the permission rules of a session, because the poller starts it. `monitor_start` accepts such a source, and the poller polls it, only under a row of this table. The first column is a JSON array of strings, the `argv` prefix, for example `["tracker-cli", "list"]`. A source matches when its first `argv` elements are equal, one by one and byte for byte, to the prefix: `["tracker-cli", "list"]` covers `["tracker-cli", "list", "--json"]` and not `["tracker-cli", "listx"]`. A row whose first cell is not such an array never matches. When bigm deletes a row, the poller stops each monitor that it covered.

| Argv prefix (JSON array) | Conditions | Owner words | Date (UTC) | Question ID |
|---|---|---|---|---|

## Merge grants

Every merge is a P1 to the owner, except under a grant in this table. A grant names the stable merger role key `clerk-<project>-merge` and its conditions for one repository. Each merge gets a new merger session under that key. One merger acts for each repository at a time. A merge is confirmed by a read of the code host API, never by an exit code.

| Repository | Merger role key | Conditions | Owner words | Date (UTC) | Question ID |
|---|---|---|---|---|---|
