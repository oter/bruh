import { expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'
import { PHASES, readPhase, SOFT } from './register.js'

// A small fake world beneath the plugin: the bruh data folder under HOME=/h,
// a ledger at /l, the output of `claude agents --json --all`, and a $.store.
const D = '/h/.claude/plugins/data/bruh-oter'
const LEDGER = `| Owner | Task | Expected deliverable | State | Next check (UTC) | Link | Source read |
|---|---|---|---|---|---|---|
| clanker-bruh | task 12: poller waits on the lock | PR on oter/bruh | working (clerk-bruh-pollerwait) | 2026-10-05T17:22:43Z |  | x |
| clanker-bruh | task 13: less ceremony | PR on oter/bruh | sent | 2026-10-05T18:52:38Z |  | x |`
const line = (o: object) => JSON.stringify(o)
const REPORT = `${D}/reports/clerk-bruh-pollerwait.jsonl`

function world(stored: Record<string, unknown> = {}) {
  const files: Record<string, string> = {
    [`${D}/repos.json`]: line({ repos: [{ repo: 'oter/bruh', host: 'github', project: 'bruh' }] }),
    [`${D}/init/config.json`]: line({ ledger_path: '/l' }),
    '/l/projects/bruh.md': `# Project: bruh\n\n## In progress\n\n${LEDGER}\n\n## Merged\n\n| Owner | Task |\n|---|---|\n| clanker-bruh | task 99: old |\n`,
    [`${D}/questions/Q-bruh-m-98.json`]: line({ id: 'Q-bruh-m-98', priority: 'P1', subject: 'merge oter/bruh#29?', asker: 'clanker-bruh', opened_at: '2026-10-05T16:20:00Z' }),
    [`${D}/questions/Q-bruh-m-97.json`]: line({ id: 'Q-bruh-m-97', priority: 'P1', subject: 'answered by bigm', asker: 'clanker-bruh', opened_at: '2026-10-05T16:00:00Z' }),
    [`${D}/questions/Q-shop-m-96.json`]: line({ id: 'Q-shop-m-96', priority: 'P0', subject: 'answered by the asker', asker: 'clerk-shop-x', opened_at: '2026-10-05T15:00:00Z' }),
    [`${D}/questions/Q-shop-m-95.json`]: line({ id: 'Q-shop-m-95', priority: 'P2', subject: 'a P2 question', asker: 'clanker-shop', opened_at: '2026-10-05T15:00:00Z' }),
    [`${D}/questions/next`]: '99',
    [`${D}/answers/bigm/Q-bruh-m-97.answer`]: 'ok',
    [`${D}/answers/clerk-shop-x/Q-shop-m-96.answer`]: 'ok',
    [REPORT]: [
      line({ at: '2026-10-05T16:10:00Z', from: 'clerk-bruh-pollerwait', kind: 'status', text: 'runs now: /bruh:deliver run wf_712df188-d86 for task 12' }),
      line({ at: '2026-10-05T16:12:00Z', from: 'clerk-bruh-pollerwait', kind: 'status', text: 'pushed commit 3f9a2b1c4d to the branch' }),
      line({ at: '2026-10-05T16:13:00Z', from: 'clerk-bruh-pollerwait', kind: 'event', text: 'mail read' }),
    ].join('\n') + '\n',
  }
  const sessions: Record<string, unknown>[] = [
    { name: 'clanker-bruh', state: 'stopped', startedAt: 1, cwd: '/w/bruh' },
    { name: 'clanker-bruh', pid: 10, status: 'busy', state: 'working', startedAt: 5, cwd: '/w/bruh' },
    { name: 'clerk-bruh-pollerwait', pid: 11, status: 'busy', state: 'working', startedAt: 6, cwd: '/w/bruh/.claude/worktrees/fix-poller-wait-lock' },
    { name: 'clerk-bruh-liveui', pid: 12, status: 'idle', state: 'blocked', startedAt: 4, cwd: '/w/bruh' },
    { name: 'clanker-shop', state: 'done', startedAt: 3, cwd: '/w/shop' },
    { name: 'clerk-shop-x', pid: 13, status: 'idle', startedAt: 2, cwd: '/w/shop' },
    { name: 'infra-4a', pid: 14, status: 'busy', startedAt: 7, cwd: '/w/infra' },
    { name: 'bigm', pid: 15, status: 'busy', startedAt: 8, cwd: '/w' },
  ]
  const store = new Map<string, unknown>(Object.entries(stored))
  const calls = { run: 0, opened: [] as string[], registered: [] as string[], writes: [] as string[], sent: [] as string[] }
  const stub = (on: On) => {
    mock.env(on, { HOME: '/h' })
    on('fs.read', ($, e) => (e.path in files ? { value: files[e.path] } : { deny: `ENOENT ${e.path}` }))
    on('fs.exists', ($, e) => ({ value: e.path in files }))
    on('fs.list', ($, e) => {
      const prefix = `${e.path}/`
      const kinds = new Map<string, 'file' | 'dir'>() // each child once: a file, or a folder of files
      for (const rest of Object.keys(files).filter(f => f.startsWith(prefix)).map(f => f.slice(prefix.length))) {
        kinds.set(rest.split('/')[0], rest.includes('/') ? 'dir' : 'file')
      }
      return kinds.size
        ? { value: [...kinds].map(([name, kind]) => ({ name, kind, size: 1, mtimeMs: 0, isLink: false })) }
        : { deny: `ENOENT ${e.path}` }
    })
    on('process.run', ($, e) => {
      calls.run += 1
      expect(e.argv).toEqual(['claude', 'agents', '--json', '--all'])
      return { value: { exitCode: 0, stdout: JSON.stringify(sessions), stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
    })
    on('store.get', ($, e) => ({ value: store.get(e.key) }))
    on('store.set', ($, e) => {
      calls.writes.push(e.key)
      store.set(e.key, e.value)
      return { value: undefined }
    })
    on('ui.open', ($, e) => {
      calls.opened.push(`${e.id}${e.focus ? ' focus' : ''}`)
      return { value: { isPlaced: true as const } }
    })
    on('prompt.submit', ($, e) => {
      calls.sent.push(e.text)
      return { text: e.text }
    })
    on('config.set', ($, e) => ({ value: e.value }))
    on('command.register', ($, e) => {
      calls.registered.push(e.name)
      return { value: { command: e.name } }
    })
    return mock.clock(on, { now: Date.parse('2026-10-05T16:30:00Z') })
  }
  return { files, sessions, store, calls, stub }
}

const ALL_OPEN = { 'open:clanker-bruh': true, 'open:clanker-shop': true, 'open:clerk-bruh-pollerwait': true, 'open:clerk-bruh-liveui': true, 'open:clerk-shop-x': true }

const DESIGNS = ['cards', 'buckets', 'pipeline'] as const
const pick = (design: string) => ({ options: { board_design: design } })
// A done and a stopped clerk of oter/bruh: they belong to the Done group.
function withEnded(w: ReturnType<typeof world>) {
  w.sessions.push(
    { name: 'clerk-bruh-merge', state: 'done', startedAt: 3, cwd: '/w/bruh/.claude/worktrees/merge-simplify' },
    { name: 'clerk-bruh-tabclose', state: 'stopped', startedAt: 2, cwd: '/w/bruh/.claude/worktrees/tab-close' },
  )
  return w
}

const mount = ($: any, bodyColumns = 200) => $.ui.mount({
  plugin: 'bruh', surface: 'terminal', component: 'Pane', requestId: 'bruh-board',
  props: { title: 'bruh board', isFocused: true, bodyColumns, placement: 'dock' as const, scroll: { offset: 0, bodyRows: 60 }, view: {} },
})
// What the pane shows: each Text, and each Button as its key, hotkey (none) and label.
const view = async (ui: any) => {
  const texts: any[] = await ui.findAll({ type: 'Text' })
  const buttons: any[] = await ui.findAll({ type: 'Button' })
  return {
    texts: texts.map(t => t.text as string),
    textNodes: texts,
    buttons: buttons.map(b => ({ key: b.props.key ?? b.key, hotkey: b.props.hotkey, label: b.props.label as string })),
  }
}
const all = async (ui: any) => {
  const v = await view(ui)
  return [...v.texts, ...v.buttons.map(b => b.label)].join('\n')
}
const labels = async (ui: any) => (await view(ui)).buttons.map(b => b.label)

test('/bruh-board opens the pane with the keys and nothing opens it unasked', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.session.start({ source: 'startup' } as any).catch(() => undefined)
  expect(w.calls.registered).toEqual(['bruh-board'])
  expect(w.calls.opened).toEqual([])
  const ran = await $.command.run({ command: 'bruh-board' })
  expect(w.calls.opened).toEqual(['bruh-board focus'])
  expect(ran.text).toBe('bruh board opened.')
})

test('the default view: the open questions, one collapsed card per clanker and the Done group, no hotkeys', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const v = await view(ui)
  expect(v.texts).toContain('P1 merge oter/bruh#29?')
  const text = await all(ui)
  expect(text).not.toContain('answered by bigm')
  expect(text).not.toContain('answered by the asker')
  expect(text).not.toContain('a P2 question')
  expect(text).not.toContain('infra-4a')
  expect(text).not.toContain('bigm')
  expect(text).not.toMatch(/last:|next:|pollerwait|liveui/)
  expect(v.buttons).toEqual([
    { key: 'toggle-clanker-bruh', hotkey: undefined, label: '▸ oter/bruh · 2 tasks' },
    { key: 'toggle-clanker-shop', hotkey: undefined, label: '▸ shop · no tasks' },
    { key: 'toggle-done', hotkey: undefined, label: '▸ Done (0)' },
  ])
  expect(w.calls.writes).toEqual([])
})

test('a clanker expands to its clerks and collapses again, and the store keeps it', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  await ui.press({ key: 'toggle-clanker-bruh' })
  const v = await view(ui)
  expect(v.buttons).toEqual([
    { key: 'toggle-clanker-bruh', hotkey: undefined, label: '▾ oter/bruh · 2 tasks' },
    { key: 'toggle-clerk-bruh-pollerwait', hotkey: undefined, label: '▸ task 12 fix-poller-wait-lock · working' },
    { key: 'toggle-clerk-bruh-liveui', hotkey: undefined, label: '▸ liveui · blocked' },
    { key: 'toggle-clanker-shop', hotkey: undefined, label: '▸ shop · no tasks' },
    { key: 'toggle-done', hotkey: undefined, label: '▸ Done (0)' },
  ])
  expect(w.store.get('open:clanker-bruh')).toBe(true)
  await ui.press({ key: 'toggle-clanker-bruh' })
  expect(await labels(ui)).toEqual(['▸ oter/bruh · 2 tasks', '▸ shop · no tasks', '▸ Done (0)'])
  expect(w.store.get('open:clanker-bruh')).toBe(false)
  expect(w.calls.writes).toEqual(['open:clanker-bruh', 'open:clanker-bruh'])
})

