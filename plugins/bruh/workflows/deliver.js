export const meta = {
  name: 'deliver',
  description: 'Deliver one bruh task: plan, implement, review, refute, fix, and review again.',
  whenToUse: 'A bruh clerk runs it with the args of its start message.',
  phases: [
    { title: 'Plan', detail: 'pin the base SHA, write a plan with STOP conditions' },
    { title: 'Implement', detail: 'implement the plan, record deviations, stop at a conflict' },
    { title: 'Review', detail: 'adversarial reviewer, invariant checker, simplicity reviewer, and gates; one refuter for each finding' },
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
// The gates that are test suites; only they must run tests. A lint or build gate has no test count.
const testGates = list(A.test_gates)
const cap = Number.isInteger(A.round_cap) && A.round_cap >= 1 ? A.round_cap : 2
const deadline = typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0 ? A.deadline_seconds : 3600
const answers = A.answers && typeof A.answers === 'object' ? A.answers : {}
const decided = A.decided && typeof A.decided === 'object' ? A.decided : {}
const isPath = (p) => typeof p === 'string' && p.startsWith('/') && !/['\n]/.test(p)
// The guides folder of the task (an absolute path with INDEX.md), or '' for none.
const guides = typeof A.guides === 'string' ? A.guides.trim().replace(/\/+$/, '') : ''

let head = base
let tests = { ran: 0, passed: 0, failed: 0, skipped: 0 }
const deviations = []
const found = new Map() // key -> {id, file, line, summary, state, gate}
let seq = 0 // the last finding ID: a finding gets F<n> when it first enters found and keeps it
const refuted = new Set() // keys of refuted review findings; gate findings never go here

const norm = (file) => String(file || '').replace(/^(\.\/)+/, '')
const key = (f) => (f.key ? f.key : `${norm(f.file)}:${f.line}`)

// A small deterministic string hash (djb2), for the dedup key of a gate failure.
function hash(text) {
  let h = 5381
  for (let i = 0; i < text.length; i++) h = ((h * 33) ^ text.charCodeAt(i)) >>> 0
  return h.toString(16)
}

function result(status, question = null) {
  const findings = [...found.values()].map(({ file, line, summary, state }) => ({ file, line, summary, state }))
  return { status, branch, base_sha: base, head_sha: head, tests, findings, question, deviations }
}

// STOP: the task cannot go on as written. FAILED: an agent did not return a
// result (for example at a usage limit); the clerk relaunches with resumeFromRunId.
function stop(reason) {
  deviations.push(`STOP: ${reason}`)
  return result('stopped')
}

function fail(reason) {
  deviations.push(`FAILED: ${reason}`)
  return result('stopped')
}

const problems = []
if (!task) problems.push('args.task is empty')
if (!/^[0-9a-f]{40}$/.test(base)) problems.push('args.base_sha is not a 40-character hex SHA')
if (!branch) problems.push('args.branch is empty')
if (!Array.isArray(A.gates)) problems.push('args.gates is not a list')
else if (!gateCommands.length) problems.push('args.gates is empty; a run without a required suite cannot prove the change')
if (A.test_gates !== undefined && !Array.isArray(A.test_gates)) problems.push('args.test_gates is not a list')
else if (testGates.some((c) => !gateCommands.includes(c))) problems.push('args.test_gates has a command that is not in args.gates')
if (guides && !isPath(guides)) problems.push('args.guides is not empty and not an absolute path')
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

// With a guides folder, the reviewers and the refuters cite its rules. Without one, both
// strings are '' and the prompts stay byte for byte, so resumeFromRunId keeps its cache.
const GUIDES = guides
  ? `\nGUIDES ARE MANDATORY: before you read the diff, read ${guides}/INDEX.md and each guide file that it names for the files of the diff, in full. A review without them is invalid. Cite the guide file and the rule ID or heading for each finding, or a house rule, or a concrete failing scenario. Drop a finding that has none of them. If a technology of the diff has no guide, say so; do not review it from memory. The accepted deviations of ${guides}/INDEX.md are not findings.`
  : ''
const REFUTE_GUIDES = guides
  ? `${GUIDES}\nRead the cited guide rule under ${guides}/. Refute a finding that cites no guide rule, no house rule, and no concrete failing scenario.`
  : ''

const ASK = `Questions: when you need a decision that the task, the plan, the house rules, and the code do not answer, do not guess.
1. Call the bruh MCP tool question_open (mcp__plugin_bruh_bruh__question_open; load it with ToolSearch) with priority (P0, P1, or P2: your estimate), subject (one line), body (the question, the options ranked, and your recommendation), and blocks (the work that waits for the answer), and options (2 to 4, each a label and a description) when the question has fixed answers.
2. Send the returned header, and only the header, to main with SendMessage.
3. Call answer_wait (mcp__plugin_bruh_bruh__answer_wait) with question_id set to the returned id and deadline_seconds ${deadline}.
4. If it returns answered, use the answer text and continue.
5. If it returns pending, stop at once and do no more work. Return your result with question set to the id and the header of the question, and the body that question_open returned (with its OPTION lines).`

// Task 30: an agent at the start of a step writes the phase of the task to the report of the
// clerk, so the board shows the step. Kind event: no notice to bigm.
const PHASE = (p) => `First call the bruh MCP tool report_write (mcp__plugin_bruh_bruh__report_write; load it with ToolSearch) with kind event, text "phase ${p}", and phase "${p}". If the call fails, go on. Then do this step.`

const QUESTION = {
  type: 'object',
  properties: { id: { type: 'string' }, header: { type: 'string' }, body: { type: 'string' } },
  required: ['id', 'header', 'body'],
}
// A fixed finding: its ID closes it; file and line are for display only (a fix can move the line).
const FIXED = {
  type: 'object',
  properties: { id: { type: 'string' }, file: { type: 'string' }, line: { type: 'integer' } },
  required: ['id'],
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
    tests: { type: 'object', properties: COUNTS, required: ['ran', 'passed', 'failed', 'skipped'] },
    gates: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          command: { type: 'string' },
          exit_code: { type: 'integer' },
          ...COUNTS,
          output_tail: { type: 'string' },
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
        required: ['command', 'exit_code', 'ran', 'passed', 'failed', 'skipped', 'output_tail', 'problems'],
      },
    },
  },
  required: ['tests', 'gates'],
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
    fixed: { type: 'array', items: FIXED },
    deviations: { type: 'array', items: { type: 'string' } },
    question: QUESTION,
  },
  required: ['head_sha', 'fixed', 'deviations'],
}
const DONE = { type: 'object', properties: { ok: { type: 'boolean' } }, required: ['ok'] }
const VERDICT = {
  type: 'object',
  properties: { confirmed: { type: 'boolean' }, reason: { type: 'string' } },
  required: ['confirmed', 'reason'],
}

