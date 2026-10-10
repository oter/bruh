// Control-flow tests for the deliver workflow, with stub Workflow globals.
// Run: node --test plugins/bruh/workflows/deliver.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const source = readFileSync(join(import.meta.dirname, 'deliver.js'), 'utf8')
const guide = readFileSync(join(import.meta.dirname, '../skills/implement/references/simplicity.md'), 'utf8')
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
    simplicity: () => ({ findings: [] }),
    gates: cleanGates,
    refute: () => ({ confirmed: false, reason: 'not shown' }),
    fix: (p, o, n, fs) => ({ head_sha: HEAD, fixed: fs, deviations: [] }),
    phase: () => ({ ok: true }),
    ...h,
  }
  const agent = async (prompt, opts = {}) => {
    const word = (opts.label || '').split(' ')[0]
    const n = (counts[word] = (counts[word] || 0) + 1)
    calls.push({ prompt, opts, word })
    assert.ok(handlers[word], `no handler for label ${opts.label}`)
    // A fixer gets the findings of its batch, read from its prompt lines "- [F<n>] file:line: summary".
    const batch = [...prompt.matchAll(/^- \[(F\d+)\] (\S+):(\d+): /gm)].map((m) => ({ id: m[1], file: m[2], line: Number(m[3]) }))
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

// Runs check on each row, whose first item is its label, and fails once with every row that
// failed, so one failing row does not hide another.
async function eachRow(rows, check) {
  const failed = []
  for (const row of rows) {
    try {
      await check(...row)
    } catch (e) {
      failed.push(`${row[0]}: ${e.message}`)
    }
  }
  assert.deepEqual(failed, [], 'failed rows')
}

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
  await eachRow([
    ['short base_sha', baseArgs({ base_sha: 'abc' }), /^STOP: /],
    ['empty task', baseArgs({ task: '' }), /^STOP: /],
    ['empty branch', baseArgs({ branch: '' }), /^STOP: /],
    ['gates is a string', baseArgs({ gates: 'go test' }), /^STOP: /],
    ['no gates', baseArgs({ gates: [] }), /^STOP: /],
    ['test_gates is a string', baseArgs({ test_gates: 'go test ./...' }), /^STOP: .*test_gates/],
    ['a test gate that is not in gates', baseArgs({ test_gates: ['make test'] }), /^STOP: .*test_gates/],
    ['no args', null, null],
  ], async (label, args, stop) => {
    const { result, calls } = await run({}, args)
    assert.equal(result.status, 'stopped')
    assert.equal(calls.length, 0)
    if (stop) assert.match(result.deviations.at(-1), stop)
  })
})

test('reviewers diff against base_sha, never origin/main', async () => {
  const { byWord } = await run({}, baseArgs({ test_gates: ['go test ./...'] }))
  for (const w of ['adversarial', 'invariants', 'simplicity', 'gates']) {
    const [c] = byWord(w)
    assert.ok(c.prompt.includes(`git diff ${SHA} HEAD`), `${w} does not diff against base_sha`)
    assert.doesNotMatch(c.prompt, /git diff[^\n]*origin\/main/)
    assert.ok(c.prompt.includes('HOUSE RULES TEXT'), `${w} has no house rules`)
    assert.ok(c.prompt.includes('the global lock'), `${w} has no deliberate choices`)
  }
  // The gate agent gets the list of the test gates.
  assert.match(byWord('gates')[0].prompt, /These gates are test suites and must run tests:\n- go test \.\/\.\.\./)
})

test('reviewers run at low effort and implementers at high effort', async () => {
  const { byWord } = await run({
    adversarial: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 3)] : [] }),
    refute: () => ({ confirmed: true, reason: 'shown' }),
  })
  for (const w of ['adversarial', 'invariants', 'simplicity', 'gates', 'refute']) {
    for (const c of byWord(w)) assert.equal(c.opts.effort, 'low', `${w} effort`)
  }
  for (const w of ['plan', 'implement', 'fix']) {
    assert.ok(byWord(w).length > 0, `no ${w} call`)
    for (const c of byWord(w)) assert.equal(c.opts.effort, 'high', `${w} effort`)
  }
})

