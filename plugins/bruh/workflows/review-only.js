export const meta = {
  name: 'review-only',
  description: 'Review a change that bruh did not write: one reviewer for each lens, one refuter for each unique finding. No edits and no posts.',
  whenToUse: 'Review the pull request or merge request of another author. Post the saved result afterwards with scripts/post-findings.sh, after the owner said yes or under a post grant.',
  phases: [
    { title: 'Review', detail: 'one reviewer for each lens: a file slice and a guide set' },
    { title: 'Verify', detail: 'one refuter for each unique finding' },
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
const deviations = []
const summaries = []
const confirmed = []
const refuted = []

function result(status) {
  confirmed.sort(bySeverity)
  return { status, workflow: 'review-only', base, head_sha: head, summaries, confirmed, refuted, deviations }
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
problems.push(...lensProblems(A.lenses))
if (problems.length) return stop(problems.join('; '))

const bullets = (xs) => (xs.length ? xs.map((x) => `- ${x}`).join('\n') : '(none)')
const DIFF = `git -C '${root}' diff ${base} ${head}`

const COMMON = `Scope: a detached, clean worktree at ${root}, at the commit ${head}. The change under review is \`${DIFF}\`, against the pinned base SHA. Never diff against origin/main or another moving ref. Read the full files that you comment on, not only the hunks.
Spec: ${A.spec ? `${A.spec}. The code must follow it.` : '(none)'}
READ-ONLY REVIEW: never edit, create, or delete a file in ${root}. You can run read-only commands (build, vet, tests, git show, git log). Test an empirical claim with a throwaway program outside ${root}.
GUIDES ARE MANDATORY: before you read the diff, read ${A.guides}/INDEX.md and each guide file that your lens names under ${A.guides}/, in full. A review without them is invalid. Cite the guide file and the rule ID or heading for each finding. Drop a finding that has no rule and no concrete failing scenario. If a technology of the diff has no guide, say so in your summary; do not review it from memory.
Report only findings that a reviewer would block on or ask a change for: bugs, security, guideline violations with a concrete rule, and readability problems with a concrete fix. No praise, and no summary of what the code does. The accepted deviations of ${A.guides}/INDEX.md are not findings.
Deliberate choices (do not report them as findings):
${bullets(list(A.deliberate))}
House rules:
${typeof A.house_rules === 'string' && A.house_rules.trim() ? A.house_rules : '(none)'}

Rules of this run:
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of the tree.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails. The session posts the result later.`

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
const VERDICT = {
  type: 'object',
  properties: { confirmed: { type: 'boolean' }, reason: { type: 'string' }, adjusted_fix: { type: 'string' } },
  required: ['confirmed', 'reason', 'adjusted_fix'],
}

phase('Review')
const raw = await parallel(lenses.map((l) => () => agent(
  `${l.prompt}\n\n${COMMON}\n\nReturn the findings with file paths relative to ${root} and 1-based line numbers in the file at ${head}.`,
  { label: `review ${l.key}`, phase: 'Review', effort: 'low', schema: FINDINGS },
)))
const deadLenses = lenses.filter((l, i) => !raw[i]).map((l) => l.key)
if (deadLenses.length) return fail(`the reviewer of the lens ${deadLenses.join(', ')} did not return a result`)

// Deduplicate by file:line. The first finding wins; the problems of the others are appended.
const seen = new Map()
lenses.forEach((l, i) => {
  summaries.push({ key: l.key, summary: raw[i].summary })
  for (const x of raw[i].findings) {
    const k = key(x)
    if (seen.has(k)) seen.get(k).problem += ` | also (${l.key}): ${x.problem}`
    else seen.set(k, { file: norm(x.file), line: x.line, lens: l.key, rule: x.rule, severity: x.severity, problem: x.problem, fix: x.fix, round: 1 })
  }
})
const unique = [...seen.values()]
log(`${unique.length} unique findings, one refuter each`)

phase('Verify')
const verdicts = await parallel(unique.map((x) => () => agent(
  `Try to refute this code review finding. Read the file at the line, the code around it, the spec when the finding is about behavior, and the cited guide rule under ${A.guides}/. Confirm it only when the problem is real in this code and the fix is correct and proportionate. Refute it when the rule does not apply, the code already complies, the spec chose this behavior on purpose, or it is taste. When you are not sure, return confirmed = false. When you confirm it and the fix needs a change, give the better fix in adjusted_fix; else return an empty adjusted_fix.
Finding: ${JSON.stringify(x)}

${COMMON}`,
  { label: `verify ${x.file}:${x.line}`, phase: 'Verify', effort: 'low', schema: VERDICT },
)))

// A refuter that returned nothing gave no verdict. Its finding is neither
// confirmed nor refuted, and the run stops, so the session relaunches it.
let dead = 0
unique.forEach((x, i) => {
  const v = verdicts[i]
  if (!v) dead++
  else if (v.confirmed === true) confirmed.push({ ...x, fix: v.adjusted_fix || x.fix, reason: v.reason })
  else refuted.push({ ...x, reason: v.reason })
})
if (dead) return fail(`${dead} refuter(s) did not return a result`)
return result('done')
