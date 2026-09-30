// Structural checks for the role agents, the defaults, and the ledger template.
// Run: node --test plugins/bruh/agents/agents_test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, existsSync } from 'node:fs'
import { join } from 'node:path'

const here = import.meta.dirname
const plugin = join(here, '..')
const repo = join(plugin, '..', '..')
const read = (p) => readFileSync(p, 'utf8')

// Tool names of interfaces section 2: the "Existing tools" sentence and the
// first cell of each row of the "New tools" table.
function contractTools() {
  const text = read(join(repo, 'docs/superpowers/plans/2026-09-30-v0.1-interfaces.md'))
  const section = text.split('## 2. MCP server tools')[1].split('\n## 3.')[0]
  const names = new Set()
  const existing = section.split('\n').find((l) => l.includes('Existing tools'))
  for (const m of existing.split('Existing tools')[1].matchAll(/`([a-z_]+)`/g)) names.add(m[1])
  for (const m of section.matchAll(/^\| `([a-z_]+)` \|/gm)) names.add(m[1])
  return names
}

function frontmatter(text) {
  const lines = text.split('\n')
  assert.equal(lines[0], '---', 'the first line must be ---')
  const end = lines.indexOf('---', 1)
  assert.ok(end > 0, 'no closing ---')
  const fm = {}
  for (const line of lines.slice(1, end)) {
    const m = line.match(/^([A-Za-z]+):\s*(.*)$/)
    assert.ok(m, `bad frontmatter line: ${line}`)
    fm[m[1]] = m[2].trim()
  }
  return { fm, body: lines.slice(end + 1).join('\n') }
}

const ROLES = ['bigm', 'clanker', 'clerk']
const TOOL_FAMILY = /^(mail|handoff|answer|report|role_settings|lease|question|session|init|bruh)_[a-z_]+$/
const NOT_TOOLS = new Set(['session_id', 'question_id'])

// The tools and other terms that each procedure must name.
const REQUIRED = {
  bigm: [
    'session_launch', 'session_resume', 'session_list', 'mail_post', 'mail_read', 'role_settings_write',
    'report_read', 'handoff_write', 'question_open', 'lease_define', 'lease_grant', 'lease_release',
    'lease_list', 'bruh_info', 'SendMessage', 'CronCreate', 'Monitor', 'watcher.sh', 'mode.md',
    'owed.md', 'rules.md', 'grants.md', 'questions.md', 'leases.md', 'priorities.md', 'clerk-ledger',
    'orca orchestration check --wait', 'orca orchestration worker-start', '${user_config.user_name}',
    'START: ', 'role-settings clanker-<project>', 'mcp__plugin_telegram_telegram__reply', 'chat_id',
    'clerk-<project>-merge', 'has no section "Never without the owner"',
    'not running; mail pending', 'unread mail',
  ],
  clanker: [
    'session_launch', 'session_resume', 'session_list', 'mail_post', 'mail_read', 'role_settings_write',
    'report_write', 'question_open', 'lease_request', 'lease_grant', 'lease_release', 'lease_list',
    'handoff_write', 'bruh_info', 'SendMessage', 'priorities.md', 'rules.md', 'house-rules.md',
    '${user_config.max_busy_clerks}', 'CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS', 'orca orchestration ask',
    'START: ', 'accepted: <head SHA>', 'clerk-<project>-merge', 'orca orchestration send',
    'orca orchestration check --wait', '`DONE: mode is now <mode>`', 'The task name `merge` is reserved',
  ],
  clerk: [
    'mail_read', 'mail_post', 'answer_write', 'report_write', 'question_open', 'handoff_write',
    'bruh_info', 'lease_request', 'lease_release', 'SendMessage', '/bruh:deliver', 'resumeFromRunId',
    'merge-train.sh', 'base_sha', 'round_cap', 'deadline_seconds', 'answers',
    'START: ', 'accepted: <head SHA>', 'FAILED:', 'clerk-<project>-merge', 'session_list',
    'merge-train.sh --data <data_dir> <owner/repo> <pull request number>', 'Verify',
    'not running; mail pending', 'CronCreate', 'at most 3 retries', 'in 15 minutes', 'REPEAT:',
    'git merge-base --is-ancestor',
    '/bruh:implement', '/bruh:tickets', '/bruh:implement-tickets', '/bruh:review-and-fix', '/bruh:review-only',
    'post-findings.sh --dry-run', 'post-findings.sh --data <data_dir>', 'Post grants', '(exit code 3)',
    'result_save', '`--answer Q-<n>`', '`ANSWER Q-<n>: post <owner/repo>#<number> approved`', '`<workflow> args:`', '`<workflow> retry <n>`',
  ],
}

