<script setup lang="ts">
import { onMounted, ref } from 'vue'

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
  region: 'US',
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
const regions = ref<string[]>(['US'])

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
    msg.value = (await r.json().catch(() => ({ error: '保存失败' }))).error
    return
  }
  v.value = await r.json()
  key.value = ''
  proxy.value = ''
  msg.value = '已保存'
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
      <h1>设置</h1>
      <p>密钥与代理只存在本机，接口返回打码</p>
    </div>
  </div>

  <div class="card">
    <h2>接码平台</h2>
    <label class="f">API Key</label>
    <input
      v-model="key"
      :placeholder="v.has_api_key ? v.api_key_masked : '粘贴 API Key'"
      @paste="rememberSoon"
      @change="remember"
    />
    <p class="hint">留空则沿用已保存的值</p>

    <label class="f">代理（socks5://用户-region-XX-sid-XXXX-密码@主机:端口）</label>
    <input v-model="proxy" :placeholder="v.proxy_host || 'socks5://user-region-US-sid-abc:pass@host:3010'" @change="remember" />
    <p class="hint">多余空格、换行、全角冒号会自动清掉；不写协议自动补 socks5h://</p>

    <label class="f">出口地区</label>
    <select v-model="v.region">
      <option v-for="r in regions" :key="r" :value="r">{{ r }}</option>
    </select>
    <p class="hint">写入代理串的 region-XX 段，工作台可直接切换</p>
  </div>

  <div class="card">
    <h2>默认行为</h2>
    <div class="grid cols-3">
      <div>
        <label class="f">默认数量</label>
        <input v-model.number="v.default_jobs" type="number" min="1" />
      </div>
      <div>
        <label class="f">默认间隔（秒）</label>
        <input v-model.number="v.default_wait" type="number" min="0" />
      </div>
      <div>
        <label class="f">邮箱域名</label>
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
        <span class="track"></span><span>开户时写入随机密码</span>
      </label>
    </div>
    <div class="check">
      <label class="switch">
        <input v-model="v.add_mfa" type="checkbox" />
        <span class="track"></span><span>开户后自动投双因子队列</span>
      </label>
    </div>
  </div>

  <div class="row">
    <button class="btn-primary" @click="save">保存</button>
    <span v-if="msg" :style="{ color: bad ? 'var(--danger)' : 'var(--ok)' }">{{ msg }}</span>
  </div>
</template>
