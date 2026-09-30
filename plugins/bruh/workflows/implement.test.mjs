// Control-flow tests for the workflows of the implement skillset, with stub Workflow globals.
// Run: node --test plugins/bruh/workflows/implement.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const AsyncFunction = (async () => {}).constructor
const GLOBALS = ['agent', 'parallel', 'pipeline', 'phase', 'log', 'args', 'budget']
const META = /export const meta = (\{[\s\S]*?\n\})\n/

function load(name) {
  const source = readFileSync(join(import.meta.dirname, `${name}.js`), 'utf8')
  const script = new AsyncFunction(...GLOBALS, source.replace('export const meta', 'const meta'))
  const meta = new Function(`return ${source.match(META)[1]}`)()
  return { source, script, meta }
}

const SCRIPTS = ['tickets', 'implement-tickets', 'review-and-fix', 'review-only']
const loaded = Object.fromEntries(SCRIPTS.map((n) => [n, load(n)]))

// Runs a script. `h` maps the first word of an agent label to a handler
// (prompt, opts, callIndexForThatWord) => result. A handler that throws
// gives null, as a dead agent does in the runtime.
async function run(name, h, args) {
  const calls = []
  const counts = {}
  const phases = []
  const agent = async (prompt, opts = {}) => {
    const word = (opts.label || '').split(' ')[0]
    const n = (counts[word] = (counts[word] || 0) + 1)
    calls.push({ prompt, opts, word })
    assert.ok(h[word], `no handler for label ${opts.label}`)
    return h[word](prompt, opts, n)
  }
  const parallel = async (thunks) => Promise.all(thunks.map((t) => t().catch(() => null)))
  const pipeline = async () => {
    throw new Error('pipeline is not used')
  }
  const budget = { total: null, spent: () => 0, remaining: () => Infinity }
  const result = await loaded[name].script(agent, parallel, pipeline, (p) => phases.push(p), () => {}, args, budget)
  return { result, calls, phases, byWord: (w) => calls.filter((c) => c.word === w) }
}

const SHA = 'a'.repeat(40)
const HEAD = 'b'.repeat(40)
const GIT_RULE = 'Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase'
const POST_RULE = 'Do not post outside the project'
const Q = { id: 'Q-5', header: 'P1 Q-5: which table?', body: 'Use table A or table B?' }

// ---------- shared checks ----------

const BASE_ARGS = {
  tickets: () => ({ root: '/r', spec: '/r/.scratch/f/spec.md', issues: '/r/.scratch/f/issues', guides: '/r/.scratch/f/guides', rules: 'RULES TEXT' }),
  'implement-tickets': () => ({
    root: '/r', spec: '/r/.scratch/f/spec.md', guides: '/r/.scratch/f/guides', issues: '/r/.scratch/f/issues/',
    lane: '/p/scripts/lane.sh', waves: [['t-01-a.md']], gates: ['make test'],
  }),
  'review-and-fix': () => ({
    root: '/r', base: SHA, head: HEAD, spec: '/r/spec.md', guides: '/r/guides',
    lenses: [{ key: 'correctness', prompt: 'Check the handlers.' }, { key: 'security', prompt: 'Check the auth code.' }],
    deliberate: ['the fixed session TTL'], house_rules: 'HOUSE RULES TEXT', gates: ['make test'],
  }),
  'review-only': () => ({
    root: '/r', base: SHA, head: HEAD, guides: '/r/guides',
    lenses: [{ key: 'correctness', prompt: 'Check the handlers.' }, { key: 'security', prompt: 'Check the auth code.' }],
    deliberate: ['the fixed session TTL'],
  }),
}

const COMMON_BAD = [{ deadline_seconds: 0 }, { deadline_seconds: 'x' }, { answers: [] }, { answers: 'x' }]
const BAD_ARGS = {
  tickets: [{ root: 'relative' }, { spec: '' }, { issues: "/x'y" }, { guides: undefined }, { rules: '' }, { round_cap: 0 }, { ground: 'x' }, ...COMMON_BAD],
  'implement-tickets': [
    { root: '' }, { lane: 'lane.sh' }, { waves: [] }, { waves: [[]] }, { waves: 'x' }, { waves: [['../x.md']] },
    { waves: [['a.md'], ['a.md']] }, { gates: [] }, { fix_cap: -1 }, { issues: '/a\nb' },
    { test_gates: 'make test' }, { test_gates: ['make lint'] }, ...COMMON_BAD,
  ],
  'review-and-fix': [
    { root: 'r' }, { base: 'origin/main' }, { head: 'abc' }, { guides: '' }, { lenses: [] },
    { lenses: [{ key: 'A B', prompt: 'x' }] }, { lenses: [{ key: 'a', prompt: 'x' }, { key: 'a', prompt: 'y' }] },
    { lenses: [{ key: 'a', prompt: '' }] }, { gates: [] }, { round_cap: 0 }, { spec: 'rel' }, { deliberate: 'x' },
    { test_gates: 'make test' }, { test_gates: ['make lint'] }, ...COMMON_BAD,
  ],
  'review-only': [{ root: 'r' }, { base: 'HEAD~1' }, { head: '' }, { guides: 1 }, { lenses: 'x' }, { spec: 'rel' }, ...COMMON_BAD],
}

