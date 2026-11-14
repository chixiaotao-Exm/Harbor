<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { t, tf, lang } from '../i18n'

export type Region = { code: string; zh: string; en: string }

const props = defineProps<{
  modelValue: string
  regions: Region[]
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', code: string): void
  (e: 'select', code: string): void
}>()

const open = ref(false)
const query = ref('')
const active = ref(0)
const root = ref<HTMLElement | null>(null)
const inputEl = ref<HTMLInputElement | null>(null)
const listEl = ref<HTMLElement | null>(null)

const selected = computed(() => props.regions.find((r) => r.code === props.modelValue))

function labelOf(r: Region) {
  const name = lang.value === 'zh' ? r.zh : r.en
  return `${name} · ${r.code}`
}

const selectedLabel = computed(() => (selected.value ? labelOf(selected.value) : props.modelValue))

// 匹配代码、中文名、英文名；不分大小写。
// 排序按相关度：代码全等 > 代码前缀 > 名称前缀 > 任意包含，同级保持原顺序。
function rank(r: Region, q: string): number {
  const code = r.code.toLowerCase()
  const zh = r.zh.toLowerCase()
  const en = r.en.toLowerCase()
  if (code === q) return 0
  if (code.startsWith(q)) return 1
  if (zh.startsWith(q) || en.startsWith(q)) return 2
  if (code.includes(q) || zh.includes(q) || en.includes(q)) return 3
  return -1
}

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.regions
  const scored: { r: Region; rank: number; i: number }[] = []
  props.regions.forEach((r, i) => {
    const k = rank(r, q)
    if (k >= 0) scored.push({ r, rank: k, i })
  })
  scored.sort((a, b) => a.rank - b.rank || a.i - b.i)
  return scored.map((s) => s.r)
})

const optionId = (i: number) => `region-opt-${i}`

function openList() {
  if (props.disabled) return
  open.value = true
  active.value = 0
  nextTick(() => {
    listEl.value?.scrollTo({ top: 0 })
  })
}

function closeList() {
  open.value = false
  query.value = ''
}

function pick(r: Region) {
  emit('update:modelValue', r.code)
  emit('select', r.code)
  closeList()
  inputEl.value?.blur()
}

function move(delta: number) {
  if (!open.value) {
    openList()
    return
  }
  const n = filtered.value.length
  if (!n) return
  active.value = (active.value + delta + n) % n
  nextTick(() => {
    const el = document.getElementById(optionId(active.value))
    el?.scrollIntoView({ block: 'nearest' })
  })
}

function onKeydown(e: KeyboardEvent) {
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      move(1)
      break
    case 'ArrowUp':
      e.preventDefault()
      move(-1)
      break
    case 'Enter':
      if (open.value && filtered.value[active.value]) {
        e.preventDefault()
        pick(filtered.value[active.value])
      }
      break
    case 'Escape':
      if (open.value) {
        e.preventDefault()
        closeList()
      }
      break
    case 'Tab':
      closeList()
      break
  }
}

// 点到外面就收起
function onDocPointerDown(e: PointerEvent) {
  if (!open.value) return
  if (root.value && !root.value.contains(e.target as Node)) closeList()
}

watch(filtered, () => {
  active.value = 0
})

// 结果条数用 live region 播报，打字时防抖，别让读屏每键都念
const liveCount = ref(0)
let countTimer: number | null = null
watch(
  () => filtered.value.length,
  (n) => {
    if (countTimer) clearTimeout(countTimer)
    countTimer = window.setTimeout(() => (liveCount.value = n), 400)
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (countTimer) clearTimeout(countTimer)
})

onMounted(() => document.addEventListener('pointerdown', onDocPointerDown))
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocPointerDown))
</script>

<template>
  <div ref="root" class="rs" :class="{ open, disabled }">
    <input
      ref="inputEl"
      class="rs-input"
      role="combobox"
      :aria-expanded="open"
      :aria-controls="'region-listbox'"
      :aria-activedescendant="open && filtered.length ? optionId(active) : undefined"
      aria-autocomplete="list"
      :placeholder="selectedLabel"
      :value="query"
      :disabled="disabled"
      @focus="openList"
      @input="(e) => { query = (e.target as HTMLInputElement).value; openList() }"
      @keydown="onKeydown"
    />
    <span class="rs-caret" aria-hidden="true">▾</span>

    <ul v-if="open" id="region-listbox" ref="listEl" class="rs-list" role="listbox">
      <li
        v-for="(r, i) in filtered"
        :id="optionId(i)"
        :key="r.code"
        class="rs-opt"
        :class="{ active: i === active, chosen: r.code === modelValue }"
        role="option"
        :aria-selected="r.code === modelValue"
        @mouseenter="active = i"
        @mousedown.prevent="pick(r)"
      >
        {{ labelOf(r) }}
      </li>
      <li v-if="!filtered.length" class="rs-empty">{{ t('region.none') }}</li>
    </ul>

    <p v-if="open" class="rs-count" aria-live="polite">{{ tf('region.count', { n: liveCount }) }}</p>
  </div>
</template>
