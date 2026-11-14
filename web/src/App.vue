<script setup lang="ts">
import { onMounted, ref } from 'vue'

import { lang, tf, toggleLang } from './i18n'

const counts = ref({ mfa: 0, plain: 0 })
const ready = ref(false)

async function refresh() {
  try {
    const r = await fetch('/api/accounts/stats')
    if (r.ok) counts.value = await r.json()
  } catch {
    /* 后端没起也不用报错 */
  } finally {
    // 首帧数据到位才撤掉骨架，避免白屏
    ready.value = true
  }
}

onMounted(() => {
  refresh()
  setInterval(refresh, 5000)
})
</script>

<template>
  <div v-if="!ready" id="boot">
    <div class="b-side"></div>
    <div class="b-main">
      <div class="b-bar s"></div>
      <div class="b-card"></div>
      <div class="b-card"></div>
    </div>
  </div>
  <div v-else class="shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark">H</div>
        <div>
          <div class="brand-name">Harbor</div>
          <div class="brand-meta">{{ tf('app.subtitle', {}) }}</div>
        </div>
      </div>
      <nav class="nav">
        <div class="nav-group">{{ tf('nav.group.run', {}) }}</div>
        <RouterLink to="/" exact-active-class="active">{{ tf('nav.workbench', {}) }}</RouterLink>
        <RouterLink to="/async" active-class="active">
          {{ tf('nav.async', {}) }}
          <span class="pill">Q1–Q5</span>
        </RouterLink>
        <div class="nav-group">{{ tf('nav.group.data', {}) }}</div>
        <RouterLink to="/accounts" active-class="active">
          {{ tf('nav.accounts', {}) }}
          <span class="pill">{{ counts.mfa + counts.plain }}</span>
        </RouterLink>
        <div class="nav-group">{{ tf('nav.group.config', {}) }}</div>
        <RouterLink to="/settings" active-class="active">{{ tf('nav.settings', {}) }}</RouterLink>
      </nav>
      <div class="side-foot">
        <button class="lang-btn" :title="tf('lang.title', {})" @click="toggleLang">
          {{ lang === 'zh' ? 'EN' : '中文' }}
        </button>
        <span>{{ tf('nav.foot', { mfa: counts.mfa, plain: counts.plain }) }}</span>
      </div>
    </aside>
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>