test('dedup by file:line, after the paths are normalized', async () => {
  await eachRow([
    ['same file:line from both reviewers', {
      adversarial: () => ({ findings: [finding('src/a.go', 3, 'first'), finding('src/a.go', 3, 'again')] }),
      invariants: () => ({ findings: [finding('src/a.go', 3, 'same line'), finding('src/a.go', 4)] }),
    }, ({ result, byWord }) => {
      assert.equal(byWord('refute').length, 2)
      assert.equal(result.findings.length, 2)
      assert.equal(result.findings[0].summary, 'first')
      assert.ok(result.findings.every((f) => f.state === 'refuted'))
      assert.equal(result.status, 'done')
    }],
    ['./src/a.go and src/a.go', {
      adversarial: () => ({ findings: [finding('./src/a.go', 3)] }),
      invariants: () => ({ findings: [finding('src/a.go', 3)] }),
    }, ({ byWord }) => assert.equal(byWord('refute').length, 1)],
  ], async (label, h, check) => check(await run(h)))
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

// Task 45 (owner Q-219): a simplicity reviewer runs next to the adversarial reviewer and the
// invariant checker, and its findings take the same refuter and fix loop.
test('the simplicity reviewer runs in each review round and its findings join the refuter loop', async () => {
  const simpler = 'a wrapper for one call; call os.ReadFile directly'
  const confirmed = await run({
    simplicity: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 5, simpler)] : [] }),
    refute: () => ({ confirmed: true, reason: 'shown' }),
  })
  assert.equal(confirmed.byWord('simplicity').length, confirmed.byWord('adversarial').length, 'one simplicity reviewer in each round')
  assert.equal(confirmed.byWord('simplicity').length, 2)
  assert.equal(confirmed.byWord('simplicity')[1].opts.label, 'simplicity 2')
  const prompt = confirmed.byWord('simplicity')[0].prompt
  // simplicity.md is the source of the finding kinds and of the list of what is not a finding.
  const bullets = guide.split('Finding kinds:')[1].match(/^- .+$/gm).map((l) => l.slice(2))
  assert.equal(bullets.length, 13)
  for (const s of [...bullets, 'concrete simpler replacement', 'never overrides the acceptance criteria or a deliberate choice']) {
    assert.ok(prompt.includes(s), `the simplicity prompt does not name ${s}`)
  }
  assert.deepEqual(confirmed.byWord('refute').map((c) => c.opts.label), ['refute src/a.go:5'])
  assert.match(confirmed.byWord('fix')[0].prompt, /^- \[F1\] src\/a\.go:5: a wrapper for one call; call os\.ReadFile directly$/m)
  assert.equal(confirmed.result.status, 'done')
  assert.deepEqual(confirmed.result.findings, [{ file: 'src/a.go', line: 5, summary: simpler, state: 'fixed' }])

  // The same file:line from the adversarial reviewer and the simplicity reviewer gets one refuter.
  const same = await run({
    adversarial: () => ({ findings: [finding('src/a.go', 5, 'adversarial')] }),
    simplicity: () => ({ findings: [finding('./src/a.go', 5, 'simplicity')] }),
  })
  assert.equal(same.byWord('refute').length, 1)
  assert.deepEqual(same.result.findings.map((f) => f.summary), ['adversarial'])

  // A refuted simplicity finding stays refuted in the next round.
  const refuted = await run({
    adversarial: () => ({ findings: [finding('src/b.go', 2)] }),
    simplicity: () => ({ findings: [finding('src/a.go', 5, simpler)] }),
    refute: (p) => ({ confirmed: p.includes('src/b.go:2'), reason: '' }),
  })
  assert.deepEqual(refuted.byWord('refute').map((c) => c.opts.label), ['refute src/b.go:2', 'refute src/a.go:5', 'refute src/b.go:2'])
  assert.equal(refuted.result.findings.find((f) => f.file === 'src/a.go').state, 'refuted')
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

