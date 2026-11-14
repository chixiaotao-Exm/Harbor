<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import { t, tf } from '../i18n'
import { ensureLive, useLive } from '../live'

type Row = { id: number; email: string; q: number }
type Failed = { id: number; email: string; reason: string; times: number }

const phases = [
  { q: 1, key: 'async.q1' },
  { q: 2, key: 'async.q2' },
  { q: 3, key: 'async.q3' },
  { q: 4, key: 'async.q4' },
  { q: 5, key: 'async.q5' },
]

const rows = ref<Row[]>([])
const failed = ref<Failed[]>([])
const running = ref(false)
const { logs, items } = useLive()
const msg = ref('')
const picked = ref<Set<number>>(new Set())
let timer: number | null = null

function byQ(q: number) {
  return rows.value.filter((r) => r.q === q)
}

const allPicked = computed(
  () => failed.value.length > 0 && failed.value.every((f) => picked.value.has(f.id)),
)

async function loadBoard() {
  try {
    const r = await fetch('/api/mfa/board')
    if (r.ok) {
      const j = await r.json()
      rows.value = j.rows ?? []
      running.value = !!j.running
    }
    const rf = await fetch('/api/mfa/failed')
    if (rf.ok) failed.value = await rf.json()
  } catch {
    /* 忽略 */
  }
}

function toggle(id: number) {
  const s = new Set(picked.value)
  s.has(id) ? s.delete(id) : s.add(id)
  picked.value = s
}

function toggleAll() {
  if (allPicked.value) picked.value = new Set()
  else picked.value = new Set(failed.value.map((f) => f.id))
}

async function retry(all: boolean) {
  const ids = all ? [] : [...picked.value]
  const r = await fetch('/api/mfa/retry', { method: 'POST', body: JSON.stringify({ ids, all }) })
  if (!r.ok) {
    const j = await r.json().catch(() => ({}))
    msg.value = j.error ?? t('acc.pushFailed')
    return
  }
  const j = await r.json()
  msg.value = tf('async.retried', { n: j.queued })
  picked.value = new Set()
  await loadBoard()
}

async function startAll() {
  const r = await fetch('/api/mfa/start', { method: 'POST', body: JSON.stringify({ all: true }) })
  const j = await r.json().catch(() => ({ queued: 0 }))
  msg.value = j.error ?? tf('async.queued', { n: j.queued })
  await loadBoard()
}

onMounted(() => {
  ensureLive()
  loadBoard()
  timer = window.setInterval(loadBoard, 3000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="head">
    <div>
      <h1>{{ t('async.title') }}</h1>
      <p>{{ t('async.sub') }}</p>
    </div>
    <div class="row">
      <button class="btn-primary" :disabled="running" @click="startAll">{{ t('async.pushAll') }}</button>
      <button class="btn-plain" :disabled="!failed.length" @click="retry(true)">{{ t('async.retryAll') }}</button>
    </div>
  </div>

  <p v-if="msg" class="hint">{{ msg }}</p>

  <div class="board">
    <div v-for="p in phases" :key="p.q" class="phase">
      <h3>{{ t(p.key) }} <b>{{ byQ(p.q).length }}</b></h3>
      <div class="items">
        <div v-for="r in byQ(p.q)" :key="r.id" class="job">{{ r.email }}</div>
        <p v-if="!byQ(p.q).length" class="empty">{{ t('async.empty') }}</p>
      </div>
    </div>
  </div>

  <div class="split" style="margin-top: 14px">
    <div class="card">
      <h2>
        {{ t('async.logs') }}
        <span class="muted" style="font-weight: 400; margin-left: 8px">{{ t('work.newestFirst') }}</span>
      </h2>
      <div class="log" style="height: 300px">
        <div v-for="l in logs" :key="l.t + l.m" v-memo="[l.t, l.m]" class="l">
          <span class="t">{{ l.t }}</span><span class="m">{{ l.m }}</span>
        </div>
        <p v-if="!logs.length" class="empty">{{ t('async.logEmpty') }}</p>
      </div>
    </div>

    <div class="card">
      <h2>{{ t('async.items') }}</h2>
      <div class="items-scroll">
        <table>
          <thead>
            <tr>
              <th>{{ t('work.col.email') }}</th>
              <th>{{ t('work.col.ip') }}</th>
              <th>{{ t('work.col.stage') }}</th>
              <th>{{ t('work.col.state') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="it in items" :key="it.email || it.stage" v-memo="[it.stage, it.state, it.note, it.ip]">
              <td class="mono">{{ it.email || '—' }}</td>
              <td class="mono">{{ it.ip || '—' }}</td>
              <td class="muted">{{ it.stage }}<span v-if="it.note"> · {{ it.note }}</span></td>
              <td>
                <span class="tag" :class="{ ok: it.state === 'ok', bad: it.state === 'fail', run: it.state === 'running' }">
                  {{ it.state === 'ok' ? t('work.state.ok') : it.state === 'fail' ? t('work.state.fail') : t('work.state.running') }}
                </span>
              </td>
            </tr>
            <tr v-if="!items.length"><td colspan="4" class="empty">{{ t('work.itemsEmpty') }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="row" style="justify-content: space-between">
      <h2 style="margin: 0">{{ tf('async.failed', { n: failed.length }) }}</h2>
      <div class="row">
        <button class="btn-ghost" @click="toggleAll">{{ allPicked ? t('async.unselectAll') : t('async.selectAll') }}</button>
        <button class="btn-plain" :disabled="!picked.size" @click="retry(false)">{{ t('async.retryPick') }}</button>
      </div>
    </div>
    <table>
      <thead>
        <tr>
          <th style="width: 36px"></th>
          <th>{{ t('async.col.email') }}</th>
          <th>{{ t('async.col.reason') }}</th>
          <th>{{ t('async.col.times') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="f in failed" :key="f.id">
          <td><label class="sw"><input type="checkbox" :checked="picked.has(f.id)" @change="toggle(f.id)" /><span class="t2"></span></label></td>
          <td class="mono">{{ f.email }}</td>
          <td class="muted">{{ f.reason }}</td>
          <td class="mono">{{ f.times }}</td>
        </tr>
        <tr v-if="!failed.length"><td colspan="4" class="empty">{{ t('async.noFail') }}</td></tr>
      </tbody>
    </table>
  </div>
</template>