test('a clerk expands to its name, last done and next step and collapses again', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  await ui.press({ key: 'toggle-clanker-bruh' })
  await ui.press({ key: 'toggle-clerk-bruh-pollerwait' })
  let v = await view(ui)
  // R-10 name; last done prefers the last status line over a later event line; a SHA is masked (R-5)
  expect(v.texts).toContain('oter/bruh pollerwait (clerk, task 12 fix-poller-wait-lock)')
  expect(v.texts).toContain('last: pushed commit … to the branch')
  expect(v.texts).toContain('next: PR on oter/bruh')
  expect(await all(ui)).not.toContain('3f9a2b1c4d')
  expect(w.store.get('open:clerk-bruh-pollerwait')).toBe(true)
  await ui.press({ key: 'toggle-clerk-bruh-pollerwait' })
  v = await view(ui)
  expect(v.texts.join('\n')).not.toMatch(/last:|next:|\(clerk/)
  expect(w.store.get('open:clerk-bruh-pollerwait')).toBe(false)
})

test('the expanded state saved in the store shows at the first mount', async ($, on) => {
  const w = world({ 'open:clanker-bruh': true, 'open:clerk-bruh-pollerwait': true })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const v = await view(ui)
  expect(v.buttons.map(b => b.label)).toEqual([
    '▾ oter/bruh · 2 tasks', '▾ task 12 fix-poller-wait-lock · working', '▸ liveui · blocked', '▸ shop · no tasks', '▸ Done (0)',
  ])
  expect(v.texts).toContain('last: pushed commit … to the branch')
  expect(w.calls.writes).toEqual([])
})

