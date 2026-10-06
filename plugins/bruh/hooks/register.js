// The bruh board: /bruh-board opens a pane with the open P0 and P1 questions
// and a collapsible tree of the role sessions by project: one line per
// clanker, its clerks under it, and a clerk's last done and next step under
// the clerk. It reads the bruh data folder, the ledger "In progress" rows and
// `claude agents --json --all`. Its one write is the expanded state: one
// `open:<role key>` key per item in its own $.store, shared by the sessions.
// A press on an answer button submits "Q-<id>: <label>" as a prompt into this
// session (bigm), which records it as the owner answer.
// The data lives in module variables; a hot reload loses them and the next
// refresh fills them again.

const PANE = 'bruh-board'
const FRAME_MS = 500
const REFRESH_TICKS = 20 // 20 x 500 ms = 10 s

// Spinner: the role picks the glyphs, the state picks the motion and the colour.
const FRAMES = { clanker: '⣾⣽⣻⢿⡿⣟⣯⣷', clerk: '◐◓◑◒' }
const COLORS = { working: 'green', owner: 'yellow', blocked: 'red', done: 'gray' }
const WORDS = { working: 'working', owner: 'waits on you', blocked: 'blocked', idle: 'idle', done: 'done' }
const URGENCY = ['owner', 'blocked', 'working', 'idle', 'done'] // a clanker's summary: its most urgent state
const LETTERS = 'abcdefghijklmnopqrstuvwxyz'

export function spinner(role, state, tick) {
  if (state === 'done') return { glyph: '✓', color: 'gray' }
  const frames = [...FRAMES[role]]
  const step = state === 'working' ? tick : state === 'owner' ? Math.floor(tick / 4) : 0
  const mark = state === 'owner' ? '?' : state === 'blocked' ? '!' : ''
  return { glyph: frames[step % frames.length] + mark, color: COLORS[state], dim: state === 'idle' }
}

// Role keys as plugins/bruh/mcp/env.go ParseRoleKey reads them; the board shows clankers and clerks.
const PROJECT = /^[a-z0-9]+(-[a-z0-9]+)*$/
export function parseKey(key) {
  if (key.startsWith('clanker-')) {
    const project = key.slice('clanker-'.length)
    return PROJECT.test(project) ? { role: 'clanker', project } : null
  }
  if (key.startsWith('clerk-')) {
    const rest = key.slice('clerk-'.length)
    const cut = rest.lastIndexOf('-')
    const project = rest.slice(0, cut)
    const short = rest.slice(cut + 1)
    return cut > 0 && PROJECT.test(project) && /^[a-z0-9]+$/.test(short) ? { role: 'clerk', project, short } : null
  }
  return null
}

// The table rows after "## In progress" up to the next "## " heading,
// mapped by header name.
export function parseInProgress(md) {
  const rows = []
  let isIn = false
  let head
  for (const raw of md.split('\n')) {
    const line = raw.trim()
    if (line.startsWith('## ')) { isIn = line === '## In progress'; head = undefined; continue }
    if (!isIn || !line.startsWith('|')) continue
    const cells = line.replace(/^\||\|$/g, '').split('|').map(cell => cell.trim())
    if (!head) { head = cells; continue }
    if (cells.every(cell => /^:?-+:?$/.test(cell))) continue
    const at = name => cells[head.indexOf(name)] ?? ''
    rows.push({
      owner: at('Owner'),
      number: at('Task').match(/^task (\d+):/)?.[1],
      clerk: at('State').match(/\((clerk-[a-z0-9-]+)\)/)?.[1],
      deliverable: at('Expected deliverable'),
    })
  }
  return rows
}

export function taskLabel(number, slug) {
  return [number && `task ${number}`, slug].filter(Boolean).join(' ') || undefined
}

// R-5 and the trim: no commit SHA, timestamp, run ID or question ID reaches the pane.
const MASK = /\b\d{4}-\d\d-\d\dT[\d:.]*Z?|\b\d\d:\d\d(?::\d\d)?Z\b|\bwf_[0-9a-f-]+|\bQ-[a-z0-9-]+|\b[0-9a-f]{7,40}\b/g
const mask = text => String(text ?? '').replace(MASK, '…')
// One line per item: cut to the pane width, as a Button label does not wrap.
const fit = (text, cols) => {
  const chars = [...text]
  return chars.length <= cols ? text : `${chars.slice(0, Math.max(cols - 1, 0)).join('')}…`
}
// The answer buttons of a question: its option labels, else ok and hold for a
// refusal P0 (question.go holdRecord.lines writes its "CATEGORY: " line).
function answers(q) {
  if (q.options?.length) return q.options.map(o => o.label)
  return q.priority === 'P0' && /^CATEGORY: /m.test(q.body ?? '') ? ['ok', 'hold'] : []
}
const SLUG = /\/\.claude\/worktrees\/([^/]+)\/?$/

