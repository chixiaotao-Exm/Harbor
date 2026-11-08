<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { ensureLive, loadBalance, useLive, clearLogs } from '../live'
import type { Item } from '../live'

const form = ref({ count: 1, threads: 1, delay: 5, domain: 'icloud.com', auto_enqueue: true })
const { stat, items, logs, balance } = useLive()
const err = ref('')
const region = ref('US')
const regions = ref<string[]>(['US'])
const regionBusy = ref(false)

const canStart = computed(() => !stat.value.running)

function addLog(t: string, m: string) {
  logs.value = [{ t, m }, ...logs.value].slice(0, 200)
}

async function refreshStat() {
  try {
    const r = await fetch('/api/run/stat')
    if (r.ok) stat.value = await r.json()
  } catch {
    /* 忽略 */
  }
}

async function loadRegion() {
  try {
    const r = await fetch('/api/regions')
    if (r.ok) {
      const j = await r.json()
      if (Array.isArray(j.regions) && j.regions.length) regions.value = j.regions
    }
    const rs = await fetch('/api/settings')
    if (rs.ok) {
      const s = await rs.json()
      if (s.region) region.value = s.region
    }
  } catch {
    /* 忽略 */
  }
}

// 切换地区即写回设置，代理串由后端改写
async function changeRegion() {
  regionBusy.value = true
  try {
    const rs = await fetch('/api/settings')
    const cur = rs.ok ? await rs.json() : {}
    await fetch('/api/settings', {
      method: 'PUT',
      body: JSON.stringify({
        region: region.value,
        domain: cur.domain,
        default_jobs: cur.default_jobs,
        default_wait: cur.default_wait,
        add_password: cur.add_password,
        add_mfa: cur.add_mfa,
      }),
    })
    addLog(new Date().toTimeString().slice(0, 8), `出口地区已切换为 ${region.value}`)
    await loadBalance()
  } finally {
    regionBusy.value = false
  }
}

async function start() {
  err.value = ''
  clearLogs()
  const r = await fetch('/api/run/start', { method: 'POST', body: JSON.stringify(form.value) })
  if (!r.ok) {
    err.value = (await r.json().catch(() => ({ error: '启动失败' }))).error
    return
  }
  await refreshStat()
}

async function stop() {
  await fetch('/api/run/stop', { method: 'POST' })
}

onMounted(() => {
  ensureLive()
  loadRegion()
})
</script>

<template>
  <div class="head">
    <div>
      <h1>工作台</h1>
      <p>一条任务一个出口，取号串行，跑完一单再补下一单</p>
    </div>
    <div class="row">
      <button class="btn-primary" :disabled="!canStart" @click="start">开工</button>
      <button class="btn-plain" :disabled="canStart" @click="stop">停止</button>
    </div>
  </div>

  <p v-if="err" class="hint" style="color: var(--danger)">{{ err }}</p>

  <div class="stats">
    <div class="stat"><span>余额</span><b style="font-size: 22px">{{ balance }}</b></div>
    <div class="stat ok"><span>成功</span><b>{{ stat.ok }}</b></div>
    <div class="stat bad"><span>失败</span><b>{{ stat.fail }}</b></div>
    <div class="stat"><span>成功率</span><b>{{ stat.rate.toFixed(1) }}%</b></div>
  </div>

  <div class="card">
    <h2>参数</h2>
    <div class="grid cols-3">
      <div>
        <label class="f">数量</label>
        <input v-model.number="form.count" type="number" min="1" />
      </div>
      <div>
        <label class="f">并发</label>
        <input v-model.number="form.threads" type="number" min="1" max="8" />
      </div>
      <div>
        <label class="f">间隔（秒）</label>
        <input v-model.number="form.delay" type="number" min="0" />
      </div>
    </div>
    <div class="grid cols-2">
      <div>
        <label class="f">邮箱域名</label>
        <select v-model="form.domain">
          <option>icloud.com</option>
          <option>gmail.com</option>
          <option>outlook.com</option>
        </select>
      </div>
      <div>
        <label class="f">出口地区<span v-if="regionBusy" class="muted"> · 切换中</span></label>
        <select v-model="region" :disabled="regionBusy || stat.running" @change="changeRegion">
          <option v-for="r in regions" :key="r" :value="r">{{ r }}</option>
        </select>
      </div>
    </div>
    <div class="check">
      <label class="switch">
        <input v-model="form.auto_enqueue" type="checkbox" />
        <span class="track"></span>
        <span>开户成功后自动投递到异步任务（绑双因子）</span>
      </label>
    </div>
    <p class="hint">数量是总配额，并发是同时在跑的槽位；任一时刻在手邮箱不会超过并发数。</p>
  </div>

  <div class="split">
    <div class="card">
      <h2>日志<span class="muted" style="font-weight: 400; margin-left: 8px">最新在上</span></h2>
      <div class="log" style="height: 300px">
        <div v-for="l in logs" :key="l.t + l.m + logs.length" v-memo="[l.t, l.m]" class="l">
          <span class="t">{{ l.t }}</span><span class="m">{{ l.m }}</span>
        </div>
        <p v-if="!logs.length" class="empty">开工后这里会逐条显示谁在做什么</p>
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
</template>
