<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'

import { useRouter } from 'vue-router'

import { t, tf } from '../i18n'

type Acc = {
  id: number
  email: string
  password: string
  secret: string
  mfa_state: string
  attempts: number
  error: string
  created_at: number
}

const kind = ref<'mfa' | 'plain'>('mfa')
const page = ref(1)
const size = ref(20)
const q = ref('')
const router = useRouter()
const rows = ref<Acc[]>([])
const total = ref(0)
const pages = ref(1)
const picked = ref<Set<number>>(new Set())
const busy = ref(false)
const msg = ref('')

const allPicked = computed(
  () => rows.value.length > 0 && rows.value.every((r) => picked.value.has(r.id)),
)

// 请求序号，晚到的旧响应不覆盖新结果
let seq = 0

async function load() {
  const mine = ++seq
  const u = `/api/accounts?kind=${kind.value}&page=${page.value}&size=${size.value}&q=${encodeURIComponent(q.value)}`
  const r = await fetch(u)
  if (!r.ok || mine !== seq) return
  const j = await r.json()
  if (mine !== seq) return
  rows.value = j.items ?? []
  total.value = j.total ?? 0
  pages.value = j.pages ?? 1
  if (page.value > pages.value && pages.value > 0) page.value = pages.value
}

async function copy(text: string, labelKey: string) {
  if (!text) return
  const note = tf('acc.copied', { label: tf(labelKey, {}) })
  try {
    await navigator.clipboard.writeText(text)
    toast(note)
  } catch {
    // 非安全上下文时退回旧接口
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
    toast(note)
  }
}

let toastTimer: number | null = null
function toast(text: string) {
  msg.value = text
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => (msg.value = ''), 1800)
}

// 动态码现算，点击即取即复制
async function copyCode(r: Acc) {
  const resp = await fetch(`/api/accounts/${r.id}/code`)
  if (!resp.ok) {
    const j = await resp.json().catch(() => ({}))
    toast(j.error ?? t('acc.codeFailed'))
    return
  }
  const j = await resp.json()
  await copy(j.code, 'acc.copiedCode')
}

function copyAll(r: Acc) {
  const parts = [r.email, r.password]
  if (kind.value === 'mfa' && r.secret) parts.push(r.secret)
  copy(parts.filter(Boolean).join('----'), 'acc.copiedRow')
}

function toggle(id: number) {
  const s = new Set(picked.value)
  s.has(id) ? s.delete(id) : s.add(id)
  picked.value = s
}

function toggleAll() {
  const s = new Set(picked.value)
  if (allPicked.value) rows.value.forEach((r) => s.delete(r.id))
  else rows.value.forEach((r) => s.add(r.id))
  picked.value = s
}

async function selectAllPages() {
  const r = await fetch('/api/accounts/select', {
    method: 'POST',
    body: JSON.stringify({ kind: kind.value, q: q.value }),
  })
  if (!r.ok) return
  const j = await r.json()
  picked.value = new Set<number>(j.ids ?? [])
  msg.value = tf('acc.selected', { n: j.total })
}

function clearPicked() {
  picked.value = new Set()
  msg.value = ''
}

async function exportTxt() {
  busy.value = true
  try {
    const ids = [...picked.value]
    const r = await fetch('/api/accounts/export', {
      method: 'POST',
      body: JSON.stringify({ kind: kind.value, ids, all: ids.length === 0 }),
    })
    if (!r.ok) return
    const text = await r.text()
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = kind.value === 'mfa' ? 'accounts-mfa.txt' : 'accounts-plain.txt'
    a.click()
    URL.revokeObjectURL(a.href)
    msg.value = tf('acc.exported', { n: ids.length || total.value })
  } finally {
    busy.value = false
  }
}

const pushResult = ref<{ text: string; bad: boolean } | null>(null)

async function pushToAsync() {
  const ids = [...picked.value]
  pushResult.value = null
  const r = await fetch('/api/mfa/start', {
    method: 'POST',
    body: JSON.stringify({ ids, all: ids.length === 0 }),
  })
  if (!r.ok) {
    const j = await r.json().catch(() => ({}))
    pushResult.value = { text: j.error ?? t('acc.pushFailed'), bad: true }
    return
  }
  const j = await r.json()
  pushResult.value = { text: tf('acc.pushed', { n: j.queued }), bad: false }
  picked.value = new Set()
  setTimeout(() => {
    if (pushResult.value && !pushResult.value.bad) router.push('/async')
  }, 900)
}

watch([kind, page, size], load)

// 输入即搜：350ms 防抖，回到第一页并清掉勾选
let searchTimer: number | null = null
function onSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    page.value = 1
    clearPicked()
    load()
  }, 350)
}

watch(kind, () => {
  page.value = 1
  clearPicked()
})
onMounted(() => {
  load()
})
</script>