const agents = Object.fromEntries(
  ROLES.map((r) => {
    const file = join(here, `${r}.md`)
    return [r, existsSync(file) ? read(file) : null]
  }),
)

for (const role of ROLES) {
  test(`${role}: frontmatter is valid`, () => {
    assert.ok(agents[role], `agents/${role}.md is missing`)
    const { fm, body } = frontmatter(agents[role])
    assert.equal(fm.name, role)
    assert.ok(!fm.name.includes(':'))
    assert.ok(fm.description, 'description is empty')
    assert.match(fm.model, /^(opus|sonnet|fable)\[1m\]$/, 'roles run on a 1M-context model (spec 3.3)')
    assert.ok(['low', 'medium', 'high', 'xhigh', 'max'].includes(fm.effort), `bad effort ${fm.effort}`)
    for (const ignored of ['hooks', 'mcpServers', 'permissionMode']) {
      assert.ok(!(ignored in fm), `a plugin agent ignores ${ignored}`)
    }
    assert.ok(body.trim().length > 0)
  })

  test(`${role}: every MCP tool name in the agent text exists`, () => {
    assert.ok(agents[role], `agents/${role}.md is missing`)
    const known = contractTools()
    const unknown = []
    for (const m of agents[role].matchAll(/`([a-z_]+)`/g)) {
      const t = m[1]
      if (TOOL_FAMILY.test(t) && !NOT_TOOLS.has(t) && !known.has(t)) unknown.push(t)
    }
    for (const m of agents[role].matchAll(/mcp__plugin_bruh_bruh__([a-z_]+)/g)) {
      if (!known.has(m[1])) unknown.push(m[1])
    }
    assert.deepEqual(unknown, [], `unknown tools in agents/${role}.md`)
  })

  test(`${role}: each role names the tools of its procedure`, () => {
    assert.ok(agents[role], `agents/${role}.md is missing`)
    const missing = REQUIRED[role].filter((t) => !agents[role].includes(t))
    assert.deepEqual(missing, [], `agents/${role}.md does not name these`)
  })

  test(`${role}: every message header has the grammar of spec 5`, () => {
    assert.ok(agents[role], `agents/${role}.md is missing`)
    const header = /^((P[012]|ANSWER|REC) Q-(\d+|<n>|<id>)|RULE R-(\d+|<n>)|DONE|START): \S/
    for (const m of agents[role].matchAll(/`((?:P[012]|ANSWER|REC|RULE|DONE|START)\b[^`]*)`/g)) {
      if (/^(P[012]|ANSWER|REC|RULE|DONE|START):?$/.test(m[1])) continue // the bare word, not a header
      assert.match(m[1], header, `bad header in agents/${role}.md`)
    }
  })
}

test('the clanker cannot write files', () => {
  assert.ok(agents.clanker, 'agents/clanker.md is missing')
  const { fm } = frontmatter(agents.clanker)
  const denied = (fm.disallowedTools || '').split(',').map((s) => s.trim())
  for (const t of ['Edit', 'Write', 'NotebookEdit']) assert.ok(denied.includes(t), `clanker must deny ${t}`)
})

test('the default priorities have the sections of spec 13 and 14.1', () => {
  const text = read(join(plugin, 'defaults/priorities.md'))
  for (const h of ['## P0', '## P1', '## P2', '## Delegated P1 classes', '## Never without the owner', '## Deny rules']) {
    assert.ok(text.includes(`\n${h}\n`), `priorities.md has no "${h}"`)
  }
  const deny = JSON.parse(read(join(plugin, 'defaults/role-settings.json'))).permissions.deny
  for (const d of deny) assert.ok(text.includes(d), `priorities.md does not list the deny rule ${d}`)
})

test('the default house rules name the review rules of spec 6.3', () => {
  const text = read(join(plugin, 'defaults/house-rules.md'))
  for (const s of ['base SHA', 'origin/main', 'skipped', 'file:line', 'STOP']) {
    assert.ok(text.includes(s), `house-rules.md does not name "${s}"`)
  }
})

test('the ledger template has the layout of spec 8', () => {
  const t = join(plugin, 'ledger-template')
  for (const f of ['README.md', 'mode.md', 'priorities.md', 'rules.md', 'grants.md', 'questions.md', 'owed.md', 'leases.md', 'projects/_template.md']) {
    assert.ok(existsSync(join(t, f)), `ledger-template/${f} is missing`)
  }
  const mode = read(join(t, 'mode.md'))
  assert.match(mode, /^mode: human$/m)
  for (const k of ['changed', 'reason', 'p1_batch_minutes', 'p1_batch_size', 'review_round_cap', 'status_cadence']) {
    assert.match(mode, new RegExp(`^${k}: \\S`, 'm'), `mode.md has no ${k}`)
  }
  const project = read(join(t, 'projects/_template.md'))
  for (const s of ['Summary', 'In progress', 'Merged', 'Live', 'Decisions', 'Questions and answers', 'Sessions', 'Identities', 'Shared resources and clerk leases', 'Waiting on others']) {
    assert.ok(project.includes(`\n## ${s}\n`), `projects/_template.md has no section ${s}`)
  }
})