test('a refresh after 10 seconds shows new data', async ($, on) => {
  const w = world({ 'open:clanker-bruh': true, 'open:clerk-bruh-pollerwait': true })
  const clock = w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  expect(await all(ui)).not.toContain('phase two')
  w.files[REPORT] += line({ at: '2026-10-05T16:31:00Z', kind: 'status', text: 'phase two' }) + '\n'
  await clock.advance(9500)
  expect(await all(ui)).not.toContain('phase two')
  await clock.advance(500)
  expect(w.calls.run).toBe(2)
  expect((await view(ui)).texts).toContain('last: phase two')
})

test('the spinner differs by role and by state, a clanker shows the most urgent, and only working moves', async ($, on) => {
  const w = world(ALL_OPEN)
  const clock = w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  // the spinner is a keyed Box around one Text: its text is the glyph, its props the colour
  const spin = async (key: string) => {
    const box: any = await ui.find({ key: `spin-${key}` })
    return box && { text: box.text, props: box.children[0].props }
  }
  const owner = await spin('clanker-bruh') // asker of the open P1
  const working = await spin('clerk-bruh-pollerwait')
  const blocked = await spin('clerk-bruh-liveui')
  const shop = await spin('clanker-shop') // a done clanker with an idle clerk
  const idle = await spin('clerk-shop-x')
  expect(owner?.text).toMatch(/^[⣾⣽⣻⢿⡿⣟⣯⣷]\?$/)
  expect(owner?.props.color).toBe('yellow')
  expect(working?.text).toMatch(/^[◐◓◑◒]$/)
  expect(working?.props.color).toBe('green')
  expect(blocked?.text).toMatch(/^[◐◓◑◒]!$/)
  expect(blocked?.props.color).toBe('red')
  expect(shop?.text).toMatch(/^[⣾⣽⣻⢿⡿⣟⣯⣷]$/)
  expect(shop?.props.dimColor).toBe(true)
  expect(idle?.props.dimColor).toBe(true)
  await clock.advance(500)
  expect((await spin('clerk-bruh-pollerwait'))?.text).not.toBe(working?.text)
  expect((await spin('clerk-bruh-liveui'))?.text).toBe(blocked?.text)
})