function boardState(session, askers) {
  if (!session) return 'idle'
  if (askers.has(session.name) || session.waitingFor) return 'owner'
  if (session.state === 'blocked' || session.state === 'failed') return 'blocked'
  if (session.state === 'working' || session.status === 'busy') return 'working'
  if ((session.state === 'done' || session.state === 'stopped') && !session.pid) return 'done'
  return 'idle'
}

let board = { questions: [], clankers: [], notes: ['loading…'] }
let tick = 0
let timer = null
let isRefreshing = false
const skipped = new Set() // question files that are answered or below P1 stay so
const expanded = new Map() // role key -> true while the owner has it open; $.store keeps it

async function readText($, path) {
  try { return await $.fs.read(path) } catch { return undefined }
}
async function readJson($, path) {
  try { return JSON.parse(await readText($, path)) } catch { return undefined }
}
async function exists($, path) {
  try { return await $.fs.exists(path) } catch { return false }
}
async function readOpen($, key) {
  try { expanded.set(key, (await $.store.get(`open:${key}`)) === true) } catch {}
}
async function toggle($, key) {
  const isOpen = !expanded.get(key)
  expanded.set(key, isOpen)
  $.ui.invalidate('ui.render')
  await $.store.set(`open:${key}`, isOpen)
}

async function openQuestions($, data) {
  let names = []
  try { names = (await $.fs.list(`${data}/questions`)).map(entry => entry.name) } catch {}
  const open = []
  for (const name of names.filter(one => /^Q-.*\.json$/.test(one) && !skipped.has(one))) {
    const q = await readJson($, `${data}/questions/${name}`)
    if (!q) continue
    const isAnswered = (await exists($, `${data}/answers/bigm/${q.id}.answer`))
      || (await exists($, `${data}/answers/${q.asker}/${q.id}.answer`))
    if (isAnswered || (q.priority !== 'P0' && q.priority !== 'P1')) { skipped.add(name); continue }
    open.push(q)
  }
  return open.sort((a, b) => a.priority.localeCompare(b.priority) || String(a.opened_at).localeCompare(String(b.opened_at)))
}

// The last status or result line of a role's report, else its last line.
async function lastDone($, data, key) {
  const text = (await readText($, `${data}/reports/${key}.jsonl`)) ?? ''
  const lines = text.split('\n').flatMap(line => { try { return [JSON.parse(line)] } catch { return [] } })
  return (lines.findLast(line => line.kind === 'status' || line.kind === 'result') ?? lines.at(-1))?.text
}

async function refresh($) {
  if (isRefreshing) return
  isRefreshing = true
  try {
    const data = (await $.env.get('BRUH_DATA')) || `${await $.env.get('HOME')}/.claude/plugins/data/bruh-oter`
    const notes = []
    let sessions = []
    try {
      const ran = await $.process.run(['claude', 'agents', '--json', '--all'], { timeoutMs: 20000 })
      if (ran.exitCode === 0) sessions = JSON.parse(ran.stdout)
      else notes.push('sessions: claude agents failed')
    } catch {
      notes.push('sessions: claude agents failed')
    }
    const newest = new Map()
    for (const session of Array.isArray(sessions) ? sessions : []) {
      if (typeof session?.name !== 'string' || !parseKey(session.name)) continue
      const old = newest.get(session.name)
      if (!old || session.startedAt > old.startedAt) newest.set(session.name, session)
    }
    const order = (a, b) => Boolean(b.pid) - Boolean(a.pid) || b.startedAt - a.startedAt
    const live = [...newest.values()].sort(order)

    const questions = await openQuestions($, data)
    const askers = new Set(questions.map(q => q.asker))
    const repos = (await readJson($, `${data}/repos.json`))?.repos ?? []
    const ledger = (await readJson($, `${data}/init/config.json`))?.ledger_path
    if (!ledger) notes.push('ledger path unknown')

    const projects = [...new Set(live.map(s => parseKey(s.name).project))]
    projects.sort((a, b) => {
      const isLive = p => live.some(s => s.pid && parseKey(s.name).project === p)
      return isLive(b) - isLive(a) || a.localeCompare(b)
    })
    const clankers = []
    for (const project of projects) {
      const path = repos.filter(r => r.project === project).map(r => r.repo).join(', ') || project
      const md = ledger ? await readText($, `${ledger}/projects/${project}.md`) : undefined
      const rows = md ? parseInProgress(md) : []
      const key = `clanker-${project}`
      const clerks = []
      for (const session of live.filter(s => parseKey(s.name).role === 'clerk' && parseKey(s.name).project === project)) {
        const ledgerRow = rows.find(r => r.clerk === session.name)
        const number = ledgerRow?.number
        const slug = session.cwd?.match(SLUG)?.[1]
        await readOpen($, session.name)
        clerks.push({
          key: session.name, number, slug, state: boardState(session, askers),
          name: taskLabel(number, slug) ?? parseKey(session.name).short,
          last: await lastDone($, data, session.name), next: ledgerRow?.deliverable,
        })
      }
      // The clanker's tasks: its ledger rows plus its live clerks (number, else slug), with or without a clanker session.
      const tasks = new Set([...rows.filter(r => r.owner === key).map(r => r.number), ...clerks.map(c => c.number ?? c.slug)].filter(Boolean)).size
      const states = [boardState(newest.get(key), askers), ...clerks.map(c => c.state)]
      await readOpen($, key)
      clankers.push({ key, path, tasks, state: URGENCY.find(s => states.includes(s)), clerks })
    }
    board = { questions, clankers, notes }
  } finally {
    isRefreshing = false
  }
  $.ui.invalidate('ui.render')
}