// Interfaces section 4a: the start header, the post marker, and the remote role settings command.
test('the agents follow interfaces section 4a', () => {
  const text = read(join(repo, 'docs/superpowers/plans/2026-09-30-v0.1-interfaces.md'))
  const section = text.split('## 4a. Messages and posts')[1].split('\n## 5.')[0]
  const marker = section.match(/`(<!-- bruh:<role key> -->)`/)[1]
  assert.ok(agents.clerk.includes(marker), 'clerk.md does not use the post marker of the contract')
  assert.ok(section.includes('`START: <subject>`'))
  assert.ok(section.includes('. role-settings <role key>'))
  assert.ok(agents.bigm.includes('. role-settings clanker-<project>'), 'bigm.md does not start a remote clanker with the role-settings command')
  for (const role of ROLES) {
    assert.ok(!/DONE: start message/.test(agents[role]), `agents/${role}.md still sends a start message with DONE`)
  }
})

test('the clanker and the clerk agree on the acceptance message', () => {
  for (const role of ['clanker', 'clerk']) {
    assert.ok(agents[role].includes('`DONE: result accepted for <task>`'), `${role}: acceptance header`)
    assert.ok(agents[role].includes('`accepted: <head SHA>`'), `${role}: acceptance body line`)
  }
})

test('the busy-clerk cap counts live clerks, not busy status', () => {
  assert.doesNotMatch(agents.clanker, /`status` equal to `busy`/)
  assert.match(agents.clanker, /`state` is not `done`, `failed`, or `stopped`/)
})

test('the ledger clerk skips the task start and has no worktree', () => {
  const ledger = agents.clerk.split('## The ledger clerk')[1].split('\n## ')[0]
  assert.match(ledger, /Do not run `EnterWorktree`/)
  assert.match(agents.clerk.split('## Start')[1].split('\n## ')[0], /`clerk-ledger`.*skip/)
})

test('the ledger template priorities placeholder has no hard-stop list, and bigm checks for it', () => {
  const placeholder = read(join(plugin, 'ledger-template/priorities.md'))
  assert.ok(!placeholder.includes('## Never without the owner'))
  assert.match(agents.bigm, /has no section "Never without the owner"/)
})

// Every header that any agent sends must be on the header list of every agent.
test('every header kind is in the allowed list of every agent', () => {
  const kind = (h) => h.match(/^(P[012]|ANSWER|REC|RULE|DONE|START)\b/)[1]
  const used = new Set()
  for (const role of ROLES) {
    for (const m of agents[role].matchAll(/`((?:P[012]|ANSWER|REC|RULE|DONE|START) ?(?:Q-|R-)?[^`]*: [^`]*)`/g)) used.add(kind(m[1]))
  }
  for (const k of ['START', 'DONE', 'ANSWER', 'RULE', 'P0']) assert.ok(used.has(k), `no agent sends ${k}`)
  for (const role of ROLES) {
    const line = agents[role].split('\n').find((l) => l.includes('The message headers of bruh are'))
    assert.ok(line, `agents/${role}.md has no header list`)
    const allowed = new Set([...line.matchAll(/`((?:P[012]|ANSWER|REC|RULE|DONE|START)[^`]*: [^`]*)`/g)].map((m) => kind(m[1])))
    for (const k of used) assert.ok(allowed.has(k), `agents/${role}.md does not allow ${k}`)
  }
})