test('a clanker whose sessions are all done is one gray line of the Done group', async ($, on) => {
  const w = world()
  w.sessions.splice(w.sessions.findIndex(s => s.name === 'clerk-shop-x'), 1)
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  expect(await labels(ui)).toEqual(['▸ oter/bruh · 2 tasks', '▸ Done (1)'])
  await ui.press({ key: 'toggle-done' })
  const done: any = await ui.find({ key: 'done-clanker-shop' })
  expect(done.text).toBe('✓ shop (clanker) · done')
  expect(done.children[0].props.color).toBe('gray')
})

test('no line holds a timestamp, a run ID, a question ID, a SHA, a pid or a start time', async ($, on) => {
  const w = world(ALL_OPEN)
  for (const s of w.sessions) {
    if (s.pid) s.pid = 90000 + (s.pid as number)
    s.startedAt = 1759600000000 + (s.startedAt as number)
  }
  w.files[REPORT] += line({ at: '2026-10-05T16:40:00Z', kind: 'status', text: 'at 2026-10-05T16:39:12Z and 16:39Z run wf_712df188-d86 asked Q-bruh-m-98 on 3f9a2b1' }) + '\n'
  w.files[`${D}/questions/Q-bruh-m-98.json`] = line({ id: 'Q-bruh-m-98', priority: 'P1', subject: 'merge at 2026-10-05T16:20:00Z?', asker: 'clanker-bruh', opened_at: '2026-10-05T16:20:00Z' })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const text = await all(ui)
  expect(text).toContain('last: at … and … run … asked … on …')
  expect(text).toContain('P1 merge at …?')
  expect(text).not.toMatch(/\d{4}-\d\d-\d\dT|\d\d:\d\dZ|wf_|Q-|\b[0-9a-f]{7,40}\b|9001\d|17596/)
})

// The columns a drawn element takes, as the terminal lays it out: a Box is a row
// unless it says column, a border and its padding take columns, a Button draws
// "[ label ]" unless plain. A Box with a width checks its content fits it.
const columns = (node: any): number => {
  if (typeof node === 'string') return [...node].length
  if (!node) return 0
  if (node.type === 'Button') return [...node.props.label].length + (node.props.plain ? 0 : 4)
  const kids: number[] = (node.children ?? []).map(columns)
  if (node.type !== 'Box') return kids.reduce((a, b) => a + b, 0)
  const inner = node.props.flexDirection === 'column' ? Math.max(0, ...kids) : kids.reduce((a, b) => a + b, 0)
  if (node.props.width !== undefined) expect(inner <= node.props.width).toBe(true)
  return Math.max(inner, node.props.width ?? 0) + (node.props.borderStyle ? 2 : 0) + 2 * (node.props.paddingX ?? 0)
}

