# Grants

A grant is an answer that the owner gave in advance, for one kind of outward-facing action. Only the owner gives a grant, explicitly. bigm records it here with the words of the owner, the date, and the question ID. Each row is one line of a table. Put new rows at the end of their table.

## Post grants

A post on a code host (a comment on a pull request or a merge request) goes out under an account of the owner. Every post is a P1 to the owner, except under a post grant. A post grant names one poster role key, one host (`gitlab` or `github`), and one repository. A clerk reads only this section: in a role session, it posts without an approval `ANSWER` of bigm only when a row here names `BRUH_ROLE_KEY` in the first column, the host in the second column, and the repository in the third column.

| Poster role key | Host | Repository | Conditions | Owner words | Date (UTC) | Question ID |
|---|---|---|---|---|---|---|
| clerk-shop-post | gitlab | group/shop | review findings only | "post the findings" | 2026-09-29T09:00:00Z | Q-3 |

## Merge grants

Every merge is a P1 to the owner, except under a grant in this table. A grant names the stable merger role key `clerk-<project>-merge` and its conditions for one repository. Each merge gets a new merger session under that key. One merger acts for each repository at a time. A merge is confirmed by a read of the code host API, never by an exit code. The init skill adds its rows at the end of this file, so this table stays the last section.

| Repository | Merger role key | Conditions | Owner words | Date (UTC) | Question ID |
|---|---|---|---|---|---|
| group/shop | clerk-shop-merge | green pipeline | "merge when green" | 2026-09-29T09:05:00Z | Q-4 |
