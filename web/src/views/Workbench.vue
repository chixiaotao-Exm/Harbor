<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { tf, regionLabel } from '../i18n'
import { ensureLive, loadBalance, useLive, clearLogs } from '../live'

type Region = { code: string; zh: string; en: string }

const form = ref({ count: 1, threads: 1, delay: 5, domain: 'icloud.com', auto_enqueue: true })
const { stat, items, logs, balance } = useLive()
const err = ref('')
const region = ref('RAND')
const regions = ref<Region[]>([])
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
    const picked = regions.value.find((x) => x.code === region.value)
    const label = picked ? regionLabel(picked) : region.value
    addLog(new Date().toTimeString().slice(0, 8), tf('work.regionChanged', { region: label }))
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
    const j = await r.json().catch(() => ({}))
    err.value = j.error ?? tf('work.startFailed', {})
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
      <h1>{{ tf('work.title', {}) }}</h1>
      <p>{{ tf('work.sub', {}) }}</p>
    </div>
    <div class="row">
      <button class="btn-primary" :disabled="!canStart" @click="start">{{ tf('work.start', {}) }}</button>
      <button class="btn-plain" :disabled="canStart" @click="stop">{{ tf('work.stop', {}) }}</button>
    </div>
  </div>

  <p v-if="err" class="hint" style="color: var(--danger)">{{ err }}</p>

  <div class="stats">
    <div class="stat"><span>{{ tf('work.balance', {}) }}</span><b style="font-size: 22px">{{ balance }}</b></div>
    <div class="stat ok"><span>{{ tf('work.ok', {}) }}</span><b>{{ stat.ok }}</b></div>
    <div class="stat bad"><span>{{ tf('work.fail', {}) }}</span><b>{{ stat.fail }}</b></div>
    <div class="stat"><span>{{ tf('work.rate', {}) }}</span><b>{{ stat.rate.toFixed(1) }}%</b></div>
  </div>

  <div class="card">
    <h2>{{ tf('work.params', {}) }}</h2>
    <div class="grid cols-3">
      <div>
        <label class="f">{{ tf('work.count', {}) }}</label>
        <input v-model.number="form.count" type="number" min="1" />
      </div>
      <div>
        <label class="f">{{ tf('work.threads', {}) }}</label>
        <input v-model.number="form.threads" type="number" min="1" max="8" />
      </div>
      <div>
        <label class="f">{{ tf('work.delay', {}) }}</label>
        <input v-model.number="form.delay" type="number" min="0" />
      </div>
    </div>
    <div class="grid cols-2">
      <div>
        <label class="f">{{ tf('work.domain', {}) }}</label>
        <select v-model="form.domain">
          <option>icloud.com</option>
          <option>gmail.com</option>
          <option>outlook.com</option>
        </select>
      </div>
      <div>
        <label class="f">
          {{ tf('work.region', {}) }}
          <span v-if="regionBusy" class="muted"> · {{ tf('work.regionBusy', {}) }}</span>
        </label>
        <select v-model="region" :disabled="regionBusy || stat.running" @change="changeRegion">
          <option v-for="r in regions" :key="r.code" :value="r.code">{{ regionLabel(r) }}</option>
        </select>
      </div>
    </div>
    <div class="check">
      <label class="switch">
        <input v-model="form.auto_enqueue" type="checkbox" />
        <span class="track"></span>
        <span>{{ tf('work.autoEnqueue', {}) }}</span>
      </label>
    </div>
    <p class="hint">{{ tf('work.hint', {}) }}</p>
  </div>

  <div class="split">
    <div class="card">
      <h2>
        {{ tf('work.logs', {}) }}
        <span class="muted" style="font-weight: 400; margin-left: 8px">{{ tf('work.newestFirst', {}) }}</span>
      </h2>
      <div class="log" style="height: 300px">
        <div v-for="l in logs" :key="l.t + l.m + logs.length" v-memo="[l.t, l.m]" class="l">
          <span class="t">{{ l.t }}</span><span class="m">{{ l.m }}</span>
        </div>
        <p v-if="!logs.length" class="empty">{{ tf('work.logEmpty', {}) }}</p>
      </div>
    </div>

    <div class="card">
      <h2>{{ tf('work.items', {}) }}</h2>
      <div class="items-scroll">
        <table>
          <thead>
            <tr>
              <th>{{ tf('work.col.email', {}) }}</th>
              <th>{{ tf('work.col.ip', {}) }}</th>
              <th>{{ tf('work.col.stage', {}) }}</th>
              <th>{{ tf('work.col.state', {}) }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="it in items" :key="it.email || it.stage" v-memo="[it.stage, it.state, it.note, it.ip]">
              <td class="mono">{{ it.email || '—' }}</td>
              <td class="mono">{{ it.ip || '—' }}</td>
              <td class="muted">{{ it.stage }}<span v-if="it.note"> · {{ it.note }}</span></td>
              <td>
                <span class="tag" :class="{ ok: it.state === 'ok', bad: it.state === 'fail', run: it.state === 'running' }">
                  {{ it.state === 'ok' ? tf('work.state.ok', {}) : it.state === 'fail' ? tf('work.state.fail', {}) : tf('work.state.running', {}) }}
                </span>
              </td>
            </tr>
            <tr v-if="!items.length"><td colspan="4" class="empty">{{ tf('work.itemsEmpty', {}) }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