for (const design of DESIGNS) {
  test(`long text is cut to the pane width, not wrapped (${design})`, pick(design), async ($, on) => {
    const w = withEnded(world({ ...ALL_OPEN, 'open:done': true }))
    w.files[`${D}/questions/Q-bruh-m-98.json`] = line({ id: 'Q-bruh-m-98', priority: 'P1', subject: 'merge oter/bruh#29 after the long review of the board?', asker: 'clanker-bruh', opened_at: '2026-10-05T16:20:00Z', options: [{ label: 'merge it now please' }, { label: 'wait for the review' }] })
    w.files[REPORT] += line({ kind: 'event', text: 'review', phase: 'review' }) + '\n'
    w.stub(on)
    await $.command.run({ command: 'bruh-board' })
    const ui = await mount($, 24)
    const v = await view(ui)
    for (const t of v.textNodes) if (t.props.wrap) expect(t.props.wrap).toBe('truncate-end')
    const root: any = await ui.drawn()
    for (const row of root.children) expect(columns(row) <= 24).toBe(true)
    expect(v.texts.some(t => t.includes('pushed co'))).toBe(true)
    if (design === 'cards') {
      expect(v.texts).toContain('P1 merge oter/bruh#…')
      expect(v.buttons.find(b => b.key === 'answer-Q-bruh-m-98-0')?.label).toBe('mer…')
      expect(v.buttons.find(b => b.key === 'toggle-clanker-bruh')?.label).toBe('▾ oter/bruh · 4 …')
      expect(v.texts).toContain('last: pushed co…')
    }
  })
}

test('a clanker with no session still counts the tasks of its rows and its live clerks', async ($, on) => {
  const w = world()
  w.sessions.splice(0, 2) // no clanker-bruh session
  w.sessions.find(s => s.name === 'clerk-shop-x')!.cwd = '/w/shop/.claude/worktrees/fix-x'
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  expect(await labels(ui)).toEqual(['▸ oter/bruh · 2 tasks', '▸ shop · 1 task', '▸ Done (0)'])
})

test('two open questions with one subject show their numbers', async ($, on) => {
  const w = world()
  w.files[`${D}/questions/Q-bruh-m-94.json`] = line({ id: 'Q-bruh-m-94', priority: 'P1', subject: 'merge oter/bruh#29?', asker: 'clerk-bruh-liveui', opened_at: '2026-10-05T16:25:00Z' })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const v = await view(ui)
  expect(v.texts).toContain('P1 (98) merge oter/bruh#29?')
  expect(v.texts).toContain('P1 (94) merge oter/bruh#29?')
})

test('a delegated answer and a replaced duplicate do not show as open (task 24)', async ($, on) => {
  const w = world()
  // A P1 of a clerk that its clanker answered (delegated): the answer file is in answers/clanker-shop/.
  w.files[`${D}/questions/Q-shop-m-90.json`] = line({ id: 'Q-shop-m-90', priority: 'P1', subject: 'delegated', asker: 'clerk-shop-x', opened_at: '2026-10-05T14:00:00Z' })
  w.files[`${D}/answers/clanker-shop/Q-shop-m-90.answer`] = 'ok'
  // A P0 that Q-shop-m-92 replaces: answer_write of Q-shop-m-92 wrote both answer files of bigm.
  w.files[`${D}/questions/Q-shop-m-91.json`] = line({ id: 'Q-shop-m-91', priority: 'P0', subject: 'replaced duplicate', asker: 'clerk-shop-x', opened_at: '2026-10-05T14:01:00Z' })
  w.files[`${D}/questions/Q-shop-m-92.json`] = line({ id: 'Q-shop-m-92', priority: 'P0', subject: 'its replacement', asker: 'clerk-shop-x', opened_at: '2026-10-05T14:02:00Z', replaces: 'Q-shop-m-91' })
  w.files[`${D}/answers/bigm/Q-shop-m-91.answer`] = 'ok'
  w.files[`${D}/answers/bigm/Q-shop-m-92.answer`] = 'ok'
  w.files[`${D}/questions/Q-shop-m-89.json`] = line({ id: 'Q-shop-m-89', priority: 'P1', subject: 'not answered', asker: 'clerk-shop-x', opened_at: '2026-10-05T14:03:00Z' })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const text = await all(ui)
  expect(text).toContain('P1 not answered')
  expect(text).not.toContain('delegated')
  expect(text).not.toContain('replaced duplicate')
  expect(text).not.toContain('its replacement')
})

const OPTIONS = [{ label: 'merge now', description: 'squash' }, { label: 'wait', description: 'after the review' }]
// An open P1 with options of the clanker, and a refusal P0 of a clerk.
function withAnswers(w: ReturnType<typeof world>) {
  w.files[`${D}/questions/Q-bruh-m-98.json`] = line({ id: 'Q-bruh-m-98', priority: 'P1', subject: 'merge oter/bruh#29?', asker: 'clanker-bruh', opened_at: '2026-10-05T16:20:00Z', options: OPTIONS })
  w.files[`${D}/questions/Q-bruh-m-93.json`] = line({ id: 'Q-bruh-m-93', priority: 'P0', subject: 'refused', body: 'why\n\nCOMMAND: x\nCATEGORY: classifier y', asker: 'clerk-bruh-liveui', opened_at: '2026-10-05T16:21:00Z' })
  return w
}

