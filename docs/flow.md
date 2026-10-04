# bruh flow

This file shows the process of bruh as diagrams. It matches [spec.md](spec.md) version 0.4. When this file and the specification differ, the specification is correct.

The diagrams:

1. [Topology](#1-topology): the sessions, the files, and how they connect.
2. [Task lifecycle](#2-task-lifecycle): one task from the owner to a delivered and merged branch.
3. [Question loop](#3-question-loop): how a question goes up and how the answer comes back.
4. [Delivery loop](#4-delivery-loop): the steps of the `deliver` workflow.
5. [Context loop](#5-context-loop): how each session stays below 55 percent of its context window.
6. [Liveness and failure loop](#6-liveness-and-failure-loop): how a parent finds and restarts a stopped session.
7. [Sweep loop](#7-sweep-loop): the recurring task of bigm that keeps the ledger true.

## 1. Topology

Local sessions talk through the mailbox of the bruh MCP server plus a `SendMessage` nudge. A remote clanker talks to bigm only through Orca, because the mailbox is on one machine. The remote clanker uses the mailbox of its own machine for its clerks.

```mermaid
flowchart TB
    owner([Owner])
    subgraph chans["Channels"]
        tg["Telegram"]
        sl["Slack"]
    end
    host[("Code host")]
    subgraph local["Machine of bigm"]
        bigm["bigm<br/>interactive<br/>in the ledger folder<br/>in an Orca terminal<br/>for remote work<br/>started with<br/>--channels"]
        watcher["Watcher<br/>background Monitor"]
        ledger[("Ledger<br/>private repository")]
        cl["clerk-ledger"]
        mcp[("bruh MCP server<br/>plugin data folder:<br/>mailbox, handoffs, answers,<br/>leases, reports")]
        c1["clanker-projectA"]
        k1["clerk-projectA-task1"]
        w1[["deliver workflow"]]
    end
    subgraph remote["Remote machine"]
        c2["clanker-projectB"]
        k2["clerk-projectB-task1"]
        w2[["deliver workflow"]]
    end
    owner <-->|terminal| bigm
    owner <-->|"P0, P1"| chans
    chans <--> bigm
    bigm -->|commits| ledger
    cl -->|pushes| ledger
    bigm -->|runs| watcher
    watcher -->|reads events| host
    watcher -->|writes events to report files| mcp
    bigm <-->|"claude --bg, mailbox, SendMessage nudge"| c1
    c1 <-->|"claude --bg, mailbox, SendMessage nudge"| k1
    bigm -.->|"mailbox, SendMessage nudge"| cl
    bigm -.->|MCP tools| mcp
    c1 & k1 -.->|MCP tools| mcp
    k1 -->|Workflow tool| w1
    bigm <-->|"Orca send, ask, reply"| c2
    c2 <-->|"claude --bg, own mailbox"| k2
    k2 -->|Workflow tool| w2
```

## 2. Task lifecycle

Each start writes a start message to the mailbox and a role settings file, then runs `claude --bg`. Routine status goes to report files, not to messages. A merge is a P1 `P1 Q-project-host-n: merge?` to the owner, unless a merge grant covers it. A merger clerk `clerk-<project>-merge` does each merge in a new session and stops. For a remote project, bigm starts the merger on its own machine. `mail_post` accepts messages only between a parent and its child, from bigm, and a P0 from a clerk to bigm.

```mermaid
sequenceDiagram
    actor Owner
    participant bigm
    participant MCP as bruh MCP server
    participant Clanker
    participant Clerk
    participant WF as deliver workflow
    participant Merger as clerk-project-merge
    participant CL as clerk-ledger
    Owner->>bigm: Work request for a project
    bigm->>MCP: ledger_edit records the request and commits
    bigm->>MCP: mail_post start message, role_settings_write
    bigm->>Clanker: claude --bg --agent bruh:clanker --settings role file
    Clanker->>MCP: mail_read
    Clanker->>Clanker: Divide the work into tasks
    loop For each task
        Clanker->>MCP: mail_post start message, role_settings_write
        Clanker->>Clerk: claude --bg from the main checkout
        Clerk->>MCP: mail_read
        Clerk->>WF: /bruh:deliver
        WF-->>Clerk: Branch, SHAs, test counts, review findings
        Clerk->>Clerk: Push the branch
        Clerk->>Clanker: DONE with evidence
        Clanker->>MCP: report_write status, no STATUS message
        Clanker->>Clanker: Accept the result
        alt Merge grant for the repository
            Clanker->>Merger: START merge owner/repo#n with the grant as the cover
        else No merge grant
            Clanker->>bigm: P1 Q-project-host-n: merge owner/repo#n?
            bigm->>Owner: P1 in the next batch
            Owner->>bigm: Answer
            bigm->>Clanker: ANSWER Q-project-host-n
            bigm->>MCP: mail_post ANSWER Q-project-host-n: merge owner/repo#n approved, to clerk-project-merge
            Clanker->>Merger: START merge owner/repo#n with the ANSWER as the cover
        end
        Merger->>Merger: merge-train.sh merges only #n, confirms with the code host API
        Merger->>MCP: report_write merge result
        Merger->>Clanker: DONE merged, then stop
        Clerk->>Clerk: Remove the worktree and stop
    end
    bigm->>MCP: Sweep reads the report files
    bigm->>MCP: ledger_edit updates the ledger and commits
    bigm->>CL: Push the ledger
    CL->>CL: git push
```

## 3. Question loop

A question goes up until a role can answer it. A clanker answers P2 and the delegated P1 classes. Items of the never-without-the-owner list always go to the owner, also in autonomous mode.

```mermaid
sequenceDiagram
    actor Owner
    participant bigm
    participant Clanker
    participant Clerk
    participant Agent as Workflow agent
    participant MCP as bruh MCP server
    Agent->>Clerk: SendMessage to main: P1 Q-shop-dev-mac-7: subject
    Agent->>MCP: answer_wait Q-shop-dev-mac-7, deadline from args
    Clerk->>MCP: mail_post to the clanker
    Clerk->>Clanker: SendMessage nudge: P1 Q-shop-dev-mac-7: subject
    Clanker->>MCP: mail_read
    Clanker->>Clanker: Read priorities.md again, set the final P-level
    alt P2 or a delegated P1 class
        Clanker->>MCP: report_write a copy of the answer
        Clanker->>Clerk: ANSWER Q-shop-dev-mac-7
    else P0 or another P1
        Clanker->>bigm: P0 or P1 Q-shop-dev-mac-7: subject
        bigm->>bigm: Add Q-shop-dev-mac-7 to questions.md
        alt P0
            bigm->>Owner: At once, terminal and channel
            Owner->>bigm: Answer
        else P1, human mode or never-without-the-owner item
            bigm->>Owner: In the batch, interval and size from mode.md
            Owner->>bigm: Answer
        else P1, autonomous mode
            bigm->>bigm: Decide, log bigm decision with date and reasons
        end
        bigm->>bigm: Record the question and the answer in the ledger
        bigm->>Clanker: ANSWER Q-shop-dev-mac-7
        Clanker->>Clerk: ANSWER Q-shop-dev-mac-7
    end
    alt Agent still waits
        Clerk->>MCP: answer_write Q-shop-dev-mac-7
        MCP-->>Agent: The answer, in the same run
    else answer_wait returned pending at the deadline
        Note over Agent,Clerk: The run returned the question and ended
        Clerk->>MCP: answer_write Q-shop-dev-mac-7
        Clerk->>Clerk: Launch the workflow again with resumeFromRunId
    end
```

A message cannot approve a permission prompt. So a P0 for a prompt carries the command `claude attach <id>`, and the owner answers in that session.

## 4. Delivery loop

Each review diffs against the pinned base SHA. A finding that the refuter does not confirm stays refuted. The round cap comes from `args`, with default 2.

```mermaid
flowchart TD
    start([Start: args from the clerk]) --> plan["Plan: read the task, the code, the guides<br/>pin the base SHA<br/>write STOP conditions for each step"]
    plan --> q{Open questions?}
    q -->|yes| ask[Ask through the question loop] --> plan
    q -->|no| impl["Implement the plan<br/>record deviations, stop at a conflict"]
    impl --> review
    subgraph review["Review round, three checks in parallel"]
        adv["Adversarial reviewer:<br/>tries to refute the change"]
        chk["Independent checker:<br/>verifies invariants in the code"]
        btl["Build, test, lint:<br/>ran, passed, failed, skipped<br/>a skip in a required suite is a finding"]
    end
    review --> dedup["Deduplicate findings by file:line"]
    dedup --> refute[One refuter for each finding]
    refute --> conf{Confirmed findings?}
    conf -->|no| done(["Return the branch or pull request<br/>with evidence"])
    conf -->|yes| cap{Round cap reached?}
    cap -->|no| fix["Fix by area,<br/>one batch at a time"] --> review
    cap -->|yes| p1(["Stop. The clerk raises a P1<br/>with the findings"])
```

## 5. Context loop

The status line tap and the hooks are plugin code, and they write only in the plugin data folder. A hook does nothing in a subagent or in a session without `BRUH_ROLE_KEY`.

```mermaid
flowchart TD
    sline([Status line event]) --> tap["Tap writes the context percent<br/>and the five-hour rate limit percent"]
    tool([Tool call ends]) --> hook{"PostToolUse hook:<br/>agent_id in the input,<br/>or no BRUH_ROLE_KEY?"}
    hook -->|yes| quit([Exit, no action])
    hook -->|no| read["Read the context percent from the tap file,<br/>fallback: token usage from transcript_path"]
    tap -.-> read
    read --> pct{"At or above handoff_percent,<br/>default 50?"}
    pct -->|no| cont([Continue the work])
    pct -->|"yes, already told"| cont
    pct -->|"yes, first crossing"| nudge[Tell the agent once:<br/>update your handoff]
    nudge --> write["handoff_write: replace the file,<br/>below 8000 characters, UTC stamp"]
    write --> auto{"Context at the auto-compact<br/>window, 55 percent?"}
    auto -->|no| cont
    auto -->|yes| compact[Claude Code compacts]
    other(["/clear or resume"]) --> pickup
    compact --> pickup["SessionStart hook,<br/>matchers compact, clear, resume:<br/>reads BRUH_ROLE_KEY"]
    pickup --> inject["Inject the handoff. First line:<br/>the compaction summary is not a source"]
    inject --> reread[Read the live source again<br/>for each pending item]
    reread --> cont
```

## 6. Liveness and failure loop

A parent checks a child before each nudge. A reboot check comes first, because after a reboot many sessions show as `failed` or `stopped` at the same time. A `status` of `waiting` means between turns, not stuck.

```mermaid
flowchart TD
    send([Parent must send a message]) --> post[mail_post writes the mailbox]
    post --> read[Read claude agents --json --all]
    read --> reboot{"Many sessions failed<br/>or stopped at once?"}
    reboot -->|yes| rec["Reconcile the ledger, update the role key map,<br/>resume long-lived roles:<br/>claude --resume session-id --bg"] --> pid
    reboot -->|no| pid{pid of the child alive?}
    pid -->|yes| wf{"waitingFor is<br/>permission prompt?"}
    wf -->|yes| p0a(["P0 to the owner:<br/>claude attach id"])
    wf -->|"no, status waiting is fine"| nudge[SendMessage nudge]
    pid -->|"no, idle stop"| resume["claude --resume session-id --bg:<br/>read your mailbox"]
    pid -->|"no, state failed or stopped,<br/>task not done"| first{First restart?}
    first -->|yes| respawn[claude respawn id] --> read
    first -->|no| p0b(["P0 to the owner:<br/>the session failed twice"])
    nudge --> ans{"Answer in the next turn<br/>of the receiver?"}
    resume --> ans
    ans -->|yes| ok([Done])
    ans -->|no| again[Send again with the next attempt counter] --> read
    limit([Usage limit]) --> stop["Session stops and reports,<br/>no model change"]
    stop --> probe[bigm probes the account]
    probe --> p1[P1: the owner chooses the account]
    p1 --> reset["After the reset: the clerk launches<br/>the workflow again with resumeFromRunId"]
```

## 7. Sweep loop

bigm runs the sweep as a recurring `CronCreate` task, and a message wakes it sooner. A `Monitor` and a background command are not restored on resume, so bigm starts them again.

```mermaid
flowchart TD
    cron(["CronCreate task, every 15 minutes,<br/>or a message wakes bigm"]) --> reports[Read all report files]
    reports --> rec["Reconcile claude agents --json --all<br/>and Orca worker-list"]
    rec --> rows[Read the source again<br/>for each row past its next check]
    rows --> due{"P1 batch due?<br/>interval and size from mode.md"}
    due -->|yes| batch[Send the P1 batch,<br/>one item for each message] --> upd
    due -->|no| upd[Update the ledger]
    upd --> commit[ledger_edit commits]
    commit --> push[clerk-ledger pushes the ledger]
    push --> age{Task 6 days old?}
    age -->|yes| again["Create the CronCreate task again,<br/>it expires after 7 days"] --> wait
    age -->|no| wait([Wait for the next run])
    wait --> cron
    resume([bigm resumes]) --> restore[CronCreate tasks are restored]
    restore --> restart["Start the watcher Monitor and<br/>orca orchestration check --wait again"]
    restart --> cron
```
