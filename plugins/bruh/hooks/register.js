// The bruh board: /bruh-board opens a pane with the role sessions by project
// (clankers, their clerks, a clerk's last done and next step), a collapsed
// Done group of the done and stopped ones, and at the bottom the open P0 and
// P1 questions with their answer buttons: what waits on the owner (R-16).
// It reads the bruh data folder, the ledger "In progress" rows and
// `claude agents --json --all`. Its one write is the expanded state: one
// `open:<role key>` key per item (`open:done` for the Done group) in its own
// $.store, shared by the sessions.
// Three designs draw the same data: cards (bordered cards), buckets (sections
// by state) and pipeline (a strip across the deliver phases). The plugin
// option board_design picks one; a /config change draws at once (config.set)
// and then reloads the module with the new options.
// A press on an answer button submits "Q-<id>: <label>" as a prompt into this
// session (bigm), which records it as the owner answer.
// The data lives in module variables; a hot reload loses them and the drop of
// its timer, so the next draw starts the refresh again and it fills them.

const PANE = 'bruh-board'
const FRAME_MS = 500
const REFRESH_TICKS = 20 // 20 x 500 ms = 10 s

// Spinner: the role picks the glyphs, the state picks the motion and the colour.
const FRAMES = { clanker: '⣾⣽⣻⢿⡿⣟⣯⣷', clerk: '◐◓◑◒' }
const COLORS = { working: 'green', owner: 'yellow', blocked: 'red', done: 'gray', stopped: 'gray' }
const WORDS = { working: 'working', owner: 'waits on you', blocked: 'blocked', idle: 'idle', done: 'done', stopped: 'stopped' }
const URGENCY = ['owner', 'blocked', 'working', 'idle', 'done', 'stopped'] // a clanker's summary: its most urgent state
const ENDED = ['done', 'stopped'] // these go to the Done group

// Buckets: one bar per state, softer colours than the plain terminal ones (owner, 2026-10-06).
const BARS = { owner: 'WAITS ON YOU', blocked: 'BLOCKED', working: 'WORKING', idle: 'IDLE' }
export const SOFT = { owner: '#5c4b26', blocked: '#5c2f2f', working: '#2b4a35', idle: '#3a3f47', done: '#3a3f47' }
const BAR_TEXT = '#e6edf3'
// Pipeline: the deliver phases a clerk reports in the top-level "phase" field of a report line.
export const PHASES = ['plan', 'implement', 'review', 'fix', 'merge', 'done']
const STRIP = PHASES.slice(0, 5)
const CELL = 6 // "━━━━━ ": one phase of the strip
const NAME = 26 // the clerk's Button column of a strip row

export function spinner(role, state, tick) {
  if (state === 'done') return { glyph: '✓', color: 'gray' }
  if (state === 'stopped') return { glyph: '■', color: 'gray' }
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

// The phase of the latest report line whose top-level "phase" is one of PHASES:
// an exact-value check of one closed field, never the text of a line.
export function readPhase(lines) {
  return lines.findLast(line => PHASES.includes(line?.phase))?.phase
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
  if (ENDED.includes(session.state) && !session.pid) return session.state
  return 'idle'
}

let board = { questions: [], clankers: [], done: [], notes: ['loading…'] }
let design = 'cards'
let tick = 0
let timer = null
let isRefreshing = false
const skipped = new Set() // question files that are answered or below P1 stay so
const expanded = new Map() // role key (or "done") -> true while the owner has it open; $.store keeps it

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
  // A question is answered when any role folder has its answer file: bigm, the asker, or the
  // clanker of a delegated answer. answer_write also writes it for each question that it replaces.
  let roles = []
  try { roles = (await $.fs.list(`${data}/answers`)).map(entry => entry.name) } catch {}
  const answered = async id => {
    for (const role of roles) if (await exists($, `${data}/answers/${role}/${id}.answer`)) return true
    return false
  }
  const open = []
  for (const name of names.filter(one => /^Q-.*\.json$/.test(one) && !skipped.has(one))) {
    const q = await readJson($, `${data}/questions/${name}`)
    if (!q) continue
    const isAnswered = await answered(q.id)
    if (isAnswered || (q.priority !== 'P0' && q.priority !== 'P1')) { skipped.add(name); continue }
    open.push(q)
  }
  return open.sort((a, b) => a.priority.localeCompare(b.priority) || String(a.opened_at).localeCompare(String(b.opened_at)))
}

