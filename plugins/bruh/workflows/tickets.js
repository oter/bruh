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

const A = (typeof args === 'object' && args) || {}
const isPath = (p) => typeof p === 'string' && p.startsWith('/') && !/['\n]/.test(p)
const cap = Number.isInteger(A.round_cap) && A.round_cap >= 1 ? A.round_cap : 5
const deviations = []
let draft = { tickets: [], notes: '' }
let issues = []
let rounds = 0

function result(status) {
  return { status, tickets: draft.tickets, notes: draft.notes, issues, rounds, deviations }
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
if (problems.length) return stop(problems.join('; '))

const ground = A.ground || []
const RULES = `Rules of this run:
- Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase. Do not change the index or the history of ${A.root}.
- Do not post outside the project: no comments on pull requests, merge requests, or issues, no chat messages, no emails.
- Write only in ${A.issues}/. Do not edit ${A.spec} or the code.`

const TICKETS = {
  type: 'object',
  properties: {
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

phase('Write')
const written = await agent(`You break a settled design into tickets. Other agents implement them one at a time.

Read ${A.spec} in full first. Then read the current code of ${A.root} to ground each path${ground.length ? `, and these paths: ${ground.join(', ')}` : ''}. Read ${A.guides}/INDEX.md for the map from file globs to review guides.

Then write the tickets as files into ${A.issues}/, and write ${A.issues}/INDEX.md with the execution waves. Follow these rules exactly:
${A.rules}

${RULES}

Return the full list of tickets that you wrote, and notes on each part of the spec that you could not turn into a ticket.`, { label: 'write', phase: 'Write', effort: 'high', schema: TICKETS })
if (!written) return fail('the ticket writer did not return a result')
draft = written

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
  const fixed = await agent(`Fix a set of tickets after a review. Read ${A.spec} and the rules, then each file in ${A.issues}/, then apply each fix below.${note}
Add, split, merge, or delete ticket files as the fixes need. Keep the numbers in dependency order (renumber when you insert), and write ${A.issues}/INDEX.md again with the waves. Do not change anything that the review did not flag, unless a fix forces it.

Findings:
${issues.map((x, i) => `${i + 1}. [${x.ticket}] ${x.problem}\n   FIX: ${x.fix}`).join('\n')}

Rules:
${A.rules}

${RULES}

Return the full list of tickets after the fixes, and notes on each finding that you could not apply, with the reason.`, { label: `fix ${round}`, phase: 'Fix', effort: 'high', schema: TICKETS })
  if (!fixed) return fail(`the fixer of round ${round} did not return a result`)
  draft = fixed
}
