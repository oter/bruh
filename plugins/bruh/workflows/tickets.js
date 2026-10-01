export const meta = {
  name: 'tickets',
  description: 'Break a settled spec into function-sized Markdown tickets with TDD pairs and execution waves, reviewed in a fix loop up to the round cap. No commits and no posts.',
  whenToUse: 'Turn a spec with no open questions into tickets for /bruh:implement-tickets. Pass the ticket rules in args.rules: references/ticket-template.md plus the rules of the project.',
  phases: [
    { title: 'Write', detail: 'one writer drafts one Markdown ticket for each unit, and INDEX.md with the waves' },
    { title: 'Review', detail: 'one reviewer checks the tickets against the spec, the template, and the rules' },
    { title: 'Fix', detail: 'one fixer applies the review findings' },
  ],
}

// The script has no clock and no filesystem. The writer and the fixer write the ticket files.
// Each helper is copied from deliver.js: the Workflow runtime has no imports.
// Prompts before a question never contain an answer, so a relaunch with
// resumeFromRunId returns their cached results (spec 6.2).

const A = (typeof args === 'object' && args) || {}
const isPath = (p) => typeof p === 'string' && p.startsWith('/') && !/['\n]/.test(p)
const cap = Number.isInteger(A.round_cap) && A.round_cap >= 1 ? A.round_cap : 5
const deadline = typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0 ? A.deadline_seconds : 3600
const answers = A.answers && typeof A.answers === 'object' && !Array.isArray(A.answers) ? A.answers : {}
const deviations = []
let draft = { tickets: [], notes: '' }
let issues = []
let rounds = 0

function result(status, question = null) {
  return { status, tickets: draft.tickets, notes: draft.notes, issues, rounds, question, deviations }
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
for (const k of ['root', 'spec', 'issues', 'guides']) if (!isPath(A[k])) problems.push(`args.${k} is not an absolute path`)
if (typeof A.rules !== 'string' || !A.rules.trim()) problems.push('args.rules is empty: pass the ticket rules text')
if (A.ground !== undefined && !(Array.isArray(A.ground) && A.ground.every(isPath))) problems.push('args.ground is not a list of absolute paths')
if (A.round_cap !== undefined && !(Number.isInteger(A.round_cap) && A.round_cap >= 1)) problems.push('args.round_cap is not a positive integer')
if (A.deadline_seconds !== undefined && !(typeof A.deadline_seconds === 'number' && A.deadline_seconds > 0)) problems.push('args.deadline_seconds is not a positive number')
if (A.answers !== undefined && answers !== A.answers) problems.push('args.answers is not an object')
if (problems.length) return stop(problems.join('; '))

const ground = A.ground || []
const RULES = `Rules of this run:
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of ${A.root}.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails.
- Write only in ${A.issues}/. Do not edit ${A.spec} or the code.`

const ASK = `Questions: when you need a decision that the spec, the rules, and the code do not answer, do not guess.
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
const TICKETS = {
  type: 'object',
  properties: {
    question: QUESTION,
    tickets: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          id: { type: 'string' }, file: { type: 'string' }, title: { type: 'string' },
          kind: { type: 'string', enum: ['test', 'implementation', 'other'] },
          blocked_by: { type: 'array', items: { type: 'string' } },
          files: { type: 'array', items: { type: 'string' } },
          parallel_safe: { type: 'boolean' },
        },
        required: ['id', 'file', 'title', 'kind', 'blocked_by', 'files', 'parallel_safe'],
      },
    },
    notes: { type: 'string' },
  },
  required: ['tickets', 'notes'],
}
const REVIEW = {
  type: 'object',
  properties: {
    pass: { type: 'boolean' },
    issues: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          ticket: { type: 'string', description: 'ticket ID, or INDEX, or COVERAGE for a missing ticket' },
          problem: { type: 'string' }, fix: { type: 'string' },
        },
        required: ['ticket', 'problem', 'fix'],
      },
    },
    summary: { type: 'string' },
  },
  required: ['pass', 'issues', 'summary'],
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
      ? `\n\nEarlier questions of this step and their answers. Check ${A.issues}/ first: work of an earlier attempt can be in it already.\n${given.map((g) => `- ${g.q.id} (${g.q.header}): ${g.text}`).join('\n')}`
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

phase('Write')
const written = await askable('write', `You break a settled design into tickets. Other agents implement them one at a time.

Read ${A.spec} in full first. Then read the current code of ${A.root} to ground each path${ground.length ? `, and these paths: ${ground.join(', ')}` : ''}. Read ${A.guides}/INDEX.md for the map from file globs to review guides.

Then write the tickets as files into ${A.issues}/, and write ${A.issues}/INDEX.md with the execution waves. Follow these rules exactly:
${A.rules}

${RULES}

Return the full list of tickets that you wrote, and notes on each part of the spec that you could not turn into a ticket.`, { phase: 'Write', effort: 'high', schema: TICKETS })
if (written.failed) return fail(written.failed)
if (written.stop) return stop(written.stop)
if (written.question) return asked(written)
draft = written.value

for (let round = 1; ; round++) {
  rounds = round
  phase('Review')
  const review = await agent(`Review a set of tickets against a spec and a set of rules. Be strict and concrete: each issue names the ticket ID and the exact fix. Do not edit files.

Read ${A.spec}, then ${A.issues}/INDEX.md, then each file in ${A.issues}/ (list the folder). Check, in this order:
1. Template: each ticket has exactly the sections and fields of the rules below, in order, with the verify command as the last acceptance criterion.
2. Size: each ticket is one function, type, file, resource group, or entry. Split a larger one; merge one that you cannot verify alone.
3. TDD pairs: each function that is not trivial has a test ticket (the test and a stub that panics) and then an implementation ticket that the test ticket blocks. The signatures of the pair match byte for byte.
4. Blocking edges: no cycle; a ticket that edits a file that another ticket creates is blocked by it; no edge that does not gate.
5. Files and parallel safety: the file lists are complete and specific; the shared-file rule applies; the waves of INDEX.md agree with it.
6. Existing paths: each file that a ticket edits (not creates) exists on disk now. Check with ls.
7. Coverage: walk the normative sections of the spec sentence by sentence, and confirm that each maps to a ticket. Report a gap with the ticket COVERAGE. An item of the out-of-scope list of the spec in a ticket is an issue.
8. Contradictions with the spec.
9. The Review guides section of each ticket agrees with the map of ${A.guides}/INDEX.md for its files.

Rules that the tickets must follow:
${A.rules}

${RULES}

Return pass = true only when there are no issues.`, { label: `review ${round}`, phase: 'Review', effort: 'low', schema: REVIEW })
  if (!review) return fail(`the reviewer of round ${round} did not return a result`)
  issues = review.pass ? [] : review.issues || []
  log(`Review ${round}: ${review.pass ? 'pass' : `${issues.length} issues`}`)
  if (review.pass) return result('done')
  if (round >= cap) return result('findings_left')

  phase('Fix')
  const note = round >= 3 && typeof A.amendments === 'string' && A.amendments.trim()
    ? `\nSpec amendments (already decided; do not ask again): ${A.amendments}\n`
    : ''
  const fixed = await askable(`fix ${round}`, `Fix a set of tickets after a review. Read ${A.spec} and the rules, then each file in ${A.issues}/, then apply each fix below.${note}
Add, split, merge, or delete ticket files as the fixes need. Keep the numbers in dependency order (renumber when you insert), and write ${A.issues}/INDEX.md again with the waves. Do not change anything that the review did not flag, unless a fix forces it.

Findings:
${issues.map((x, i) => `${i + 1}. [${x.ticket}] ${x.problem}\n   FIX: ${x.fix}`).join('\n')}

Rules:
${A.rules}

${RULES}

Return the full list of tickets after the fixes, and notes on each finding that you could not apply, with the reason.`, { phase: 'Fix', effort: 'high', schema: TICKETS })
  if (fixed.failed) return fail(fixed.failed)
  if (fixed.stop) return stop(fixed.stop)
  if (fixed.question) return asked(fixed)
  draft = fixed.value
}
