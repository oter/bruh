export const meta = {
  name: 'review-and-fix',
  description: 'Review an own change with several lenses and the gates, refute each finding, fix the confirmed findings one area at a time with an independent check of each fix, and review again up to the round cap. No commits and no posts.',
  whenToUse: 'Review and fix a change that bruh wrote, on a committed and clean branch. Commit the fixes afterwards. Save the result with result_save and post it with scripts/post-findings.sh after the approval of the owner or under a post grant.',
  phases: [
    { title: 'Check', detail: 'HEAD of root, a clean tree, and the base SHA as an ancestor' },
    { title: 'Review', detail: 'one reviewer for each lens, and the gate commands' },
    { title: 'Verify', detail: 'one refuter for each unique finding' },
    { title: 'Fix', detail: 'the confirmed findings, one area at a time, each fix checked by another agent' },
  ],
}

// The script has no clock and no filesystem. It returns the findings; a session
// posts them with post-findings.sh (spec 6.1: no agent of a workflow posts).
// Each helper is copied from deliver.js: the Workflow runtime has no imports.
// Prompts before a question never contain an answer, so a relaunch with
// resumeFromRunId returns their cached results (spec 6.2).

const A = (typeof args === 'object' && args) || {}
const list = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [])
const isPath = (p) => typeof p === 'string' && p.startsWith('/') && !/['\n]/.test(p)
const SHA = /^[0-9a-f]{40}$/
const norm = (file) => String(file || '').replace(/^(\.\/)+/, '')
const key = (f) => (f.key ? f.key : `${norm(f.file)}:${f.line}`)
const ORDER = { security: 0, bug: 1, guideline: 2, nit: 3 }
const RANK = { open: 2, fixed: 1, refuted: 0 }
const bySeverity = (a, b) => (ORDER[a.severity] ?? 4) - (ORDER[b.severity] ?? 4)

const root = isPath(A.root) ? A.root : ''
const base = typeof A.base === 'string' ? A.base : ''
let head = typeof A.head === 'string' ? A.head : ''
const lenses = Array.isArray(A.lenses) ? A.lenses : []
const gateCommands = list(A.gates)
// The gates that are test suites; only they must run tests. A lint or build gate has no test count.
const testGates = list(A.test_gates)
const cap = Number.isInteger(A.round_cap) && A.round_cap >= 1 ? A.round_cap : 2
const deadline = typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0 ? A.deadline_seconds : 3600
const answers = A.answers && typeof A.answers === 'object' && !Array.isArray(A.answers) ? A.answers : {}
const deviations = []
const summaries = new Map() // lens key -> summary of the last round
const found = new Map() // key -> finding with state open, fixed, or refuted
const refutedKeys = new Set()
let tests = { ran: 0, passed: 0, failed: 0, skipped: 0 }

function result(status, question = null) {
  const all = [...found.values()].map(({ key: _k, gate: _g, ...f }) => f)
  return {
    status,
    workflow: 'review-and-fix',
    base,
    head_sha: head,
    tests,
    summaries: [...summaries].map(([k, summary]) => ({ key: k, summary })),
    confirmed: all.filter((f) => f.state !== 'refuted').sort(bySeverity),
    refuted: all.filter((f) => f.state === 'refuted'),
    question,
    deviations,
  }
}
function stop(reason) {
  deviations.push(`STOP: ${reason}`)
  return result('stopped')
}
function fail(reason) {
  deviations.push(`FAILED: ${reason}`)
  return result('stopped')
}

function lensProblems(ls) {
  if (!Array.isArray(ls) || !ls.length) return ['args.lenses is not a list of {key, prompt}']
  const out = []
  const keys = new Set()
  for (const l of ls) {
    if (!l || typeof l.key !== 'string' || !/^[a-z0-9][a-z0-9-]*$/.test(l.key)) out.push(`args.lenses has a bad key: ${JSON.stringify(l && l.key)}`)
    else if (keys.has(l.key)) out.push(`args.lenses has the key ${l.key} twice`)
    else keys.add(l.key)
    if (!l || typeof l.prompt !== 'string' || !l.prompt.trim()) out.push('args.lenses has an empty prompt')
  }
  return out
}

const problems = []
if (!root) problems.push('args.root is not an absolute path')
if (!SHA.test(base)) problems.push('args.base is not a 40-character hex SHA')
if (A.head !== undefined && !SHA.test(head)) problems.push('args.head is not a 40-character hex SHA')
if (A.spec !== undefined && !isPath(A.spec)) problems.push('args.spec is not an absolute path')
if (!isPath(A.guides)) problems.push('args.guides is not an absolute path')
if (A.deliberate !== undefined && !Array.isArray(A.deliberate)) problems.push('args.deliberate is not a list')
if (!gateCommands.length) problems.push('args.gates is not a list of gate commands')
if (A.test_gates !== undefined && !Array.isArray(A.test_gates)) problems.push('args.test_gates is not a list')
else if (testGates.some((c) => !gateCommands.includes(c))) problems.push('args.test_gates has a command that is not in args.gates')
if (A.round_cap !== undefined && !(Number.isInteger(A.round_cap) && A.round_cap >= 1)) problems.push('args.round_cap is not a positive integer')
if (A.deadline_seconds !== undefined && !(typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0)) problems.push('args.deadline_seconds is not a positive number')
if (A.answers !== undefined && answers !== A.answers) problems.push('args.answers is not an object')
problems.push(...lensProblems(A.lenses))
if (problems.length) return stop(problems.join('; '))

const bullets = (xs) => (xs.length ? xs.map((x) => `- ${x}`).join('\n') : '(none)')

const RULES = `Rules of this run:
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of the tree. The clerk or the owner commits after the run.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails. The session posts the result later.
- Do not edit anything in the scratch folder that holds the spec.`

const ASK = `Questions: when you need a decision that the spec, the guides, the findings, and the code do not answer, do not guess.
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
const CHECK = {
  type: 'object',
  properties: { head_sha: { type: 'string' }, status_porcelain: { type: 'string' }, base_is_ancestor: { type: 'boolean' } },
  required: ['head_sha', 'status_porcelain', 'base_is_ancestor'],
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
        properties: {
          file: { type: 'string' }, line: { type: 'integer' },
          rule: { type: 'string', description: 'guide file and rule ID or heading' },
          problem: { type: 'string' }, fix: { type: 'string' },
          severity: { type: 'string', enum: ['security', 'bug', 'guideline', 'nit'] },
        },
        required: ['file', 'line', 'rule', 'problem', 'fix', 'severity'],
      },
    },
    summary: { type: 'string' },
  },
  required: ['findings', 'summary'],
}
const COUNTS = {
  ran: { type: 'integer' }, passed: { type: 'integer' }, failed: { type: 'integer' }, skipped: { type: 'integer' },
}
const GATES = {
  type: 'object',
  properties: {
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
  required: ['gates'],
}
const VERDICT = {
  type: 'object',
  properties: {
    confirmed: { type: 'boolean' }, reason: { type: 'string' }, adjusted_fix: { type: 'string' },
    file: { type: 'string', description: 'the correct file, only when the finding names the wrong one' },
    line: { type: 'integer', description: 'the correct 1-based line, only when the finding names the wrong one' },
  },
  required: ['confirmed', 'reason', 'adjusted_fix'],
}

// A refuter that confirms a finding can correct its location. The corrected
// file and line replace the ones of the reviewer, and the dedup runs after that.
function located(x, v) {
  const file = typeof v.file === 'string' && v.file.trim() ? norm(v.file.trim()) : x.file
  const line = Number.isInteger(v.line) && v.line > 0 ? v.line : x.line
  return { ...x, file, line }
}
const FIX = {
  type: 'object',
  properties: { fixed: { type: 'array', items: LOCATION }, report: { type: 'string' }, question: QUESTION },
  required: ['fixed', 'report'],
}
const CONFIRM = {
  type: 'object',
  properties: {
    results: {
      type: 'array',
      items: {
        type: 'object',
        properties: { file: { type: 'string' }, line: { type: 'integer' }, fixed: { type: 'boolean' }, reason: { type: 'string' } },
        required: ['file', 'line', 'fixed', 'reason'],
      },
    },
  },
  required: ['results'],
}

// The answers of the session for one question ID: a string, or a list of
// strings when the session answered the same question more than one time.
function answerList(id) {
  const a = answers[id]
  if (typeof a === 'string') return [a]
  return Array.isArray(a) ? a.filter((x) => typeof x === 'string') : []
}

// Runs a step that can ask a question (deliver.js). The first attempt has the
// plain prompt. Each later attempt adds the earlier questions of this step and
// their answers from args.answers, in order, so the prompts of the earlier
// attempts never change. Returns {value}, {question, repeat}, {stop}, or {failed}.
const MAX_ANSWERS = 3
async function askable(label, prompt, opts) {
  const given = []
  for (;;) {
    const earlier = given.length
      ? `\n\nEarlier questions of this step and their answers. Check the working tree first: work of an earlier attempt can be in it already.\n${given.map((g) => `- ${g.q.id} (${g.q.header}): ${g.text}`).join('\n')}`
      : ''
    const r = await agent(`${prompt}${earlier}\n\n${ASK}`, { ...opts, label: given.length ? `${label} attempt ${given.length + 1}` : label })
    if (!r) return { failed: `the ${label} agent did not return a result` }
    if (!r.question) return { value: r }
    const q = { id: r.question.id, header: r.question.header, body: r.question.body }
    const used = given.filter((g) => g.q.id === q.id).length
    const texts = answerList(q.id)
    if (used >= MAX_ANSWERS) return { stop: `${q.id} is pending again after ${used} answers` }
    if (used < texts.length) {
      given.push({ q, text: texts[used] })
      continue
    }
    return { question: q, repeat: used }
  }
}
function asked(r) {
  if (r.repeat) deviations.push(`REPEAT: ${r.question.id} ${r.repeat} of ${MAX_ANSWERS - 1}`)
  return result('question', r.question)
}

// A small deterministic string hash (djb2), for the dedup key of a gate failure.
function hash(text) {
  let h = 5381
  for (let i = 0; i < text.length; i++) h = ((h * 33) ^ text.charCodeAt(i)) >>> 0
  return h.toString(16)
}

// The gate rules of deliver.js: each gate of args.gates is a required suite. A
// failed or skipped test is a finding; a missing or an extra gate result is a
// finding; a gate of args.test_gates that ran no test is a finding. The key
// names the gate and the failing test, or a hash of the failing output.
function gateFindings(g) {
  const out = []
  const add = (cmd, file, line, summary, k, tail = '') => out.push({ file, line, summary, gate: cmd, key: k, output_tail: tail })
  for (const gate of g.gates || []) {
    const named = gate.problems || []
    for (const p of named) {
      const kind = p.kind === 'skipped' ? 'Skipped' : 'Failed'
      add(gate.command, norm(p.file) || `gate: ${gate.command}`, p.line || 0,
        `${kind} test in the required suite \`${gate.command}\`: ${p.summary}`, `gate ${gate.command} ${kind} ${norm(p.file)}:${p.line || 0} ${p.summary}`)
    }
    const unnamed = []
    if (gate.skipped > 0 && !named.some((p) => p.kind === 'skipped')) unnamed.push(`${gate.skipped} skipped`)
    if ((gate.failed > 0 || gate.exit_code !== 0) && !named.some((p) => p.kind === 'failed')) unnamed.push(`exit code ${gate.exit_code}, ${gate.failed} failed`)
    if (unnamed.length) {
      add(gate.command, `gate: ${gate.command}`, 0, `The required suite \`${gate.command}\` has ${unnamed.join(' and ')}.`,
        `gate ${gate.command} #${hash(`${unnamed.join(' and ')} ${gate.output_tail || ''}`)}`, gate.output_tail || '')
    }
  }
  const reported = new Set((g.gates || []).map((x) => x.command))
  for (const cmd of gateCommands) {
    if (!reported.has(cmd)) add(cmd, `gate: ${cmd}`, 0, `The required suite \`${cmd}\` has no result: the gate check did not run it.`, `gate ${cmd} missing`)
  }
  for (const x of g.gates || []) {
    if (!gateCommands.includes(x.command)) add(x.command, `gate: ${x.command}`, 0, `The gate check returned \`${x.command}\`, which is not a gate of the run.`, `gate ${x.command} extra`)
    else if (testGates.includes(x.command) && !(x.ran > 0)) add(x.command, `gate: ${x.command}`, 0, `The required suite \`${x.command}\` ran no tests.`, `gate ${x.command} ran 0`)
  }
  return out
}
function gateClean(g, cmd) {
  const x = (g.gates || []).find((y) => y.command === cmd)
  if (!gateCommands.includes(cmd)) return !x
  return Boolean(x) && x.exit_code === 0 && x.failed === 0 && x.skipped === 0 && (x.ran > 0 || !testGates.includes(cmd))
}
const clean = (g) => gateCommands.every((cmd) => gateClean(g, cmd)) && (g.gates || []).every((x) => gateCommands.includes(x.command))
const sum = (g, k) => (g.gates || []).reduce((n, x) => n + (Number.isInteger(x[k]) ? x[k] : 0), 0)

