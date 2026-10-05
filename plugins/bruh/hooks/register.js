// The bruh board: /bruh-board opens a pane with the open P0 and P1 questions
// and a tree of the role sessions by project. Read-only: it reads the bruh
// data folder, the ledger "In progress" rows and `claude agents --json --all`.
// It writes nothing. The data lives in module variables; a hot reload loses
// them and the next refresh fills them again.

const PANE = 'bruh-board'
const FRAME_MS = 500
const REFRESH_TICKS = 20 // 20 x 500 ms = 10 s

// Spinner: the role picks the glyphs, the state picks the motion and the colour.
const FRAMES = { clanker: '⣾⣽⣻⢿⡿⣟⣯⣷', clerk: '◐◓◑◒', run: '▁▃▅▇▅▃', bigm: '◇◆' }
const COLORS = { working: 'green', owner: 'yellow', blocked: 'red', done: 'gray' }
const WORDS = { working: 'working', owner: 'waits on you', blocked: 'blocked', idle: 'idle', done: 'done' }

export function spinner(role, state, tick) {
  if (state === 'done') return { glyph: '✓', color: 'gray' }
  const frames = [...(FRAMES[role] ?? FRAMES.bigm)]
  const step = state === 'working' ? tick : state === 'owner' ? Math.floor(tick / 4) : 0
  const mark = state === 'owner' ? '?' : state === 'blocked' ? '!' : ''
  return { glyph: frames[step % frames.length] + mark, color: COLORS[state], dim: state === 'idle' }
}

// Role keys as plugins/bruh/mcp/env.go ParseRoleKey reads them.
const PROJECT = /^[a-z0-9]+(-[a-z0-9]+)*$/
export function parseKey(key) {
  if (key === 'bigm') return { role: 'bigm' }
  if (key === 'clerk-ledger') return { role: 'ledger' }
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
      check: at('Next check (UTC)'),
    })
  }
  return rows
}

export function taskLabel(number, slug) {
  return [number && `task ${number}`, slug].filter(Boolean).join(' ') || undefined
}

// R-5: no commit SHA reaches the pane. A wf_ ID keeps its hex part ("_" is a word character).
const mask = text => String(text ?? '').replace(/\b[0-9a-f]{7,40}\b/g, '…')
const hhmm = at => (/^\d{4}-\d\d-\d\dT\d\d:\d\d/.test(at ?? '') ? `${at.slice(11, 16)}Z` : mask(at))
const RUN_ID = /\bwf_[0-9a-f]+-[0-9a-f]+\b/
const SLUG = /\/\.claude\/worktrees\/([^/]+)\/?$/

function boardState(session, askers) {
  if (!session) return 'idle'
  if (askers.has(session.name) || session.waitingFor) return 'owner'
  if (session.state === 'blocked' || session.state === 'failed') return 'blocked'
  if (session.state === 'working' || session.status === 'busy') return 'working'
  if ((session.state === 'done' || session.state === 'stopped') && !session.pid) return 'done'
  return 'idle'
}

let board = { questions: [], groups: [], notes: ['loading…'] }
let tick = 0
let timer = null
let isRefreshing = false
const skipped = new Set() // question files that are answered or below P1 stay so