// The answers of the clerk for one question ID: a string, or a list of
// strings when the clerk answered the same question more than one time.
function answerList(id, from = answers) {
  const a = from[id]
  if (typeof a === 'string') return [a]
  return Array.isArray(a) ? a.filter((x) => typeof x === 'string') : []
}

// The answers of args.decided, for each askable prompt. A clerk sets it only
// in a new run, which has no earlier questions; a resume keeps its prompts.
const decidedLines = Object.keys(decided).flatMap((id) => answerList(id, decided).map((t) => `- ${id}: ${t}`))
const DECIDED = decidedLines.length ? `\n\nDecisions already made:\n${decidedLines.join('\n')}` : ''

// Runs a step that can ask a question. The first attempt has the plain prompt.
// Each later attempt adds the earlier questions of this step and their answers
// from args.answers, in order, so the prompts of the earlier attempts never
// change. Returns {value}, {question, repeat}, {stop}, or {failed}.
async function askable(label, prompt, schema) {
  const given = [] // {q, text}, in the order of the attempts
  for (;;) {
    const earlier = given.length
      ? `\n\nEarlier questions of this step and their answers. Check the working tree first: work of an earlier attempt can be in it already.\n${given.map((g) => `- ${g.q.id} (${g.q.header}): ${g.text}`).join('\n')}`
      : ''
    const r = await agent(`${CONTEXT}\n\n${prompt}${DECIDED}${earlier}\n\n${ASK}`, {
      label: given.length ? `${label} attempt ${given.length + 1}` : label,
      effort: 'high',
      schema,
    })
    if (!r) return { failed: `the ${label} agent did not return a result` }
    if (!r.question) return { value: r }
    const q = { id: r.question.id, header: r.question.header, body: r.question.body }
    const used = given.filter((g) => g.q.id === q.id).length
    const list = answerList(q.id)
    if (used >= MAX_ANSWERS) return { stop: `${q.id} is pending again after ${used} answers` }
    if (used < list.length) {
      given.push({ q, text: list[used] })
      continue
    }
    return { question: q, repeat: used }
  }
}