// A red gate is refuted (a flake) or confirmed, and stays red or is clean in the next round.
test('a red gate never gives done while it stays red', async () => {
  const red = (tail, tests = {}) => ({
    head_sha: HEAD, tests,
    gates: [{ command: 'go test ./...', exit_code: 1, ran: 5, passed: 4, failed: 1, skipped: 0, output_tail: tail, problems: [] }],
  })
  const confirmed = { refute: () => ({ confirmed: true }) }
  await eachRow([
    ['refuted, stays red: a new refuter in each round', { gates: () => red('FAIL x', { ran: 5, passed: 5, failed: 0, skipped: 0 }) }, baseArgs(), ({ result, byWord }) => {
      assert.equal(result.status, 'findings_left')
      assert.deepEqual(result.tests, { ran: 5, passed: 4, failed: 1, skipped: 0 }, 'totals come from the per-gate counts')
      assert.equal(byWord('refute').length, 2, 'one refuter in each round')
      assert.equal(result.findings.length, 1)
      assert.equal(result.findings[0].state, 'open')
    }],
    ['refuted, clean next round: done', { gates: (p, o, n) => (n === 1 ? red('flaky', { ran: 5, passed: 4, failed: 1, skipped: 0 }) : cleanGates()) }, baseArgs(), ({ result, byWord }) => {
      assert.equal(result.status, 'done')
      assert.equal(byWord('gates').length, 2)
      assert.equal(byWord('fix').length, 0)
    }],
    // The output tail differs in each round, so the finding is matched by its gate, not its text.
    ['confirmed, stays red: one open finding, never a fixed one', {
      ...confirmed,
      gates: (p, o, n) => ({ head_sha: HEAD, tests: {}, gates: [{ command: 'make test', exit_code: 1, ran: 1, passed: 0, failed: 1, skipped: 0, output_tail: `FAIL pkg 0.${n}s`, problems: [] }] }),
    }, baseArgs({ gates: ['make test'], round_cap: 3 }), ({ result }) => {
      assert.equal(result.status, 'findings_left')
      assert.deepEqual(result.findings.map((f) => f.state), ['open'])
    }],
    ['confirmed, clean next round: fixed', { ...confirmed, gates: (p, o, n) => (n === 1 ? red('FAIL') : cleanGates()) }, baseArgs(), ({ result }) => {
      assert.equal(result.status, 'done')
      assert.deepEqual(result.findings.map((f) => f.state), ['fixed'])
    }],
  ], async (label, h, args, check) => check(await run(h, args)))
})

// Task 49: at the round cap a refuted gate finding stays refuted (a Go gate without -v prints no
// count), and a real gate failure (exit not 0, a failed or skipped test) stays open.
test('at the round cap a refuted gate finding stays refuted and a real gate failure stays open', async () => {
  const gate = (x) => () => ({ head_sha: HEAD, tests: {}, gates: [{ command: 'go test ./...', exit_code: 0, ran: 5, passed: 5, failed: 0, skipped: 0, problems: [], ...x }] })
  await eachRow([
    ['a test gate with no count: refuted', { ran: 0, passed: 0 }, 'refuted'],
    ['a gate that exits 1 with a failed test: open', { exit_code: 1, passed: 4, failed: 1 }, 'open'],
    ['a gate that skips a test: open', { passed: 4, skipped: 1 }, 'open'],
  ], async (label, x, state) => {
    const { result } = await run({ gates: gate(x) }, baseArgs({ round_cap: 1, test_gates: ['go test ./...'] }))
    assert.equal(result.status, 'findings_left')
    assert.deepEqual(result.findings.map((f) => f.state), [state])
  })
})

test('the gates prompt counts a Go gate that prints no test count', async () => {
  const { byWord } = await run()
  const prompt = byWord('gates')[0].prompt
  assert.match(prompt, /no test count \(go test without -v prints only `ok <pkg>` or `ok <pkg> \(cached\)`\)/)
  assert.match(prompt, /-count=1 -v/)
})