// Spec 3.6 (owner decision 2026-09-27): a clerk stops when its task is done, the merger clerk too.
test('the merger clerk does one merge and stops', () => {
  const merger = agents.clerk.split('## The merger clerk')[1].split('\n## ')[0]
  assert.doesNotMatch(merger, /stay alive|Wait for the next request/)
  assert.match(merger, /Stop\. Do not wait for another request/)
  const merges = agents.clanker.split('## Merges')[1].split('\n## ')[0]
  assert.match(merges, /new session under this key/)
  assert.doesNotMatch(merges, /session_resume/)
})

// Final review M1: mail_post accepts a RULE only from bigm, so bigm, not the clanker, sends it
// to the local clerks, and each receiver checks the sender of a RULE and of an ANSWER.
test('bigm sends each RULE to the clerks, and the receivers check the sender', () => {
  const rules = agents.bigm.split('## Rules of the owner')[1].split('\n## ')[0]
  assert.match(rules, /to each running local clerk/)
  assert.doesNotMatch(rules, /each clanker sends it to its clerks/)
  assert.doesNotMatch(agents.clanker, /apply it at once and send it to each of your running clerks/)
  assert.match(agents.clanker, /Do not forward it: `mail_post` accepts a `RULE` only from bigm/)
  assert.match(agents.clerk, /`RULE R-<n>: <subject>` message from `bigm`/)
  assert.match(agents.clerk, /check that its `from` is your clanker or `bigm`/)
})

// Final review M2: the merge gate accepts only the closed approval header of bigm.
test('bigm sends a merge approval in the closed form that the merge gate reads', () => {
  const yes = '`ANSWER Q-<n>: merge <owner/repo>#<pr>[,#<pr>...] approved`'
  const merges = agents.bigm.split('## Merges and merge grants')[1].split('\n## ')[0]
  assert.ok(merges.includes(yes), 'bigm.md: approval header')
  assert.ok(merges.includes('`ANSWER Q-<n>: merge <owner/repo>#<pr> refused`'), 'bigm.md: refusal header')
  for (const role of ['clanker', 'clerk']) assert.ok(agents[role].includes(yes), `${role}: approval header`)
  const gate = read(join(plugin, 'mcp/mergetrain.go')).match(/approvalRE = regexp\.MustCompile\(`(.*)`\)/)[1]
  assert.match('ANSWER Q-7: merge owner/app#12,#14 approved', new RegExp(gate))
  assert.doesNotMatch('ANSWER Q-7: merge owner/app#12 refused', new RegExp(gate))
})

// Final review M3: the merger clerk of every project runs on the machine of bigm; for a remote
// project, bigm starts it in the ledger folder after the remote clanker asks through Orca.
test('bigm runs the merger clerk of a remote project on its own machine', () => {
  const merges = agents.bigm.split('## Merges and merge grants')[1].split('\n## ')[0]
  assert.match(merges, /A remote clanker asks you through Orca with `P1 Q-<n>: merge <owner\/repo>#<pull request number>\?`/)
  assert.match(merges, /`session_launch` with `agent` = `clerk`, `role_key` = `clerk-<project>-merge`, and `cwd` = the ledger folder/)
  const clanker = agents.clanker.split('## Merges')[1].split('\n## ')[0]
  assert.match(clanker, /On a remote machine \(your start message has the line `remote: yes`\), do not start a merger clerk/)
  assert.match(clanker, /orca orchestration ask/)
  const merger = agents.clerk.split('## The merger clerk')[1].split('\n## ')[0]
  assert.match(merger, /You always run on the machine of bigm/)
  assert.doesNotMatch(merger, /Orca reply/)
})

// Final review M4: question IDs are unique only on one machine.
test('bigm keys a question by its asker and its ID', () => {
  const questions = agents.bigm.split('## Questions')[1].split('\n## ')[0]
  assert.match(questions, /Never match a question by its ID alone/)
  assert.ok(questions.includes('`<orca environment>/Q-<n>`'))
  assert.doesNotMatch(questions, /If you already answered this question ID, ignore it/)
})