// A question that is pending again after its answer goes back to the clerk
// with a counter; after MAX_ANSWERS answers the run stops.
const MAX_ANSWERS = 3
function asked(r) {
  if (r.repeat) deviations.push(`REPEAT: ${r.question.id} ${r.repeat} of ${MAX_ANSWERS - 1}`)
  return result('question', r.question)
}

// Each gate is a required suite: each failed or skipped test is a finding.
// The key names the gate and the failing test, or a hash of the failing
// output, so a new failure of the same command is a new finding.
function gateFindings(g) {
  const out = []
  for (const gate of g.gates || []) {
    const named = gate.problems || []
    for (const p of named) {
      const kind = p.kind === 'skipped' ? 'Skipped' : 'Failed'
      out.push({
        file: norm(p.file) || `gate: ${gate.command}`,
        line: p.line || 0,
        summary: `${kind} test in the required suite \`${gate.command}\`: ${p.summary}`,
        gate: gate.command,
        key: `gate ${gate.command} ${kind} ${norm(p.file)}:${p.line || 0} ${p.summary}`,
      })
    }
    const unnamed = []
    if (gate.skipped > 0 && !named.some((p) => p.kind === 'skipped')) unnamed.push(`${gate.skipped} skipped`)
    if ((gate.failed > 0 || gate.exit_code !== 0) && !named.some((p) => p.kind === 'failed')) {
      unnamed.push(`exit code ${gate.exit_code}, ${gate.failed} failed`)
    }
    if (unnamed.length) {
      const text = `${unnamed.join(' and ')} ${gate.output_tail || ''}`
      out.push({
        file: `gate: ${gate.command}`,
        line: 0,
        summary: `The required suite \`${gate.command}\` has ${unnamed.join(' and ')}.`,
        gate: gate.command,
        key: `gate ${gate.command} #${hash(text)}`,
      })
    }
  }
  // The gate results must cover exactly args.gates, and each gate of args.test_gates must run tests.
  const reported = new Set((g.gates || []).map((x) => x.command))
  for (const cmd of gateCommands) {
    if (!reported.has(cmd)) {
      out.push({ file: `gate: ${cmd}`, line: 0, summary: `The required suite \`${cmd}\` has no result: the gate check did not run it.`, gate: cmd, key: `gate ${cmd} missing` })
    }
  }
  for (const x of g.gates || []) {
    if (!gateCommands.includes(x.command)) {
      out.push({ file: `gate: ${x.command}`, line: 0, summary: `The gate check returned \`${x.command}\`, which is not a gate of the task.`, gate: x.command, key: `gate ${x.command} extra` })
    } else if (testGates.includes(x.command) && !(x.ran > 0)) {
      out.push({ file: `gate: ${x.command}`, line: 0, summary: `The required suite \`${x.command}\` ran no tests.`, gate: x.command, key: `gate ${x.command} ran 0` })
    }
  }
  return out
}

