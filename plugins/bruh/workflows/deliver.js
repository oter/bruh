export const meta = {
  name: 'deliver',
  description: 'Deliver one bruh task: plan, implement, review in three parallel checks, refute each finding, fix by area, and review again up to the round cap.',
  whenToUse: 'A bruh clerk runs it with the args of its start message.',
  phases: [
    { title: 'Plan', detail: 'pin the base SHA, write a plan with STOP conditions' },
    { title: 'Implement', detail: 'implement the plan, record deviations, stop at a conflict' },
    { title: 'Review', detail: 'adversarial reviewer, invariant checker, and gates; one refuter for each finding' },
    { title: 'Fix', detail: 'fix the confirmed findings, one area at a time' },
  ],
}

// The script has no clock and no filesystem. Agents do all reads and writes.
// Prompts before a question never contain an answer, so a relaunch with
// resumeFromRunId returns their cached results (spec 6.2).

const A = (typeof args === 'object' && args) || {}
const list = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [])
const task = typeof A.task === 'string' ? A.task.trim() : ''
const base = typeof A.base_sha === 'string' ? A.base_sha : ''
const branch = typeof A.branch === 'string' ? A.branch.trim() : ''
const gateCommands = list(A.gates)
const cap = Number.isInteger(A.round_cap) && A.round_cap >= 1 ? A.round_cap : 2
const deadline = typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0 ? A.deadline_seconds : 3600
const answers = A.answers && typeof A.answers === 'object' ? A.answers : {}

let head = base
let tests = { ran: 0, passed: 0, failed: 0, skipped: 0 }
const deviations = []
const found = new Map() // "file:line" -> {file, line, summary, state}
const refuted = new Set()

const key = (f) => `${f.file}:${f.line}`

function result(status, question = null) {
  const findings = [...found.values()].map(({ file, line, summary, state }) => ({ file, line, summary, state }))
  return { status, branch, base_sha: base, head_sha: head, tests, findings, question, deviations }
}

function stop(reason) {
  deviations.push(`STOP: ${reason}`)
  return result('stopped')
}

const problems = []
if (!task) problems.push('args.task is empty')
if (!/^[0-9a-f]{40}$/.test(base)) problems.push('args.base_sha is not a 40-character hex SHA')
if (!branch) problems.push('args.branch is empty')
if (!Array.isArray(A.gates)) problems.push('args.gates is not a list')
if (problems.length) return stop(problems.join('; '))

const bullets = (xs) => (xs.length ? xs.map((x) => `- ${x}`).join('\n') : '(none)')

const CONTEXT = `You are an agent of the bruh deliver workflow for one task.
Branch: ${branch}
Pinned base SHA: ${base}

Task:
${task}

Acceptance criteria:
${bullets(list(A.acceptance))}

Guides index of the project:
${typeof A.guides === 'string' && A.guides.trim() ? A.guides : '(none)'}

House rules:
${typeof A.house_rules === 'string' ? A.house_rules : ''}

Deliberate choices (do not flag them, do not undo them):
${bullets(list(A.deliberate))}

Rules of this run:
- Do not post outside the project: no comments on pull requests or issues, no chat messages, no emails.
- Do not push. The clerk pushes.
- Work only in the current working tree, on the branch ${branch}.`

const DIFF = `Review the change with \`git diff ${base} HEAD\`, against the pinned base SHA.
Never diff against origin/main or another moving ref.`

const ASK = `Questions: when you need a decision that the task, the plan, the house rules, and the code do not answer, do not guess.
1. Call the bruh MCP tool question_open (mcp__plugin_bruh_bruh__question_open; load it with ToolSearch) with priority (P0, P1, or P2: your estimate), subject (one line), body (the question, the options ranked, and your recommendation), and blocks (the work that waits for the answer).
2. Send the returned header, and only the header, to main with SendMessage.
3. Call answer_wait (mcp__plugin_bruh_bruh__answer_wait) with question_id set to the returned id and deadline_seconds ${deadline}.
4. If it returns answered, use the answer text and continue.
5. If it returns pending, stop at once and do no more work. Return your result with question set to the id, header, and body of the question.`

