// Control-flow tests for the deliver workflow, with stub Workflow globals.
// Run: node --test plugins/bruh/workflows/deliver.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const source = readFileSync(join(import.meta.dirname, 'deliver.js'), 'utf8')
const AsyncFunction = (async () => {}).constructor
const GLOBALS = ['agent', 'parallel', 'pipeline', 'phase', 'log', 'args', 'budget']
const script = new AsyncFunction(...GLOBALS, source.replace('export const meta', 'const meta'))
const meta = new Function(`return ${source.match(/export const meta = (\{[\s\S]*?\n\})\n/)[1]}`)()

const SHA = 'a'.repeat(40)
const HEAD = 'b'.repeat(40)

function baseArgs(extra = {}) {
  return {
    task: 'Add the thing.\nFiles this task may touch: src/a.go',
    acceptance: ['the thing works'],
    base_sha: SHA,
    branch: 'task-thing',
    gates: ['go test ./...'],
    house_rules: 'HOUSE RULES TEXT',
    guides: '',
    deliberate: ['the global lock'],
    deadline_seconds: 600,
    round_cap: 2,
    ...extra,
  }
}

const cleanGates = () => ({
  head_sha: HEAD,
  tests: { ran: 10, passed: 10, failed: 0, skipped: 0 },
  gates: [{ command: 'go test ./...', exit_code: 0, ran: 10, passed: 10, failed: 0, skipped: 0, problems: [] }],
})

// Runs the script. `h` maps the first word of an agent label to a handler
// (prompt, opts, callIndexForThatWord) => result.
async function run(h = {}, args = baseArgs()) {
  const calls = []
  const counts = {}
  const handlers = {
    plan: () => ({ plan: '1. Do it. STOP if the table is missing.', stop: false, reason: '' }),
    implement: () => ({ head_sha: HEAD, deviations: [], conflict: '' }),
    adversarial: () => ({ findings: [] }),
    invariants: () => ({ findings: [] }),
    gates: cleanGates,
    refute: () => ({ confirmed: false, reason: 'not shown' }),
    fix: (p, o, n, fs) => ({ head_sha: HEAD, fixed: fs, deviations: [] }),
    ...h,
  }
  const agent = async (prompt, opts = {}) => {
    const word = (opts.label || '').split(' ')[0]
    const n = (counts[word] = (counts[word] || 0) + 1)
    calls.push({ prompt, opts, word })
    assert.ok(handlers[word], `no handler for label ${opts.label}`)
    // A fixer gets the findings of its batch, read from its prompt lines "- file:line: summary".
    const batch = [...prompt.matchAll(/^- (\S+):(\d+): /gm)].map((m) => ({ file: m[1], line: Number(m[2]) }))
    return handlers[word](prompt, opts, n, batch)
  }
  const parallel = async (thunks) => Promise.all(thunks.map((t) => t().catch(() => null)))
  const pipeline = async () => {
    throw new Error('pipeline is not used')
  }
  const budget = { total: null, spent: () => 0, remaining: () => Infinity }
  const result = await script(agent, parallel, pipeline, () => {}, () => {}, args, budget)
  return { result, calls, byWord: (w) => calls.filter((c) => c.word === w) }
}

const finding = (file, line, summary = 'bad') => ({ file, line, summary })