// A gate command is clean when its result exits 0, runs tests when it is in
// args.test_gates, and has no failed and no skipped test. A command that is not
// in args.gates is clean only when the gate check did not return it.
function gateClean(g, cmd) {
  const x = (g.gates || []).find((y) => y.command === cmd)
  if (!gateCommands.includes(cmd)) return !x
  return Boolean(x) && x.exit_code === 0 && x.failed === 0 && x.skipped === 0 && (x.ran > 0 || !testGates.includes(cmd))
}
const clean = (g) => gateCommands.every((cmd) => gateClean(g, cmd)) && (g.gates || []).every((x) => gateCommands.includes(x.command))
const sum = (g, k) => (g.gates || []).reduce((n, x) => n + (Number.isInteger(x[k]) ? x[k] : 0), 0)

const area = (f) => (f.file.startsWith('gate: ') ? 'gates' : f.file.includes('/') ? f.file.slice(0, f.file.indexOf('/')) : '.')

// 1. Plan
phase('Plan')
const plan = await askable(
  'plan',
  `Step: plan. Do not edit files in this step.
${PHASE('plan')}
1. Read the task, the code, and the guides of the project.
2. Pin the base SHA: run \`git cat-file -e ${base}^{commit}\` and \`git merge-base --is-ancestor ${base} HEAD\`. If one fails, return stop = true with the reason.
3. Write a plan: numbered steps, and for each step a written STOP condition (the observation that makes the implementer stop instead of guessing).
4. If the task cannot be done as written, return stop = true with the reason. Otherwise return stop = false and an empty reason.`,
  PLAN,
)
if (plan.failed) return fail(plan.failed)
if (plan.stop) return stop(plan.stop)
if (plan.question) return asked(plan)
if (plan.value.stop) return stop(plan.value.reason || 'the plan agent stopped')

