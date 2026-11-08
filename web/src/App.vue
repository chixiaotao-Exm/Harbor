<script setup lang="ts">
import { onMounted, ref } from 'vue'

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
          <div class="brand-meta">批量开户工作台</div>
        </div>
      </div>
      <nav class="nav">
        <div class="nav-group">运行</div>
        <RouterLink to="/" exact-active-class="active">工作台</RouterLink>
        <RouterLink to="/async" active-class="active">
          异步任务
          <span class="pill">Q1–Q5</span>
        </RouterLink>
        <div class="nav-group">数据</div>
        <RouterLink to="/accounts" active-class="active">
          账号
          <span class="pill">{{ counts.mfa + counts.plain }}</span>
        </RouterLink>
        <div class="nav-group">配置</div>
        <RouterLink to="/settings" active-class="active">设置</RouterLink>
      </nav>
      <div class="side-foot">
        已绑双因子 {{ counts.mfa }} · 未绑 {{ counts.plain }}
      </div>
    </aside>
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>
