export const meta = {
  name: 'implement-tickets',
  description: 'Implement tickets wave by wave: one implementer for each ticket, in parallel lanes inside a wave, one reviewer, a fix loop, the merge of the lanes, and a whole-branch gate. No commits and no posts.',
  whenToUse: 'Implement the tickets of a settled spec that /bruh:tickets wrote. Pass the waves of issues/INDEX.md. After a stop, relaunch with only the waves that are not done; do not resume.',
  phases: [
    { title: 'Implement', detail: 'one implementer, one reviewer, and a fix loop for each ticket; parallel inside a wave' },
    { title: 'Merge', detail: 'patch the lanes of a wave back onto the shared tree, in wave order' },
    { title: 'Gate', detail: 'the gate commands over the whole branch' },
  ],
}

// The script has no clock and no filesystem. Agents do all reads and writes.
// Each helper is copied from deliver.js: the Workflow runtime has no imports.

const A = (typeof args === 'object' && args) || {}
const list = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === 'string') : [])
const isPath = (p) => typeof p === 'string' && p.startsWith('/') && !/['\n]/.test(p)
const TICKET_FILE = /^[A-Za-z0-9][A-Za-z0-9._-]*\.md$/

const root = isPath(A.root) ? A.root : ''
const issues = isPath(A.issues) ? A.issues.replace(/\/+$/, '') : ''
const gateCommands = list(A.gates)
const fixCap = Number.isInteger(A.fix_cap) && A.fix_cap >= 0 ? A.fix_cap : 3
const deviations = []
const tickets = []
let gate = null

function result(status) {
  return { status, tickets, gate, deviations }
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
if (A.fix_cap !== undefined && !(Number.isInteger(A.fix_cap) && A.fix_cap >= 0)) problems.push('args.fix_cap is not an integer of 0 or more')
const waves = Array.isArray(A.waves) ? A.waves : []
if (!waves.length || !waves.every((w) => Array.isArray(w) && w.length)) problems.push('args.waves is not a list of lists of ticket file names')
else {
  const seen = new Set()
  for (const file of waves.flat()) {
    if (typeof file !== 'string' || !TICKET_FILE.test(file)) problems.push(`args.waves has a bad ticket file name: ${JSON.stringify(file)}`)
    else if (seen.has(file)) problems.push(`args.waves has ${file} twice`)
    else seen.add(file)
  }
}
if (problems.length) return stop(problems.join('; '))

const bullets = (xs) => (xs.length ? xs.map((x) => `- ${x}`).join('\n') : '(none)')
const LANE = `ROOT='${root}' sh '${A.lane}'`

const COMMON = `Ground rules of this run (binding):
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of the shared tree ${root}. The owner or the clerk commits later.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails.
- Do not edit anything in the scratch folder that holds ${A.spec}, except where a step below says so.
- Change only the files that the ticket lists under Files. If the ticket is not possible without another file, do the minimum and name it in deviations.
- The spec is ${A.spec}. The ticket wins on detail; the spec wins on intent. If they conflict, stop and return the conflict with the reason. Do not guess.
- MODULES: when a ticket needs a module that the module file does not require yet, add it at the version that the spec or the ticket names, run the tidy command, and list the module files in files_changed with a one-line note in deviations. That is in scope, and reviewers must not flag it. The tidy command must be idempotent afterwards.
- Some tickets leave the build broken on purpose until a later ticket (the ticket text says so). Only the verify commands of the acceptance criteria of the ticket decide pass or fail.
- The guides are mandatory for each reviewer: the Review guides section of the ticket names files in ${A.guides}/.`

const WORK = {
  type: 'object',
  properties: {
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
const REPORT = { type: 'object', properties: { ok: { type: 'boolean' }, report: { type: 'string' } }, required: ['ok', 'report'] }
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

const laneDiff = (workdir, useLane) => (useLane
  ? `In the lane, \`git -C '${workdir}' diff refs/lane/base\` plus the untracked files is exactly the work of this ticket.`
  : `See what changed with \`git -C '${workdir}' status --porcelain\` and \`git -C '${workdir}' diff\` (read the new files), and focus on the Files of the ticket.`)

function implPrompt(t, useLane) {
  return `You implement exactly one ticket of a larger change.

Ticket: ${t.file}. Read it in full. Read ${A.spec} for the intent. Read each existing file that the ticket lists under Files, and the files around them, before you write anything.

Working folder: ${useLane
    ? `run \`${LANE} start ${t.id}\` first. It prints the path of a private copy of the repository. Do all work and run all commands inside that copy. Do not touch ${root} in this ticket.`
    : `${root}. Work there. You are the only agent that edits it now.`}

Do the work that the ticket describes, and nothing more. For a ticket of Kind: test, the test must compile and fail against the stub (red); do not make it pass. For a ticket of Kind: implementation, make the paired test pass without an edit to the test. Format the files that you touched. Then run each verify command of the acceptance criteria and paste the real output.

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

const entry = (t, status, extra = {}) => ({ id: t.id, file: t.file, status, workdir: '', rounds: 0, findings: [], deviations: '', ...extra })

async function runTicket(t, useLane) {
  let work = await agent(implPrompt(t, useLane), { label: `impl ${t.id}`, phase: 'Implement', effort: 'high', schema: WORK })
  if (!work) return entry(t, 'error', { deviations: 'the implementer did not return a result' })
  if (work.conflict) return entry(t, 'conflict', { deviations: work.conflict })
  const workdir = useLane ? work.workdir : root
  if (useLane && (!isPath(workdir) || workdir === root)) return entry(t, 'error', { deviations: `the implementer returned no lane folder: ${JSON.stringify(work.workdir)}` })
  let review = null
  for (let round = 0; ; round++) {
    review = await agent(reviewPrompt(t, workdir, useLane), { label: `review ${t.id}${round ? ` #${round + 1}` : ''}`, phase: 'Implement', effort: 'low', schema: REVIEW })
    if (!review) return entry(t, 'error', { workdir, rounds: round, deviations: 'the reviewer did not return a result' })
    if (review.pass) return entry(t, 'done', { workdir, rounds: round, deviations: work.deviations || '' })
    if (round >= fixCap) break
    work = await agent(fixPrompt(t, workdir, useLane, review.findings || []), { label: `fix ${t.id} #${round + 1}`, phase: 'Implement', effort: 'high', schema: WORK })
    if (!work) return entry(t, 'error', { workdir, rounds: round + 1, deviations: 'the fixer did not return a result' })
    if (work.conflict) return entry(t, 'conflict', { workdir, rounds: round + 1, deviations: work.conflict })
  }
  return entry(t, 'failed', { workdir, rounds: fixCap, findings: review.findings || [] })
}

for (let w = 0; w < waves.length; w++) {
  const n = w + 1
  const wave = waves[w].map((file) => ({ id: file.slice(0, -3), file: `${issues}/${file}` }))
  const useLane = wave.length > 1
  phase('Implement')
  log(`Wave ${n}: ${wave.map((t) => t.id).join(', ')}${useLane ? ' (parallel lanes)' : ''}`)
  const out = (await parallel(wave.map((t) => () => runTicket(t, useLane)))).map((r, i) => r || entry(wave[i], 'error', { deviations: 'the ticket run threw' }))
  tickets.push(...out)
  const good = out.filter((r) => r.status === 'done')
  const bad = out.filter((r) => r.status !== 'done')

  // Merge the done lanes one at a time, in wave order. A failed lane stays for a look.
  if (useLane && good.length) {
    phase('Merge')
    const merge = await agent(`Merge the finished lanes of wave ${n} back onto the shared tree ${root}, one at a time, in this order. Run each command and paste its output:
${good.map((r) => `- ${LANE} patch ${r.id}\n- ${LANE} apply ${r.id}\n- ${LANE} clean ${r.id}`).join('\n')}
If an apply reports a conflict or a rejected hunk, stop at once, leave the tree as it is, and report the file and the hunk with ok = false.
Leave these failed lanes untouched (no patch, no clean): ${bad.map((r) => r.id).join(', ') || 'none'}.
After the last apply, run \`git -C '${root}' status --porcelain\` and paste it.

${COMMON}`, { label: `merge ${n}`, phase: 'Merge', effort: 'low', schema: REPORT })
    if (!merge) return fail(`the merge agent of wave ${n} did not return a result`)
    if (!merge.ok) return stop(`the merge of wave ${n} failed: ${merge.report}`)
  }

  const errors = bad.filter((r) => r.status === 'error')
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
const g = await agent(`Run the whole-branch gate in ${root}. Run each gate command, in this order:
${bullets(gateCommands)}
Return exactly one result for each command, with the command text unchanged: its exit code, and the last 40 lines of its output when it fails (else an empty output_tail). Run no other command as a gate. Do not fix anything.

${COMMON}`, { label: 'gate', phase: 'Gate', effort: 'low', schema: GATES })
if (!g) return fail('the gate agent did not return a result')
const reported = g.gates || []
const ok = gateCommands.every((cmd) => reported.some((x) => x.command === cmd && x.exit_code === 0))
gate = { ok, gates: reported }
return result(ok ? 'done' : 'findings_left')
