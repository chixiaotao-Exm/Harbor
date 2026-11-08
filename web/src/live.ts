import { ref, shallowRef } from 'vue'

export type Stat = { running: boolean; total: number; done: number; ok: number; fail: number; rate: number }
export type Item = { email: string; ip: string; stage: string; state: string; note: string }
export type Log = { t: string; m: string }

const MAX_LOGS = 200
const MAX_ITEMS = 400

// 全局单例：切页面不重连 SSE，避免历史重复回放
const stat = ref<Stat>({ running: false, total: 0, done: 0, ok: 0, fail: 0, rate: 0 })
const items = shallowRef<Item[]>([])
const logs = shallowRef<Log[]>([])
const balance = ref('—')

let es: EventSource | null = null
let poll: number | null = null
let balTimer: number | null = null
let started = false

function addLog(t: string, m: string) {
  const cur = logs.value
  if (cur.length && cur[0].t === t && cur[0].m === m) return
  const next = [{ t, m }, ...cur]
  logs.value = next.length > MAX_LOGS ? next.slice(0, MAX_LOGS) : next
}

function upsertItem(it: Item) {
  const cur = items.value
  const i = cur.findIndex((x) => x.email && x.email === it.email)
  if (i >= 0) {
    if (cur[i].stage === it.stage && cur[i].state === it.state && cur[i].note === it.note) return
    const next = cur.slice()
    next[i] = it
    items.value = next
    return
  }
  const next = [it, ...cur]
  items.value = next.length > MAX_ITEMS ? next.slice(0, MAX_ITEMS) : next
}

async function refreshStat() {
  try {
    const r = await fetch('/api/run/stat')
    if (r.ok) stat.value = await r.json()
  } catch {
    /* 忽略 */
  }
}

async function loadItems() {
  try {
    const r = await fetch('/api/run/items')
    if (!r.ok) return
    const list: Item[] = await r.json()
    items.value = list.slice().reverse().slice(0, MAX_ITEMS)
  } catch {
    /* 忽略 */
  }
}

async function loadHistory() {
  try {
    const r = await fetch('/api/events/history')
    if (!r.ok) return
    const list: { kind: string; at: string; text?: string }[] = await r.json()
    const only = list.filter((e) => e.kind === 'log' && e.text && e.text.trim())
    logs.value = only
      .slice(-MAX_LOGS)
      .reverse()
      .map((e) => ({ t: e.at, m: e.text as string }))
  } catch {
    /* 忽略 */
  }
}

export async function loadBalance() {
  try {
    const r = await fetch('/api/balance')
    if (!r.ok) return
    const j = await r.json()
    if (!j.ok) {
      balance.value = j.error === '还没填 API Key' ? '未设置 Key' : '查询失败'
      return
    }
    const n = String(j.raw).split(':').pop()
    balance.value = isNaN(Number(n)) ? String(j.raw) : Number(n).toFixed(2)
  } catch {
    /* 忽略 */
  }
}

/** 首次进站时启动一次；之后切页面不会重复建连。 */
export function ensureLive() {
  if (started) return
  started = true

  refreshStat()
  loadItems()
  loadHistory()
  loadBalance()

  poll = window.setInterval(refreshStat, 3000)
  balTimer = window.setInterval(loadBalance, 30000)

  es = new EventSource('/api/events')
  es.onmessage = (ev) => {
    try {
      const e = JSON.parse(ev.data)
      if (e.kind === 'log' && typeof e.text === 'string' && e.text.trim()) addLog(e.at, e.text)
      if (e.kind === 'stat') stat.value = e.data
      if (e.kind === 'item') upsertItem(e.data as Item)
    } catch {
      /* 忽略坏帧 */
    }
  }
  // 断流期间可能漏帧，重连后补一次
  es.onerror = () => {
    setTimeout(loadItems, 1200)
  }
}

export function clearLogs() {
  logs.value = []
}

export function useLive() {
  return { stat, items, logs, balance, loadBalance, clearLogs }
}