// Handlers for a busy run of each script: every stage runs at least once.
const f = (file, line, extra = {}) => ({ file, line, rule: 'guide R1', problem: `bad ${file}:${line}`, fix: 'do it', severity: 'bug', ...extra })
const gateResult = (command, extra = {}) => ({ command, exit_code: 0, ran: 1, passed: 1, failed: 0, skipped: 0, output_tail: '', problems: [], ...extra })
const cleanGate = (cmds = ['make test']) => ({ gates: cmds.map((c) => gateResult(c)) })
const cleanCheck = () => ({ head_sha: HEAD, status_porcelain: '', base_is_ancestor: true })
// The fixer and the fix check read the findings of their prompt: lines "1. file:line ...".
const listed = (p) => [...p.matchAll(/^\d+\. (\S+):(\d+)[: ]/gm)].map((m) => ({ file: m[1], line: Number(m[2]) }))
const confirmAll = (p) => ({ results: listed(p).map((l) => ({ ...l, fixed: true, reason: 'fixed' })) })
const BUSY = {
  tickets: {
    write: () => ({ tickets: [], notes: '' }),
    review: (p, o, n) => ({ pass: n > 1, issues: n > 1 ? [] : [{ ticket: 't1', problem: 'x', fix: 'y' }], summary: '' }),
    fix: () => ({ tickets: [], notes: '' }),
  },
  'implement-tickets': {
    impl: () => ({ done: true, workdir: '/lanes/x', summary: '', verify_output: '', files_changed: [], deviations: '', conflict: '' }),
    review: (p, o, n) => ({ pass: n > 2, findings: [{ file: 'a.go', problem: 'x', fix: 'y' }], verify_output: '', summary: '' }),
    fix: () => ({ done: true, workdir: '/lanes/x', summary: '', verify_output: '', files_changed: [], deviations: '', conflict: '' }),
    merge: () => ({ ok: true, report: '' }),
    gate: () => cleanGate(),
  },
  'review-and-fix': {
    check: cleanCheck,
    review: (p, o, n) => ({ findings: n === 1 ? [f('src/a.go', 1)] : [], summary: 'ok' }),
    gate: () => cleanGate(),
    verify: () => ({ confirmed: true, reason: 'shown', adjusted_fix: '' }),
    fix: () => ({ fixed: [{ file: 'src/a.go', line: 1 }], report: '' }),
    confirm: confirmAll,
  },
  'review-only': {
    check: cleanCheck,
    review: () => ({ findings: [f('src/a.go', 1)], summary: 'ok' }),
    verify: () => ({ confirmed: true, reason: 'shown', adjusted_fix: '' }),
  },
}
const BUSY_ARGS = {
  tickets: () => BASE_ARGS.tickets(),
  'implement-tickets': () => ({ ...BASE_ARGS['implement-tickets'](), waves: [['t-01-a.md', 't-02-b.md']] }),
  'review-and-fix': () => BASE_ARGS['review-and-fix'](),
  'review-only': () => BASE_ARGS['review-only'](),
}
// The first agent that can ask a question in each script.
const ASKER = { tickets: 'write', 'implement-tickets': 'impl', 'review-and-fix': 'fix', 'review-only': 'review' }