// An area is the first two folders of the path, so one top-level source folder
// does not become one batch.
function area(f) {
  if (f.gate) return 'gates'
  const parts = f.file.split('/')
  return parts.length > 2 ? parts.slice(0, 2).join('/') : parts.length === 2 ? parts[0] : '.'
}

// Check: the reviewers must read the commit that the result names.
phase('Check')
const check = await agent(`Step: check the tree. Do not edit files. Run these commands and return their output:
1. \`git -C '${root}' rev-parse HEAD\`: return it in head_sha.
2. \`git -C '${root}' status --porcelain\`: return its full output in status_porcelain (empty when the tree is clean).
3. \`git -C '${root}' merge-base --is-ancestor ${base} HEAD\`: return base_is_ancestor = true only when it exits 0.

${RULES}`, { label: 'check', phase: 'Check', effort: 'low', schema: CHECK })
if (!check) return fail('the check agent did not return a result')
if (!SHA.test(check.head_sha || '')) return stop(`HEAD of ${root} is not a commit: ${JSON.stringify(check.head_sha)}`)
if (A.head !== undefined && check.head_sha !== A.head) return stop(`HEAD of ${root} is ${check.head_sha}, not args.head ${A.head}`)
head = check.head_sha
if ((check.status_porcelain || '').trim()) return stop(`the tree of ${root} is not clean: commit it before the review: ${check.status_porcelain.trim()}`)
if (check.base_is_ancestor !== true) return stop(`the base ${base} is not an ancestor of ${head}`)