for (const design of DESIGNS) {
  test(`a question shows one button per option, a refusal P0 ok and hold, and a press submits the exact text (${design})`, pick(design), async ($, on) => {
    const w = withAnswers(world(ALL_OPEN))
    w.stub(on)
    await $.command.run({ command: 'bruh-board' })
    const ui = await mount($)
    const answers = (await view(ui)).buttons.filter(b => b.key.startsWith('answer-'))
    const p0 = [
      { key: 'answer-Q-bruh-m-93-0', hotkey: undefined, label: 'ok' },
      { key: 'answer-Q-bruh-m-93-1', hotkey: undefined, label: 'hold' },
    ]
    const p1 = [
      { key: 'answer-Q-bruh-m-98-0', hotkey: undefined, label: 'merge now' },
      { key: 'answer-Q-bruh-m-98-1', hotkey: undefined, label: 'wait' },
    ]
    // every design: the P0 first, then the P1
    expect(answers).toEqual([...p0, ...p1])
    await ui.press({ key: 'answer-Q-bruh-m-98-0' })
    await ui.press({ key: 'answer-Q-bruh-m-93-1' })
    expect(w.calls.sent).toEqual(['Q-bruh-m-98: merge now', 'Q-bruh-m-93: hold'])
    expect(w.calls.writes).toEqual([])
  })
}

// R-16 (owner, 2026-10-08): "what waits on me - must be in the bottom".
const ASK = /^(q-|card-Q-|answers-|bar-questions$)/
for (const design of DESIGNS) {
  test(`what waits on you is drawn last, below the clankers, clerks and the Done group (${design})`, pick(design), async ($, on) => {
    const w = withEnded(withAnswers(world({ ...ALL_OPEN, 'open:done': true })))
    w.stub(on)
    await $.command.run({ command: 'bruh-board' })
    const ui = await mount($)
    const root: any = await ui.drawn()
    const keys: string[] = root.children.map((c: any) => c.props?.key ?? '').filter(Boolean)
    const first = keys.findIndex(k => ASK.test(k))
    const other = keys.filter(k => !ASK.test(k))
    expect(first).toBeGreaterThan(0)
    expect(keys.slice(first).every(k => ASK.test(k))).toBe(true)
    expect(keys.slice(first).filter(k => /Q-bruh-m-9[38]$/.test(k)).length).toBeGreaterThan(1)
    // the clanker, the clerk and the Done items all come before the first question
    const done = design === 'cards' ? 'card-done' : design === 'buckets' ? 'bar-done' : 'done-clerk-bruh-tabclose'
    expect(other).toContain(done)
    expect(other.some(k => k.includes('clanker-bruh'))).toBe(true)
    // the last answer button is the last button of the pane
    expect((await view(ui)).buttons.at(-1)?.key).toBe('answer-Q-bruh-m-98-1')
  })
}

// The cards, buckets and pipeline of one world, each test drawing one design.

test('cards: a bold card per clanker, a round card per clerk in its state colour, a dashed Done card, then a double card per question', pick('cards'), async ($, on) => {
  const w = withAnswers(world(ALL_OPEN))
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const boxes: any[] = (await ui.findAll({ type: 'Box' })).filter((b: any) => b.props.borderStyle)
  expect(boxes.map(b => [b.key, b.props.borderStyle, b.props.borderColor])).toEqual([
    ['card-clanker-bruh', 'bold', 'gray'],
    ['card-clerk-bruh-pollerwait', 'round', 'green'],
    ['card-clerk-bruh-liveui', 'round', 'yellow'], // the asker of the P0 waits on you
    ['card-clanker-shop', 'bold', 'gray'],
    ['card-clerk-shop-x', 'round', 'gray'],
    ['card-done', 'dashed', 'gray'],
    ['card-Q-bruh-m-93', 'double', 'red'],
    ['card-Q-bruh-m-98', 'double', 'yellow'],
  ])
  const pollerwait = boxes[1]
  expect(pollerwait.text).toContain('last: pushed commit … to the branch')
  expect((await ui.find({ key: 'card-Q-bruh-m-98' }))?.text).toContain('P1 merge oter/bruh#29?')
})