for (const name of SCRIPTS) {
  const { meta, source } = loaded[name]

  test(`${name}: meta.name and a literal meta`, () => {
    assert.equal(meta.name, name)
    assert.ok(meta.description)
    assert.doesNotMatch(source.match(META)[1], /[`(]|\.\.\.|\$\{/, 'meta must be a pure literal')
  })

  test(`${name}: no Post phase and no post command`, () => {
    assert.ok(!(meta.phases || []).some((p) => /post/i.test(p.title)), 'a phase named Post')
    assert.doesNotMatch(source, /phase\(\s*'Post'/)
    assert.doesNotMatch(source, /-X POST|--method POST|glab api|gh api/)
  })

  test(`${name}: invalid args stop the run before any agent`, async () => {
    for (const bad of BAD_ARGS[name]) {
      const { result, calls } = await run(name, {}, { ...BASE_ARGS[name](), ...bad })
      assert.equal(result.status, 'stopped', JSON.stringify(bad))
      assert.equal(calls.length, 0, JSON.stringify(bad))
      assert.match(result.deviations.at(-1), /^STOP: /)
      assert.equal(result.question, null)
    }
    for (const a of [null, undefined, 'x']) {
      const { result, calls } = await run(name, {}, a)
      assert.equal(result.status, 'stopped')
      assert.equal(calls.length, 0)
    }
  })

  test(`${name}: every prompt has the git rule and the post rule`, async () => {
    const { calls } = await run(name, BUSY[name], BUSY_ARGS[name]())
    const words = new Set(calls.map((c) => c.word))
    for (const w of Object.keys(BUSY[name])) assert.ok(words.has(w), `the busy run has no ${w} agent`)
    for (const c of calls) {
      assert.ok(c.prompt.includes(GIT_RULE), `${c.opts.label} has no git rule`)
      assert.ok(c.prompt.includes(POST_RULE), `${c.opts.label} has no post rule`)
      assert.equal(c.opts.model, undefined, `${c.opts.label} sets a model`)
      assert.ok(!c.prompt.includes('post-findings'), `${c.opts.label} names the post script`)
    }
  })

  test(`${name}: phases of the agents are in meta.phases`, async () => {
    const { calls, phases } = await run(name, BUSY[name], BUSY_ARGS[name]())
    const titles = new Set((meta.phases || []).map((p) => p.title))
    for (const p of phases) assert.ok(titles.has(p), `phase(${p}) is not in meta.phases`)
    for (const c of calls) if (c.opts.phase) assert.ok(titles.has(c.opts.phase), `opts.phase ${c.opts.phase} is not in meta.phases`)
  })

  // Spec 6.2 (M7): the question path of deliver.js in each workflow.
  const asker = ASKER[name]
  const askArgs = () => {
    const a = BUSY_ARGS[name]()
    return name === 'implement-tickets' ? { ...a, waves: [['t-01-a.md']], deadline_seconds: 600 } : { ...a, deadline_seconds: 600 }
  }
  const asking = () => ({
    ...BUSY[name],
    // Only the first call of the asker asks, and its attempt with the answer does not.
    [asker]: (p, o, n) => (n > 1 || p.includes('Q-5') ? BUSY[name][asker](p, o, n) : { ...BUSY[name][asker](p, o, n), question: Q }),
  })

  test(`${name}: a pending question returns status question`, async () => {
    const { result, byWord } = await run(name, asking(), askArgs())
    assert.equal(result.status, 'question')
    assert.deepEqual(result.question, Q)
    const p = byWord(asker)[0].prompt
    for (const s of ['question_open', 'SendMessage', 'main', 'answer_wait', 'deadline_seconds 600', 'pending']) {
      assert.ok(p.includes(s), `the ${asker} prompt does not name ${s}`)
    }
    if (name === 'implement-tickets') assert.equal(byWord('review').length, 0)
  })

  test(`${name}: the answer reaches only prompts after the question`, async () => {
    const first = await run(name, asking(), askArgs())
    const answer = 'Use table B. (owner, 2026-09-30, Q-5)'
    const second = await run(name, asking(), { ...askArgs(), answers: { 'Q-5': answer } })
    assert.notEqual(second.result.status, 'question')
    assert.notEqual(second.result.status, 'stopped', JSON.stringify(second.result.deviations))
    // Each call before the question is the same in both runs, so resumeFromRunId returns it cached.
    const n = first.calls.length
    for (let i = 0; i < n; i++) {
      assert.equal(second.calls[i].prompt, first.calls[i].prompt, `call ${i} changed`)
      assert.deepEqual(second.calls[i].opts, first.calls[i].opts)
    }
    const withAnswer = second.calls.filter((c) => c.prompt.includes(answer))
    assert.equal(withAnswer.length, 1)
    assert.equal(withAnswer[0], second.calls[n])
  })

  test(`${name}: a question pending again after its answer counts, then stops`, async () => {
    const always = { ...BUSY[name], [asker]: (p, o, n) => ({ ...BUSY[name][asker](p, o, n), question: Q }) }
    const one = await run(name, always, { ...askArgs(), answers: { 'Q-5': 'B' } })
    assert.equal(one.result.status, 'question')
    assert.equal(one.result.deviations.at(-1), 'REPEAT: Q-5 1 of 2')
    const three = await run(name, always, { ...askArgs(), answers: { 'Q-5': ['B', 'C', 'D'] } })
    assert.equal(three.result.status, 'stopped')
    assert.match(three.result.deviations.at(-1), /Q-5 is pending again after 3 answers/)
  })
}

// ---------- tickets ----------

test('tickets: a pass in the first review gives done', async () => {
  const { result, byWord } = await run('tickets', {
    write: () => ({ tickets: [{ id: 't1', file: 't-01.md' }], notes: 'n' }),
    review: () => ({ pass: true, issues: [], summary: '' }),
  }, BASE_ARGS.tickets())
  assert.equal(result.status, 'done')
  assert.equal(result.rounds, 1)
  assert.deepEqual(result.tickets, [{ id: 't1', file: 't-01.md' }])
  assert.equal(byWord('fix').length, 0)
  assert.ok(byWord('write')[0].prompt.includes('RULES TEXT'))
  assert.equal(byWord('write')[0].opts.effort, 'high')
  assert.equal(byWord('review')[0].opts.effort, 'low')
})

test('tickets: the round cap stops with findings_left, default 5', async () => {
  const never = {
    write: () => ({ tickets: [], notes: '' }),
    review: (p, o, n) => ({ pass: false, issues: [{ ticket: 't1', problem: `p${n}`, fix: 'f' }], summary: '' }),
    fix: () => ({ tickets: [], notes: '' }),
  }
  const three = await run('tickets', never, { ...BASE_ARGS.tickets(), round_cap: 3 })
  assert.equal(three.result.status, 'findings_left')
  assert.equal(three.byWord('review').length, 3)
  assert.equal(three.byWord('fix').length, 2)
  assert.deepEqual(three.result.issues, [{ ticket: 't1', problem: 'p3', fix: 'f' }])
  const def = await run('tickets', never, BASE_ARGS.tickets())
  assert.equal(def.byWord('review').length, 5)
  assert.ok(def.byWord('fix')[0].prompt.includes('p1'), 'the fixer gets the findings')
})

test('tickets: amendments reach the fixers from round 3 on', async () => {
  const never = {
    write: () => ({ tickets: [], notes: '' }),
    review: () => ({ pass: false, issues: [{ ticket: 't1', problem: 'p', fix: 'f' }], summary: '' }),
    fix: () => ({ tickets: [], notes: '' }),
  }
  const { byWord } = await run('tickets', never, { ...BASE_ARGS.tickets(), amendments: 'USE TABLE B' })
  const has = byWord('fix').map((c) => c.prompt.includes('USE TABLE B'))
  assert.deepEqual(has, [false, false, true, true])
})

test('tickets: a dead agent stops the run', async () => {
  for (const dead of ['write', 'review', 'fix']) {
    const h = { ...BUSY.tickets, [dead]: () => null }
    const { result } = await run('tickets', h, BASE_ARGS.tickets())
    assert.equal(result.status, 'stopped', dead)
    assert.match(result.deviations.at(-1), /^FAILED: /)
  }
})

// ---------- implement-tickets ----------

const work = (extra = {}) => ({ done: true, workdir: '/lanes/x', summary: 's', verify_output: 'ok', files_changed: [], deviations: '', conflict: '', ...extra })
const IT = (extra = {}) => ({ ...BASE_ARGS['implement-tickets'](), ...extra })
const itHandlers = (extra = {}) => ({
  impl: () => work(),
  review: () => ({ pass: true, findings: [], verify_output: '', summary: '' }),
  fix: () => work(),
  merge: () => ({ ok: true, report: 'applied' }),
  gate: () => cleanGate(),
  ...extra,
})

test('implement-tickets: a wave of one works in the shared tree, with no lane and no merge', async () => {
  const { result, byWord } = await run('implement-tickets', itHandlers({ impl: () => work({ workdir: '/r' }) }), IT())
  assert.equal(result.status, 'done')
  const p = byWord('impl')[0].prompt
  assert.doesNotMatch(p, /lane\.sh' start/)
  assert.ok(p.includes('/r/.scratch/f/issues/t-01-a.md'))
  assert.equal(byWord('merge').length, 0)
  assert.equal(result.tickets[0].id, 't-01-a')
  assert.equal(result.tickets[0].status, 'done')
  assert.deepEqual(result.gate, { ok: true, gates: cleanGate().gates, findings: [] })
  assert.deepEqual(result.tests, { ran: 1, passed: 1, failed: 0, skipped: 0 })
  assert.equal(byWord('impl')[0].opts.effort, 'high')
  for (const w of ['review', 'gate']) assert.equal(byWord(w)[0].opts.effort, 'low')
})

test('implement-tickets: lanes merge in wave order and skip failed lanes', async () => {
  const { result, byWord } = await run('implement-tickets', itHandlers({
    impl: (p) => work({ workdir: `/lanes/${p.match(/start (\S+)/)[1]}` }),
    review: (p) => ({ pass: !p.includes('t-02-b'), findings: [{ file: 'b.go', problem: 'x', fix: 'y' }], verify_output: '', summary: '' }),
  }), IT({ waves: [['t-03-c.md', 't-02-b.md', 't-01-a.md']], fix_cap: 0 }))
  for (const c of byWord('impl')) assert.match(c.prompt, /ROOT='\/r' sh '\/p\/scripts\/lane\.sh' start t-0\d-\w/)
  assert.equal(byWord('merge').length, 1)
  const m = byWord('merge')[0].prompt
  const order = [...m.matchAll(/lane\.sh' (patch|apply|clean) (\S+)/g)].map((x) => `${x[1]} ${x[2]}`)
  assert.deepEqual(order, ['patch t-03-c', 'apply t-03-c', 'clean t-03-c', 'patch t-01-a', 'apply t-01-a', 'clean t-01-a'])
  assert.match(m, /untouched[^\n]*t-02-b/)
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.tickets.map((t) => `${t.id} ${t.status}`), ['t-03-c done', 't-02-b failed', 't-01-a done'])
  assert.equal(byWord('gate').length, 0, 'no gate after a failed wave')
})

test('implement-tickets: a wave with a pending question is not merged', async () => {
  const lanes = (p) => work({ workdir: `/lanes/${p.match(/start (\S+)/)[1]}` })
  const h = itHandlers({ impl: (p) => (p.includes('t-02-b') && !p.includes('Q-5') ? { ...lanes(p), question: Q } : lanes(p)) })
  const a = IT({ waves: [['t-01-a.md', 't-02-b.md']] })
  const first = await run('implement-tickets', h, a)
  assert.equal(first.result.status, 'question')
  assert.equal(first.byWord('merge').length, 0, 'no merge before the answer')
  const second = await run('implement-tickets', h, { ...a, answers: { 'Q-5': 'B' } })
  assert.equal(second.result.status, 'done')
  assert.equal(second.byWord('merge').length, 1)
  assert.match(second.byWord('merge')[0].prompt, /patch t-01-a[\s\S]*patch t-02-b/)
})

test('implement-tickets: a failed wave stops the later waves', async () => {
  const { result, byWord } = await run('implement-tickets', itHandlers({
    review: (p) => ({ pass: !p.includes('t-01-a'), findings: [{ file: 'a.go', problem: 'x', fix: 'y' }], verify_output: '', summary: '' }),
  }), IT({ waves: [['t-01-a.md'], ['t-02-b.md']] }))
  assert.equal(result.status, 'findings_left')
  assert.equal(byWord('impl').length, 1)
  assert.equal(byWord('review').length, 4, 'fix_cap default 3: four reviews')
  assert.equal(byWord('fix').length, 3)
  assert.ok(byWord('fix')[0].prompt.includes('a.go: x'))
  assert.match(result.deviations.at(-1), /wave 1/)
})

test('implement-tickets: fix_cap from args', async () => {
  const h = itHandlers({ review: () => ({ pass: false, findings: [], verify_output: '', summary: '' }) })
  const one = await run('implement-tickets', h, IT({ fix_cap: 1 }))
  assert.equal(one.byWord('review').length, 2)
  assert.equal(one.byWord('fix').length, 1)
  assert.equal(one.result.tickets[0].status, 'failed')
})

test('implement-tickets: a dead agent stops the run, never done', async () => {
  const lanes = IT({ waves: [['t-01-a.md', 't-02-b.md']] })
  const cases = [['impl', IT()], ['review', IT()], ['merge', lanes], ['gate', IT()], ['fix', IT()]]
  for (const [dead, a] of cases) {
    const extra = { [dead]: () => null }
    if (dead === 'fix') extra.review = () => ({ pass: false, findings: [], verify_output: '', summary: '' })
    const { result } = await run('implement-tickets', itHandlers(extra), a)
    assert.equal(result.status, 'stopped', dead)
    assert.match(result.deviations.at(-1), /^FAILED: /, dead)
  }
})

test('implement-tickets: a merge that reports a rejected hunk stops the run', async () => {
  const { result, byWord } = await run('implement-tickets', itHandlers({ merge: () => ({ ok: false, report: 'rejected hunk in a.go' }) }), IT({ waves: [['t-01-a.md', 't-02-b.md'], ['t-03-c.md']] }))
  assert.equal(result.status, 'stopped')
  assert.match(result.deviations.at(-1), /^STOP: .*rejected hunk/)
  assert.equal(byWord('impl').length, 2)
})

test('implement-tickets: a conflict stops the run', async () => {
  const { result, byWord } = await run('implement-tickets', itHandlers({ impl: () => work({ conflict: 'the ticket and the spec disagree' }) }), IT())
  assert.equal(result.status, 'stopped')
  assert.match(result.deviations.at(-1), /^CONFLICT: t-01-a: the ticket and the spec disagree/)
  assert.equal(byWord('review').length, 0)
})

test('implement-tickets: a lane implementer that works in the shared tree is an error', async () => {
  const { result } = await run('implement-tickets', itHandlers({ impl: () => work({ workdir: '/r' }) }), IT({ waves: [['t-01-a.md', 't-02-b.md']] }))
  assert.equal(result.status, 'stopped')
  assert.match(result.deviations.at(-1), /^FAILED: /)
})

// M5: the gate rules of deliver.js.
test('implement-tickets: the gate must cover exactly args.gates, with no failed or skipped test', async () => {
  const two = IT({ gates: ['make test', 'make lint'] })
  const missing = await run('implement-tickets', itHandlers(), two)
  assert.equal(missing.result.status, 'findings_left')
  assert.match(missing.result.gate.findings[0].summary, /`make lint` has no result/)
  const extra = await run('implement-tickets', itHandlers({ gate: () => cleanGate(['make test', 'rm -rf x']) }), IT())
  assert.equal(extra.result.status, 'findings_left', 'an extra gate result is a finding')
  assert.match(extra.result.gate.findings[0].summary, /not a gate of the run/)
  const red = await run('implement-tickets', itHandlers({ gate: () => ({ gates: [gateResult('make test', { exit_code: 1, failed: 1, output_tail: 'FAIL' })] }) }), IT())
  assert.equal(red.result.status, 'findings_left')
  const skipped = await run('implement-tickets', itHandlers({ gate: () => ({ gates: [gateResult('make test', { skipped: 1, problems: [{ file: 'a_test.go', line: 3, summary: 'TestX skipped', kind: 'skipped' }] })] }) }), IT())
  assert.equal(skipped.result.status, 'findings_left', 'a skipped test in a required suite is a finding')
  assert.deepEqual(skipped.result.gate.findings.map((x) => x.file), ['a_test.go'])
  const none = await run('implement-tickets', itHandlers({ gate: () => ({ gates: [gateResult('make test', { ran: 0, passed: 0 })] }) }), IT({ test_gates: ['make test'] }))
  assert.equal(none.result.status, 'findings_left', 'a test gate that ran no test is a finding')
  assert.match(none.result.gate.findings[0].summary, /ran no tests/)
  const noTestGates = await run('implement-tickets', itHandlers({ gate: () => ({ gates: [gateResult('make test', { ran: 0, passed: 0 })] }) }), IT())
  assert.equal(noTestGates.result.status, 'done', 'without test_gates, no gate needs a test count')
  const ok = await run('implement-tickets', itHandlers({ gate: () => cleanGate(['make test', 'make lint']) }), two)
  assert.equal(ok.result.status, 'done')
  const p = ok.byWord('gate')[0].prompt
  assert.ok(p.includes('- make test') && p.includes('- make lint'))
})

// ---------- review-and-fix ----------

const RF = (extra = {}) => ({ ...BASE_ARGS['review-and-fix'](), ...extra })
const rfHandlers = (extra = {}) => ({
  check: cleanCheck,
  review: () => ({ findings: [], summary: 'ok' }),
  gate: () => cleanGate(),
  verify: () => ({ confirmed: false, reason: 'not shown', adjusted_fix: '' }),
  fix: (p) => ({ fixed: listed(p), report: '' }),
  confirm: confirmAll,
  ...extra,
})

test('review-and-fix: a clean review gives done', async () => {
  const { result, byWord } = await run('review-and-fix', rfHandlers(), RF())
  assert.equal(result.status, 'done')
  assert.equal(result.workflow, 'review-and-fix')
  assert.equal(result.base, SHA)
  assert.equal(result.head_sha, HEAD)
  assert.deepEqual(result.tests, { ran: 1, passed: 1, failed: 0, skipped: 0 })
  assert.deepEqual(result.summaries, [{ key: 'correctness', summary: 'ok' }, { key: 'security', summary: 'ok' }])
  assert.deepEqual(result.confirmed, [])
  assert.equal(byWord('verify').length, 0)
  for (const w of ['review', 'gate']) {
    const c = byWord(w)[0]
    assert.equal(c.opts.effort, 'low')
    assert.ok(c.prompt.includes(`git -C '/r' diff ${SHA}`), `${w} diffs against the base`)
    assert.doesNotMatch(c.prompt, /git diff[^\n]*origin\/main/)
  }
  const lens = byWord('review')[0].prompt
  for (const s of ['Check the handlers.', 'HOUSE RULES TEXT', 'the fixed session TTL', '/r/guides', '/r/spec.md']) assert.ok(lens.includes(s), s)
})

// M4: the reviewed commit is checked, not echoed.
for (const name of ['review-and-fix', 'review-only']) {
  const H = name === 'review-and-fix' ? rfHandlers : (extra = {}) => ({ check: cleanCheck, review: () => ({ findings: [], summary: 'ok' }), verify: () => null, ...extra })
  const args = BASE_ARGS[name]
  test(`${name}: the check of HEAD, the tree, and the base stops a wrong tree before any review`, async () => {
    const other = 'c'.repeat(40)
    const cases = [
      [{ head_sha: other, status_porcelain: '', base_is_ancestor: true }, /not args.head/],
      [{ head_sha: HEAD, status_porcelain: ' M src/a.go', base_is_ancestor: true }, /not clean/],
      [{ head_sha: HEAD, status_porcelain: '', base_is_ancestor: false }, /not an ancestor/],
      [{ head_sha: 'HEAD', status_porcelain: '', base_is_ancestor: true }, /not a commit/],
    ]
    for (const [c, why] of cases) {
      const { result, byWord } = await run(name, H({ check: () => c }), args())
      assert.equal(result.status, 'stopped')
      assert.match(result.deviations.at(-1), why)
      assert.equal(byWord('review').length, 0)
    }
    const dead = await run(name, H({ check: () => null }), args())
    assert.match(dead.result.deviations.at(-1), /^FAILED: /)
    const { head: _h, ...noHead } = args()
    const fromHead = await run(name, H({ check: () => ({ ...cleanCheck(), head_sha: 'd'.repeat(40) }) }), noHead)
    assert.equal(fromHead.result.status, 'done')
    assert.equal(fromHead.result.head_sha, 'd'.repeat(40), 'head_sha is the checked value')
    assert.ok(fromHead.calls[0].prompt.includes('rev-parse HEAD') && fromHead.calls[0].prompt.includes(`merge-base --is-ancestor ${SHA} HEAD`))
  })
}

test('review-and-fix: dedup by file:line, one refuter each, refuted stays refuted', async () => {
  const { result, byWord } = await run('review-and-fix', rfHandlers({
    review: (p) => ({ findings: [f('src/a.go', 1), f('./src/a.go', 1, { problem: 'again' }), f('src/b.go', 2)], summary: p.includes('handlers') ? 'c' : 's' }),
    verify: (p) => ({ confirmed: p.includes('src/b.go'), reason: '', adjusted_fix: p.includes('src/b.go') ? 'better fix' : '' }),
  }), RF())
  const labels = byWord('verify').map((c) => c.opts.label)
  // Round 1: a and b. Round 2: a is refuted and not asked again; b was fixed and is reported again.
  assert.deepEqual(labels, ['verify src/a.go:1', 'verify src/b.go:2', 'verify src/b.go:2'])
  assert.match(byWord('verify')[0].prompt, /also \(correctness\): again/)
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.refuted.map((x) => `${x.file}:${x.line}`), ['src/a.go:1'])
  assert.deepEqual(result.confirmed.map((x) => `${x.file}:${x.line} ${x.state} ${x.round} ${x.fix}`), ['src/b.go:2 open 2 better fix'])
  for (const c of byWord('verify')) assert.equal(c.opts.effort, 'low')
  assert.match(byWord('verify')[0].prompt, /throwaway program/)
})

test('review-and-fix: a dead refuter stops the run, never done', async () => {
  const { result, byWord } = await run('review-and-fix', rfHandlers({
    review: () => ({ findings: [f('src/a.go', 1)], summary: '' }),
    verify: () => null,
  }), RF())
  assert.equal(result.status, 'stopped')
  assert.match(result.deviations.at(-1), /^FAILED: /)
  assert.deepEqual(result.refuted, [])
  assert.equal(byWord('fix').length, 0)
})

test('review-and-fix: a dead lens, gate, fixer, or fix check stops the run', async () => {
  const confirmed = { review: () => ({ findings: [f('src/a.go', 1)], summary: '' }), verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }) }
  for (const [dead, extra] of [['review', {}], ['gate', {}], ['fix', confirmed], ['confirm', confirmed]]) {
    const { result } = await run('review-and-fix', rfHandlers({ ...extra, [dead]: () => null }), RF())
    assert.equal(result.status, 'stopped', dead)
    assert.match(result.deviations.at(-1), /^FAILED: /, dead)
  }
})

test('review-and-fix: round cap from args, default 2', async () => {
  const always = rfHandlers({
    review: () => ({ findings: [f('src/a.go', 7)], summary: '' }),
    verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }),
  })
  const def = await run('review-and-fix', always, RF())
  assert.equal(def.result.status, 'findings_left')
  assert.equal(def.byWord('gate').length, 2)
  assert.equal(def.byWord('fix').length, 1)
  const three = await run('review-and-fix', always, RF({ round_cap: 3 }))
  assert.equal(three.byWord('gate').length, 3)
  assert.equal(three.byWord('fix').length, 2)
})

test('review-and-fix: fixes run in sequential batches by area of two folders', async () => {
  let active = 0
  let peak = 0
  const { result, byWord } = await run('review-and-fix', rfHandlers({
    review: (p, o, n) => ({ findings: n === 1 ? [f('src/a.go', 1), f('web/x.js', 2), f('src/b.go', 3), f('internal/store/s.go', 4), f('internal/api/h.go', 5)] : [], summary: '' }),
    verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }),
    fix: async (p) => {
      active++
      peak = Math.max(peak, active)
      await new Promise((r) => setTimeout(r, 5))
      active--
      return rfHandlers().fix(p)
    },
  }), RF())
  assert.equal(peak, 1)
  assert.deepEqual(byWord('fix').map((c) => c.opts.label), ['fix src', 'fix web', 'fix internal/store', 'fix internal/api'])
  assert.equal(result.status, 'done')
  assert.deepEqual(result.confirmed.map((x) => x.state), ['fixed', 'fixed', 'fixed', 'fixed', 'fixed'])
  for (const c of byWord('fix')) assert.equal(c.opts.effort, 'high')
})

// M9: only a fix that an independent agent checked is fixed.
test('review-and-fix: a fix that the fix check does not confirm stays open', async () => {
  const { result, byWord } = await run('review-and-fix', rfHandlers({
    review: (p, o, n) => ({ findings: n === 1 ? [f('src/a.go', 1), f('src/b.go', 2)] : [], summary: '' }),
    verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }),
    confirm: (p) => ({ results: listed(p).map((l) => ({ ...l, fixed: l.file === 'src/a.go', reason: '' })) }),
  }), RF())
  assert.equal(byWord('confirm').length, 1)
  assert.match(byWord('confirm')[0].prompt, /Do not trust the report of the fixer/)
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.confirmed.map((x) => `${x.file} ${x.state}`), ['src/a.go fixed', 'src/b.go open'])
})

test('review-and-fix: a fixer cannot mark a finding of another area fixed', async () => {
  const { result } = await run('review-and-fix', rfHandlers({
    review: (p, o, n) => ({ findings: n <= 2 ? [f('src/a.go', 1), f('web/x.js', 2)] : [], summary: '' }),
    verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }),
    fix: (p) => ({ fixed: p.includes('area src') ? [{ file: 'src/a.go', line: 1 }, { file: 'web/x.js', line: 2 }] : [], report: '' }),
    confirm: () => ({ results: [{ file: 'src/a.go', line: 1, fixed: true, reason: '' }, { file: 'web/x.js', line: 2, fixed: true, reason: '' }] }),
  }), RF({ round_cap: 2 }))
  const web = result.confirmed.find((x) => x.file === 'web/x.js')
  assert.equal(web.state, 'open')
})

test('review-and-fix: a fix stays on record when a new finding at its key is refuted', async () => {
  const { result } = await run('review-and-fix', rfHandlers({
    review: () => ({ findings: [f('src/a.go', 1)], summary: '' }),
    verify: (p, o, n) => ({ confirmed: n === 1, reason: '', adjusted_fix: '' }),
  }), RF())
  assert.equal(result.status, 'done')
  assert.deepEqual(result.confirmed.map((x) => `${x.file} ${x.state}`), ['src/a.go fixed'])
})

test('review-and-fix: a finding that the fixer did not fix stays open', async () => {
  const { result, byWord } = await run('review-and-fix', rfHandlers({
    review: (p, o, n) => ({ findings: n <= 2 ? [f('src/a.go', 1)] : [], summary: '' }),
    verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }),
    fix: () => ({ fixed: [], report: 'could not' }),
  }), RF())
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.confirmed.map((x) => x.state), ['open'])
  assert.equal(byWord('confirm').length, 0)
})

// M5: the gate rules of deliver.js in each review round.
test('review-and-fix: a red gate is an open finding without a refuter, fixed when the gate is clean', async () => {
  const { result, byWord } = await run('review-and-fix', rfHandlers({
    gate: (p, o, n) => (n === 1 ? { gates: [gateResult('make test', { exit_code: 2, failed: 1, output_tail: 'FAIL x /home/user/secret' })] } : cleanGate()),
    fix: () => ({ fixed: [], report: '' }),
  }), RF())
  assert.equal(byWord('verify').length, 0)
  assert.deepEqual(byWord('fix').map((c) => c.opts.label), ['fix gates'])
  assert.equal(result.status, 'done')
  assert.deepEqual(result.confirmed.map((x) => `${x.file} ${x.state}`), ['gate: make test fixed'])
  assert.doesNotMatch(result.confirmed[0].problem, /secret/, 'the gate output is not in the posted problem')
})

test('review-and-fix: the gates must cover exactly args.gates, with no skipped test', async () => {
  const one = RF({ round_cap: 1 })
  const missing = await run('review-and-fix', rfHandlers({ gate: () => ({ gates: [] }) }), one)
  assert.equal(missing.result.status, 'findings_left')
  assert.match(missing.result.confirmed[0].problem, /has no result/)
  const extra = await run('review-and-fix', rfHandlers({ gate: () => cleanGate(['make test', 'make deploy']) }), one)
  assert.equal(extra.result.status, 'findings_left')
  assert.match(extra.result.confirmed[0].problem, /not a gate of the run/)
  const skipped = await run('review-and-fix', rfHandlers({ gate: () => ({ gates: [gateResult('make test', { skipped: 2 })] }) }), one)
  assert.equal(skipped.result.status, 'findings_left', 'a skipped test in a required suite is a finding')
  assert.match(skipped.result.confirmed[0].problem, /2 skipped/)
  assert.deepEqual(skipped.result.tests, { ran: 1, passed: 1, failed: 0, skipped: 2 })
  const none = await run('review-and-fix', rfHandlers({ gate: () => ({ gates: [gateResult('make test', { ran: 0, passed: 0 })] }) }), RF({ round_cap: 1, test_gates: ['make test'] }))
  assert.equal(none.result.status, 'findings_left')
  assert.match(none.result.confirmed[0].problem, /ran no tests/)
  const lint = await run('review-and-fix', rfHandlers({ gate: () => ({ gates: [gateResult('make test', { ran: 0, passed: 0 })] }) }), one)
  assert.equal(lint.result.status, 'done', 'without test_gates, no gate needs a test count')
})

// ---------- review-only ----------

const RO = (extra = {}) => ({ ...BASE_ARGS['review-only'](), ...extra })

test('review-only: dedup by file:line, one refuter each, sorted by severity', async () => {
  const { result, byWord } = await run('review-only', {
    check: cleanCheck,
    review: (p) => (p.includes('handlers')
      ? { findings: [f('src/a.go', 1, { severity: 'nit' }), f('src/b.go', 2, { severity: 'guideline' })], summary: 'c' }
      : { findings: [f('src/a.go', 1, { problem: 'same line' }), f('src/c.go', 3, { severity: 'security' })], summary: 's' }),
    verify: (p) => ({ confirmed: !p.includes('src/b.go'), reason: 'r', adjusted_fix: '' }),
  }, RO())
  assert.equal(result.status, 'done')
  assert.equal(result.workflow, 'review-only')
  assert.equal(result.head_sha, HEAD)
  assert.deepEqual(byWord('verify').map((c) => c.opts.label), ['verify src/a.go:1', 'verify src/b.go:2', 'verify src/c.go:3'])
  assert.deepEqual(result.confirmed.map((x) => `${x.file} ${x.severity} ${x.lens}`), ['src/c.go security security', 'src/a.go nit correctness'])
  assert.match(result.confirmed[1].problem, /also \(security\): same line/)
  assert.deepEqual(result.refuted.map((x) => x.file), ['src/b.go'])
  assert.deepEqual(result.summaries, [{ key: 'correctness', summary: 'c' }, { key: 'security', summary: 's' }])
  for (const c of byWord('review')) {
    assert.ok(c.prompt.includes(`git -C '/r' diff ${SHA} ${HEAD}`))
    assert.match(c.prompt, /READ-ONLY/)
    assert.equal(c.opts.effort, 'low')
  }
})

test('review-only: a dead refuter or reviewer stops the run', async () => {
  const dead = await run('review-only', { check: cleanCheck, review: () => ({ findings: [f('src/a.go', 1)], summary: '' }), verify: () => null }, RO())
  assert.equal(dead.result.status, 'stopped')
  assert.match(dead.result.deviations.at(-1), /^FAILED: /)
  assert.deepEqual(dead.result.confirmed, [])
  assert.deepEqual(dead.result.refuted, [])
  const rev = await run('review-only', { check: cleanCheck, review: (p) => (p.includes('handlers') ? null : { findings: [], summary: '' }), verify: () => null }, RO())
  assert.equal(rev.result.status, 'stopped')
  assert.equal(rev.byWord('verify').length, 0)
})

test('review-only: a refuter that is not sure refutes', async () => {
  const { result, byWord } = await run('review-only', {
    check: cleanCheck,
    review: () => ({ findings: [f('src/a.go', 1)], summary: '' }),
    verify: () => ({ confirmed: false, reason: 'not sure', adjusted_fix: '' }),
  }, RO())
  assert.equal(result.status, 'done')
  assert.equal(result.refuted.length, 1)
  assert.match(byWord('verify')[0].prompt, /When you are not sure, return confirmed = false/)
})