const QUESTION = {
  type: 'object',
  properties: { id: { type: 'string' }, header: { type: 'string' }, body: { type: 'string' } },
  required: ['id', 'header', 'body'],
}
const LOCATION = {
  type: 'object',
  properties: { file: { type: 'string' }, line: { type: 'integer' } },
  required: ['file', 'line'],
}
const FINDINGS = {
  type: 'object',
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object',
        properties: { file: { type: 'string' }, line: { type: 'integer' }, summary: { type: 'string' } },
        required: ['file', 'line', 'summary'],
      },
    },
  },
  required: ['findings'],
}
const COUNTS = {
  ran: { type: 'integer' }, passed: { type: 'integer' }, failed: { type: 'integer' }, skipped: { type: 'integer' },
}
const GATES = {
  type: 'object',
  properties: {
    head_sha: { type: 'string' },
    tests: { type: 'object', properties: COUNTS, required: ['ran', 'passed', 'failed', 'skipped'] },
    gates: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          command: { type: 'string' },
          exit_code: { type: 'integer' },
          ...COUNTS,
          problems: {
            type: 'array',
            items: {
              type: 'object',
              properties: {
                file: { type: 'string' }, line: { type: 'integer' }, summary: { type: 'string' },
                kind: { type: 'string', enum: ['failed', 'skipped'] },
              },
              required: ['file', 'line', 'summary', 'kind'],
            },
          },
        },
        required: ['command', 'exit_code', 'ran', 'passed', 'failed', 'skipped', 'problems'],
      },
    },
  },
  required: ['head_sha', 'tests', 'gates'],
}
const PLAN = {
  type: 'object',
  properties: { plan: { type: 'string' }, stop: { type: 'boolean' }, reason: { type: 'string' }, question: QUESTION },
  required: ['plan', 'stop', 'reason'],
}
const IMPLEMENT = {
  type: 'object',
  properties: {
    head_sha: { type: 'string' },
    deviations: { type: 'array', items: { type: 'string' } },
    conflict: { type: 'string' },
    question: QUESTION,
  },
  required: ['head_sha', 'deviations', 'conflict'],
}
const FIX = {
  type: 'object',
  properties: {
    head_sha: { type: 'string' },
    fixed: { type: 'array', items: LOCATION },
    deviations: { type: 'array', items: { type: 'string' } },
    question: QUESTION,
  },
  required: ['head_sha', 'fixed', 'deviations'],
}
const VERDICT = {
  type: 'object',
  properties: { confirmed: { type: 'boolean' }, reason: { type: 'string' } },
  required: ['confirmed', 'reason'],
}

// Runs a step that can ask a question. The first attempt has the plain prompt.
// Each later attempt adds the earlier questions of this step and their answers
// from args.answers. Returns {value}, {question}, or {failed}.
async function askable(label, prompt, schema) {
  const asked = []
  for (;;) {
    const earlier = asked.length
      ? `\n\nEarlier questions of this step and their answers. Check the working tree first: work of an earlier attempt can be in it already.\n${asked.map((q) => `- ${q.id} (${q.header}): ${answers[q.id]}`).join('\n')}`
      : ''
    const r = await agent(`${CONTEXT}\n\n${prompt}${earlier}\n\n${ASK}`, {
      label: asked.length ? `${label} after ${asked.at(-1).id}` : label,
      effort: 'high',
      schema,
    })
    if (!r) return { failed: `the ${label} agent did not return a result` }
    if (!r.question) return { value: r }
    const q = { id: r.question.id, header: r.question.header, body: r.question.body }
    if (asked.some((x) => x.id === q.id)) return { failed: `${q.id} is pending again after its answer` }
    if (typeof answers[q.id] !== 'string') return { question: q }
    asked.push(q)
  }
}

