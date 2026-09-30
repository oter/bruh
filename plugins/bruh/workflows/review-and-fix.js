export const meta = {
  name: 'review-and-fix',
  description: 'Review an own change with several lenses and the gates, refute each finding, fix the confirmed findings one area at a time, and review again up to the round cap. No commits and no posts.',
  whenToUse: 'Review and fix a change that bruh wrote, on a committed and clean branch. Commit the fixes afterwards, and post the saved result with scripts/post-findings.sh after the owner said yes or under a post grant.',
  phases: [
    { title: 'Review', detail: 'one reviewer for each lens, and the gate commands' },
    { title: 'Verify', detail: 'one refuter for each unique finding' },
    { title: 'Fix', detail: 'the confirmed findings, one area at a time' },
  ],
}

// The script has no clock and no filesystem. It returns the findings; a session
// posts them with post-findings.sh (spec 6.1: no agent of a workflow posts).
// Each helper is copied from deliver.js: the Workflow runtime has no imports.

const A = (typeof args === 'object' && args) || {}
const list = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [])
const isPath = (p) => typeof p === 'string' && p.startsWith('/') && !/['\n]/.test(p)
const SHA = /^[0-9a-f]{40}$/
const norm = (file) => String(file || '').replace(/^(\.\/)+/, '')
const key = (f) => `${norm(f.file)}:${f.line}`
const ORDER = { security: 0, bug: 1, guideline: 2, nit: 3 }
const bySeverity = (a, b) => (ORDER[a.severity] ?? 4) - (ORDER[b.severity] ?? 4)

const root = isPath(A.root) ? A.root : ''
const base = typeof A.base === 'string' ? A.base : ''
const head = typeof A.head === 'string' ? A.head : ''
const lenses = Array.isArray(A.lenses) ? A.lenses : []
const gateCommands = list(A.gates)
const cap = Number.isInteger(A.round_cap) && A.round_cap >= 1 ? A.round_cap : 2
const deviations = []
const summaries = new Map() // lens key -> summary of the last round
const found = new Map() // key -> finding with state open, fixed, or refuted
const refutedKeys = new Set()

function result(status) {
  const all = [...found.values()]
  return {
    status,
    workflow: 'review-and-fix',
    base,
    head_sha: head,
    summaries: [...summaries].map(([k, summary]) => ({ key: k, summary })),
    confirmed: all.filter((f) => f.state !== 'refuted').sort(bySeverity),
    refuted: all.filter((f) => f.state === 'refuted'),
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
if (!SHA.test(head)) problems.push('args.head is not a 40-character hex SHA')
if (A.spec !== undefined && !isPath(A.spec)) problems.push('args.spec is not an absolute path')
if (!isPath(A.guides)) problems.push('args.guides is not an absolute path')
if (A.deliberate !== undefined && !Array.isArray(A.deliberate)) problems.push('args.deliberate is not a list')
if (!gateCommands.length) problems.push('args.gates is not a list of gate commands')
if (A.round_cap !== undefined && !(Number.isInteger(A.round_cap) && A.round_cap >= 1)) problems.push('args.round_cap is not a positive integer')
problems.push(...lensProblems(A.lenses))
if (problems.length) return stop(problems.join('; '))

const bullets = (xs) => (xs.length ? xs.map((x) => `- ${x}`).join('\n') : '(none)')

const COMMON = `Scope: the branch checked out in ${root}. At the start of the run it was committed and clean at ${head}. The fixes of this run stay in the working tree, not committed. The change is \`git -C '${root}' diff ${base}\` (the working tree against the pinned base SHA), plus the untracked files of \`git -C '${root}' status --porcelain\`. Never diff against origin/main or another moving ref. Read the full files that you comment on, not only the hunks.
Spec: ${A.spec ? `${A.spec}. The code must follow it.` : '(none)'}
GUIDES ARE MANDATORY: before you read the diff, read ${A.guides}/INDEX.md and each guide file that your lens names under ${A.guides}/, in full. A review without them is invalid. Cite the guide file and the rule ID or heading for each finding. Drop a finding that has no rule and no concrete failing scenario. If a technology of the diff has no guide, say so in your summary; do not review it from memory.
Report only findings that a reviewer would block on or ask a change for: bugs, security, guideline violations with a concrete rule, and readability problems with a concrete fix. No praise, no summary of what the code does, and no findings on generated code or mocks. The accepted deviations of ${A.guides}/INDEX.md are not findings.
Deliberate choices (do not report them as findings, do not undo them):
${bullets(list(A.deliberate))}
House rules:
${typeof A.house_rules === 'string' && A.house_rules.trim() ? A.house_rules : '(none)'}

Rules of this run:
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of the tree. The clerk or the owner commits after the run.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails. The session posts the result later.
- Do not edit anything in the scratch folder that holds the spec.`

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
const GATES = {
  type: 'object',
  properties: {
    gates: {
      type: 'array',
      items: {
        type: 'object',
        properties: { command: { type: 'string' }, exit_code: { type: 'integer' }, output_tail: { type: 'string' } },
        required: ['command', 'exit_code', 'output_tail'],
      },
    },
  },
  required: ['gates'],
}
const VERDICT = {
  type: 'object',
  properties: { confirmed: { type: 'boolean' }, reason: { type: 'string' }, adjusted_fix: { type: 'string' } },
  required: ['confirmed', 'reason', 'adjusted_fix'],
}
const FIX = {
  type: 'object',
  properties: {
    fixed: { type: 'array', items: { type: 'object', properties: { file: { type: 'string' }, line: { type: 'integer' } }, required: ['file', 'line'] } },
    report: { type: 'string' },
  },
  required: ['fixed', 'report'],
}

const area = (f) => (f.file.startsWith('gate: ') ? 'gates' : f.file.includes('/') ? f.file.slice(0, f.file.indexOf('/')) : '.')

for (let round = 1; ; round++) {
  phase('Review')
  log(`Review round ${round} of ${cap}`)
  const checks = await parallel([
    ...lenses.map((l) => () => agent(
      `${l.prompt}\n\nReview round ${round}. Do not edit files.\n\n${COMMON}\n\nReturn the findings with file paths relative to ${root} and 1-based line numbers in the current file.`,
      { label: `review ${l.key}`, phase: 'Review', effort: 'low', schema: FINDINGS },
    )),
    () => agent(
      `Step: gates, round ${round}. Do not edit files. In ${root}, run each gate command, in this order:\n${bullets(gateCommands)}\nReturn exactly one result for each command, with the command text unchanged: its exit code, and the last 40 lines of its output when it fails (else an empty output_tail). Run no other command as a gate.\n\n${COMMON}`,
      { label: `gate ${round}`, phase: 'Review', effort: 'low', schema: GATES },
    ),
  ])
  if (checks.some((c) => !c)) return fail(`a review check of round ${round} did not return a result`)
  const gates = checks.pop()
  lenses.forEach((l, i) => summaries.set(l.key, checks[i].summary))

  // A gate that did not exit 0 is an open finding of this round, without a
  // refuter: an exit code is a source read. It is fixed when the gate is clean.
  for (const cmd of gateCommands) {
    const g = (gates.gates || []).find((x) => x.command === cmd)
    const k = `gate: ${cmd}:0`
    if (g && g.exit_code === 0) {
      if (found.has(k)) found.get(k).state = 'fixed'
      continue
    }
    found.set(k, {
      file: `gate: ${cmd}`, line: 0, lens: 'gate', rule: 'gate', severity: 'bug',
      problem: g ? `The gate \`${cmd}\` exits ${g.exit_code}: ${g.output_tail}` : `The gate check did not run \`${cmd}\`.`,
      fix: 'Make the gate pass.', round, state: 'open', reason: 'gate result',
    })
  }

  // Deduplicate by file:line; the first finding wins and the problems of the
  // others are appended. A refuted finding stays refuted.
  const fresh = new Map()
  lenses.forEach((l, i) => {
    for (const x of checks[i].findings) {
      const k = key(x)
      if (refutedKeys.has(k)) continue
      if (fresh.has(k)) fresh.get(k).problem += ` | also (${l.key}): ${x.problem}`
      else fresh.set(k, { file: norm(x.file), line: x.line, lens: l.key, rule: x.rule, severity: x.severity, problem: x.problem, fix: x.fix, round })
    }
  })
  const candidates = [...fresh.values()]
  log(`Round ${round}: ${candidates.length} findings, one refuter each`)

  phase('Verify')
  const verdicts = await parallel(candidates.map((x) => () => agent(
    `Try to refute this code review finding. Do not edit files. Read the file at the line, the code around it, the spec when the finding is about behavior, and the cited guide rule under ${A.guides}/. Confirm it only when the problem is real in this code and the fix is correct and proportionate. Refute it when the rule does not apply, the code already complies, the spec chose this behavior on purpose, the fix would break a test or the spec, or it is taste. When you are not sure, return confirmed = false. When you confirm it and the fix needs a change, give the better fix in adjusted_fix; else return an empty adjusted_fix.
Finding: ${JSON.stringify(x)}

${COMMON}`,
    { label: `verify ${x.file}:${x.line}`, phase: 'Verify', effort: 'low', schema: VERDICT },
  )))

  // A refuter that returned nothing gave no verdict: the run stops, and the
  // finding is neither confirmed nor refuted (deliver.js, final review B1).
  let dead = 0
  candidates.forEach((x, i) => {
    const v = verdicts[i]
    const k = key(x)
    if (!v) dead++
    else if (v.confirmed === true) found.set(k, { ...x, fix: v.adjusted_fix || x.fix, state: 'open', reason: v.reason })
    else {
      refutedKeys.add(k)
      found.set(k, { ...x, state: 'refuted', reason: v.reason })
    }
  })
  if (dead) return fail(`${dead} refuter(s) of round ${round} did not return a result`)

  const open = [...found.values()].filter((x) => x.state === 'open')
  if (!open.length) return result('done')
  if (round >= cap) return result('findings_left')

  // Fix one area at a time: parallel fixers in one tree collide.
  phase('Fix')
  const areas = new Map()
  for (const x of open) areas.set(area(x), [...(areas.get(area(x)) || []), x])
  for (const [name, batch] of areas) {
    const fix = await agent(
      `Step: fix these confirmed review findings of the area ${name} in ${root}. Read each file in full before you edit it. Keep each change minimal and in the spirit of the finding; do not refactor past it. When a finding needs a test change to stay honest, change the test too. When a fix touches a source that a generator reads, run the generator. If a finding cannot be fixed without breaking a test or the spec, leave it and say why in the report.
Findings:
${batch.map((x, i) => `${i + 1}. ${x.file}:${x.line} [${x.severity}] ${x.rule}\n   ${x.problem}\n   FIX: ${x.fix}`).join('\n')}

After the edits, run the gate commands:
${bullets(gateCommands)}
Return fixed with the file and the line of each finding that you fixed, and a report with the gate output and each finding that you left, with the reason.

${COMMON}`,
      { label: `fix ${name}`, phase: 'Fix', effort: 'high', schema: FIX },
    )
    if (!fix) return fail(`the fixer of the area ${name} did not return a result`)
    // Only a fix that the fixer reports makes a finding fixed.
    for (const loc of fix.fixed || []) {
      const x = found.get(`${norm(loc.file)}:${loc.line}`)
      if (x && x.state === 'open') x.state = 'fixed'
    }
    if (fix.report) deviations.push(`fix ${name}: ${fix.report}`)
  }
}