// 2. Implement
phase('Implement')
const impl = await askable(
  'implement',
  `Step: implement.
${PHASE('implement')}

Plan:
${plan.value.plan}

1. Implement the plan on the branch ${branch}. Commit your work on the branch.
2. Record each deviation from the plan, with its reason, in deviations.
3. Edit only the files that the task lists. If you must edit another file, or you meet a conflict with the plan, the task, or the code, stop and return conflict with the reason. Do not guess. Otherwise return an empty conflict.
4. Obey each STOP condition of the plan.
5. After your last commit, read head_sha with a plain single \`git rev-parse HEAD\` Bash call, never chained with another command, and return it.

The gates of the task (the review runs each alone, as a single plain Bash call; run them the same way, never chained with git):
${bullets(gateCommands)}`,
  IMPLEMENT,
)
if (impl.failed) return fail(impl.failed)
if (impl.stop) return stop(impl.stop)
if (impl.question) return asked(impl)
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
  const [adv, simp, inv, gates] = await parallel([
    () => agent(`${CONTEXT}\n\nStep: adversarial review, round ${round}. Do not edit files.\n${PHASE('review')}\n${DIFF}${GUIDES}\nTry to refute the change: find where it is wrong, where it is incomplete, and where it does not meet the acceptance criteria. Report each finding with the file, the line of the problem, and a summary. Report nothing that you cannot show in the code.`, { label: `adversarial ${round}`, effort: 'low', schema: FINDINGS }),
    // Task 45 (owner Q-219): the simplicity reviewer. The simpler replacement goes in the summary.
    // The two lists come from skills/implement/references/simplicity.md; deliver.test.mjs checks that they agree.
    () => agent(`${CONTEXT}\n\nStep: simplicity review, round ${round}. Do not edit files.\n${DIFF}${GUIDES}\nFind where the change is more complex than the task needs. Finding kinds: an abstraction that the task did not ask for (for example, an interface that only one type implements, a factory that makes only one kind of object, or an option for a constant); code for a speculative need; a re-implemented standard library function, or a helper that the codebase has already; a new dependency that a few lines can replace; scaffolding for later; a diff that is longer than the problem needs. Report each finding with the file, the line, and a summary that ends with a concrete simpler replacement. A simplicity finding never overrides the acceptance criteria or a deliberate choice. Do not report: something that the task or the acceptance criteria ask for; a deliberate choice of the task; input validation at a trust boundary; error handling that prevents data loss; a security check; accessibility basics; the test that the task needs. Report nothing that you cannot show in the code.`, { label: `simplicity ${round}`, effort: 'low', schema: FINDINGS }),
    () => agent(`${CONTEXT}\n\nStep: invariant check, round ${round}. Do not edit files.\n${DIFF}${GUIDES}\nDo not trust the claims of the author. List the invariants that the code must keep, from the code, the tests, and the docs. Verify each invariant against the changed code. Report each broken invariant as a finding with the file, the line, and a summary.`, { label: `invariants ${round}`, effort: 'low', schema: FINDINGS }),
    // Task 41 part 2 (Q-207): the gate agent runs no git; head comes from the step that made the last commit.
    () => agent(`${CONTEXT}\n\nStep: gates, round ${round}. Do not edit files.\nThe head SHA of the change is ${head}, from the step that made the last commit; do not run git.\nRun each gate command alone, as a single plain Bash call, in this order:\n${bullets(gateCommands)}\nEach gate is required: it must exit 0. These gates are test suites and must run tests:\n${bullets(testGates)}\nA gate that is not a test suite (for example lint or build) returns 0 for each count when its output has no test count. When the output of a Go test gate has no test count (go test without -v prints only \`ok <pkg>\` or \`ok <pkg> (cached)\`), count its tests with a second plain run of the same go test part, from the same directory, with -count=1 -v added, as a single Bash call. Read ran, passed, failed, and skipped from its \`--- PASS\`, \`--- FAIL\`, and \`--- SKIP\` lines. The exit code stays that of the gate command. Return exactly one result for each gate command above, with the command text unchanged, and run no other gate. For each gate, return its exit code and how many tests ran, passed, failed, and were skipped. Read the counts from the output; do not estimate. For each failed or skipped test, give the file and the line of the test, a summary, and the kind (failed or skipped). When a gate fails or skips a test, return the last 40 lines of its output in output_tail; otherwise return an empty output_tail. Return tests with the totals of all gates.`, { label: `gates ${round}`, effort: 'low', schema: GATES }),
  ])
  if (!adv || !simp || !inv || !gates) return fail(`a review check of round ${round} did not return a result`)
  // The totals come from the per-gate counts, added in code.
  tests = { ran: sum(gates, 'ran'), passed: sum(gates, 'passed'), failed: sum(gates, 'failed'), skipped: sum(gates, 'skipped') }

  // Deduplicate by key (file:line for review findings; first occurrence wins).
  // Refuted review findings stay refuted. Gate findings always get a refuter.
  const fresh = new Map()
  for (const f of [...adv.findings, ...inv.findings, ...simp.findings]) {
    const k = key(f)
    if (!fresh.has(k) && !refuted.has(k)) fresh.set(k, { ...f, file: norm(f.file) })
  }
  const gateNow = gateFindings(gates)
  // An open gate finding of an earlier round is fixed only when its gate is
  // clean now. When the gate is still red, a finding of this round replaces it.
  const gateKeys = new Set(gateNow.map(key))
  for (const [k, f] of found) {
    if (!f.gate || f.state !== 'open' || gateKeys.has(k)) continue
    if (gateClean(gates, f.gate)) f.state = 'fixed'
    else found.delete(k)
  }
  for (const f of gateNow) if (!fresh.has(key(f))) fresh.set(key(f), f)
  const candidates = [...fresh.values()]
  log(`Round ${round}: ${candidates.length} findings, one refuter each`)
  const verdicts = await parallel(candidates.map((f) => () => agent(
    `${CONTEXT}\n\nStep: refute one finding. Do not edit files.\n${DIFF}${REFUTE_GUIDES}\nFinding: ${f.file}:${f.line}: ${f.summary}\n${f.gate ? `This finding comes from the gate command \`${f.gate}\`. Run it again. Confirm the finding only when the new run still shows the failure or the skip.\n` : ''}Try to refute the finding against the code. Confirm it only when you can show it. When you are not sure, return confirmed = false.`,
    { label: `refute ${f.file}:${f.line}`, effort: 'low', schema: VERDICT },
  )))

  // A refuter that returned nothing gave no verdict: the finding stays open
  // and the run stops, so the clerk relaunches it (spec 15).
  let dead = 0
  candidates.forEach((f, i) => {
    const k = key(f)
    const prior = found.get(k)
    const entry = { id: prior ? prior.id : `F${++seq}`, file: f.file, line: f.line, summary: f.summary, gate: f.gate }
    if (!verdicts[i]) {
      dead++
      found.set(k, { ...entry, state: 'open' })
    } else if (verdicts[i].confirmed === true) {
      found.set(k, { ...entry, state: 'open' })
    } else {
      if (!f.gate) refuted.add(k)
      found.set(k, { ...entry, state: 'refuted' })
    }
  })
  if (dead) return fail(`${dead} refuter(s) of round ${round} did not return a result`)

  // A real gate failure (exit not 0, a failed or skipped test) is a fact of this round: at the end
  // of the run it stays open even when its refuter did not confirm it. A refuted gate finding of
  // another kind (ran 0, a missing or extra result) stays refuted (task 49).
  const open = [...found.entries()].filter(([, f]) => f.state === 'open')
  if (!open.length && clean(gates)) {
    // The phase line is a board mark, not evidence: a dead agent does not change the status.
    await agent(`${CONTEXT}\n\nStep: phase done. Do not edit files. Do nothing else.\n${PHASE('done')}\nReturn ok = true.`, { label: 'phase done', effort: 'low', schema: DONE })
    return result('done')
  }
  if (round >= cap) {
    for (const f of gateNow) {
      if ((gates.gates || []).some((x) => x.command === f.gate && (x.exit_code !== 0 || x.failed > 0 || x.skipped > 0))) found.get(key(f)).state = 'open'
    }
    return result('findings_left')
  }
  if (!open.length) continue // only refuted gate failures: run the gates again in the next round

  // Fix one area at a time: parallel fixers in one tree collide.
  phase('Fix')
  const areas = new Map()
  for (const [, f] of open) areas.set(area(f), [...(areas.get(area(f)) || []), f])
  let first = true
  for (const [name, batch] of areas) {
    const fix = await askable(
      `fix ${name}`,
      `${first ? `${PHASE('fix')}\n` : ''}Step: fix these confirmed findings of the area ${name}:
${batch.map((f) => `- [${f.id}] ${f.file}:${f.line}: ${f.summary}`).join('\n')}

1. Fix each finding at its root. Edit only the files that the task lists.
2. Commit your work on the branch ${branch}.
3. Return fixed with the ID (the [F<n>] before the finding) of each finding that you fixed, and its file and line now, deviations with each deviation and its reason, and head_sha, read after your last commit with a plain single \`git rev-parse HEAD\` Bash call, never chained with another command.`,
      FIX,
    )
    if (fix.failed) return fail(fix.failed)
    if (fix.stop) return stop(fix.stop)
    if (fix.question) return asked(fix)
    first = false
    deviations.push(...list(fix.value.deviations))
    if (fix.value.head_sha) head = fix.value.head_sha
    // Only a fix that the fixer reports makes a finding fixed. The ID closes it, because the fix
    // can move the line of the finding (task 32).
    for (const x of fix.value.fixed || []) {
      for (const f of batch) if (f.state === 'open' && f.id === x.id) f.state = 'fixed'
    }
  }
}