// Each gate is a required suite: each failed or skipped test is a finding.
function gateFindings(g) {
  const out = []
  for (const gate of g.gates || []) {
    const named = gate.problems || []
    for (const p of named) {
      out.push({
        file: p.file || `gate: ${gate.command}`,
        line: p.line || 0,
        summary: `${p.kind === 'skipped' ? 'Skipped' : 'Failed'} test in the required suite \`${gate.command}\`: ${p.summary}`,
        gate: gate.command,
      })
    }
    const unnamed = []
    if (gate.skipped > 0 && !named.some((p) => p.kind === 'skipped')) unnamed.push(`${gate.skipped} skipped`)
    if ((gate.failed > 0 || gate.exit_code !== 0) && !named.some((p) => p.kind === 'failed')) {
      unnamed.push(`exit code ${gate.exit_code}, ${gate.failed} failed`)
    }
    if (unnamed.length) {
      out.push({ file: `gate: ${gate.command}`, line: 0, summary: `The required suite \`${gate.command}\` has ${unnamed.join(' and ')}.`, gate: gate.command })
    }
  }
  return out
}

const area = (f) => (f.file.startsWith('gate: ') ? 'gates' : f.file.includes('/') ? f.file.slice(0, f.file.indexOf('/')) : '.')

// 1. Plan
phase('Plan')
const plan = await askable(
  'plan',
  `Step: plan. Do not edit files in this step.
1. Read the task, the code, and the guides of the project.
2. Pin the base SHA: run \`git cat-file -e ${base}^{commit}\` and \`git merge-base --is-ancestor ${base} HEAD\`. If one fails, return stop = true with the reason.
3. Write a plan: numbered steps, and for each step a written STOP condition (the observation that makes the implementer stop instead of guessing).
4. If the task cannot be done as written, return stop = true with the reason. Otherwise return stop = false and an empty reason.`,
  PLAN,
)
if (plan.failed) return stop(plan.failed)
if (plan.question) return result('question', plan.question)
if (plan.value.stop) return stop(plan.value.reason || 'the plan agent stopped')

// 2. Implement
phase('Implement')
const impl = await askable(
  'implement',
  `Step: implement.

Plan:
${plan.value.plan}

1. Implement the plan on the branch ${branch}. Commit your work on the branch.
2. Record each deviation from the plan, with its reason, in deviations.
3. Edit only the files that the task lists. If you must edit another file, or you meet a conflict with the plan, the task, or the code, stop and return conflict with the reason. Do not guess. Otherwise return an empty conflict.
4. Obey each STOP condition of the plan.
5. Return head_sha from \`git rev-parse HEAD\` after your last commit.`,
  IMPLEMENT,
)
if (impl.failed) return stop(impl.failed)
if (impl.question) return result('question', impl.question)
deviations.push(...list(impl.value.deviations))
if (impl.value.head_sha) head = impl.value.head_sha
if (impl.value.conflict) {
  deviations.push(`CONFLICT: ${impl.value.conflict}`)
  return result('stopped')
}

