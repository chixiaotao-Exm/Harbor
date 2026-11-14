<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import RegionSelect from '../components/RegionSelect.vue'
import { tf, regionLabel } from '../i18n'

type Region = { code: string; zh: string; en: string }

type View = {
  api_key_masked: string
  proxy_masked: string
  has_api_key: boolean
  proxy_host: string
  region: string
  domain: string
  default_jobs: number
  default_wait: number
  add_password: boolean
  add_mfa: boolean
}

const v = ref<View>({
  api_key_masked: '',
  proxy_masked: '',
  has_api_key: false,
  proxy_host: '',
  region: 'RAND',
  domain: 'icloud.com',
  default_jobs: 1,
  default_wait: 5,
  add_password: true,
  add_mfa: true,
})

const key = ref('')
const proxy = ref('')
const msg = ref('')
const bad = ref(false)
const regions = ref<Region[]>([])

const regionHint = computed(() => {
  const r = regions.value.find((x) => x.code === v.value.region)
  return r ? regionLabel(r) : v.value.region
})

async function load() {
  const r = await fetch('/api/settings')
  if (r.ok) v.value = await r.json()
  const rr = await fetch('/api/regions')
  if (rr.ok) {
    const j = await rr.json()
    if (Array.isArray(j.regions) && j.regions.length) regions.value = j.regions
  }
}

async function save() {
  msg.value = ''
  bad.value = false
  const body = {
    api_key: key.value.trim(),
    proxy: proxy.value.trim(),
    region: v.value.region,
    domain: v.value.domain,
    default_jobs: v.value.default_jobs,
    default_wait: v.value.default_wait,
    add_password: v.value.add_password,
    add_mfa: v.value.add_mfa,
  }
  const r = await fetch('/api/settings', { method: 'PUT', body: JSON.stringify(body) })
  if (!r.ok) {
    bad.value = true
    const j = await r.json().catch(() => ({}))
    msg.value = j.error ?? tf('settings.saveFailed', {})
    return
  }
  v.value = await r.json()
  key.value = ''
  proxy.value = ''
  msg.value = tf('settings.saved', {})
}

onMounted(async () => {
  await load()
  const k = localStorage.getItem('harbor_key')
  const p = localStorage.getItem('harbor_proxy')
  if (k && !k.includes('…')) key.value = k
  if (p && !p.includes('…')) proxy.value = p
})

function remember() {
  if (key.value.trim()) localStorage.setItem('harbor_key', key.value.trim())
  if (proxy.value.trim()) localStorage.setItem('harbor_proxy', proxy.value.trim())
}

function rememberSoon() {
  setTimeout(remember, 0)
}
</script>

<template>
  <div class="head">
    <div>
      <h1>{{ tf('settings.title', {}) }}</h1>
      <p>{{ tf('settings.sub', {}) }}</p>
    </div>
  </div>

  <div class="card">
    <h2>{{ tf('settings.sms', {}) }}</h2>
    <label class="f">{{ tf('settings.apiKey', {}) }}</label>
    <input
      v-model="key"
      :placeholder="v.has_api_key ? v.api_key_masked : tf('settings.apiKeyPh', {})"
      @paste="rememberSoon"
      @change="remember"
    />
    <p class="hint">{{ tf('settings.keepExisting', {}) }}</p>

    <label class="f">{{ tf('settings.proxy', {}) }}</label>
    <input
      v-model="proxy"
      :placeholder="v.proxy_host || 'socks5://user-region-US-sid-abc:pass@host:3010'"
      @change="remember"
    />
    <p class="hint">{{ tf('settings.proxyHint', {}) }}</p>

    <label class="f">{{ tf('settings.region', {}) }}</label>
    <RegionSelect v-model="v.region" :regions="regions" />
    <p class="hint">{{ tf('settings.regionHint', {}) }} · {{ regionHint }}</p>
  </div>

  <div class="card">
    <h2>{{ tf('settings.defaults', {}) }}</h2>
    <div class="grid cols-3">
      <div>
        <label class="f">{{ tf('settings.defaultCount', {}) }}</label>
        <input v-model.number="v.default_jobs" type="number" min="1" />
      </div>
      <div>
        <label class="f">{{ tf('settings.defaultDelay', {}) }}</label>
        <input v-model.number="v.default_wait" type="number" min="0" />
      </div>
      <div>
        <label class="f">{{ tf('work.domain', {}) }}</label>
        <select v-model="v.domain">
          <option>icloud.com</option>
          <option>gmail.com</option>
          <option>outlook.com</option>
        </select>
      </div>
    </div>
    <div class="check">
      <label class="switch">
        <input v-model="v.add_password" type="checkbox" />
        <span class="track"></span><span>{{ tf('settings.addPassword', {}) }}</span>
      </label>
    </div>
    <div class="check">
      <label class="switch">
        <input v-model="v.add_mfa" type="checkbox" />
        <span class="track"></span><span>{{ tf('settings.addMFA', {}) }}</span>
      </label>
    </div>
  </div>

  <div class="row">
    <button class="btn-primary" @click="save">{{ tf('settings.save', {}) }}</button>
    <span v-if="msg" :style="{ color: bad ? 'var(--danger)' : 'var(--ok)' }">{{ msg }}</span>
  </div>
</template>
