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
  const logs = []
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
  const result = await loaded[name].script(agent, parallel, pipeline, (p) => phases.push(p), (m) => logs.push(m), args, budget)
  return { result, calls, phases, logs, byWord: (w) => calls.filter((c) => c.word === w) }
}

const SHA = 'a'.repeat(40)
const HEAD = 'b'.repeat(40)
const GIT_RULE = 'Never run git commit, git push, git stash, git checkout, git switch, git reset, or git rebase'
const POST_RULE = 'Do not post outside the project'
const Q = { id: 'Q-app-host-5', header: 'P1 Q-app-host-5: which table?', body: 'Use table A or table B?' }

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
// The fixer and the fix check read the findings of their prompt: lines "1. [F<n>] file:line ...".
const listed = (p) => [...p.matchAll(/^\d+\. \[(F\d+)\] (\S+):(\d+)[: ]/gm)].map((m) => ({ id: m[1], file: m[2], line: Number(m[3]) }))
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
    fix: () => ({ fixed: [{ id: 'F1', file: 'src/a.go', line: 1 }], report: '' }),
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
      assert.ok(!c.prompt.includes('gh pr review') && !c.prompt.includes('glab mr note'), `${c.opts.label} names a post command`)
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
    [asker]: (p, o, n) => (n > 1 || p.includes('Q-app-host-5') ? BUSY[name][asker](p, o, n) : { ...BUSY[name][asker](p, o, n), question: Q }),
  })

  test(`${name}: a pending question returns status question`, async () => {
    const { result, byWord } = await run(name, asking(), askArgs())
    assert.equal(result.status, 'question')
    assert.deepEqual(result.question, Q)
    const p = byWord(asker)[0].prompt
    for (const s of ['question_open', 'SendMessage', 'main', 'answer_wait', 'deadline_seconds 600', 'pending', 'options', 'the body that question_open returned']) {
      assert.ok(p.includes(s), `the ${asker} prompt does not name ${s}`)
    }
    if (name === 'implement-tickets') assert.equal(byWord('review').length, 0)
  })

  test(`${name}: the answer reaches only prompts after the question`, async () => {
    const first = await run(name, asking(), askArgs())
    const answer = 'Use table B. (owner, 2026-09-30, Q-app-host-5)'
    const second = await run(name, asking(), { ...askArgs(), answers: { 'Q-app-host-5': answer } })
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
    const one = await run(name, always, { ...askArgs(), answers: { 'Q-app-host-5': 'B' } })
    assert.equal(one.result.status, 'question')
    assert.equal(one.result.deviations.at(-1), 'REPEAT: Q-app-host-5 1 of 2')
    const three = await run(name, always, { ...askArgs(), answers: { 'Q-app-host-5': ['B', 'C', 'D'] } })
    assert.equal(three.result.status, 'stopped')
    assert.match(three.result.deviations.at(-1), /Q-app-host-5 is pending again after 3 answers/)
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
  merge: () => ({ ok: true, applied: [], report: 'applied' }),
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
  assert.equal(result.tickets[0].status, 'merged', 'a wave of one is merged by its pass')
  assert.deepEqual(result.remaining_waves, [])
  assert.equal(result.gate_only, false)
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
  for (const c of byWord('impl')) assert.match(c.prompt, /ROOT='\/r' LANES="\$\{TMPDIR:-\/tmp\}\/bruh-lanes\/[0-9a-f]+" LANE_RUN='[0-9a-f]+' sh '\/p\/scripts\/lane\.sh' start t-0\d-\w/)
  assert.equal(byWord('merge').length, 1)
  const m = byWord('merge')[0].prompt
  const order = [...m.matchAll(/lane\.sh' (patch|apply|clean) (\S+)/g)].map((x) => `${x[1]} ${x[2]}`)
  assert.deepEqual(order, ['patch t-03-c', 'apply t-03-c', 'clean t-03-c', 'patch t-01-a', 'apply t-01-a', 'clean t-01-a'])
  assert.match(m, /untouched[^\n]*t-02-b/)
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.tickets.map((t) => `${t.id} ${t.status}`), ['t-03-c merged', 't-02-b failed', 't-01-a merged'])
  assert.deepEqual(result.remaining_waves, [['t-02-b.md']])
  assert.equal(byWord('gate').length, 0, 'no gate after a failed wave')
})

test('implement-tickets: a wave with a pending question is not merged', async () => {
  const lanes = (p) => work({ workdir: `/lanes/${p.match(/start (\S+)/)[1]}` })
  const h = itHandlers({ impl: (p) => (p.includes('t-02-b') && !p.includes('Q-app-host-5') ? { ...lanes(p), question: Q } : lanes(p)) })
  const a = IT({ waves: [['t-01-a.md', 't-02-b.md']] })
  const first = await run('implement-tickets', h, a)
  assert.equal(first.result.status, 'question')
  assert.equal(first.byWord('merge').length, 0, 'no merge before the answer')
  const second = await run('implement-tickets', h, { ...a, answers: { 'Q-app-host-5': 'B' } })
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

// Fix round 2, N1: a ticket is merged only after its lane merge returned ok.
test('implement-tickets: a dead merge agent leaves each ticket of the wave in remaining_waves', async () => {
  const lanes = (p) => work({ workdir: `/lanes/${p.match(/start (\S+)/)[1]}` })
  const a = IT({ waves: [['t-00-z.md'], ['t-01-a.md', 't-02-b.md'], ['t-03-c.md']] })
  const { result } = await run('implement-tickets', itHandlers({ impl: (p) => (p.includes('t-00-z') ? work({ workdir: '/r' }) : lanes(p)), merge: () => null }), a)
  assert.equal(result.status, 'stopped')
  assert.match(result.deviations.at(-1), /^FAILED: .*not merged/)
  assert.deepEqual(result.tickets.map((t) => `${t.id} ${t.status}`), ['t-00-z merged', 't-01-a done', 't-02-b done'])
  assert.deepEqual(result.remaining_waves, [['t-01-a.md', 't-02-b.md'], ['t-03-c.md']], 'the relaunch set has the unapplied tickets')
  assert.equal(result.gate_only, false)
  // The relaunch with remaining_waves implements exactly those tickets.
  const again = await run('implement-tickets', itHandlers({ impl: lanes }), { ...a, waves: result.remaining_waves })
  assert.deepEqual(again.byWord('impl').map((c) => c.opts.label), ['impl t-01-a', 'impl t-02-b', 'impl t-03-c'])
  assert.match(again.byWord('impl')[0].prompt, /keeps it with its work only when it was made for the same root, the same HEAD, and the same run/)
  assert.doesNotMatch(again.byWord('impl')[0].prompt, /never wipes/)
  assert.match(again.byWord('impl')[0].prompt, /Check first what is there already/)
})

test('implement-tickets: a merge that stops at a hunk merges only the applied tickets', async () => {
  const lanes = (p) => work({ workdir: `/lanes/${p.match(/start (\S+)/)[1]}` })
  const { result } = await run('implement-tickets', itHandlers({ impl: lanes, merge: () => ({ ok: false, applied: ['t-01-a'], report: 'rejected hunk in b.go' }) }), IT({ waves: [['t-01-a.md', 't-02-b.md']] }))
  assert.equal(result.status, 'stopped')
  assert.deepEqual(result.tickets.map((t) => `${t.id} ${t.status}`), ['t-01-a merged', 't-02-b done'])
  assert.deepEqual(result.remaining_waves, [['t-02-b.md']])
})

// Fix round 3, R1: the lanes of a run are named by args.root and args.spec only.
test('implement-tickets: the lane folder and run key come from args.root and args.spec, the same in each run', async () => {
  const lanes = (p) => work({ workdir: `/lanes/${p.match(/start (\S+)/)[1]}` })
  const a = IT({ waves: [['t-01-a.md', 't-02-b.md']] })
  const key = (r) => r.byWord('impl')[0].prompt.match(/LANES="[^"]*\/bruh-lanes\/([0-9a-f]+)" LANE_RUN='([0-9a-f]+)'/).slice(1)
  const one = await run('implement-tickets', itHandlers({ impl: lanes }), a)
  const two = await run('implement-tickets', itHandlers({ impl: lanes }), a)
  assert.deepEqual(key(one), key(two))
  assert.equal(key(one)[0], key(one)[1])
  const otherRoot = await run('implement-tickets', itHandlers({ impl: lanes }), { ...a, root: '/other' })
  const otherSpec = await run('implement-tickets', itHandlers({ impl: lanes }), { ...a, spec: '/r/.scratch/g/spec.md' })
  assert.notDeepEqual(key(otherRoot), key(one))
  assert.notDeepEqual(key(otherSpec), key(one))
  for (const c of one.byWord('merge')) assert.match(c.prompt, /LANE_RUN='[0-9a-f]+' sh '\/p\/scripts\/lane\.sh' patch/)
})

// Fix round 3: a leftover ticket with a lane keeps working in its lane on relaunch, also alone.
test('implement-tickets: a leftover lane ticket keeps its lane when it is alone in its wave', async () => {
  const lanes = (p) => work({ workdir: `/lanes/${p.match(/start (\S+)/)[1]}` })
  const a = IT({ waves: [['t-01-a.md', 't-02-b.md']], fix_cap: 0 })
  const first = await run('implement-tickets', itHandlers({
    impl: lanes,
    review: (p) => ({ pass: !p.includes('t-02-b'), findings: [], verify_output: '', summary: '' }),
  }), a)
  assert.deepEqual(first.result.remaining_waves, [['t-02-b.md']])
  assert.deepEqual(first.result.lane_tickets, ['t-02-b.md'])
  const again = await run('implement-tickets', itHandlers({ impl: lanes }), { ...a, waves: first.result.remaining_waves, lane_tickets: first.result.lane_tickets })
  assert.equal(again.result.status, 'done')
  assert.match(again.byWord('impl')[0].prompt, /lane\.sh' start t-02-b/)
  assert.equal(again.byWord('merge').length, 1, 'the lane is merged and cleaned, not orphaned')
  assert.match(again.byWord('merge')[0].prompt, /clean t-02-b/)
  assert.deepEqual(again.result.lane_tickets, [])
  // Fix round 4: a lane ticket of args.lane_tickets that did not run stays in lane_tickets.
  const either = (p) => (/start (\S+)/.test(p) ? lanes(p) : work({ workdir: '/r' }))
  const early = await run('implement-tickets', itHandlers({ impl: either, review: (p) => ({ pass: !p.includes('t-03-c'), findings: [], verify_output: '', summary: '' }) }),
    { ...a, waves: [['t-03-c.md'], ['t-02-b.md']], lane_tickets: ['t-02-b.md'] })
  assert.equal(early.result.status, 'findings_left')
  assert.equal(early.byWord('impl').length, 1, 'the wave of t-02-b did not run')
  assert.deepEqual(early.result.remaining_waves, [['t-03-c.md'], ['t-02-b.md']])
  assert.deepEqual(early.result.lane_tickets, ['t-02-b.md'], 'its lane is not orphaned')
  const alone = await run('implement-tickets', itHandlers({ impl: () => work({ workdir: '/r' }) }), { ...a, waves: [['t-02-b.md']] })
  assert.doesNotMatch(alone.byWord('impl')[0].prompt, /lane\.sh' start/, 'without lane_tickets a wave of one works in the shared tree')
  const bad = await run('implement-tickets', itHandlers(), { ...a, lane_tickets: ['t-09-z.md'] })
  assert.equal(bad.result.status, 'stopped')
  assert.equal(bad.calls.length, 0)
})

// Fix round 2: when only the gate is left, the result says so, and the relaunch runs only the gate.
test('implement-tickets: a dead gate agent gives gate_only, and a gate_only run runs only the gate', async () => {
  const dead = await run('implement-tickets', itHandlers({ gate: () => null }), IT())
  assert.equal(dead.result.status, 'stopped')
  assert.deepEqual(dead.result.remaining_waves, [])
  assert.equal(dead.result.gate_only, true)
  // Fix round 3: gate_only only when the gate agent died, not for findings_left or a validation stop.
  const red = await run('implement-tickets', itHandlers({ gate: () => ({ gates: [gateResult('make test', { exit_code: 1, failed: 1 })] }) }), IT())
  assert.equal(red.result.status, 'findings_left')
  assert.equal(red.result.gate_only, false)
  const invalid = await run('implement-tickets', itHandlers(), IT({ waves: [] }))
  assert.equal(invalid.result.status, 'stopped')
  assert.equal(invalid.result.gate_only, false)
  const deadTicket = await run('implement-tickets', itHandlers({ review: () => null }), IT())
  assert.equal(deadTicket.result.gate_only, false)
  const only = await run('implement-tickets', itHandlers(), IT({ waves: [], gate_only: true }))
  assert.equal(only.result.status, 'done')
  assert.deepEqual(only.calls.map((c) => c.word), ['gate'])
  const noWaves = await run('implement-tickets', itHandlers(), (({ waves: _w, ...a }) => ({ ...a, gate_only: true }))(IT()))
  assert.equal(noWaves.result.status, 'done')
  // Fix round 4: a gate-only run takes no lane_tickets; an empty list is fine.
  const withLanes = await run('implement-tickets', itHandlers(), IT({ waves: [], gate_only: true, lane_tickets: ['t-01-a.md'] }))
  assert.equal(withLanes.result.status, 'stopped')
  assert.equal(withLanes.calls.length, 0)
  assert.match(withLanes.result.deviations.at(-1), /gate_only needs no lane_tickets/)
  const emptyLanes = await run('implement-tickets', itHandlers(), IT({ waves: [], gate_only: true, lane_tickets: [] }))
  assert.equal(emptyLanes.result.status, 'done')
  for (const bad of [{ gate_only: true }, { gate_only: 'yes', waves: [] }]) {
    const r = await run('implement-tickets', itHandlers(), IT(bad))
    assert.equal(r.result.status, 'stopped', JSON.stringify(bad))
    assert.equal(r.calls.length, 0)
  }
})

test('implement-tickets: an unnamed skip count of the gate is a finding', async () => {
  const { result } = await run('implement-tickets', itHandlers({ gate: () => ({ gates: [gateResult('make test', { skipped: 2 })] }) }), IT())
  assert.equal(result.status, 'findings_left')
  assert.match(result.gate.findings[0].summary, /has 2 skipped/)
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

// Fix round 4, M1: a refuted re-report never erases an open confirmed finding at its key.
test('review-and-fix: an open finding survives a refuted report at its key', async () => {
  const { result } = await run('review-and-fix', rfHandlers({
    review: (p, o, n) => (n === 1 ? { findings: [f('src/a.go', 5, { problem: 'real A' })], summary: '' }
      : n === 3 ? { findings: [f('src/a.go', 5, { problem: 'taste B' })], summary: '' }
      : { findings: [], summary: '' }),
    verify: (p) => ({ confirmed: p.includes('real A'), reason: p.includes('real A') ? 'shown' : 'taste', adjusted_fix: '' }),
    confirm: (p) => ({ results: listed(p).map((l) => ({ ...l, fixed: false, reason: '' })) }),
  }), RF())
  assert.notEqual(result.status, 'done')
  assert.equal(result.status, 'findings_left')
  assert.deepEqual(result.confirmed.map((x) => `${x.file}:${x.line} ${x.state} ${x.problem}`), ['src/a.go:5 open real A'])
})

// Fix round 3: a finding at the key of an earlier finding merges with it, never replaces it.
test('review-and-fix: a finding moved or reported at an earlier key merges with the earlier finding', async () => {
  for (const moved of [true, false]) {
    const { result } = await run('review-and-fix', rfHandlers({
      review: (p, o, n) => (n === 1 ? { findings: [f('src/a.go', 5, { problem: 'first problem' })], summary: '' }
        : n === 3 ? { findings: [f('src/a.go', moved ? 7 : 5, { problem: 'second problem' })], summary: '' }
        : { findings: [], summary: '' }),
      verify: (p) => ({ confirmed: true, reason: p.includes('first problem') ? 'reason one' : 'reason two', adjusted_fix: '', ...(p.includes('"line":7') ? { line: 5 } : {}) }),
      confirm: (p) => ({ results: listed(p).map((l) => ({ ...l, fixed: false, reason: '' })) }),
    }), RF())
    assert.equal(result.confirmed.length, 1, `moved ${moved}`)
    const x = result.confirmed[0]
    assert.equal(`${x.file}:${x.line} ${x.state} ${x.round}`, 'src/a.go:5 open 2')
    assert.match(x.problem, /second problem \| earlier \(round 1, open\): first problem/)
    assert.equal(x.reason, 'reason two | reason one')
  }
})

// Fix round 2: a dropped refuted key is logged, not silent.
test('review-and-fix: a finding at a refuted key is dropped with a log line', async () => {
  const { logs } = await run('review-and-fix', rfHandlers({
    review: () => ({ findings: [f('src/a.go', 1)], summary: '' }),
    verify: () => ({ confirmed: false, reason: '', adjusted_fix: '' }),
    gate: (p, o, n) => (n === 1 ? { gates: [gateResult('make test', { exit_code: 1, failed: 1 })] } : cleanGate()),
    fix: () => ({ fixed: [], report: '' }),
  }), RF())
  assert.ok(logs.some((m) => /Round 2: 2 findings at refuted file:line keys dropped/.test(m)), logs.join('\n'))
})

// Fix round 2: the refuter can correct the location, and the dedup runs after it.
for (const name of ['review-only', 'review-and-fix']) {
  test(`${name}: a refuter corrects the line of a confirmed finding, then dedup merges it`, async () => {
    const h = {
      ...(name === 'review-only' ? { check: cleanCheck } : rfHandlers()),
      review: (p, o, n) => (n > 2 ? { findings: [], summary: '' } : p.includes('handlers')
        ? { findings: [f('src/a.go', 17, { problem: 'off by one' })], summary: '' }
        : { findings: [f('src/a.go', 18, { problem: 'bound' })], summary: '' }),
      verify: (p) => (p.includes('"line":17') ? { confirmed: true, reason: '', adjusted_fix: '', line: 18 } : { confirmed: true, reason: '', adjusted_fix: '' }),
    }
    const { result, byWord } = await run(name, h, BASE_ARGS[name]())
    assert.match(byWord('verify')[0].prompt, /return the correct file and line/)
    const locs = result.confirmed.map((x) => `${x.file}:${x.line}`)
    assert.deepEqual(locs, ['src/a.go:18'], 'one finding at the corrected line')
    assert.match(result.confirmed[0].problem, /off by one \| also \(security\): bound/)
    const moved = await run(name, { ...h, verify: (p) => ({ confirmed: true, reason: '', adjusted_fix: '', ...(p.includes('"line":17') ? { file: './src/b.go', line: 3 } : {}) }) }, BASE_ARGS[name]())
    assert.deepEqual(moved.result.confirmed.map((x) => `${x.file}:${x.line}`).sort(), ['src/a.go:18', 'src/b.go:3'])
  })
}

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
    fix: (p) => ({ fixed: p.includes('area src') ? [{ id: 'F1', file: 'src/a.go', line: 1 }, { id: 'F2', file: 'web/x.js', line: 2 }] : [], report: '' }),
    confirm: () => ({ results: [{ id: 'F1', file: 'src/a.go', line: 1, fixed: true, reason: '' }, { id: 'F2', file: 'web/x.js', line: 2, fixed: true, reason: '' }] }),
  }), RF({ round_cap: 2 }))
  const web = result.confirmed.find((x) => x.file === 'web/x.js')
  assert.equal(web.state, 'open')
})

// Task 32: the fix of src/a.go:61 inserts lines above it, so the fixer and the fix check report
// line 67. The ID closes the finding, so the run ends done, not findings_left.
test('review-and-fix: a fix that moves the line of its finding closes it by ID', async () => {
  const moved = (p) => listed(p).map((l) => ({ ...l, line: l.line + 6 }))
  const { result, byWord } = await run('review-and-fix', rfHandlers({
    review: (p, o, n) => ({ findings: n === 1 ? [f('src/a.go', 61)] : [], summary: '' }),
    verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }),
    fix: (p) => ({ fixed: moved(p), report: '' }),
    confirm: (p) => ({ results: moved(p).map((l) => ({ ...l, fixed: true, reason: '' })) }),
  }), RF())
  assert.match(byWord('fix')[0].prompt, /^1\. \[F1\] src\/a\.go:61 /m)
  assert.match(byWord('confirm')[0].prompt, /^1\. \[F1\] src\/a\.go:61: /m)
  assert.equal(result.status, 'done')
  assert.deepEqual(result.confirmed.map((x) => `${x.file}:${x.line} ${x.state} ${'id' in x}`), ['src/a.go:61 fixed false'])
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

// Fix round 2: a gate finding is fixed only when its gate is clean: exit 0, no failed and no skipped test.
test('review-and-fix: a gate finding is not fixed while its gate is still red or skips a test', async () => {
  for (const later of [
    gateResult('make test', { exit_code: 1, failed: 1, output_tail: 'another failure' }),
    gateResult('make test', { skipped: 1, problems: [{ file: 'a_test.go', line: 4, summary: 'TestY skipped', kind: 'skipped' }] }),
  ]) {
    const { result } = await run('review-and-fix', rfHandlers({
      gate: (p, o, n) => (n === 1 ? { gates: [gateResult('make test', { exit_code: 2, failed: 1, output_tail: 'first failure' })] } : { gates: [later] }),
      fix: () => ({ fixed: [], report: '' }),
    }), RF())
    assert.equal(result.status, 'findings_left')
    assert.deepEqual(result.confirmed.filter((x) => x.state === 'fixed'), [], 'no gate finding is fixed while the gate is not clean')
    assert.equal(result.confirmed.filter((x) => x.state === 'open').length, 1)
  }
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

// Task 30: the gate agent of each round writes phase review, the first fixer of each round phase fix.
test('review-and-fix: the gate and the first fixer of each round write the phases review and fix', async () => {
  const { result, calls, byWord } = await run('review-and-fix', rfHandlers({
    review: (p, o, n) => ({ findings: n <= 2 ? [f('src/a.go', 1), f('web/x.js', 2)] : [], summary: '' }),
    verify: () => ({ confirmed: true, reason: '', adjusted_fix: '' }),
  }), RF())
  assert.equal(result.status, 'done')
  const phaseOf = (p) => p.match(/report_write \(mcp__plugin_bruh_bruh__report_write; load it with ToolSearch\) with kind event, text "phase (\w+)", and phase "\1"/)?.[1]
  assert.deepEqual(calls.map((c) => phaseOf(c.prompt)).filter(Boolean), ['review', 'fix', 'review'])
  assert.ok(phaseOf(byWord('gate')[0].prompt) === 'review' && phaseOf(byWord('fix')[0].prompt) === 'fix')
  assert.equal(phaseOf(byWord('fix')[1].prompt), undefined, 'only the first fixer of a round writes the phase')
})