// 3 and 4. Review, refute, fix, and review again, up to the round cap.
for (let round = 1; ; round++) {
  phase('Review')
  log(`Review round ${round} of ${cap}`)
  const [adv, inv, gates] = await parallel([
    () => agent(`${CONTEXT}\n\nStep: adversarial review, round ${round}. Do not edit files.\n${DIFF}\nTry to refute the change: find where it is wrong, where it is incomplete, and where it does not meet the acceptance criteria. Report each finding with the file, the line of the problem, and a summary. Report nothing that you cannot show in the code.`, { label: `adversarial ${round}`, effort: 'low', schema: FINDINGS }),
    () => agent(`${CONTEXT}\n\nStep: invariant check, round ${round}. Do not edit files.\n${DIFF}\nDo not trust the claims of the author. List the invariants that the code must keep, from the code, the tests, and the docs. Verify each invariant against the changed code. Report each broken invariant as a finding with the file, the line, and a summary.`, { label: `invariants ${round}`, effort: 'low', schema: FINDINGS }),
    () => agent(`${CONTEXT}\n\nStep: gates, round ${round}. Do not edit files.\n${DIFF}\nRun each gate command, in this order:\n${bullets(gateCommands)}\nEach gate is a required suite. For each gate, return its exit code and how many tests ran, passed, failed, and were skipped. Read the counts from the output; do not estimate. For each failed or skipped test, give the file and the line of the test, a summary, and the kind (failed or skipped). Return tests with the totals of all gates, and head_sha from \`git rev-parse HEAD\`.`, { label: `gates ${round}`, effort: 'low', schema: GATES }),
  ])
  if (!adv || !inv || !gates) return stop(`a review check of round ${round} did not return a result`)
  tests = { ran: gates.tests.ran, passed: gates.tests.passed, failed: gates.tests.failed, skipped: gates.tests.skipped }
  if (gates.head_sha) head = gates.head_sha

  // Deduplicate by file:line (first occurrence wins) and drop refuted keys.
  const fresh = new Map()
  for (const f of [...adv.findings, ...inv.findings, ...gateFindings(gates)]) {
    const k = key(f)
    if (!fresh.has(k) && !refuted.has(k)) fresh.set(k, f)
  }
  const candidates = [...fresh.values()]
  const verdicts = await parallel(candidates.map((f) => () => agent(
    `${CONTEXT}\n\nStep: refute one finding. Do not edit files.\n${DIFF}\nFinding: ${key(f)}: ${f.summary}\n${f.gate ? `This finding comes from the gate command \`${f.gate}\`. Run it again. Confirm the finding only when the new run still shows the failure or the skip.\n` : ''}Try to refute the finding against the code. Confirm it only when you can show it. When you are not sure, return confirmed = false.`,
    { label: `refute ${key(f)}`, effort: 'low', schema: VERDICT },
  )))

  // Each finding confirmed before and not confirmed again is gone from the code.
  const confirmed = []
  candidates.forEach((f, i) => {
    const k = key(f)
    if (verdicts[i] && verdicts[i].confirmed === true) {
      confirmed.push(f)
      found.set(k, { file: f.file, line: f.line, summary: f.summary, state: 'open' })
    } else {
      refuted.add(k)
      found.set(k, { file: f.file, line: f.line, summary: f.summary, state: 'refuted' })
    }
  })
  const now = new Set(confirmed.map(key))
  for (const [k, f] of found) if (f.state === 'open' && !now.has(k)) f.state = 'fixed'

  if (!confirmed.length) return result('done')
  if (round >= cap) return result('findings_left')

  // Fix one area at a time: parallel fixers in one tree collide.
  phase('Fix')
  const areas = new Map()
  for (const f of confirmed) areas.set(area(f), [...(areas.get(area(f)) || []), f])
  for (const [name, batch] of areas) {
    const fix = await askable(
      `fix ${name}`,
      `Step: fix these confirmed findings of the area ${name}:
${batch.map((f) => `- ${key(f)}: ${f.summary}`).join('\n')}

1. Fix each finding at its root. Edit only the files that the task lists.
2. Commit your work on the branch ${branch}.
3. Return fixed with the file and the line of each finding that you fixed, deviations with each deviation and its reason, and head_sha from \`git rev-parse HEAD\`.`,
      FIX,
    )
    if (fix.failed) return stop(fix.failed)
    if (fix.question) return result('question', fix.question)
    deviations.push(...list(fix.value.deviations))
    if (fix.value.head_sha) head = fix.value.head_sha
    for (const loc of fix.value.fixed || []) {
      const f = found.get(key(loc))
      if (f) f.state = 'fixed'
    }
  }
}