// A role's report (plugins/bruh/mcp/report.go: <data>/reports/<role key>.jsonl,
// one JSON object per line): the text of its last status or result line, else
// of its last line, and its phase.
async function readReport($, data, key) {
  const text = (await readText($, `${data}/reports/${key}.jsonl`)) ?? ''
  const lines = text.split('\n').flatMap(line => { try { return [JSON.parse(line)] } catch { return [] } })
  return {
    last: (lines.findLast(line => line?.kind === 'status' || line?.kind === 'result') ?? lines.at(-1))?.text,
    phase: readPhase(lines),
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
    const done = []
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
        const { short } = parseKey(session.name)
        await readOpen($, session.name)
        const report = await readReport($, data, session.name)
        clerks.push({
          key: session.name, number, slug, state: boardState(session, askers),
          name: taskLabel(number, slug) ?? short,
          // R-10: "<repo> <name> (clerk, task <n> <slug>)"
          title: `${path} ${short} (${['clerk', taskLabel(number, slug)].filter(Boolean).join(', ')})`,
          last: report.last, phase: report.phase, next: ledgerRow?.deliverable,
        })
      }
      // The clanker's tasks: its ledger rows plus its clerks (number, else slug), done ones too, with or without a clanker session.
      const tasks = new Set([...rows.filter(r => r.owner === key).map(r => r.number), ...clerks.map(c => c.number ?? c.slug)].filter(Boolean)).size
      const ended = clerks.filter(c => ENDED.includes(c.state))
      done.push(...ended.map(c => ({ key: c.key, role: 'clerk', state: c.state, label: c.name })))
      const active = clerks.filter(c => !ended.includes(c))
      const states = [boardState(newest.get(key), askers), ...active.map(c => c.state)]
      await readOpen($, key)
      // A clanker whose own session and clerks all ended is one line of the Done group.
      if (states.every(s => ENDED.includes(s))) done.push({ key, role: 'clanker', state: states[0], label: `${path} (clanker)` })
      else clankers.push({ key, path, tasks, state: URGENCY.find(s => states.includes(s)), clerks: active })
    }
    await readOpen($, 'done')
    board = { questions, clankers, done, notes }
  } finally {
    isRefreshing = false
  }
  $.ui.invalidate('ui.render')
}

// The 500 ms frame timer and the first refresh; the timer refreshes every 10 s.
function start($) {
  timer = $.clock.every(FRAME_MS, () => {
    tick += 1
    $.ui.invalidate('ui.render')
    if (tick % REFRESH_TICKS === 0) void refresh($)
  })
  return refresh($)
}