<template>
  <div class="head">
    <div>
      <h1>{{ t('acc.title') }}</h1>
      <p>{{ t('acc.sub') }}</p>
    </div>
    <div class="row">
      <button class="btn-primary" :disabled="busy" @click="exportTxt">{{ t('acc.export') }}</button>
      <button class="btn-plain" :disabled="kind === 'mfa'" @click="pushToAsync">{{ t('acc.push') }}</button>
    </div>
  </div>

  <div v-if="pushResult" class="banner" :class="{ bad: pushResult.bad }">
    <span>{{ pushResult.text }}</span>
    <a v-if="!pushResult.bad" href="#" @click.prevent="router.push('/async')">{{ t('acc.goAsync') }}</a>
    <a v-else href="#" @click.prevent="pushResult = null">{{ t('acc.close') }}</a>
  </div>

  <div class="tabs">
    <button :class="{ on: kind === 'mfa' }" @click="kind = 'mfa'">{{ t('acc.tab.mfa') }}</button>
    <button :class="{ on: kind === 'plain' }" @click="kind = 'plain'">{{ t('acc.tab.plain') }}</button>
  </div>

  <div class="card">
    <div class="row">
      <input v-model="q" :placeholder="t('acc.search')" style="max-width: 260px" @input="onSearch" />
      <select v-model.number="size" style="max-width: 140px">
        <option :value="10">{{ tf('acc.perPage', { n: 10 }) }}</option>
        <option :value="20">{{ tf('acc.perPage', { n: 20 }) }}</option>
        <option :value="50">{{ tf('acc.perPage', { n: 50 }) }}</option>
        <option :value="100">{{ tf('acc.perPage', { n: 100 }) }}</option>
      </select>
      <button class="btn-ghost" @click="selectAllPages">{{ tf('acc.selectAll', { n: total }) }}</button>
      <button class="btn-ghost" @click="clearPicked">{{ t('acc.clearPick') }}</button>
      <span class="muted">{{ tf('acc.selected', { n: picked.size }) }}</span>
      <span v-if="msg" class="muted">{{ msg }}</span>
    </div>
  </div>

  <div class="card">
    <table>
      <thead>
        <tr>
          <th style="width: 36px"><label class="sw"><input type="checkbox" :checked="allPicked" @change="toggleAll" /><span class="t2"></span></label></th>
          <th>{{ t('acc.col.email') }}</th>
          <th>{{ t('acc.col.password') }}</th>
          <th v-if="kind === 'mfa'">{{ t('acc.col.secret') }}</th>
          <th v-if="kind === 'mfa'">{{ t('acc.col.code') }}</th>
          <th>{{ t('acc.col.state') }}</th>
          <th style="width: 78px">{{ t('acc.col.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.id">
          <td><label class="sw"><input type="checkbox" :checked="picked.has(r.id)" @change="toggle(r.id)" /><span class="t2"></span></label></td>
          <td class="mono">
            <a href="#" class="cp" @click.prevent="copy(r.email, 'acc.copiedEmail')">{{ r.email }}</a>
          </td>
          <td class="mono">
            <a v-if="r.password" href="#" class="cp" @click.prevent="copy(r.password, 'acc.copiedPassword')">
              {{ r.password }}
            </a>
            <span v-else>—</span>
          </td>
          <td v-if="kind === 'mfa'" class="mono">
            <a v-if="r.secret" href="#" class="cp" @click.prevent="copy(r.secret, 'acc.copiedSecret')">
              {{ r.secret }}
            </a>
            <span v-else>—</span>
          </td>
          <td v-if="kind === 'mfa'">
            <button v-if="r.secret" class="btn-plain" style="padding: 4px 10px; font-size: 12px" @click="copyCode(r)">
              {{ t('acc.getCode') }}
            </button>
            <span v-else class="muted">—</span>
          </td>
          <td>
            <span class="tag" :class="{ ok: r.mfa_state === 'ok', bad: r.mfa_state === 'fail' }">
              {{ r.mfa_state === 'ok' ? t('acc.state.bound') : r.mfa_state === 'fail' ? t('acc.state.fail') : r.mfa_state }}
            </span>
          </td>
          <td>
            <button class="btn-ghost" style="padding: 4px 6px; font-size: 12px" @click="copyAll(r)">{{ t('acc.copyRow') }}</button>
          </td>
        </tr>
        <tr v-if="!rows.length"><td colspan="7" class="empty">{{ t('acc.emptyPage') }}</td></tr>
      </tbody>
    </table>

    <div class="pager">
      <span>{{ tf('acc.pageInfo', { total, page, pages }) }}</span>
      <div class="row">
        <button class="btn-plain" :disabled="page <= 1" @click="page--">{{ t('acc.prev') }}</button>
        <button class="btn-plain" :disabled="page >= pages" @click="page++">{{ t('acc.next') }}</button>
      </div>
    </div>
  </div>
</template>