const COMMON = `Scope: the branch checked out in ${root}. At the start of the run it was committed and clean at ${head}. The fixes of this run stay in the working tree, not committed. The change is \`git -C '${root}' diff ${base}\` (the working tree against the pinned base SHA), plus the untracked files of \`git -C '${root}' status --porcelain\`. Never diff against origin/main or another moving ref. Read the full files that you comment on, not only the hunks.
Spec: ${A.spec ? `${A.spec}. The code must follow it.` : '(none)'}
GUIDES ARE MANDATORY: before you read the diff, read ${A.guides}/INDEX.md and each guide file that your lens names under ${A.guides}/, in full. A review without them is invalid. Cite the guide file and the rule ID or heading for each finding. Drop a finding that has no rule and no concrete failing scenario. If a technology of the diff has no guide, say so in your summary; do not review it from memory.
Report only findings that a reviewer would block on or ask a change for: bugs, security, guideline violations with a concrete rule, and readability problems with a concrete fix. No praise, no summary of what the code does, and no findings on generated code or mocks. The accepted deviations of ${A.guides}/INDEX.md are not findings.
Deliberate choices (do not report them as findings, do not undo them):
${bullets(list(A.deliberate))}
House rules:
${typeof A.house_rules === 'string' && A.house_rules.trim() ? A.house_rules : '(none)'}

${RULES}`

