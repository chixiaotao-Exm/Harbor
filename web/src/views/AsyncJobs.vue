<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import { ensureLive, useLive } from '../live'

type Row = { id: number; email: string; q: number }
type Failed = { id: number; email: string; reason: string; times: number }

const phases = [
  { q: 1, name: 'Q1 校验登录态' },
  { q: 2, name: 'Q2 账号密码登录' },
  { q: 3, name: 'Q3 提交双因子登记' },
  { q: 4, name: 'Q4 动态码激活' },
  { q: 5, name: 'Q5 收尾确认' },
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
    msg.value = (await r.json().catch(() => ({ error: '重试失败' }))).error
    return
  }
  const j = await r.json()
  msg.value = `已重投 ${j.queued} 条`
  picked.value = new Set()
  await loadBoard()
}

async function startAll() {
  const r = await fetch('/api/mfa/start', { method: 'POST', body: JSON.stringify({ all: true }) })
  const j = await r.json().catch(() => ({ queued: 0, error: '投递失败' }))
  msg.value = j.error ?? `已投递 ${j.queued} 条`
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
      <h1>异步任务</h1>
      <p>每步失败会自动重试 3 次并换出口，超限落失败页</p>
    </div>
    <div class="row">
      <button class="btn-primary" :disabled="running" @click="startAll">投递未绑账号</button>
      <button class="btn-plain" :disabled="!failed.length" @click="retry(true)">全部重试</button>
    </div>
  </div>

  <p v-if="msg" class="hint">{{ msg }}</p>

  <div class="board">
    <div v-for="p in phases" :key="p.q" class="phase">
      <h3>{{ p.name }} <b>{{ byQ(p.q).length }}</b></h3>
      <div class="items">
        <div v-for="r in byQ(p.q)" :key="r.id" class="job">{{ r.email }}</div>
        <p v-if="!byQ(p.q).length" class="empty">空</p>
      </div>
    </div>
  </div>

  <div class="split" style="margin-top: 14px">
    <div class="card">
      <h2>日志<span class="muted" style="font-weight: 400; margin-left: 8px">最新在上</span></h2>
      <div class="log" style="height: 300px">
        <div v-for="l in logs" :key="l.t + l.m" v-memo="[l.t, l.m]" class="l">
          <span class="t">{{ l.t }}</span><span class="m">{{ l.m }}</span>
        </div>
        <p v-if="!logs.length" class="empty">队列跑起来后这里会显示每一步在做什么</p>
      </div>
    </div>

    <div class="card">
      <h2>工作明细</h2>
      <div class="items-scroll">
        <table>
          <thead>
            <tr><th>账号</th><th>IP</th><th>环节</th><th>状态</th></tr>
          </thead>
          <tbody>
            <tr v-for="it in items" :key="it.email || it.stage" v-memo="[it.stage, it.state, it.note, it.ip]">
              <td class="mono">{{ it.email || '—' }}</td>
              <td class="mono">{{ it.ip || '—' }}</td>
              <td class="muted">{{ it.stage }}<span v-if="it.note"> · {{ it.note }}</span></td>
              <td>
                <span class="tag" :class="{ ok: it.state === 'ok', bad: it.state === 'fail', run: it.state === 'running' }">
                  {{ it.state === 'ok' ? '成功' : it.state === 'fail' ? '失败' : '进行中' }}
                </span>
              </td>
            </tr>
            <tr v-if="!items.length"><td colspan="4" class="empty">还没有明细</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="row" style="justify-content: space-between">
      <h2 style="margin: 0">失败页（{{ failed.length }}）</h2>
      <div class="row">
        <button class="btn-ghost" @click="toggleAll">{{ allPicked ? '取消全选' : '全选' }}</button>
        <button class="btn-plain" :disabled="!picked.size" @click="retry(false)">重试选中</button>
      </div>
    </div>
    <table>
      <thead>
        <tr><th style="width: 36px"></th><th>账号</th><th>原因</th><th>次数</th></tr>
      </thead>
      <tbody>
        <tr v-for="f in failed" :key="f.id">
          <td><label class="sw"><input type="checkbox" :checked="picked.has(f.id)" @change="toggle(f.id)" /><span class="t2"></span></label></td>
          <td class="mono">{{ f.email }}</td>
          <td class="muted">{{ f.reason }}</td>
          <td class="mono">{{ f.times }}</td>
        </tr>
        <tr v-if="!failed.length"><td colspan="4" class="empty">没有失败</td></tr>
      </tbody>
    </table>
  </div>
</template>
