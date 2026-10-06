import { expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'

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
      const names = Object.keys(files).filter(f => f.startsWith(prefix)).map(f => f.slice(prefix.length)).filter(n => !n.includes('/'))
      return names.length
        ? { value: names.map(name => ({ name, kind: 'file' as const, size: 1, mtimeMs: 0, isLink: false })) }
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
    on('command.register', ($, e) => {
      calls.registered.push(e.name)
      return { value: { command: e.name } }
    })
    return mock.clock(on, { now: Date.parse('2026-10-05T16:30:00Z') })
  }
  return { files, sessions, store, calls, stub }
}

const ALL_OPEN = { 'open:clanker-bruh': true, 'open:clanker-shop': true, 'open:clerk-bruh-pollerwait': true, 'open:clerk-bruh-liveui': true, 'open:clerk-shop-x': true }

const mount = ($: any, bodyColumns = 200) => $.ui.mount({
  plugin: 'bruh', surface: 'terminal', component: 'Pane', requestId: 'bruh-board',
  props: { title: 'bruh board', isFocused: true, bodyColumns, placement: 'dock' as const, scroll: { offset: 0, bodyRows: 60 }, view: {} },
})
// What the pane shows: each Text, and each Button as its key, hotkey and label.
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

test('the default view: the open questions and one collapsed line per clanker', async ($, on) => {
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
    { key: 'toggle-clanker-bruh', hotkey: '1', label: '▸ oter/bruh · 2 tasks' },
    { key: 'toggle-clanker-shop', hotkey: '2', label: '▸ shop · no tasks' },
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
    { key: 'toggle-clanker-bruh', hotkey: '1', label: '▾ oter/bruh · 2 tasks' },
    { key: 'toggle-clerk-bruh-pollerwait', hotkey: 'a', label: '▸ task 12 fix-poller-wait-lock · working' },
    { key: 'toggle-clerk-bruh-liveui', hotkey: 'b', label: '▸ liveui · blocked' },
    { key: 'toggle-clanker-shop', hotkey: '2', label: '▸ shop · no tasks' },
  ])
  expect(w.store.get('open:clanker-bruh')).toBe(true)
  await ui.press({ key: 'toggle-clanker-bruh' })
  expect(await labels(ui)).toEqual(['▸ oter/bruh · 2 tasks', '▸ shop · no tasks'])
  expect(w.store.get('open:clanker-bruh')).toBe(false)
  expect(w.calls.writes).toEqual(['open:clanker-bruh', 'open:clanker-bruh'])
})

test('a clerk expands to its last done and next step and collapses again', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  await ui.press({ key: 'toggle-clanker-bruh' })
  await ui.press({ key: 'toggle-clerk-bruh-pollerwait' })
  let v = await view(ui)
  // last done prefers the last status line over a later event line; a SHA is masked (R-5)
  expect(v.texts).toContain('    last: pushed commit … to the branch')
  expect(v.texts).toContain('    next: PR on oter/bruh')
  expect(await all(ui)).not.toContain('3f9a2b1c4d')
  expect(w.store.get('open:clerk-bruh-pollerwait')).toBe(true)
  await ui.press({ key: 'toggle-clerk-bruh-pollerwait' })
  v = await view(ui)
  expect(v.texts.join('\n')).not.toMatch(/last:|next:/)
  expect(w.store.get('open:clerk-bruh-pollerwait')).toBe(false)
})