async function readText($, path) {
  try { return await $.fs.read(path) } catch { return undefined }
}
async function readJson($, path) {
  try { return JSON.parse(await readText($, path)) } catch { return undefined }
}
async function exists($, path) {
  try { return await $.fs.exists(path) } catch { return false }
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

async function report($, data, key) {
  const text = (await readText($, `${data}/reports/${key}.jsonl`)) ?? ''
  const lines = text.split('\n').flatMap(line => { try { return [JSON.parse(line)] } catch { return [] } })
  const last = lines.findLast(line => line.kind === 'status' || line.kind === 'result') ?? lines.at(-1)
  const runLine = lines.findLast(line => RUN_ID.test(line.text ?? ''))
  return {
    last: last && `last: ${hhmm(last.at)} ${mask(last.text)}`,
    run: runLine && { id: runLine.text.match(RUN_ID)[0], isDone: runLine.kind === 'result' },
  }
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
      else notes.push(`sessions: claude agents failed (exit ${ran.exitCode})`)
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
    if (!ledger) notes.push(`next steps: ledger path unknown (${data}/init/config.json)`)

    const row = async (key, session, role, name, depth, isLastChild, nextRow) => {
      const state = boardState(session, askers)
      const { last, run } = await report($, data, key)
      const next = nextRow && `next: check ${hhmm(nextRow.check)} · ${mask(nextRow.deliverable)}`
      return { key, role, state, name, depth, isLastChild, last, next, run }
    }

    const groups = []
    const top = []
    for (const key of ['bigm', 'clerk-ledger']) {
      const session = newest.get(key)
      if (session) top.push(await row(key, session, 'bigm', key === 'bigm' ? 'bigm' : 'clerk-ledger (ledger clerk)', 0))
    }
    if (top.length) groups.push(top)

    const projects = [...new Set(live.map(s => parseKey(s.name).project).filter(Boolean))]
    projects.sort((a, b) => {
      const isLive = p => live.some(s => s.pid && parseKey(s.name).project === p)
      return isLive(b) - isLive(a) || a.localeCompare(b)
    })
    for (const project of projects) {
      const path = repos.filter(r => r.project === project).map(r => r.repo).join(', ') || project
      const md = ledger ? await readText($, `${ledger}/projects/${project}.md`) : undefined
      const rows = md ? parseInProgress(md) : []
      const clankerKey = `clanker-${project}`
      const owned = rows.filter(r => r.owner === clankerKey)
      const numbers = owned.map(r => r.number).filter(Boolean)
      const firstCheck = owned.filter(r => r.check).sort((a, b) => a.check.localeCompare(b.check))[0]
      const clanker = newest.get(clankerKey)
      const label = !clanker ? 'no session' : numbers.length ? `task${numbers.length > 1 ? 's' : ''} ${numbers.join(', ')}` : 'idle'
      const group = [await row(clankerKey, clanker, 'clanker', `${path} (clanker, ${label})`, 0, false, firstCheck)]
      const clerks = live.filter(s => parseKey(s.name).role === 'clerk' && parseKey(s.name).project === project)
      for (const [i, clerk] of clerks.entries()) {
        const ledgerRow = rows.find(r => r.clerk === clerk.name)
        const task = taskLabel(ledgerRow?.number, clerk.cwd?.match(SLUG)?.[1]) ?? clerk.name
        const short = parseKey(clerk.name).short
        const clerkRow = await row(clerk.name, clerk, 'clerk', `${path} ${short} (clerk, ${task})`, 1, i === clerks.length - 1, ledgerRow)
        group.push(clerkRow)
        if (clerkRow.run) {
          group.push({
            key: `${clerk.name}:run`, role: 'run', depth: 2, isLastChild: true, parentIsLast: clerkRow.isLastChild,
            state: clerkRow.run.isDone ? 'done' : clerkRow.state,
            name: `${path} ${short} run ${clerkRow.run.id} (workflow run, ${task})`,
          })
        }
      }
      groups.push(group)
    }
    board = { questions, groups, notes }
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
    await $.ui.open({ id: PANE, title: 'bruh board' })
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
    const { Box, Text } = $.ui.resolve(e)
    const line = (props, text) => h(Text, { wrap: 'truncate-end', ...props }, text)
    const out = [line({ bold: true }, 'Waits on you')]
    if (board.questions.length === 0) out.push(line({ dimColor: true }, 'nothing waits on you'))
    for (const q of board.questions) {
      const id = String(q.id).replace(/^Q-.*-(\d+)$/, 'Q-…-$1')
      out.push(line({ key: `q-${q.id}`, color: q.priority === 'P0' ? 'red' : 'yellow' },
        `${q.priority} ${id} ${mask(q.subject)} · ${q.asker} · ${hhmm(q.opened_at)}`))
    }
    for (const note of board.notes) out.push(line({ dimColor: true }, note))
    for (const group of board.groups) {
      out.push(line({}, ' '))
      for (const r of group) {
        const branch = r.depth === 0 ? '' : r.depth === 1 ? (r.isLastChild ? '└─ ' : '├─ ') : `${r.parentIsLast ? '   ' : '│  '}└─ `
        const under = r.depth === 0 ? '  ' : r.depth === 1 ? (r.isLastChild ? '     ' : '│    ') : `${r.parentIsLast ? '   ' : '│  '}     `
        const spin = spinner(r.role, r.state, tick)
        out.push(h(Box, { key: r.key, flexDirection: 'column' },
          h(Box, { flexDirection: 'row' },
            h(Text, {}, branch),
            h(Box, { key: `spin-${r.key}` },
              h(Text, { ...(spin.color && { color: spin.color }), ...(spin.dim && { dimColor: true }) }, spin.glyph)),
            line({}, ` ${r.name} · ${WORDS[r.state]}`)),
          r.last && line({ dimColor: true }, under + r.last),
          r.next && line({ dimColor: true }, under + r.next)))
      }
    }
    return h(Box, { flexDirection: 'column' }, ...out)
  })
}
