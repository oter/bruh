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
  ],
  clanker: [
    'session_launch', 'session_resume', 'session_list', 'mail_post', 'mail_read', 'role_settings_write',
    'report_write', 'question_open', 'lease_request', 'lease_grant', 'lease_release', 'lease_list',
    'handoff_write', 'bruh_info', 'SendMessage', 'priorities.md', 'rules.md', 'house-rules.md',
    '${user_config.max_busy_clerks}', 'CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS', 'orca orchestration ask',
  ],
  clerk: [
    'mail_read', 'mail_post', 'answer_write', 'report_write', 'question_open', 'handoff_write',
    'bruh_info', 'lease_request', 'lease_release', 'SendMessage', '/bruh:deliver', 'resumeFromRunId',
    'merge-train.sh', 'base_sha', 'round_cap', 'deadline_seconds', 'answers',
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
    const header = /^((P[012]|ANSWER|REC) Q-(\d+|<n>|<id>)|RULE R-(\d+|<n>)|DONE): \S/
    for (const m of agents[role].matchAll(/`((?:P[012]|ANSWER|REC|RULE|DONE)\b[^`]*)`/g)) {
      if (/^(P[012]|ANSWER|REC|RULE|DONE)$/.test(m[1])) continue // the bare word, not a header
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