test('the expanded state saved in the store shows at the first mount', async ($, on) => {
  const w = world({ 'open:clanker-bruh': true, 'open:clerk-bruh-pollerwait': true })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const v = await view(ui)
  expect(v.buttons.map(b => b.label)).toEqual([
    '▾ oter/bruh · 2 tasks', '▾ task 12 fix-poller-wait-lock · working', '▸ liveui · blocked', '▸ shop · no tasks',
  ])
  expect(v.texts).toContain('    last: pushed commit … to the branch')
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
  expect((await view(ui)).texts).toContain('    last: phase two')
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

test('a clanker whose sessions are all done shows a gray check', async ($, on) => {
  const w = world()
  w.sessions.splice(w.sessions.findIndex(s => s.name === 'clerk-shop-x'), 1)
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const box: any = await ui.find({ key: 'spin-clanker-shop' })
  expect(box.text).toBe('✓')
  expect(box.children[0].props.color).toBe('gray')
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
  expect(text).toContain('    last: at … and … run … asked … on …')
  expect(text).toContain('P1 merge at …?')
  expect(text).not.toMatch(/\d{4}-\d\d-\d\dT|\d\d:\d\dZ|wf_|Q-|\b[0-9a-f]{7,40}\b|9001\d|17596/)
})

test('long text is cut to the pane width, not wrapped', async ($, on) => {
  const w = world(ALL_OPEN)
  w.files[`${D}/questions/Q-bruh-m-98.json`] = line({ id: 'Q-bruh-m-98', priority: 'P1', subject: 'merge oter/bruh#29 after the long review of the board?', asker: 'clanker-bruh', opened_at: '2026-10-05T16:20:00Z' })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($, 24)
  const v = await view(ui)
  const width = (s: string) => [...s].length
  // a line Text has wrap truncate-end and fits; the other Texts are the indent, spinner and gap pieces of a toggle line
  for (const t of v.textNodes) {
    if (t.props.wrap) expect(t.props.wrap).toBe('truncate-end')
    expect(width(t.text) <= (t.props.wrap ? 24 : 2)).toBe(true)
  }
  // a toggle line: indent, spinner and gap (3), "x: " of its hotkey (3), then the label
  for (const b of v.buttons) {
    const indent = b.key.startsWith('toggle-clerk-') ? 2 : 0
    expect(indent + 3 + (b.hotkey ? 3 : 0) + width(b.label) <= 24).toBe(true)
  }
  expect(v.texts).toContain('P1 merge oter/bruh#29 a…')
  expect(v.buttons[0].label).toBe('▾ oter/bruh · 2 t…')
  expect(v.texts.find(t => t.startsWith('    last:'))).toBe('    last: pushed commit…')
})

test('a clanker with no session still counts the tasks of its rows and its live clerks', async ($, on) => {
  const w = world()
  w.sessions.splice(0, 2) // no clanker-bruh session
  w.sessions.find(s => s.name === 'clerk-shop-x')!.cwd = '/w/shop/.claude/worktrees/fix-x'
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  expect(await labels(ui)).toEqual(['▸ oter/bruh · 2 tasks', '▸ shop · 1 task'])
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

const OPTIONS = [{ label: 'merge now', description: 'squash' }, { label: 'wait', description: 'after the review' }]

test('a question with options shows one button per option, a refusal P0 shows ok and hold', async ($, on) => {
  const w = world()
  w.files[`${D}/questions/Q-bruh-m-98.json`] = line({ id: 'Q-bruh-m-98', priority: 'P1', subject: 'merge oter/bruh#29?', asker: 'clanker-bruh', opened_at: '2026-10-05T16:20:00Z', options: OPTIONS })
  w.files[`${D}/questions/Q-bruh-m-93.json`] = line({ id: 'Q-bruh-m-93', priority: 'P0', subject: 'refused', body: 'why\n\nCOMMAND: x\nCATEGORY: classifier y', asker: 'clerk-bruh-liveui', opened_at: '2026-10-05T16:21:00Z' })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const answers = (await view(ui)).buttons.filter(b => b.key.startsWith('answer-'))
  expect(answers).toEqual([
    { key: 'answer-Q-bruh-m-93-0', hotkey: undefined, label: 'ok' },
    { key: 'answer-Q-bruh-m-93-1', hotkey: undefined, label: 'hold' },
    { key: 'answer-Q-bruh-m-98-0', hotkey: undefined, label: 'merge now' },
    { key: 'answer-Q-bruh-m-98-1', hotkey: undefined, label: 'wait' },
  ])
})

test('a press submits the exact text', async ($, on) => {
  const w = world()
  w.files[`${D}/questions/Q-bruh-m-98.json`] = line({ id: 'Q-bruh-m-98', priority: 'P1', subject: 'merge oter/bruh#29?', asker: 'clanker-bruh', opened_at: '2026-10-05T16:20:00Z', options: OPTIONS })
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  await ui.press({ key: 'answer-Q-bruh-m-98-0' })
  expect(w.calls.sent).toEqual(['Q-bruh-m-98: merge now'])
  expect(w.calls.writes).toEqual([])
})
