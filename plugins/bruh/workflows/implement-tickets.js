export const meta = {
  name: 'implement-tickets',
  description: 'Implement tickets wave by wave: one implementer for each ticket, in parallel lanes inside a wave, one reviewer, a fix loop, the merge of the lanes, and a whole-branch gate. No commits and no posts.',
  whenToUse: 'Implement the tickets of a settled spec that /bruh:tickets wrote. Pass the waves of issues/INDEX.md. After a FAILED stop, relaunch as a new run with the remaining_waves and lane_tickets of the result, or with gate_only when the gate died; do not resume.',
  phases: [
    { title: 'Implement', detail: 'one implementer, one reviewer, and a fix loop for each ticket; parallel inside a wave' },
    { title: 'Merge', detail: 'patch the lanes of a wave back onto the shared tree, in wave order' },
    { title: 'Gate', detail: 'the gate commands over the whole branch' },
  ],
}

// The script has no clock and no filesystem. Agents do all reads and writes.
// Each helper is copied from deliver.js: the Workflow runtime has no imports.
// Prompts before a question never contain an answer, so a relaunch with
// resumeFromRunId after a question returns their cached results (spec 6.2). A
// wave with an open question is not merged, so a merge prompt never changes
// between the run and its relaunch. After a FAILED stop, relaunch as a new run
// with the remaining_waves of the result (the resume lesson of the skill). A
// ticket is merged only after its lane merge returned ok (a wave of one works in
// the shared tree, so its pass merges it); remaining_waves holds every ticket that
// is not merged, and lane_tickets names each of them that has a lane, so that a
// relaunch keeps working in that lane, also when the ticket is alone in its wave.
// When the whole-branch gate agent died, the result has gate_only = true, and the
// relaunch runs only the gate. The lanes of a run are under a folder named by a
// hash of args.root and args.spec, never by time or chance, so each prompt stays
// the same between a run and its relaunch.