test('buckets: a soft bar per state, most urgent first, the Done bar, and last one bar with every open question', pick('buckets'), async ($, on) => {
  const w = withAnswers(world())
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($, 60)
  const root: any = await ui.drawn()
  const keys: string[] = root.children.map((c: any) => c.props?.key ?? '')
  const bars: any[] = (await ui.findAll({ type: 'Box' })).filter((b: any) => String(b.key).startsWith('bar-') && b.key !== 'bar-done')
  expect(bars.map(b => b.text.trimEnd())).toEqual([' WAITS ON YOU  1', ' WORKING  1', ' IDLE  1', ' WAITS ON YOU  2'])
  for (const bar of bars) {
    const props = bar.children[0].props
    expect([...bar.text].length).toBe(60)
    expect(Object.values(SOFT)).toContain(props.backgroundColor)
    expect(props.color).toBe('#e6edf3')
  }
  expect((await ui.find({ key: 'bar-done' }))?.props.backgroundColor).toBe(SOFT.done)
  expect(keys).toEqual([
    'crumb-clanker-bruh',
    'bar-clanker-bruh-owner', 'clerk-bruh-liveui',
    'bar-clanker-bruh-working', 'clerk-bruh-pollerwait',
    'crumb-clanker-shop', 'bar-clanker-shop-idle', 'clerk-shop-x',
    'bar-done',
    // what waits on you, last: a clerk's P0 and the clanker's own P1
    'bar-questions', 'q-Q-bruh-m-93', 'answers-Q-bruh-m-93', 'q-Q-bruh-m-98', 'answers-Q-bruh-m-98',
  ])
  expect((await ui.find({ key: 'crumb-clanker-bruh' }))?.text).toBe('oter/bruh · 2 tasks')
  expect((await ui.find({ key: 'q-Q-bruh-m-93' }))?.text).toBe(' P0 refused')
  expect(await labels(ui)).toContain('▸ task 12 fix-poller-wait-lock')
})

// The strip cells of a clerk row in the pipeline: each Text of its strip Box.
const cells = async (ui: any, key: string) => {
  const strip: any = await ui.find({ key: `strip-${key}` })
  return strip.children.map((c: any) => ({ text: c.children.join(''), color: c.props.color, dim: c.props.dimColor }))
}
const GREEN = { text: '━━━━━ ', color: 'green', dim: undefined }
const AHEAD = { text: '····· ', color: undefined, dim: true }

for (const phase of PHASES) {
  test(`pipeline: a report line with phase ${phase} fills the strip up to it`, pick('pipeline'), async ($, on) => {
    const w = world(ALL_OPEN)
    w.files[REPORT] += line({ kind: 'status', text: 'step', phase }) + '\n' + line({ kind: 'event', text: 'mail read' }) + '\n'
    w.stub(on)
    await $.command.run({ command: 'bruh-board' })
    const ui = await mount($)
    const got = await cells(ui, 'clerk-bruh-pollerwait')
    const at = PHASES.indexOf(phase)
    if (phase === 'done') {
      expect(got).toEqual([GREEN, GREEN, GREEN, GREEN, GREEN])
      return
    }
    expect(got).toHaveLength(5)
    expect(got.slice(0, at)).toEqual(Array(at).fill(GREEN))
    expect(got[at].text).toMatch(/^[◐◓◑◒]━━━━ $/)
    expect(got[at].color).toBe('green')
    expect(got.slice(at + 1)).toEqual(Array(4 - at).fill(AHEAD))
  })
}

test('pipeline: with no phase field the strip says phase not reported, also when the text says a phase', pick('pipeline'), async ($, on) => {
  const w = world(ALL_OPEN)
  w.files[REPORT] += line({ kind: 'status', text: 'phase review' }) + '\n' + line({ kind: 'status', text: 'x', phase: 'shipping' }) + '\n'
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const got = await cells(ui, 'clerk-bruh-pollerwait')
  expect(got[0].text).toMatch(/^[◐◓◑◒] $/)
  expect(got[0].color).toBe('green')
  expect(got[1]).toEqual({ text: '····· phase not reported', color: undefined, dim: true })
  expect(await cells(ui, 'clerk-bruh-liveui')).toEqual([
    expect.objectContaining({ text: expect.stringMatching(/^[◐◓◑◒]! $/), color: 'red' }),
    { text: '····· phase not reported', color: undefined, dim: true },
  ])
})

test('pipeline: an open question shows "?" at the phase, merged is a full green strip, stopped shows ■ stopped', pick('pipeline'), async ($, on) => {
  const w = withEnded(world({ ...ALL_OPEN, 'open:done': true }))
  w.files[`${D}/questions/Q-bruh-m-93.json`] = line({ id: 'Q-bruh-m-93', priority: 'P1', subject: 'which way?', asker: 'clerk-bruh-liveui', opened_at: '2026-10-05T16:21:00Z' })
  w.files[`${D}/reports/clerk-bruh-liveui.jsonl`] = line({ kind: 'status', text: 'asked', phase: 'review' }) + '\n'
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const liveui = await cells(ui, 'clerk-bruh-liveui')
  expect(liveui[2].text).toMatch(/^[◐◓◑◒]\?━━━ $/)
  expect(liveui[2].color).toBe('yellow')
  expect(await cells(ui, 'clerk-bruh-merge')).toEqual([GREEN, GREEN, GREEN, GREEN, GREEN])
  expect(await cells(ui, 'clerk-bruh-tabclose')).toEqual([{ text: '■ stopped', color: 'gray', dim: undefined }])
  expect((await view(ui)).texts).toContain(`${' '.repeat(28)}plan  impl  rev   fix   merge`)
})