// Final review M5: bigm registers the repositories of a project with repos_set and its project.
test('bigm calls repos_set with the project of the clanker key', () => {
  const start = agents.bigm.split('### Start a local clanker')[1].split('\n## ')[0]
  assert.match(start, /call `repos_set` with `repo` = `<owner\/repo>`, `host` = `github` or `gitea`, `api_url`/)
  assert.match(start, /`project` = the `<project>` part of the clanker key `clanker-<project>`/)
  const remote = agents.bigm.split('## Remote clankers')[1].split('\n## ')[0]
  assert.match(remote, /call `repos_set`/)
})

// Final review M6: the clerk passes its test commands in args.test_gates.
test('the clerk passes the test gates to deliver', () => {
  assert.ok(agents.clerk.includes('"test_gates": ["<gate command that runs tests>"]'))
  assert.match(agents.clanker, /the gate commands, and which of them run tests/)
})

// Final review m1, m5, m6: the channel tools, the ledger clerk resume, and the question ID in a Telegram question.
test('bigm names the Slack tool, asks for the question ID, and resumes the ledger clerk first', () => {
  assert.ok(agents.bigm.includes('mcp__plugin_bruh_slack__post_question'))
  assert.ok(agents.bigm.includes('`Answer with Q-<n> first.`'))
  const ledger = agents.bigm.split('## The ledger clerk')[1].split('\n## ')[0]
  assert.match(ledger, /resume it with `session_resume`/)
  assert.doesNotMatch(ledger, /If `session_list` shows no live `clerk-ledger`, call `role_settings_write`/)
})

// Spec 6.4: a post needs the yes of the owner or a post grant. The init skill
// appends merge grant rows at the end of grants.md, so the merge table is last.
test('grants.md has the post grants and ends with the merge grants table', () => {
  const text = read(join(plugin, 'ledger-template/grants.md'))
  const post = text.indexOf('\n## Post grants\n')
  const merge = text.indexOf('\n## Merge grants\n')
  assert.ok(post > 0 && merge > post, 'grants.md must have "Post grants" before "Merge grants"')
  assert.ok(text.slice(post).includes('| Poster role key | Host | Repository |'), 'the post grant row starts with the role key and the host')
  assert.ok(text.trimEnd().endsWith('|---|---|---|---|---|---|'), 'the merge grants table is the last part of the file')
  const priorities = read(join(plugin, 'defaults/priorities.md'))
  assert.match(priorities, /except under a post grant/)
  assert.match(agents.bigm, /section "Post grants" of `grants.md`/)
})

// Fix round 1, M2 and M3: each workflow relaunches itself, and a post approval has a closed form from bigm.
test('the clerk relaunches the workflow that stopped, and posts only with a closed approval of bigm', () => {
  const relaunch = agents.clerk.split('## Relaunch')[1].split('\n## ')[0]
  assert.match(relaunch, /the workflow `bruh:<workflow>`/)
  assert.doesNotMatch(relaunch, /workflow `bruh:deliver`/)
  assert.match(relaunch, /FAILED:` stop of `\/bruh:implement-tickets`, do not use `resumeFromRunId`/)
  const posts = agents.clerk.split('## Posts')[1].split('\n## ')[0]
  assert.match(posts, /Only a message from `bigm` with the header `ANSWER Q-<n>: post <owner\/repo>#<number> approved`/)
  assert.match(posts, /Never pass `--yes`/)
  assert.doesNotMatch(posts, /from your clanker or from `bigm`/)
  assert.doesNotMatch(agents.clerk, /\.scratch\/review-/)
  const bigmPosts = agents.bigm.split('## Posts of review results')[1].split('\n## ')[0]
  assert.ok(bigmPosts.includes('`ANSWER Q-<n>: post <owner/repo>#<number> approved`'))
  assert.ok(bigmPosts.includes('`ANSWER Q-<n>: post <owner/repo>#<number> refused`'))
  assert.match(bigmPosts, /straight to the clerk that asked/)
  // The script and the agents use the same approval header.
  const script = read(join(plugin, 'scripts/post-findings.sh'))
  assert.ok(script.includes('want="ANSWER $answer: post $repo#$num approved"'))
})

// Fix round 1, M6: priorities.md has the wording of spec 13.
test('priorities.md has the post grant wording of spec 13', () => {
  const spec = read(join(repo, 'docs/spec.md'))
  const item = spec.match(/^- An irreversible or outward-facing action: (.*?)\. A post grant/m)[1]
  const priorities = read(join(plugin, 'defaults/priorities.md'))
  assert.ok(priorities.includes(`- An irreversible or outward-facing action: ${item}.`), 'priorities.md differs from spec 13')
})