test('meta.name is deliver and meta is a literal', () => {
  assert.equal(meta.name, 'deliver')
  assert.ok(meta.description)
  const text = source.match(/export const meta = (\{[\s\S]*?\n\})\n/)[1]
  assert.doesNotMatch(text, /[`(]|\.\.\.|\$\{/, 'meta must be a pure literal: no template, call, or spread')
})

test('a clean run returns done with the evidence', async () => {
  const { result } = await run()
  assert.equal(result.status, 'done')
  assert.equal(result.branch, 'task-thing')
  assert.equal(result.base_sha, SHA)
  assert.equal(result.head_sha, HEAD)
  assert.deepEqual(result.tests, { ran: 10, passed: 10, failed: 0, skipped: 0 })
  assert.deepEqual(result.findings, [])
  assert.deepEqual(result.deviations, [])
  assert.equal(result.question, null)
  assert.deepEqual(Object.keys(result).sort(), ['base_sha', 'branch', 'deviations', 'findings', 'head_sha', 'question', 'status', 'tests'])
})

test('invalid args stop the run before any agent', async () => {
  for (const bad of [{ base_sha: 'abc' }, { task: '' }, { branch: '' }, { gates: 'go test' }, { gates: [] }]) {
    const { result, calls } = await run({}, baseArgs(bad))
    assert.equal(result.status, 'stopped')
    assert.equal(calls.length, 0)
    assert.match(result.deviations[0], /^STOP: /)
  }
  const { result } = await run({}, null)
  assert.equal(result.status, 'stopped')
})

test('reviewers diff against base_sha, never origin/main', async () => {
  const { byWord } = await run()
  for (const w of ['adversarial', 'invariants', 'gates']) {
    const [c] = byWord(w)
    assert.ok(c.prompt.includes(`git diff ${SHA} HEAD`), `${w} does not diff against base_sha`)
    assert.doesNotMatch(c.prompt, /git diff[^\n]*origin\/main/)
    assert.ok(c.prompt.includes('HOUSE RULES TEXT'), `${w} has no house rules`)
    assert.ok(c.prompt.includes('the global lock'), `${w} has no deliberate choices`)
  }
})

test('reviewers run at low effort and implementers at high effort', async () => {
  const { byWord } = await run({
    adversarial: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 3)] : [] }),
    refute: () => ({ confirmed: true, reason: 'shown' }),
  })
  for (const w of ['adversarial', 'invariants', 'gates', 'refute']) {
    for (const c of byWord(w)) assert.equal(c.opts.effort, 'low', `${w} effort`)
  }
  for (const w of ['plan', 'implement', 'fix']) {
    assert.ok(byWord(w).length > 0, `no ${w} call`)
    for (const c of byWord(w)) assert.equal(c.opts.effort, 'high', `${w} effort`)
  }
})

test('dedup by file:line', async () => {
  const { result, byWord } = await run({
    adversarial: () => ({ findings: [finding('src/a.go', 3, 'first'), finding('src/a.go', 3, 'again')] }),
    invariants: () => ({ findings: [finding('src/a.go', 3, 'same line'), finding('src/a.go', 4)] }),
  })
  assert.equal(byWord('refute').length, 2)
  assert.equal(result.findings.length, 2)
  assert.equal(result.findings[0].summary, 'first')
  assert.ok(result.findings.every((f) => f.state === 'refuted'))
  assert.equal(result.status, 'done')
})

test('refuted findings stay refuted', async () => {
  const { result, byWord } = await run({
    adversarial: () => ({ findings: [finding('src/a.go', 1), finding('src/b.go', 2)] }),
    refute: (p) => ({ confirmed: p.includes('src/b.go:2'), reason: '' }),
  })
  // Round 1: two refuters. Round 2: A is dropped as refuted, B was fixed and comes back, so one refuter.
  const refuters = byWord('refute').map((c) => c.opts.label)
  assert.deepEqual(refuters, ['refute src/a.go:1', 'refute src/b.go:2', 'refute src/b.go:2'])
  const a = result.findings.find((f) => f.file === 'src/a.go')
  assert.equal(a.state, 'refuted')
})

test('a refuter that dies is not a refutation: the finding stays open and the run stops', async () => {
  const { result, byWord } = await run({
    adversarial: () => ({ findings: [finding('src/a.go', 1, 'real bug')] }),
    refute: () => null,
  })
  assert.equal(result.status, 'stopped')
  assert.deepEqual(result.findings, [{ file: 'src/a.go', line: 1, summary: 'real bug', state: 'open' }])
  assert.match(result.deviations.at(-1), /^FAILED: /)
  assert.equal(byWord('fix').length, 0)
})

test('a refuted gate failure gets a new refuter in the next round and never gives done', async () => {
  const red = () => ({
    head_sha: HEAD,
    tests: { ran: 5, passed: 5, failed: 0, skipped: 0 },
    gates: [{ command: 'go test ./...', exit_code: 1, ran: 5, passed: 4, failed: 1, skipped: 0, output_tail: 'FAIL x', problems: [] }],
  })
  const { result, byWord } = await run({ gates: red })
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.tests, { ran: 5, passed: 4, failed: 1, skipped: 0 }, 'totals come from the per-gate counts')
  assert.equal(byWord('refute').length, 2, 'one refuter in each round')
  assert.equal(result.findings.length, 1)
  assert.equal(result.findings[0].state, 'open')
})

test('a refuted gate failure that passes in the next round gives done', async () => {
  const { result, byWord } = await run({
    gates: (p, o, n) => (n === 1
      ? { head_sha: HEAD, tests: { ran: 5, passed: 4, failed: 1, skipped: 0 }, gates: [{ command: 'go test ./...', exit_code: 1, ran: 5, passed: 4, failed: 1, skipped: 0, output_tail: 'flaky', problems: [] }] }
      : cleanGates()),
  })
  assert.equal(result.status, 'done')
  assert.equal(byWord('gates').length, 2)
  assert.equal(byWord('fix').length, 0)
})

test('a gate that stays red keeps one open finding, never a fixed one', async () => {
  const gate = (tail) => ({ command: 'make test', exit_code: 1, ran: 1, passed: 0, failed: 1, skipped: 0, output_tail: tail, problems: [] })
  const { result } = await run({
    gates: (p, o, n) => ({ head_sha: HEAD, tests: {}, gates: [gate(`FAIL pkg 0.${n}s`)] }),
    refute: () => ({ confirmed: true }),
  }, baseArgs({ gates: ['make test'], round_cap: 3 }))
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.findings.map((f) => f.state), ['open'])
})

test('a red gate that is clean in the next round is fixed', async () => {
  const { result } = await run({
    gates: (p, o, n) => (n === 1
      ? { head_sha: HEAD, tests: {}, gates: [{ command: 'go test ./...', exit_code: 1, ran: 5, passed: 4, failed: 1, skipped: 0, output_tail: 'FAIL', problems: [] }] }
      : cleanGates()),
    refute: () => ({ confirmed: true }),
  })
  assert.equal(result.status, 'done')
  assert.deepEqual(result.findings.map((f) => f.state), ['fixed'])
})

test('gate results must cover exactly args.gates', async () => {
  const empty = await run({ gates: () => ({ head_sha: HEAD, tests: {}, gates: [] }) })
  assert.notEqual(empty.result.status, 'done')
  assert.equal(empty.result.status, 'findings_left')
  assert.match(empty.result.findings[0].summary, /has no result/)

  const two = baseArgs({ gates: ['go test ./...', 'make lint'] })
  const oneMissing = await run({}, two)
  assert.equal(oneMissing.result.status, 'findings_left')
  assert.deepEqual(oneMissing.result.findings.map((f) => f.file), ['gate: make lint'])

  const extra = await run({ gates: () => ({ ...cleanGates(), gates: [...cleanGates().gates, { command: 'rm -rf x', exit_code: 0, ran: 1, passed: 1, failed: 0, skipped: 0, problems: [] }] }) })
  assert.equal(extra.result.status, 'findings_left')
  assert.match(extra.result.findings[0].summary, /not a gate of the task/)

  const none = await run({ gates: () => ({ head_sha: HEAD, tests: {}, gates: [{ command: 'go test ./...', exit_code: 0, ran: 0, passed: 0, failed: 0, skipped: 0, problems: [] }] }) })
  assert.equal(none.result.status, 'findings_left')
  assert.match(none.result.findings[0].summary, /ran no tests/)
})

test('a dead fixer, adversarial, or invariant agent stops the run, never done', async () => {
  const confirmedFinding = { adversarial: () => ({ findings: [finding('src/a.go', 1)] }), refute: () => ({ confirmed: true }) }
  const fixer = await run({ ...confirmedFinding, fix: () => null })
  assert.equal(fixer.result.status, 'stopped')
  assert.match(fixer.result.deviations.at(-1), /^FAILED: /)
  assert.equal(fixer.result.findings[0].state, 'open')
  for (const w of ['adversarial', 'invariants']) {
    const r = await run({ [w]: () => null })
    assert.equal(r.result.status, 'stopped', `dead ${w}`)
    assert.match(r.result.deviations.at(-1), /^FAILED: /)
  }
})

test('a finding that the fixer did not fix stays open', async () => {
  const { result } = await run({
    adversarial: (p, o, n) => ({ findings: [finding('src/a.go', n)] }),
    refute: () => ({ confirmed: true }),
    fix: () => ({ head_sha: HEAD, fixed: [], deviations: [] }),
  })
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.findings.map((f) => `${f.line} ${f.state}`), ['1 open', '2 open'])
})

test('file paths are normalized before dedup', async () => {
  const { byWord } = await run({
    adversarial: () => ({ findings: [finding('./src/a.go', 3)] }),
    invariants: () => ({ findings: [finding('src/a.go', 3)] }),
  })
  assert.equal(byWord('refute').length, 1)
})

test('round cap stops with findings_left', async () => {
  const { result, byWord } = await run({
    adversarial: () => ({ findings: [finding('src/a.go', 7)] }),
    refute: () => ({ confirmed: true, reason: 'shown' }),
  })
  assert.equal(result.status, 'findings_left')
  assert.equal(byWord('adversarial').length, 2)
  assert.equal(byWord('fix').length, 1)
  assert.deepEqual(result.findings, [{ file: 'src/a.go', line: 7, summary: 'bad', state: 'open' }])
})

test('round_cap from args is used, default 2', async () => {
  const always = { adversarial: () => ({ findings: [finding('src/a.go', 7)] }), refute: () => ({ confirmed: true }) }
  const three = await run(always, baseArgs({ round_cap: 3 }))
  assert.equal(three.byWord('adversarial').length, 3)
  const unset = await run(always, baseArgs({ round_cap: undefined }))
  assert.equal(unset.byWord('adversarial').length, 2)
})

test('fixes run in sequential batches by area', async () => {
  let active = 0
  let peak = 0
  const { byWord, result } = await run({
    adversarial: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 1), finding('web/x.js', 2), finding('src/b.go', 3)] : [] }),
    refute: () => ({ confirmed: true }),
    fix: async (p, o, n, fs) => {
      active++
      peak = Math.max(peak, active)
      await new Promise((r) => setTimeout(r, 5))
      active--
      return { head_sha: HEAD, fixed: fs, deviations: [`fixed batch ${n}`] }
    },
  })
  assert.equal(peak, 1)
  const labels = byWord('fix').map((c) => c.opts.label)
  assert.deepEqual(labels, ['fix src', 'fix web'])
  assert.equal(result.status, 'done')
  assert.ok(result.findings.every((f) => f.state === 'fixed'))
  assert.deepEqual(result.deviations, ['fixed batch 1', 'fixed batch 2'])
})

test('a skipped required test becomes a finding', async () => {
  const skipped = () => ({
    head_sha: HEAD,
    tests: { ran: 10, passed: 9, failed: 0, skipped: 1 },
    gates: [{
      command: 'go test ./...', exit_code: 0, ran: 10, passed: 9, failed: 0, skipped: 1,
      problems: [{ file: 'src/a_test.go', line: 12, summary: 'TestThing skipped', kind: 'skipped' }],
    }],
  })
  const { result, byWord } = await run({ gates: skipped, refute: () => ({ confirmed: true }) }, baseArgs({ round_cap: 1 }))
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.tests, { ran: 10, passed: 9, failed: 0, skipped: 1 })
  assert.equal(result.findings.length, 1)
  assert.equal(result.findings[0].file, 'src/a_test.go')
  assert.equal(result.findings[0].state, 'open')
  assert.ok(byWord('refute')[0].prompt.includes('go test ./...'), 'the refuter reruns the gate')
})

test('a skip count without a named test is still a finding', async () => {
  const gates = () => ({
    head_sha: HEAD,
    tests: { ran: 3, passed: 2, failed: 0, skipped: 1 },
    gates: [{ command: 'make test', exit_code: 0, ran: 3, passed: 2, failed: 0, skipped: 1, problems: [] }],
  })
  const { result } = await run({ gates, refute: () => ({ confirmed: true }) }, baseArgs({ round_cap: 1, gates: ['make test'] }))
  assert.equal(result.findings.length, 1)
  assert.equal(result.findings[0].file, 'gate: make test')
})

test('a failed gate without named failures is a finding', async () => {
  const gates = () => ({
    head_sha: HEAD,
    tests: { ran: 0, passed: 0, failed: 0, skipped: 0 },
    gates: [{ command: 'make lint', exit_code: 2, ran: 0, passed: 0, failed: 0, skipped: 0, problems: [] }],
  })
  const { result } = await run({ gates, refute: () => ({ confirmed: true }) }, baseArgs({ round_cap: 1, gates: ['make lint'] }))
  assert.equal(result.findings[0].file, 'gate: make lint')
})

test('a missing review check stops the run', async () => {
  const { result } = await run({ gates: () => null })
  assert.equal(result.status, 'stopped')
  assert.match(result.deviations.at(-1), /^FAILED: /)
  const p = await run({ plan: () => null })
  assert.match(p.result.deviations.at(-1), /^FAILED: /)
})

test('a plan STOP and an implement conflict stop the run', async () => {
  const p = await run({ plan: () => ({ plan: '', stop: true, reason: 'base SHA is not an ancestor of HEAD' }) })
  assert.equal(p.result.status, 'stopped')
  assert.equal(p.byWord('implement').length, 0)
  assert.deepEqual(p.result.deviations, ['STOP: base SHA is not an ancestor of HEAD'])
  const i = await run({ implement: () => ({ head_sha: HEAD, deviations: ['used a map'], conflict: 'api/router.go is not on the list' }) })
  assert.equal(i.result.status, 'stopped')
  assert.equal(i.byWord('adversarial').length, 0)
  assert.deepEqual(i.result.deviations, ['used a map', 'CONFLICT: api/router.go is not on the list'])
})

const Q = { id: 'Q-5', header: 'P1 Q-5: which table?', body: 'Use table A or table B?' }

test('question path returns status question', async () => {
  const { result, byWord, calls } = await run({
    plan: () => ({ plan: '', stop: false, reason: '', question: Q }),
  })
  assert.equal(result.status, 'question')
  assert.deepEqual(result.question, Q)
  assert.equal(byWord('implement').length, 0)
  assert.equal(calls.length, 1)
  const p = calls[0].prompt
  for (const s of ['question_open', 'SendMessage', 'main', 'answer_wait', 'deadline_seconds 600', 'pending']) {
    assert.ok(p.includes(s), `the plan prompt does not name ${s}`)
  }
})

test('answer reaches only prompts after the question', async () => {
  const plan = (p) => (p.includes('Q-5') ? { plan: 'use table B', stop: false, reason: '' } : { plan: '', stop: false, reason: '', question: Q })
  const first = await run({ plan })
  const answer = 'Use table B. (owner, 2026-09-30, Q-5)'
  const second = await run({ plan }, baseArgs({ answers: { 'Q-5': answer } }))
  assert.equal(second.result.status, 'done')
  // The first agent call is the same in both runs, so resumeFromRunId returns its cached result.
  assert.equal(second.calls[0].prompt, first.calls[0].prompt)
  assert.deepEqual(second.calls[0].opts, first.calls[0].opts)
  const withAnswer = second.calls.filter((c) => c.prompt.includes(answer))
  assert.equal(withAnswer.length, 1)
  assert.equal(withAnswer[0], second.calls[1])
})

test('a question pending again after its answer goes back to the clerk with a counter, then stops', async () => {
  const always = { plan: () => ({ plan: '', stop: false, reason: '', question: Q }) }
  const one = await run(always, baseArgs({ answers: { 'Q-5': 'B' } }))
  assert.equal(one.result.status, 'question')
  assert.deepEqual(one.result.question, Q)
  assert.equal(one.result.deviations.at(-1), 'REPEAT: Q-5 1 of 2')
  const two = await run(always, baseArgs({ answers: { 'Q-5': ['B', 'C'] } }))
  assert.equal(two.result.status, 'question')
  assert.equal(two.result.deviations.at(-1), 'REPEAT: Q-5 2 of 2')
  // The earlier attempts keep their prompts, so a relaunch returns them from the cache.
  assert.equal(two.calls[0].prompt, one.calls[0].prompt)
  assert.equal(two.calls[1].prompt, one.calls[1].prompt)
  const three = await run(always, baseArgs({ answers: { 'Q-5': ['B', 'C', 'D'] } }))
  assert.equal(three.result.status, 'stopped')
  assert.match(three.result.deviations.at(-1), /^STOP: Q-5 is pending again after 3 answers/)
})

test('a fixer question returns status question with the findings so far', async () => {
  const { result } = await run({
    adversarial: () => ({ findings: [finding('src/a.go', 1)] }),
    refute: () => ({ confirmed: true }),
    fix: () => ({ head_sha: HEAD, fixed: [], deviations: [], question: Q }),
  })
  assert.equal(result.status, 'question')
  assert.deepEqual(result.question, Q)
  assert.equal(result.findings[0].state, 'open')
})

test('no agent prompt allows posts outside the project or pushes', async () => {
  const { calls } = await run({
    adversarial: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 1)] : [] }),
    refute: () => ({ confirmed: true }),
  })
  for (const c of calls) {
    assert.ok(c.prompt.includes('Do not post outside the project'), `${c.opts.label} has no post rule`)
    assert.ok(c.prompt.includes('Do not push'), `${c.opts.label} has no push rule`)
  }
})
