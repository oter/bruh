import { expect, mock, test } from 'claude-code/testing'
import type { On } from 'claude-code'

// A small fake world beneath the plugin: the bruh data folder under HOME=/h,
// a ledger at /l, and the output of `claude agents --json --all`.
const D = '/h/.claude/plugins/data/bruh-oter'
const LEDGER = `| Owner | Task | Expected deliverable | State | Next check (UTC) | Link | Source read |
|---|---|---|---|---|---|---|
| clanker-bruh | task 12: poller waits on the lock | PR on oter/bruh | working (clerk-bruh-pollerwait) | 2026-10-05T17:22:43Z |  | x |
| clanker-bruh | task 13: less ceremony | PR on oter/bruh | sent | 2026-10-05T18:52:38Z |  | x |`
const line = (o: object) => JSON.stringify(o)

function world() {
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
    [`${D}/reports/clerk-bruh-pollerwait.jsonl`]: [
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
  ]
  const calls = { run: 0, opened: [] as string[], registered: [] as string[] }
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
    on('ui.open', ($, e) => {
      calls.opened.push(e.id)
      return { value: { isPlaced: true as const } }
    })
    on('command.register', ($, e) => {
      calls.registered.push(e.name)
      return { value: { command: e.name } }
    })
    return mock.clock(on, { now: Date.parse('2026-10-05T16:30:00Z') })
  }
  return { files, sessions, calls, stub }
}

const PANE_PROPS = {
  title: 'bruh board', isFocused: false, bodyColumns: 200, placement: 'dock' as const,
  scroll: { offset: 0, bodyRows: 60 }, view: {},
}
const mount = ($: any) => $.ui.mount({ plugin: 'bruh', surface: 'terminal', component: 'Pane', requestId: 'bruh-board', props: PANE_PROPS })
const texts = async (ui: any) => (await ui.findAll({ type: 'Text' })).map((t: any) => t.text)

test('/bruh-board opens the pane and nothing opens it unasked', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.session.start({ source: 'startup' } as any).catch(() => undefined)
  expect(w.calls.registered).toEqual(['bruh-board'])
  expect(w.calls.opened).toEqual([])
  const ran = await $.command.run({ command: 'bruh-board' })
  expect(w.calls.opened).toEqual(['bruh-board'])
  expect(ran.text).toBe('bruh board opened.')
})

test('the pane draws the open question, a role row, its last done and its next step', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const all = (await texts(ui)).join('\n')
  expect(all).toContain('P1 Q-…-98 merge oter/bruh#29? · clanker-bruh · 16:20Z')
  expect(all).not.toContain('answered by bigm')
  expect(all).not.toContain('answered by the asker')
  expect(all).not.toContain('a P2 question')
  expect(all).not.toContain('infra-4a')
  expect(await ui.find({ key: 'clerk-bruh-pollerwait' })).toBeDefined()
  expect(all).toContain('oter/bruh pollerwait (clerk, task 12 fix-poller-wait-lock) · working')
  // last done prefers the last status line over a later event line; a SHA is masked (R-5)
  expect(all).toContain('last: 16:12Z pushed commit … to the branch')
  expect(all).not.toContain('3f9a2b1c4d')
  expect(all).toContain('next: check 17:22Z · PR on oter/bruh')
})

test('a refresh after 10 seconds shows new data', async ($, on) => {
  const w = world()
  const clock = w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  expect((await texts(ui)).join('\n')).not.toContain('phase two')
  w.files[`${D}/reports/clerk-bruh-pollerwait.jsonl`] += line({ at: '2026-10-05T16:31:00Z', kind: 'status', text: 'phase two' }) + '\n'
  await clock.advance(9500)
  expect((await texts(ui)).join('\n')).not.toContain('phase two')
  await clock.advance(500)
  expect(w.calls.run).toBe(2)
  expect((await texts(ui)).join('\n')).toContain('last: 16:31Z phase two')
})

test('the tree puts each clerk under the clanker of its project', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const keys = (await ui.findAll({ type: 'Box' })).map((b: any) => b.key).filter((k: any) => k && !k.startsWith('spin-'))
  expect(keys).toEqual([
    'clanker-bruh', 'clerk-bruh-pollerwait', 'clerk-bruh-pollerwait:run', 'clerk-bruh-liveui',
    'clanker-shop', 'clerk-shop-x',
  ])
})

test('the spinner differs by role and by state, and moves only while working', async ($, on) => {
  const w = world()
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
  const run = await spin('clerk-bruh-pollerwait:run')
  const blocked = await spin('clerk-bruh-liveui')
  const idle = await spin('clerk-shop-x')
  const done = await spin('clanker-shop')
  expect(owner?.text).toMatch(/^[⣾⣽⣻⢿⡿⣟⣯⣷]\?$/)
  expect(owner?.props.color).toBe('yellow')
  expect(working?.text).toMatch(/^[◐◓◑◒]$/)
  expect(working?.props.color).toBe('green')
  expect(run?.text).toMatch(/^[▁▃▅▇]$/)
  expect(blocked?.text).toMatch(/^[◐◓◑◒]!$/)
  expect(blocked?.props.color).toBe('red')
  expect(idle?.props.dimColor).toBe(true)
  expect(done?.text).toBe('✓')
  expect(done?.props.color).toBe('gray')
  await clock.advance(500)
  expect((await spin('clerk-bruh-pollerwait'))?.text).not.toBe(working?.text)
  expect((await spin('clerk-bruh-liveui'))?.text).toBe(blocked?.text)
})

test('row names: project path or key, role, task number and slug or role key', async ($, on) => {
  const w = world()
  w.stub(on)
  await $.command.run({ command: 'bruh-board' })
  const ui = await mount($)
  const all = (await texts(ui)).join('\n')
  expect(all).toContain('oter/bruh (clanker, tasks 12, 13) · waits on you')
  expect(all).toContain('shop (clanker, idle) · done')
  expect(all).toContain('oter/bruh pollerwait (clerk, task 12 fix-poller-wait-lock)')
  expect(all).toContain('oter/bruh liveui (clerk, clerk-bruh-liveui) · blocked')
  expect(all).toContain('shop x (clerk, clerk-shop-x) · idle')
  expect(all).toContain('oter/bruh pollerwait run wf_712df188-d86 (workflow run, task 12 fix-poller-wait-lock)')
})