/** @type {import('claude-code').Register} */
export const register = (on, options) => {
  design = options?.board_design

  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'bruh-board', description: 'Open the bruh board: open questions and the role tree' })
    return next(e)
  })

  on('command.run', { command: 'bruh-board' }, async $ => {
    await $.ui.open({ id: PANE, title: 'bruh board', focus: true })
    if (!timer) await start($)
    return { text: 'bruh board opened.' }
  })

  on('ui.close', { id: PANE }, ($, e, next) => {
    timer?.cancel()
    timer = null
    return next(e)
  })

  // The person's /config pick of the design: the open pane draws it at once.
  on('config.set', { key: 'bruh.board_design' }, async ($, e, next) => {
    const result = await next(e)
    if (result.value !== undefined) {
      design = result.value
      $.ui.invalidate('ui.render')
    }
    return result
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, ($, e) => {
    if (!timer) void start($) // a reload dropped the timer while the pane stayed open
    const { Box, Text, Button } = $.ui.resolve(e)
    const cols = e.props.bodyColumns
    // One Text cut to the width; a keyed one sits in a keyed Box, as the test kit finds a Box by its key.
    const line = ({ key, ...props }, text, width = cols) => {
      const t = h(Text, { wrap: 'truncate-end', ...props }, fit(text, width))
      return key ? h(Box, { key }, t) : t
    }
    const count = c => (c.tasks === 0 ? 'no tasks' : c.tasks === 1 ? '1 task' : `${c.tasks} tasks`)
    // A plain Button "▸ <label>" that expands and collapses one item: focus and Enter, no hotkey.
    const opener = (key, label, width) => h(Button, {
      key: `toggle-${key}`, plain: true,
      label: fit(`${expanded.get(key) ? '▾' : '▸'} ${label}`, width),
      onPress: () => toggle($, key),
    })
    const spin = (item, role) => {
      const s = spinner(role, item.state, tick)
      return h(Box, { key: `spin-${item.key}` },
        h(Text, { ...(s.color && { color: s.color }), ...(s.dim && { dimColor: true }) }, s.glyph))
    }
    // One toggle line: indent, spinner and gap (3 columns with the indent), then the opener.
    const toggleLine = (item, role, indent, label, width) => h(Box, { key: item.key, flexDirection: 'row' },
      indent ? h(Text, {}, indent) : undefined,
      spin(item, role),
      h(Text, {}, ' '),
      opener(item.key, label, width - indent.length - 3))
    // A question: its line, then its answer buttons (task 15).
    const ask = (q, indent, width) => {
      // The ID number shows only to tell two questions with one subject apart.
      const isTwin = board.questions.some(other => other !== q && other.subject === q.subject)
      const number = isTwin ? ` (${String(q.id).split('-').at(-1)})` : ''
      const out = [line({ key: `q-${q.id}`, color: q.priority === 'P0' ? 'red' : 'yellow' }, `${indent}${q.priority}${number} ${mask(q.subject)}`, width)]
      const labels = answers(q)
      if (!labels.length) return out
      // A Button draws "[ label ]" (label + 4), then a 1-character gap.
      const each = Math.max(Math.floor((width - indent.length - 2) / labels.length) - 5, 1)
      out.push(h(Box, { key: `answers-${q.id}`, flexDirection: 'row' }, h(Text, {}, `${indent}  `),
        ...labels.flatMap((label, i) => [
          i ? h(Text, {}, ' ') : undefined,
          h(Button, { key: `answer-${q.id}-${i}`, label: fit(mask(label), each), onPress: () => $.prompt.submit({ text: `${q.id}: ${label}` }) }),
        ])))
      return out
    }
    const details = (clerk, indent, width) => [
      line({ dimColor: true }, `${indent}${mask(clerk.title)}`, width),
      clerk.last && line({ dimColor: true }, `${indent}last: ${mask(clerk.last)}`, width),
      clerk.next && line({ dimColor: true }, `${indent}next: ${mask(clerk.next)}`, width),
    ].filter(Boolean)
    const notes = () => board.notes.map(note => line({ dimColor: true }, note))
    // The Done group: one collapsed opener with the count; open, one gray line per item.
    const doneGroup = (width, item = d => line({ key: `done-${d.key}`, color: 'gray' }, `${spinner('clerk', d.state).glyph} ${d.label} · ${WORDS[d.state]}`, width)) => [
      opener('done', `Done (${board.done.length})`, width),
      ...(expanded.get('done') ? board.done.map(item) : []),
    ]
    const card = (key, style, color, children) => h(Box, { key: `card-${key}`, flexDirection: 'column', borderStyle: style, borderColor: color, paddingX: 1 }, ...children)

    // Design 8: every item is a card, a border (and its padding) costs 4 columns. The question cards come last (R-16).
    const cards = () => {
      const inner = cols - 4
      const out = notes()
      for (const clanker of board.clankers) {
        const body = [toggleLine(clanker, 'clanker', '', `${clanker.path} · ${count(clanker)}`, inner)]
        if (expanded.get(clanker.key)) {
          for (const clerk of clanker.clerks) {
            const lines = [toggleLine(clerk, 'clerk', '', `${clerk.name} · ${WORDS[clerk.state]}`, inner - 4)]
            if (expanded.get(clerk.key)) lines.push(...details(clerk, '', inner - 4))
            body.push(card(clerk.key, 'round', COLORS[clerk.state] ?? 'gray', lines))
          }
        }
        out.push(card(clanker.key, 'bold', 'gray', body))
      }
      out.push(card('done', 'dashed', 'gray', doneGroup(inner)))
      out.push(...board.questions.map(q => card(q.id, 'double', q.priority === 'P0' ? 'red' : 'yellow', ask(q, '', inner))))
      return out
    }

    // Design 5: per clanker a bar per state, most urgent first; then the Done bar, and last one bar with every open question (R-16).
    const buckets = () => {
      const bar = (key, state, title) => line({ key: `bar-${key}`, backgroundColor: SOFT[state], color: BAR_TEXT, bold: true }, ` ${title}`.padEnd(cols))
      const out = notes()
      for (const clanker of board.clankers) {
        out.push(line({ key: `crumb-${clanker.key}`, dimColor: true }, `${clanker.path} · ${count(clanker)}`))
        for (const state of Object.keys(BARS)) {
          const rows = clanker.clerks.filter(k => k.state === state)
          if (!rows.length) continue
          out.push(bar(`${clanker.key}-${state}`, state, `${BARS[state]}  ${rows.length}`))
          for (const clerk of rows) {
            out.push(toggleLine(clerk, 'clerk', ' ', clerk.name, cols))
            if (expanded.get(clerk.key)) out.push(...details(clerk, '    ', cols))
          }
        }
      }
      out.push(h(Box, { key: 'bar-done', backgroundColor: SOFT.done, flexDirection: 'column' }, ...doneGroup(cols)))
      if (board.questions.length) out.push(bar('questions', 'owner', `${BARS.owner}  ${board.questions.length}`), ...board.questions.flatMap(q => ask(q, ' ', cols)))
      return out
    }

    // Design 9: each clerk a strip across the deliver phases, filled up to its reported phase.
    const pipeline = () => {
      const cut = (cells, width) => {
        const out = []
        for (const cell of cells) {
          if (width <= 0) break
          const text = fit(cell.text, width)
          out.push({ ...cell, text })
          width -= [...text].length
        }
        return out
      }
      const green = () => STRIP.map(() => ({ text: '━━━━━ ', color: 'green' }))
      const strip = item => {
        if (item.state === 'stopped') return [{ text: '■ stopped', color: 'gray' }]
        if (item.state === 'done') return green()
        // An open question ("?") or a block ("!") outranks a reported done: its mark sits at merge.
        const marked = item.state === 'owner' || item.state === 'blocked'
        if (item.phase === 'done' && !marked) return green()
        const s = spinner('clerk', item.state, tick)
        const glyph = { ...(s.color && { color: s.color }), ...(s.dim && { dim: true }) }
        const at = item.phase === 'done' ? STRIP.length - 1 : STRIP.indexOf(item.phase)
        if (at < 0) return [{ ...glyph, text: `${s.glyph} ` }, { text: '····· phase not reported', dim: true }]
        return STRIP.map((phase, i) => (i < at ? { text: '━━━━━ ', color: 'green' }
          : i === at ? { ...glyph, text: `${s.glyph}${'━'.repeat(CELL - 1 - [...s.glyph].length)} ` }
          : { text: '····· ', dim: true }))
      }
      const name = Math.min(NAME, Math.max(cols - 2 - CELL * STRIP.length, 12))
      const row = (key, head, item) => h(Box, { key, flexDirection: 'row' },
        h(Text, {}, '  '),
        h(Box, { width: name }, head),
        h(Box, { key: `strip-${item.key}`, flexDirection: 'row' },
          ...cut(strip(item), cols - 2 - name).map(cell => h(Text, { wrap: 'truncate-end', ...(cell.color && { color: cell.color }), ...(cell.dim && { dimColor: true }) }, cell.text))))
      const out = notes()
      for (const clanker of board.clankers) {
        out.push(toggleLine(clanker, 'clanker', '', `${clanker.path} · ${count(clanker)}`, cols))
        if (!expanded.get(clanker.key)) continue
        out.push(line({ dimColor: true }, `${' '.repeat(2 + name)}plan  impl  rev   fix   merge`))
        for (const clerk of clanker.clerks) {
          out.push(row(clerk.key, opener(clerk.key, clerk.name, name - 1), clerk))
          if (expanded.get(clerk.key)) out.push(...details(clerk, '    ', cols))
        }
      }
      out.push(line({}, ' '), ...doneGroup(cols, d => row(`done-${d.key}`, h(Text, { wrap: 'truncate-end', color: 'gray' }, fit(d.label, name - 1)), d)))
      // What waits on the owner comes last (R-16).
      out.push(line({}, ' '), line({ bold: true }, 'Waits on you'))
      if (board.questions.length === 0) out.push(line({ dimColor: true }, 'nothing waits on you'))
      out.push(...board.questions.flatMap(q => ask(q, '', cols)))
      return out
    }

    const draw = design === 'buckets' ? buckets : design === 'pipeline' ? pipeline : cards
    return h(Box, { flexDirection: 'column' }, ...draw())
  })
}