// The gate results must cover exactly args.gates; each gate must report and exit 0, and only
// the gates of args.test_gates must run tests (final review M6).
test('gate results become findings', async () => {
  const lint = (exit) => ({ command: 'make lint', exit_code: exit, ran: 0, passed: 0, failed: 0, skipped: 0, output_tail: exit ? 'lint error' : '', problems: [] })
  const withLint = (exit) => () => ({ ...cleanGates(), gates: [...cleanGates().gates, lint(exit)] })
  const twoGates = (extra = {}) => baseArgs({ gates: ['go test ./...', 'make lint'], ...extra })
  const left = (check) => ({ result }) => {
    assert.equal(result.status, 'findings_left')
    check(result.findings)
  }
  await eachRow([
    ['no gate result', { gates: () => ({ head_sha: HEAD, tests: {}, gates: [] }) }, baseArgs(),
      left((fs) => assert.match(fs[0].summary, /has no result/))],
    ['one gate result missing', {}, twoGates(),
      left((fs) => assert.deepEqual(fs.map((f) => f.file), ['gate: make lint']))],
    ['a result of a gate that is not a gate of the task', { gates: () => ({ ...cleanGates(), gates: [...cleanGates().gates, { command: 'rm -rf x', exit_code: 0, ran: 1, passed: 1, failed: 0, skipped: 0, problems: [] }] }) }, baseArgs(),
      left((fs) => assert.match(fs[0].summary, /not a gate of the task/))],
    ['a test gate that ran no tests', { gates: () => ({ head_sha: HEAD, tests: {}, gates: [{ command: 'go test ./...', exit_code: 0, ran: 0, passed: 0, failed: 0, skipped: 0, problems: [] }] }) }, baseArgs({ test_gates: ['go test ./...'] }),
      left((fs) => assert.match(fs[0].summary, /ran no tests/))],
    ['a skip count without a named test', { refute: () => ({ confirmed: true }), gates: () => ({ head_sha: HEAD, tests: { ran: 3, passed: 2, failed: 0, skipped: 1 }, gates: [{ command: 'make test', exit_code: 0, ran: 3, passed: 2, failed: 0, skipped: 1, problems: [] }] }) },
      baseArgs({ round_cap: 1, gates: ['make test'] }),
      ({ result }) => assert.deepEqual(result.findings.map((f) => f.file), ['gate: make test'])],
    ['a failed gate without named failures', { refute: () => ({ confirmed: true }), gates: () => ({ head_sha: HEAD, tests: { ran: 0, passed: 0, failed: 0, skipped: 0 }, gates: [{ command: 'make lint', exit_code: 2, ran: 0, passed: 0, failed: 0, skipped: 0, problems: [] }] }) },
      baseArgs({ round_cap: 1, gates: ['make lint'] }),
      ({ result }) => assert.equal(result.findings[0].file, 'gate: make lint')],
    ['a lint gate with no test count and exit 0 is clean', { gates: withLint(0) }, twoGates({ test_gates: ['go test ./...'] }), ({ result }) => {
      assert.equal(result.status, 'done')
      assert.deepEqual(result.findings, [])
    }],
    ['a lint gate must still exit 0', { gates: withLint(2) }, twoGates({ test_gates: ['go test ./...'] }),
      left((fs) => assert.deepEqual(fs.map((f) => f.file), ['gate: make lint']))],
    ['without test_gates, no gate needs a test count', { gates: withLint(0) }, twoGates(), ({ result }) => assert.equal(result.status, 'done')],
  ], async (label, h, args, check) => check(await run(h, args)))
})

test('a dead agent stops the run with FAILED, never done', async () => {
  const confirmedFinding = { adversarial: () => ({ findings: [finding('src/a.go', 1)] }), refute: () => ({ confirmed: true }) }
  await eachRow([
    ['dead fixer', { ...confirmedFinding, fix: () => null }, (result) => assert.equal(result.findings[0].state, 'open')],
    ['dead adversarial reviewer', { adversarial: () => null }],
    ['dead invariant checker', { invariants: () => null }],
    ['dead simplicity reviewer', { simplicity: () => null }],
    ['dead gate agent', { gates: () => null }],
    ['dead planner', { plan: () => null }],
  ], async (label, h, check) => {
    const { result } = await run(h)
    assert.equal(result.status, 'stopped')
    assert.match(result.deviations.at(-1), /^FAILED: /)
    check?.(result)
  })
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

// Task 32: the fix of src/a.go:61 inserts lines above it, so the fixer reports line 67. The ID
// closes the finding, so the run ends done, not findings_left.
test('a fix that moves the line of its finding closes it by ID', async () => {
  const { result, byWord } = await run({
    adversarial: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 61)] : [] }),
    refute: () => ({ confirmed: true }),
    fix: (p, o, n, fs) => ({ head_sha: HEAD, fixed: fs.map((f) => ({ id: f.id, file: f.file, line: f.line + 6 })), deviations: [] }),
  })
  assert.match(byWord('fix')[0].prompt, /^- \[F1\] src\/a\.go:61: bad$/m)
  assert.equal(result.status, 'done')
  assert.deepEqual(result.findings, [{ file: 'src/a.go', line: 61, summary: 'bad', state: 'fixed' }])
})