const A = (typeof args === 'object' && args) || {}
const list = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [])
const isPath = (p) => typeof p === 'string' && p.startsWith('/') && !/['\n]/.test(p)
const TICKET_FILE = /^[A-Za-z0-9][A-Za-z0-9._-]*\.md$/

const root = isPath(A.root) ? A.root : ''
const issues = isPath(A.issues) ? A.issues.replace(/\/+$/, '') : ''
const gateCommands = list(A.gates)
const testGates = list(A.test_gates)
const fixCap = Number.isInteger(A.fix_cap) && A.fix_cap >= 0 ? A.fix_cap : 3
const deadline = typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0 ? A.deadline_seconds : 3600
const answers = A.answers && typeof A.answers === 'object' && !Array.isArray(A.answers) ? A.answers : {}
const decided = A.decided && typeof A.decided === 'object' && !Array.isArray(A.decided) ? A.decided : {}
const deviations = []
const tickets = []
let gate = null
let tests = { ran: 0, passed: 0, failed: 0, skipped: 0 }
let gateDied = false
const hadLane = new Set() // ticket file names that ran in a lane in this run

function result(status, question = null) {
  const merged = new Set(tickets.filter((t) => t.status === 'merged').map((t) => t.file.slice(t.file.lastIndexOf('/') + 1)))
  const remaining = waves.map((w) => w.filter((file) => !merged.has(file))).filter((w) => w.length)
  const withLane = new Set([...laneTickets, ...hadLane])
  const laneLeft = remaining.flat().filter((file) => withLane.has(file))
  return { status, tickets, remaining_waves: remaining, lane_tickets: laneLeft, gate_only: gateDied, gate, tests, question, deviations }
}
function stop(reason) {
  deviations.push(`STOP: ${reason}`)
  return result('stopped')
}
function fail(reason) {
  deviations.push(`FAILED: ${reason}`)
  return result('stopped')
}

const problems = []
if (!root) problems.push('args.root is not an absolute path')
for (const k of ['spec', 'guides', 'lane']) if (!isPath(A[k])) problems.push(`args.${k} is not an absolute path`)
if (!issues) problems.push('args.issues is not an absolute path')
if (!gateCommands.length) problems.push('args.gates is not a list of gate commands')
if (A.test_gates !== undefined && !Array.isArray(A.test_gates)) problems.push('args.test_gates is not a list')
else if (testGates.some((c) => !gateCommands.includes(c))) problems.push('args.test_gates has a command that is not in args.gates')
if (A.deadline_seconds !== undefined && !(typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0)) problems.push('args.deadline_seconds is not a positive number')
if (A.answers !== undefined && answers !== A.answers) problems.push('args.answers is not an object')
if (A.decided !== undefined && decided !== A.decided) problems.push('args.decided is not an object')
if (A.fix_cap !== undefined && !(Number.isInteger(A.fix_cap) && A.fix_cap >= 0)) problems.push('args.fix_cap is not an integer of 0 or more')
const gateOnly = A.gate_only === true
const laneTickets = new Set(list(A.lane_tickets))
const waves = Array.isArray(A.waves) && A.waves.every((w) => Array.isArray(w)) ? A.waves : []
if (A.gate_only !== undefined && typeof A.gate_only !== 'boolean') problems.push('args.gate_only is not a boolean')
if (gateOnly) {
  if (A.waves !== undefined && !(Array.isArray(A.waves) && A.waves.length === 0)) problems.push('args.gate_only needs no waves: pass an empty list or none')
  if (A.lane_tickets !== undefined && !(Array.isArray(A.lane_tickets) && A.lane_tickets.length === 0)) problems.push('args.gate_only needs no lane_tickets: pass an empty list or none')
} else if (!waves.length || !waves.every((w) => w.length)) problems.push('args.waves is not a list of lists of ticket file names')
else {
  const seen = new Set()
  for (const file of waves.flat()) {
    if (typeof file !== 'string' || !TICKET_FILE.test(file)) problems.push(`args.waves has a bad ticket file name: ${JSON.stringify(file)}`)
    else if (seen.has(file)) problems.push(`args.waves has ${file} twice`)
    else seen.add(file)
  }
}
if (A.lane_tickets !== undefined && !(Array.isArray(A.lane_tickets) && A.lane_tickets.every((f) => typeof f === 'string' && waves.flat().includes(f)))) problems.push('args.lane_tickets is not a list of ticket file names of args.waves')
if (problems.length) return stop(problems.join('; '))

const bullets = (xs) => (xs.length ? xs.map((x) => `- ${x}`).join('\n') : '(none)')
// The lanes of this run: a folder per args.root and args.spec, and LANE_RUN, so
// that lane.sh never keeps a lane of another repository or feature.
const RUN = hash(`${root}\n${A.spec}`)
const LANE = `ROOT='${root}' LANES="\${TMPDIR:-/tmp}/bruh-lanes/${RUN}" LANE_RUN='${RUN}' sh '${A.lane}'`

const COMMON = `Ground rules of this run (binding):
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of the shared tree ${root}. The owner or the clerk commits later.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails.
- Do not edit anything in the scratch folder that holds ${A.spec}, except where a step below says so.
- Change only the files that the ticket lists under Files. If the ticket is not possible without another file, do the minimum and name it in deviations.
- The spec is ${A.spec}. The ticket wins on detail; the spec wins on intent. Do not guess. When you need a decision, ask it as the Questions rule says. Return a conflict only for a contradiction between the ticket and the spec that no answer can settle.
- MODULES: when a ticket needs a module that the module file does not require yet, add it at the version that the spec or the ticket names, run the tidy command, and list the module files in files_changed with a one-line note in deviations. That is in scope, and reviewers must not flag it. The tidy command must be idempotent afterwards.
- Some tickets leave the build broken on purpose until a later ticket (the ticket text says so). Only the verify commands of the acceptance criteria of the ticket decide pass or fail.
- The guides are mandatory for each reviewer: the Review guides section of the ticket names files in ${A.guides}/.`

const ASK = `Questions: when you need a decision that the ticket, the spec, and the code do not answer, do not guess.
1. Call the bruh MCP tool question_open (mcp__plugin_bruh_bruh__question_open; load it with ToolSearch) with priority (P0, P1, or P2: your estimate), subject (one line), body (the question, the options ranked, and your recommendation), and blocks (the work that waits for the answer), and options (2 to 4, each a label and a description) when the question has fixed answers.
2. Send the returned header, and only the header, to main with SendMessage.
3. Call answer_wait (mcp__plugin_bruh_bruh__answer_wait) with question_id set to the returned id and deadline_seconds ${deadline}.
4. If it returns answered, use the answer text and continue.
5. If it returns pending, stop at once and do no more work. Return your result with question set to the id and the header of the question, and the body that question_open returned (with its OPTION lines).`

const QUESTION = {
  type: 'object',
  properties: { id: { type: 'string' }, header: { type: 'string' }, body: { type: 'string' } },
  required: ['id', 'header', 'body'],
}
const WORK = {
  type: 'object',
  properties: {
    question: QUESTION,
    done: { type: 'boolean' },
    workdir: { type: 'string' },
    summary: { type: 'string' },
    verify_output: { type: 'string' },
    files_changed: { type: 'array', items: { type: 'string' } },
    deviations: { type: 'string', description: 'each thing done outside the ticket text, or empty' },
    conflict: { type: 'string', description: 'a conflict between the ticket, the spec, and the code, or empty' },
  },
  required: ['done', 'workdir', 'summary', 'verify_output', 'files_changed', 'deviations', 'conflict'],
}
const REVIEW = {
  type: 'object',
  properties: {
    pass: { type: 'boolean' },
    findings: {
      type: 'array',
      items: { type: 'object', properties: { file: { type: 'string' }, problem: { type: 'string' }, fix: { type: 'string' } }, required: ['file', 'problem', 'fix'] },
    },
    verify_output: { type: 'string' },
    summary: { type: 'string' },
  },
  required: ['pass', 'findings', 'verify_output', 'summary'],
}
const MERGE = {
  type: 'object',
  properties: {
    ok: { type: 'boolean' },
    applied: { type: 'array', items: { type: 'string' }, description: 'the ticket IDs whose apply succeeded, in order' },
    report: { type: 'string' },
  },
  required: ['ok', 'applied', 'report'],
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

// The answers of the session for one question ID: a string, or a list of
// strings when the session answered the same question more than one time.
function answerList(id, from = answers) {
  const a = from[id]
  if (typeof a === 'string') return [a]
  return Array.isArray(a) ? a.filter((x) => typeof x === 'string') : []
}

// The answers of args.decided, for each askable prompt. A clerk sets it only
// in a new run, which has no earlier questions; a resume keeps its prompts.
const decidedLines = Object.keys(decided).flatMap((id) => answerList(id, decided).map((t) => `- ${id}: ${t}`))
const DECIDED = decidedLines.length ? `\n\nDecisions already made:\n${decidedLines.join('\n')}` : ''

// Runs a step that can ask a question (deliver.js). The first attempt has the
// plain prompt. Each later attempt adds the earlier questions of this step and
// their answers from args.answers, in order, so the prompts of the earlier
// attempts never change. Returns {value}, {question, repeat}, {stop}, or {failed}.
const MAX_ANSWERS = 3
async function askable(label, prompt, opts) {
  const given = []
  for (;;) {
    const earlier = given.length
      ? `\n\nEarlier questions of this step and their answers. Check the working folder first: work of an earlier attempt can be in it already.\n${given.map((g) => `- ${g.q.id} (${g.q.header}): ${g.text}`).join('\n')}`
      : ''
    const r = await agent(`${prompt}${DECIDED}${earlier}\n\n${ASK}`, { ...opts, label: given.length ? `${label} attempt ${given.length + 1}` : label })
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

// A small deterministic string hash (djb2), for the dedup key of a gate failure.
function hash(text) {
  let h = 5381
  for (let i = 0; i < text.length; i++) h = ((h * 33) ^ text.charCodeAt(i)) >>> 0
  return h.toString(16)
}

// The gate rules of deliver.js: each gate of args.gates is a required suite. A
// failed or skipped test is a finding; a missing or an extra gate result is a
// finding; a gate of args.test_gates that ran no test is a finding.
function gateFindings(g) {
  const out = []
  for (const x of g.gates || []) {
    const named = x.problems || []
    for (const p of named) out.push({ file: norm(p.file) || `gate: ${x.command}`, line: p.line || 0, summary: `${p.kind === 'skipped' ? 'Skipped' : 'Failed'} test in the required suite \`${x.command}\`: ${p.summary}` })
    const unnamed = []
    if (x.skipped > 0 && !named.some((p) => p.kind === 'skipped')) unnamed.push(`${x.skipped} skipped`)
    if ((x.failed > 0 || x.exit_code !== 0) && !named.some((p) => p.kind === 'failed')) unnamed.push(`exit code ${x.exit_code}, ${x.failed} failed`)
    if (unnamed.length) out.push({ file: `gate: ${x.command}`, line: 0, summary: `The required suite \`${x.command}\` has ${unnamed.join(' and ')}.`, key: `#${hash(x.output_tail || '')}` })
  }
  const reported = new Set((g.gates || []).map((x) => x.command))
  for (const cmd of gateCommands) if (!reported.has(cmd)) out.push({ file: `gate: ${cmd}`, line: 0, summary: `The required suite \`${cmd}\` has no result: the gate check did not run it.` })
  for (const x of g.gates || []) {
    if (!gateCommands.includes(x.command)) out.push({ file: `gate: ${x.command}`, line: 0, summary: `The gate check returned \`${x.command}\`, which is not a gate of the run.` })
    else if (testGates.includes(x.command) && !(x.ran > 0)) out.push({ file: `gate: ${x.command}`, line: 0, summary: `The required suite \`${x.command}\` ran no tests.` })
  }
  return out.map(({ key: _k, ...f }) => f)
}
const sum = (g, k) => (g.gates || []).reduce((n, x) => n + (Number.isInteger(x[k]) ? x[k] : 0), 0)
const norm = (file) => String(file || '').replace(/^(\.\/)+/, '')

const laneDiff = (workdir, useLane) => (useLane
  ? `In the lane, \`git -C '${workdir}' diff refs/lane/base\` plus the untracked files is exactly the work of this ticket.`
  : `See what changed with \`git -C '${workdir}' status --porcelain\` and \`git -C '${workdir}' diff\` (read the new files), and focus on the Files of the ticket.`)

function implPrompt(t, useLane) {
  return `You implement exactly one ticket of a larger change.

Ticket: ${t.file}. Read it in full. Read ${A.spec} for the intent. Read each existing file that the ticket lists under Files, and the files around them, before you write anything.

Working folder: ${useLane
    ? `run \`${LANE} start ${t.id}\` first. It prints the path of a private copy of the repository. When the lane exists already (an earlier attempt of this ticket, for example before a question), start keeps it with its work only when it was made for the same root, the same HEAD, and the same run; else start makes the lane again from the shared tree. Do all work and run all commands inside that copy. Do not touch ${root} in this ticket.`
    : `${root}. Work there. You are the only agent that edits it now.`}

Check first what is there already: an earlier run can have done part of the work, in the lane or in the shared tree. Do the work that the ticket describes and that is missing, and nothing more. For a ticket of Kind: test, the test must compile and fail against the stub (red); do not make it pass. For a ticket of Kind: implementation, make the paired test pass without an edit to the test. Format the files that you touched. Then run each verify command of the acceptance criteria and paste the real output.

${COMMON}

Return done = true only when each acceptance criterion holds. workdir is the absolute path of the folder where you worked. Return an empty conflict unless you stopped at a conflict.`
}

function reviewPrompt(t, workdir, useLane) {
  return `You review the implementation of one ticket. Be concrete: each finding names a file, the problem, and the exact fix.

Ticket: ${t.file}. Read it in full. Working folder with the changes: ${workdir}. ${laneDiff(workdir, useLane)}

Checks, in this order:
1. Scope: only the files under Files of the ticket changed. Report each other file.
2. Acceptance criteria: run each verify command of the ticket yourself in ${workdir} and paste the output. A ticket of Kind: test must compile and fail red. A ticket of Kind: implementation must make its paired test pass, with no edit to the test.
3. Signatures: they match the code block of the ticket byte for byte.
4. Spec: the change does not contradict ${A.spec}.
5. Guides, MANDATORY: the Review guides section of the ticket names files in ${A.guides}/. Read each of them in full before you judge the diff. A review that skipped them is invalid and must not pass. Apply their MUST rules to the diff, and cite the guide file and the rule for each finding. If the section is empty, or names a file that does not exist, fail the review with that finding. Report only material violations, not taste. The accepted deviations are in ${A.guides}/INDEX.md.
6. Conventions of the repository: comments explain why, not what; no secret or token in a log; errors wrapped with context; table tests where the neighbors use them.

If each check holds, set pass = true, and then edit the ticket file ${t.file}: change the line \`**Status:** ready-for-agent\` to \`**Status:** done\`. That is the only edit in the scratch folder that you can make. Do not edit other files.

${COMMON}`
}

function fixPrompt(t, workdir, useLane, findings) {
  return `You fix the review findings of the implementation of one ticket.

Ticket: ${t.file}. Working folder: ${workdir}. Read the ticket, then the current change. ${laneDiff(workdir, useLane)} Then apply each finding below. Do not change anything that is not flagged, unless a fix forces it. Run each verify command of the acceptance criteria again and paste the output.

Findings:
${findings.map((x, i) => `${i + 1}. ${x.file}: ${x.problem}\n   FIX: ${x.fix}`).join('\n')}

${COMMON}

Return done = true only when each acceptance criterion holds now. Return an empty conflict unless you stopped at a conflict.`
}

const entry = (t, status, extra = {}) => ({ id: t.id, file: t.file, status, workdir: '', rounds: 0, findings: [], deviations: '', question: null, ...extra })

// work runs an implementer or a fixer: an entry for an error, a stop, a
// question, or a conflict, or {value} with the work report.
async function work(t, label, prompt, extra) {
  const r = await askable(label, prompt, { phase: 'Implement', effort: 'high', schema: WORK })
  if (r.failed) return { entry: entry(t, 'error', { ...extra, deviations: r.failed }) }
  if (r.stop) return { entry: entry(t, 'error', { ...extra, deviations: `STOP: ${r.stop}` }) }
  if (r.question) return { entry: entry(t, 'question', { ...extra, question: r.question, repeat: r.repeat }) }
  if (r.value.conflict) return { entry: entry(t, 'conflict', { ...extra, deviations: r.value.conflict }) }
  return { value: r.value }
}

async function runTicket(t, useLane) {
  let w = await work(t, `impl ${t.id}`, implPrompt(t, useLane), {})
  if (w.entry) return w.entry
  const workdir = useLane ? w.value.workdir : root
  if (useLane && (!isPath(workdir) || workdir === root)) return entry(t, 'error', { deviations: `the implementer returned no lane folder: ${JSON.stringify(w.value.workdir)}` })
  let review = null
  for (let round = 0; ; round++) {
    review = await agent(reviewPrompt(t, workdir, useLane), { label: `review ${t.id}${round ? ` #${round + 1}` : ''}`, phase: 'Implement', effort: 'low', schema: REVIEW })
    if (!review) return entry(t, 'error', { workdir, rounds: round, deviations: 'the reviewer did not return a result' })
    if (review.pass) return entry(t, 'done', { workdir, rounds: round, deviations: w.value.deviations || '' })
    if (round >= fixCap) break
    w = await work(t, `fix ${t.id} #${round + 1}`, fixPrompt(t, workdir, useLane, review.findings || []), { workdir, rounds: round + 1 })
    if (w.entry) return w.entry
  }
  return entry(t, 'failed', { workdir, rounds: fixCap, findings: review.findings || [] })
}

for (let w = 0; w < (gateOnly ? 0 : waves.length); w++) {
  const n = w + 1
  // A ticket of a wave of more than one works in a lane, and so does a leftover
  // ticket of args.lane_tickets, also alone in its wave: its lane holds its work.
  const wave = waves[w].map((file) => ({ id: file.slice(0, -3), file: `${issues}/${file}`, name: file, lane: waves[w].length > 1 || laneTickets.has(file) }))
  phase('Implement')
  log(`Wave ${n}: ${wave.map((t) => `${t.id}${t.lane ? ' (lane)' : ''}`).join(', ')}`)
  for (const t of wave) if (t.lane) hadLane.add(t.name)
  const out = (await parallel(wave.map((t) => () => runTicket(t, t.lane)))).map((r, i) => r || entry(wave[i], 'error', { deviations: 'the ticket run threw' }))
  tickets.push(...out)
  // A ticket in the shared tree is merged by its pass; a lane ticket by its merge.
  out.forEach((r, i) => { if (!wave[i].lane && r.status === 'done') r.status = 'merged' })
  const good = out.filter((r, i) => wave[i].lane && r.status === 'done')
  const bad = out.filter((r) => r.status !== 'done' && r.status !== 'merged')
  const errors = bad.filter((r) => r.status === 'error')
  const questions = bad.filter((r) => r.status === 'question')

  // A wave with an open question and no error is not merged: the relaunch with
  // the answer returns the cached agents of the wave, and then merges it once.
  if (questions.length && !errors.length) {
    const q = questions[0]
    if (q.repeat) deviations.push(`REPEAT: ${q.question.id} ${q.repeat} of ${MAX_ANSWERS - 1}`)
    return result('question', q.question)
  }

  // Merge the done lanes one at a time, in wave order. A failed lane stays for a look.
  if (good.length) {
    phase('Merge')
    const merge = await agent(`Merge the finished lanes of wave ${n} back onto the shared tree ${root}, one at a time, in this order. Run each command and paste its output:
${good.map((r) => `- ${LANE} patch ${r.id}\n- ${LANE} apply ${r.id}\n- ${LANE} clean ${r.id}`).join('\n')}
Return applied with the ID of each ticket whose apply succeeded, in order. If an apply reports a conflict or a rejected hunk, stop at once, leave the tree as it is, and report the file and the hunk with ok = false.
Leave these failed lanes untouched (no patch, no clean): ${bad.map((r) => r.id).join(', ') || 'none'}.
After the last apply, run \`git -C '${root}' status --porcelain\` and paste it.

${COMMON}`, { label: `merge ${n}`, phase: 'Merge', effort: 'low', schema: MERGE })
    // A ticket is merged only when its lane is applied: after a dead merge agent,
    // each ticket of the wave stays in remaining_waves.
    if (!merge) return fail(`the merge agent of wave ${n} did not return a result; the tickets of the wave are not merged`)
    const applied = new Set(merge.ok ? good.map((r) => r.id) : list(merge.applied))
    for (const r of good) if (applied.has(r.id)) r.status = 'merged'
    if (!merge.ok) return stop(`the merge of wave ${n} failed: ${merge.report}`)
  }

  if (errors.length) return fail(`wave ${n}: ${errors.map((r) => `${r.id}: ${r.deviations}`).join('; ')}`)
  const conflicts = bad.filter((r) => r.status === 'conflict')
  if (conflicts.length) {
    for (const r of conflicts) deviations.push(`CONFLICT: ${r.id}: ${r.deviations}`)
    return result('stopped')
  }
  if (bad.length) {
    deviations.push(`wave ${n}: ${bad.map((r) => r.id).join(', ')} failed the review after ${fixCap} fix rounds`)
    return result('findings_left')
  }
}

phase('Gate')
const g = await agent(`Run the whole-branch gate in ${root}. Do not fix anything. Run each gate command, in this order:
${bullets(gateCommands)}
Each gate is required: it must exit 0. These gates are test suites and must run tests:
${bullets(testGates)}
A gate that is not a test suite (for example lint or build) returns 0 for each count when its output has no test count. Return exactly one result for each gate command above, with the command text unchanged, and run no other gate. For each gate, return its exit code and how many tests ran, passed, failed, and were skipped. Read the counts from the output; do not estimate. For each failed or skipped test, give the file and the line of the test, a summary, and the kind (failed or skipped). When a gate fails or skips a test, return the last 40 lines of its output in output_tail; otherwise return an empty output_tail.

${COMMON}`, { label: 'gate', phase: 'Gate', effort: 'low', schema: GATES })
if (!g) {
  gateDied = true
  return fail('the gate agent did not return a result')
}
tests = { ran: sum(g, 'ran'), passed: sum(g, 'passed'), failed: sum(g, 'failed'), skipped: sum(g, 'skipped') }
const findings = gateFindings(g)
gate = { ok: findings.length === 0, gates: g.gates || [], findings }
return result(gate.ok ? 'done' : 'findings_left')