const GATE_PROMPT = (round) => `Step: gates, round ${round}. Do not edit files. In ${root}, run each gate command, in this order:
${bullets(gateCommands)}
Each gate is required: it must exit 0. These gates are test suites and must run tests:
${bullets(testGates)}
A gate that is not a test suite (for example lint or build) returns 0 for each count when its output has no test count. Return exactly one result for each gate command above, with the command text unchanged, and run no other gate. For each gate, return its exit code and how many tests ran, passed, failed, and were skipped. Read the counts from the output; do not estimate. For each failed or skipped test, give the file and the line of the test, a summary, and the kind (failed or skipped). When a gate fails or skips a test, return the last 40 lines of its output in output_tail; otherwise return an empty output_tail.

${COMMON}`

for (let round = 1; ; round++) {
  phase('Review')
  log(`Review round ${round} of ${cap}`)
  const checks = await parallel([
    ...lenses.map((l) => () => agent(
      `${l.prompt}\n\nReview round ${round}. Do not edit files.\n\n${COMMON}\n\nReturn the findings with file paths relative to ${root} and 1-based line numbers in the current file.`,
      { label: `review ${l.key}`, phase: 'Review', effort: 'low', schema: FINDINGS },
    )),
    () => agent(GATE_PROMPT(round), { label: `gate ${round}`, phase: 'Review', effort: 'low', schema: GATES }),
  ])
  if (checks.some((c) => !c)) return fail(`a review check of round ${round} did not return a result`)
  const gates = checks.pop()
  lenses.forEach((l, i) => summaries.set(l.key, checks[i].summary))
  // The totals come from the per-gate counts, added in code.
  tests = { ran: sum(gates, 'ran'), passed: sum(gates, 'passed'), failed: sum(gates, 'failed'), skipped: sum(gates, 'skipped') }

  // A gate finding is a fact of this round and gets no refuter: a gate result is
  // a source read. An open gate finding of an earlier round is fixed only when its
  // gate is clean now; when the gate is still red, a finding of this round replaces it.
  const gateNow = gateFindings(gates)
  const gateKeys = new Set(gateNow.map((f) => f.key))
  for (const [k, f] of found) {
    if (!f.gate || f.state !== 'open' || gateKeys.has(k)) continue
    if (gateClean(gates, f.gate)) f.state = 'fixed'
    else found.delete(k)
  }
  for (const f of gateNow) {
    found.set(f.key, {
      file: f.file, line: f.line, lens: 'gate', rule: 'gate', severity: 'bug', problem: f.summary,
      fix: 'Make the gate pass.', round, state: 'open', reason: 'gate result', gate: f.gate, key: f.key, output_tail: f.output_tail,
    })
  }

  // Deduplicate by file:line; the first finding wins and the problems of the
  // others are appended. A refuted finding stays refuted.
  const fresh = new Map()
  let dropped = 0
  lenses.forEach((l, i) => {
    for (const x of checks[i].findings) {
      const k = key(x)
      if (refutedKeys.has(k)) {
        dropped++
        continue
      }
      if (fresh.has(k)) fresh.get(k).problem += ` | also (${l.key}): ${x.problem}`
      else fresh.set(k, { file: norm(x.file), line: x.line, lens: l.key, rule: x.rule, severity: x.severity, problem: x.problem, fix: x.fix, round })
    }
  })
  if (dropped) log(`Round ${round}: ${dropped} findings at refuted file:line keys dropped`)
  const candidates = [...fresh.values()]
  log(`Round ${round}: ${candidates.length} findings, one refuter each`)

  phase('Verify')
  const verdicts = await parallel(candidates.map((x) => () => agent(
    `Try to refute this code review finding. Do not edit files. Read the file at the line, the code around it, the spec when the finding is about behavior, and the cited guide rule under ${A.guides}/. When the finding is an empirical claim, test it with a read-only command or a throwaway program outside ${root}. Confirm it only when the problem is real in this code and the fix is correct and proportionate. Refute it when the rule does not apply, the code already complies, the spec chose this behavior on purpose, the fix would break a test or the spec, or it is taste. When you are not sure, return confirmed = false. When you confirm it and the fix needs a change, give the better fix in adjusted_fix; else return an empty adjusted_fix. When you confirm it and the problem is at another line or in another file than the finding says, return the correct file and line; else leave them out.
Finding: ${JSON.stringify(x)}

${COMMON}`,
    { label: `verify ${x.file}:${x.line}`, phase: 'Verify', effort: 'low', schema: VERDICT },
  )))

  // A refuter that returned nothing gave no verdict: the run stops, and the
  // finding is neither confirmed nor refuted (deliver.js, final review B1).
  let dead = 0
  const byLocation = new Map()
  candidates.forEach((x, i) => {
    const v = verdicts[i]
    const k = key(x)
    if (!v) dead++
    else if (v.confirmed === true) {
      const y = { ...located(x, v), fix: v.adjusted_fix || x.fix, state: 'open', reason: v.reason }
      const yk = key(y)
      if (byLocation.has(yk)) byLocation.get(yk).problem += ` | also (${y.lens}): ${y.problem}`
      else byLocation.set(yk, y)
    } else {
      refutedKeys.add(k)
      // A refutation overwrites only a refuted entry: an open finding of an earlier
      // round stays open, and a fix of an earlier round stays on record.
      if (!found.has(k) || found.get(k).state === 'refuted') found.set(k, { ...x, state: 'refuted', reason: v.reason })
    }
  })
  // A confirmed finding at the key of an earlier finding (moved there by its
  // refuter, or reported there again) merges with it: the stronger state wins
  // (open, then fixed, then refuted), and both problems and reasons stay.
  for (const [k, y] of byLocation) {
    const prior = found.get(k)
    if (!prior || prior.gate) {
      found.set(k, y)
      continue
    }
    const state = RANK[prior.state] > RANK[y.state] ? prior.state : y.state
    const problem = prior.problem === y.problem ? y.problem : `${y.problem} | earlier (round ${prior.round}, ${prior.state}): ${prior.problem}`
    const reason = [y.reason, prior.reason].filter((r) => r && r !== 'gate result').filter((r, i, a) => a.indexOf(r) === i).join(' | ')
    found.set(k, { ...y, state, problem, reason })
  }
  if (dead) return fail(`${dead} refuter(s) of round ${round} did not return a result`)

  const open = [...found.values()].filter((x) => x.state === 'open')
  if (!open.length && clean(gates)) return result('done')
  if (round >= cap) return result('findings_left')
  if (!open.length) continue

  // Fix one area at a time: parallel fixers in one tree collide.
  phase('Fix')
  const areas = new Map()
  for (const x of open) areas.set(area(x), [...(areas.get(area(x)) || []), x])
  for (const [name, batch] of areas) {
    const fix = await askable(
      `fix ${name}`,
      `Step: fix these confirmed review findings of the area ${name} in ${root}. Read each file in full before you edit it. Keep each change minimal and in the spirit of the finding; do not refactor past it. When a finding needs a test change to stay honest, change the test too. When a fix touches a source that a generator reads, run the generator. If a finding cannot be fixed without breaking a test or the spec, leave it and say why in the report.
Findings:
${batch.map((x, i) => `${i + 1}. ${x.file}:${x.line} [${x.severity}] ${x.rule}\n   ${x.problem}\n   FIX: ${x.fix}`).join('\n')}

After the edits, run the gate commands:
${bullets(gateCommands)}
Return fixed with the file and the line of each finding that you fixed, and a report with the gate output and each finding that you left, with the reason.

${COMMON}`,
      { phase: 'Fix', effort: 'high', schema: FIX },
    )
    if (fix.failed) return fail(fix.failed)
    if (fix.stop) return stop(fix.stop)
    if (fix.question) return asked(fix)
    if (fix.value.report) deviations.push(`fix ${name}: ${fix.value.report}`)

    // An independent agent checks each fix that the fixer claims, in this batch
    // only. Only a checked fix is fixed. A gate finding is fixed only when its
    // gate is clean in a later round.
    const claims = fix.value.fixed || []
    const claimed = batch.filter((x) => !x.gate && claims.some((c) => norm(c.file) === x.file && c.line === x.line))
    if (!claimed.length) continue
    const conf = await agent(`Step: check the fixes of the area ${name}. Do not edit files. Do not trust the report of the fixer. For each finding below, read the change of this run (\`git -C '${root}' diff ${head}\`, plus the untracked files) and the current file, and decide whether the change fixes the problem at its root. Return one result for each finding, with its file and line unchanged. When you are not sure, return fixed = false.
Findings:
${claimed.map((x, i) => `${i + 1}. ${x.file}:${x.line}: ${x.problem}\n   FIX: ${x.fix}`).join('\n')}

${COMMON}`, { label: `confirm ${name}`, phase: 'Fix', effort: 'low', schema: CONFIRM })
    if (!conf) return fail(`the fix check of the area ${name} did not return a result`)
    for (const x of claimed) {
      const v = (conf.results || []).find((r) => norm(r.file) === x.file && r.line === x.line)
      if (v && v.fixed === true) x.state = 'fixed'
    }
  }
}