test('a fixer that reports only file and line closes nothing', async () => {
  const { result } = await run({
    adversarial: () => ({ findings: [finding('src/a.go', 61)] }),
    refute: () => ({ confirmed: true }),
    fix: () => ({ head_sha: HEAD, fixed: [{ file: 'src/a.go', line: 61 }], deviations: [] }),
  })
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.findings.map((f) => f.state), ['open'])
})

test('round_cap from args stops with findings_left, default 2', async () => {
  const always = { adversarial: () => ({ findings: [finding('src/a.go', 7)] }), refute: () => ({ confirmed: true, reason: 'shown' }) }
  await eachRow([
    ['round_cap 2', 2, ({ result, byWord }) => {
      assert.equal(result.status, 'findings_left')
      assert.equal(byWord('adversarial').length, 2)
      assert.equal(byWord('fix').length, 1)
      assert.deepEqual(result.findings, [{ file: 'src/a.go', line: 7, summary: 'bad', state: 'open' }])
    }],
    ['round_cap 3', 3, ({ byWord }) => assert.equal(byWord('adversarial').length, 3)],
    ['no round_cap: 2', undefined, ({ byWord }) => assert.equal(byWord('adversarial').length, 2)],
  ], async (label, cap, check) => check(await run(always, baseArgs({ round_cap: cap }))))
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

const Q = { id: 'Q-app-host-5', header: 'P1 Q-app-host-5: which table?', body: 'Use table A or table B?' }

test('question path returns status question', async () => {
  const { result, byWord, calls } = await run({
    plan: () => ({ plan: '', stop: false, reason: '', question: Q }),
  })
  assert.equal(result.status, 'question')
  assert.deepEqual(result.question, Q)
  assert.equal(byWord('implement').length, 0)
  assert.equal(calls.length, 1)
  const p = calls[0].prompt
  for (const s of ['question_open', 'SendMessage', 'main', 'answer_wait', 'deadline_seconds 600', 'pending', 'options', 'the body that question_open returned']) {
    assert.ok(p.includes(s), `the plan prompt does not name ${s}`)
  }
})

test('answer reaches only prompts after the question', async () => {
  const plan = (p) => (p.includes('Q-app-host-5') ? { plan: 'use table B', stop: false, reason: '' } : { plan: '', stop: false, reason: '', question: Q })
  const first = await run({ plan })
  const answer = 'Use table B. (owner, 2026-09-30, Q-app-host-5)'
  const second = await run({ plan }, baseArgs({ answers: { 'Q-app-host-5': answer } }))
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
  const one = await run(always, baseArgs({ answers: { 'Q-app-host-5': 'B' } }))
  assert.equal(one.result.status, 'question')
  assert.deepEqual(one.result.question, Q)
  assert.equal(one.result.deviations.at(-1), 'REPEAT: Q-app-host-5 1 of 2')
  const two = await run(always, baseArgs({ answers: { 'Q-app-host-5': ['B', 'C'] } }))
  assert.equal(two.result.status, 'question')
  assert.equal(two.result.deviations.at(-1), 'REPEAT: Q-app-host-5 2 of 2')
  // The earlier attempts keep their prompts, so a relaunch returns them from the cache.
  assert.equal(two.calls[0].prompt, one.calls[0].prompt)
  assert.equal(two.calls[1].prompt, one.calls[1].prompt)
  const three = await run(always, baseArgs({ answers: { 'Q-app-host-5': ['B', 'C', 'D'] } }))
  assert.equal(three.result.status, 'stopped')
  assert.match(three.result.deviations.at(-1), /^STOP: Q-app-host-5 is pending again after 3 answers/)
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

// Task 30: the agents write each phase of the task with report_write, in step order.
const phasesOf = (calls) => calls.flatMap((c) => [...c.prompt.matchAll(/report_write \(mcp__plugin_bruh_bruh__report_write; load it with ToolSearch\) with kind event, text "phase (\w+)", and phase "\1"/g)].map((m) => m[1]))

test('the deliver flow writes the phases plan, implement, review, fix, and done in order', async () => {
  const { result, calls, byWord } = await run({
    adversarial: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 1), finding('web/x.js', 2)] : [] }),
    refute: () => ({ confirmed: true }),
  })
  assert.equal(result.status, 'done')
  assert.deepEqual(phasesOf(calls), ['plan', 'implement', 'review', 'fix', 'review', 'done'])
  assert.equal(byWord('fix').length, 2, 'two areas, only the first fixer writes the phase')
  const [done] = byWord('phase')
  assert.equal(done, calls.at(-1), 'done is the last agent')
  assert.equal(done.opts.effort, 'low')
  // A dead phase agent does not change the status.
  assert.equal((await run({ phase: () => null })).result.status, 'done')
})