test('pipeline: an open question outranks a reported done phase: "?" sits at merge', pick('pipeline'), async ($, on) => {
  const w = world(ALL_OPEN)
  w.files[`${D}/questions/Q-bruh-m-93.json`] = line({ id: 'Q-bruh-m-93', priority: 'P1', subject: 'which way?', asker: 'clerk-bruh-liveui', opened_at: '2026-10-05T16:21:00Z' })
  w.files[`${D}/reports/clerk-bruh-liveui.jsonl`] = line({ kind: 'status', text: 'asked', phase: 'done' }) + '\n'
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const liveui = await cells(ui, 'clerk-bruh-liveui')
  expect(liveui.slice(0, 4)).toEqual([GREEN, GREEN, GREEN, GREEN])
  expect(liveui[4].text).toMatch(/^[◐◓◑◒]\?━━━ $/)
  expect(liveui[4].color).toBe('yellow')
})

test('readPhase takes the latest line with a known top-level phase and reads nothing else', () => {
  for (const phase of PHASES) expect(readPhase([{ phase: 'plan' }, { phase }, { kind: 'event' }])).toBe(phase)
  expect(readPhase([])).toBeUndefined()
  expect(readPhase([{ text: 'phase review' }, { kind: 'status', text: 'review: fix' }])).toBeUndefined()
  expect(readPhase([{ phase: 'review' }, { phase: 'shipping' }, { phase: 'Review' }])).toBe('review')
  expect(readPhase([{ event: { phase: 'merge' } }, null, 3])).toBeUndefined()
})

for (const design of DESIGNS) {
  test(`the Done group sits after the clankers, collapsed with its count, and opens on a press (${design})`, pick(design), async ($, on) => {
    const w = withEnded(world())
    w.stub(on)
    await $.command.run({ command: 'bruh-board' })
    const ui = await mount($)
    let v = await view(ui)
    expect(v.buttons.at(-1)).toEqual({ key: 'toggle-done', hotkey: undefined, label: '▸ Done (2)' })
    let text = await all(ui)
    expect(text).not.toMatch(/merge-simplify|tab-close/)
    expect(text).toContain('oter/bruh · 4 tasks') // the ended clerks still count
    await ui.press({ key: 'toggle-done' })
    expect(w.store.get('open:done')).toBe(true)
    v = await view(ui)
    expect(v.buttons.at(-1)?.label).toBe('▾ Done (2)')
    text = await all(ui)
    expect(text).toContain('merge-simplify')
    expect(text).toContain('tab-close')
    if (design !== 'pipeline') {
      expect(v.texts).toContain('✓ merge-simplify · done')
      expect(v.texts).toContain('■ tab-close · stopped')
    }
    await ui.press({ key: 'toggle-done' })
    expect(w.store.get('open:done')).toBe(false)
    expect(await all(ui)).not.toMatch(/merge-simplify|tab-close/)
  })
}

test('the /config pick switches the open pane at runtime, with no new /bruh-board', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  expect(await ui.find({ key: 'card-done' })).toBeDefined()
  for (const design of ['pipeline', 'buckets', 'cards']) {
    const set = await $.config.set({ key: 'bruh.board_design', value: design })
    expect(set.value).toBe(design)
    expect(Boolean(await ui.find({ key: 'card-done' }))).toBe(design === 'cards')
    expect(Boolean(await ui.find({ key: 'bar-done' }))).toBe(design === 'buckets')
    expect((await view(ui)).texts.includes('Waits on you')).toBe(design === 'pipeline')
  }
  expect(w.calls.opened).toEqual(['bruh-board focus'])
  expect(w.calls.run).toBe(1)
})

test('a draw with no timer (after a reload) starts the refresh again', pick('pipeline'), async ($, on) => {
  const w = world()
  const clock = w.stub(on)
  const ui = await mount($) // no /bruh-board in this module's life
  expect(w.calls.run).toBe(1)
  expect(await labels(ui)).toContain('▸ oter/bruh · 2 tasks')
  await clock.advance(10000)
  expect(w.calls.run).toBe(2)
})