/** @type {import('claude-code').Register} */
export const register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'bruh-board', description: 'Open the bruh board: open questions and the role tree' })
    return next(e)
  })

  on('command.run', { command: 'bruh-board' }, async $ => {
    await $.ui.open({ id: PANE, title: 'bruh board', focus: true })
    if (!timer) {
      await refresh($)
      timer = $.clock.every(FRAME_MS, () => {
        tick += 1
        $.ui.invalidate('ui.render')
        if (tick % REFRESH_TICKS === 0) void refresh($)
      })
    }
    return { text: 'bruh board opened.' }
  })

  on('ui.close', { id: PANE }, ($, e, next) => {
    timer?.cancel()
    timer = null
    return next(e)
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, ($, e) => {
    const { Box, Text, Button } = $.ui.resolve(e)
    const cols = e.props.bodyColumns
    const line = (props, text) => h(Text, { wrap: 'truncate-end', ...props }, fit(text, cols))
    // One toggle line: indent, spinner, then a plain Button "<hotkey>: ▸ <label>".
    // ponytail: hotkeys run out after 9 clankers and 26 clerks; Tab and Enter still reach the rest.
    const toggleLine = (item, role, indent, hotkey, label) => {
      const spin = spinner(role, item.state, tick)
      return h(Box, { key: item.key, flexDirection: 'row' },
        indent ? h(Text, {}, indent) : undefined,
        h(Box, { key: `spin-${item.key}` },
          h(Text, { ...(spin.color && { color: spin.color }), ...(spin.dim && { dimColor: true }) }, spin.glyph)),
        h(Text, {}, ' '),
        h(Button, {
          key: `toggle-${item.key}`, plain: true, ...(hotkey && { hotkey }),
          label: fit(`${expanded.get(item.key) ? '▾' : '▸'} ${label}`, cols - indent.length - 3 - (hotkey ? 3 : 0)),
          onPress: () => toggle($, item.key),
        }))
    }

    const out = [line({ bold: true }, 'Waits on you')]
    if (board.questions.length === 0) out.push(line({ dimColor: true }, 'nothing waits on you'))
    for (const q of board.questions) {
      // The ID number shows only to tell two questions with one subject apart.
      const isTwin = board.questions.some(other => other !== q && other.subject === q.subject)
      const number = isTwin ? ` (${String(q.id).split('-').at(-1)})` : ''
      out.push(line({ key: `q-${q.id}`, color: q.priority === 'P0' ? 'red' : 'yellow' }, `${q.priority}${number} ${mask(q.subject)}`))
      const labels = answers(q)
      if (!labels.length) continue
      // A Button draws "[ label ]" (label + 4), then a 1-character gap.
      const width = Math.max(Math.floor((cols - 2) / labels.length) - 5, 1)
      out.push(h(Box, { key: `answers-${q.id}`, flexDirection: 'row' }, h(Text, {}, '  '),
        ...labels.flatMap((label, i) => [
          i ? h(Text, {}, ' ') : undefined,
          h(Button, { key: `answer-${q.id}-${i}`, label: fit(mask(label), width), onPress: () => $.prompt.submit({ text: `${q.id}: ${label}` }) }),
        ])))
    }
    for (const note of board.notes) out.push(line({ dimColor: true }, note))
    if (board.clankers.length) out.push(line({}, ' '))
    let letter = 0
    for (const [i, clanker] of board.clankers.entries()) {
      const count = clanker.tasks === 0 ? 'no tasks' : clanker.tasks === 1 ? '1 task' : `${clanker.tasks} tasks`
      out.push(toggleLine(clanker, 'clanker', '', i < 9 ? String(i + 1) : undefined, `${clanker.path} · ${count}`))
      if (!expanded.get(clanker.key)) continue
      for (const clerk of clanker.clerks) {
        out.push(toggleLine(clerk, 'clerk', '  ', LETTERS[letter++], `${clerk.name} · ${WORDS[clerk.state]}`))
        if (!expanded.get(clerk.key)) continue
        if (clerk.last) out.push(line({ dimColor: true }, `    last: ${mask(clerk.last)}`))
        if (clerk.next) out.push(line({ dimColor: true }, `    next: ${mask(clerk.next)}`))
      }
    }
    return h(Box, { flexDirection: 'column' }, ...out)
  })
}
