export const meta = {
  name: 'review-only',
  description: 'Review a change that bruh did not write: check the tree, one reviewer for each lens, one refuter for each unique finding. No edits and no posts.',
  whenToUse: 'Review the pull request or merge request of another author. Save the result with result_save and post it with scripts/post-findings.sh after the approval of the owner or under a post grant.',
  phases: [
    { title: 'Check', detail: 'HEAD of root, a clean tree, and the base SHA as an ancestor' },
    { title: 'Review', detail: 'one reviewer for each lens: a file slice and a guide set' },
    { title: 'Verify', detail: 'one refuter for each unique finding' },
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
const key = (f) => `${norm(f.file)}:${f.line}`
const ORDER = { security: 0, bug: 1, guideline: 2, nit: 3 }
const bySeverity = (a, b) => (ORDER[a.severity] ?? 4) - (ORDER[b.severity] ?? 4)

const root = isPath(A.root) ? A.root : ''
const base = typeof A.base === 'string' ? A.base : ''
let head = typeof A.head === 'string' ? A.head : ''
const lenses = Array.isArray(A.lenses) ? A.lenses : []
const deadline = typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0 ? A.deadline_seconds : 3600
const answers = A.answers && typeof A.answers === 'object' && !Array.isArray(A.answers) ? A.answers : {}
const deviations = []
const summaries = []
const confirmed = []
const refuted = []

function result(status, question = null) {
  confirmed.sort(bySeverity)
  return { status, workflow: 'review-only', base, head_sha: head, summaries, confirmed, refuted, question, deviations }
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
if (A.deadline_seconds !== undefined && !(typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0)) problems.push('args.deadline_seconds is not a positive number')
if (A.answers !== undefined && answers !== A.answers) problems.push('args.answers is not an object')
problems.push(...lensProblems(A.lenses))
if (problems.length) return stop(problems.join('; '))

const bullets = (xs) => (xs.length ? xs.map((x) => `- ${x}`).join('\n') : '(none)')

const RULES = `Rules of this run:
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of the tree.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails. The session posts the result later.`

const ASK = `Questions: when you need a decision that the spec, the guides, and the code do not answer, do not guess.
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
    question: QUESTION,
  },
  required: ['findings', 'summary'],
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
      ? `\n\nEarlier questions of this step and their answers:\n${given.map((g) => `- ${g.q.id} (${g.q.header}): ${g.text}`).join('\n')}`
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
if ((check.status_porcelain || '').trim()) return stop(`the tree of ${root} is not clean: ${check.status_porcelain.trim()}`)
if (check.base_is_ancestor !== true) return stop(`the base ${base} is not an ancestor of ${head}`)

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

${RULES}`

phase('Review')
const raw = await parallel(lenses.map((l) => () => askable(
  `review ${l.key}`,
  `${l.prompt}\n\n${COMMON}\n\nReturn the findings with file paths relative to ${root} and 1-based line numbers in the file at ${head}.`,
  { phase: 'Review', effort: 'low', schema: FINDINGS },
)))
const deadLenses = lenses.filter((l, i) => !raw[i] || raw[i].failed).map((l) => l.key)
if (deadLenses.length) return fail(`the reviewer of the lens ${deadLenses.join(', ')} did not return a result`)
const stopped = raw.find((r) => r.stop)
if (stopped) return stop(stopped.stop)
const question = raw.find((r) => r.question)
if (question) return asked(question)

// Deduplicate by file:line. The first finding wins; the problems of the others are appended.
const seen = new Map()
lenses.forEach((l, i) => {
  const r = raw[i].value
  summaries.push({ key: l.key, summary: r.summary })
  for (const x of r.findings) {
    const k = key(x)
    if (seen.has(k)) seen.get(k).problem += ` | also (${l.key}): ${x.problem}`
    else seen.set(k, { file: norm(x.file), line: x.line, lens: l.key, rule: x.rule, severity: x.severity, problem: x.problem, fix: x.fix, round: 1 })
  }
})
const unique = [...seen.values()]
log(`${unique.length} unique findings, one refuter each`)

phase('Verify')
const verdicts = await parallel(unique.map((x) => () => agent(
  `Try to refute this code review finding. Read the file at the line, the code around it, the spec when the finding is about behavior, and the cited guide rule under ${A.guides}/. When the finding is an empirical claim, test it with a read-only command or a throwaway program outside ${root}. Confirm it only when the problem is real in this code and the fix is correct and proportionate. Refute it when the rule does not apply, the code already complies, the spec chose this behavior on purpose, or it is taste. When you are not sure, return confirmed = false. When you confirm it and the fix needs a change, give the better fix in adjusted_fix; else return an empty adjusted_fix. When you confirm it and the problem is at another line or in another file than the finding says, return the correct file and line; else leave them out.
Finding: ${JSON.stringify(x)}

${COMMON}`,
  { label: `verify ${x.file}:${x.line}`, phase: 'Verify', effort: 'low', schema: VERDICT },
)))

// A refuter that returned nothing gave no verdict. Its finding is neither
// confirmed nor refuted, and the run stops, so the session relaunches it.
let dead = 0
const byLocation = new Map()
unique.forEach((x, i) => {
  const v = verdicts[i]
  if (!v) dead++
  else if (v.confirmed === true) {
    const y = { ...located(x, v), fix: v.adjusted_fix || x.fix, reason: v.reason }
    const k = key(y)
    if (byLocation.has(k)) byLocation.get(k).problem += ` | also (${y.lens}): ${y.problem}`
    else byLocation.set(k, y)
  } else refuted.push({ ...x, reason: v.reason })
})
confirmed.push(...byLocation.values())
if (dead) return fail(`${dead} refuter(s) did not return a result`)
return result('done')