test('a stopped or findings_left run writes no phase done', async () => {
  const left = await run({ adversarial: () => ({ findings: [finding('src/a.go', 7)] }), refute: () => ({ confirmed: true }) })
  assert.equal(left.result.status, 'findings_left')
  const stopped = await run({ implement: () => ({ head_sha: HEAD, deviations: [], conflict: 'x' }) })
  assert.equal(stopped.result.status, 'stopped')
  for (const r of [left, stopped]) {
    assert.equal(r.byWord('phase').length, 0)
    assert.ok(!phasesOf(r.calls).includes('done'))
  }
})

// Task 36: the guides folder of the task reaches only the reviewers and the refuters.
const guideHandlers = {
  adversarial: (p, o, n) => ({ findings: n === 1 ? [finding('src/a.go', 1)] : [] }),
  refute: () => ({ confirmed: true, reason: 'shown' }),
}
const reviewWords = ['adversarial', 'invariants', 'simplicity', 'refute']

test('deliver with a guides folder puts GUIDES ARE MANDATORY into the three reviewers and the refuter', async () => {
  const { result, byWord, calls } = await run(guideHandlers, baseArgs({ guides: '/tmp/clerk-x-guides/' }))
  assert.equal(result.status, 'done')
  assert.equal(byWord('adversarial').length, 2)
  assert.equal(byWord('invariants').length, 2)
  assert.equal(byWord('simplicity').length, 2)
  assert.equal(byWord('refute').length, 1)
  for (const w of reviewWords) {
    for (const c of byWord(w)) {
      assert.ok(c.prompt.includes('GUIDES ARE MANDATORY'), `${c.opts.label} has no guides rule`)
      assert.ok(c.prompt.includes('/tmp/clerk-x-guides/INDEX.md'), `${c.opts.label} does not name INDEX.md`)
    }
  }
  assert.ok(byWord('refute')[0].prompt.includes('Read the cited guide rule under /tmp/clerk-x-guides/.'))
  for (const c of calls.filter((x) => !reviewWords.includes(x.word))) {
    assert.ok(!c.prompt.includes('GUIDES ARE MANDATORY'), `${c.opts.label} got the guides rule`)
  }
})

test('deliver without a guides folder leaves the review prompts unchanged', async () => {
  const empty = await run(guideHandlers, baseArgs({ guides: '' }))
  const none = await run(guideHandlers, baseArgs({ guides: undefined }))
  for (const r of [empty, none]) {
    assert.equal(r.result.status, 'done')
    for (const c of r.calls) {
      assert.ok(!c.prompt.includes('GUIDES ARE MANDATORY'), `${c.opts.label} got the guides rule`)
      assert.ok(!c.prompt.includes('Read the cited guide rule'), `${c.opts.label} got the refuter guides rule`)
      assert.ok(c.prompt.includes('Guides index of the project:\n(none)\n'), `${c.opts.label} does not show (none)`)
    }
  }
  for (const w of reviewWords) {
    assert.deepEqual(empty.byWord(w).map((c) => c.prompt), none.byWord(w).map((c) => c.prompt))
  }
  // Without guides, the diff text goes straight into the lens text, as before task 36.
  assert.ok(empty.byWord('adversarial')[0].prompt.includes('another moving ref.\nTry to refute the change'))
  assert.ok(empty.byWord('invariants')[0].prompt.includes('another moving ref.\nDo not trust the claims'))
  assert.ok(empty.byWord('refute')[0].prompt.includes('another moving ref.\nFinding: src/a.go:1: '))
})

test('a guides value that is not an absolute path stops the run before any agent', async () => {
  for (const bad of ['guides', 'docs/guides', "/tmp/x'y"]) {
    const { result, calls } = await run({}, baseArgs({ guides: bad }))
    assert.equal(result.status, 'stopped')
    assert.equal(calls.length, 0)
    assert.match(result.deviations[0], /^STOP: .*args\.guides/)
  }
})
