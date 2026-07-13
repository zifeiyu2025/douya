<template>
  <div v-if="searching" class="search-status">
    <n-spin size="small" />
    <span>{{ query ? `正在搜索: ${query}` : '正在搜索...' }}</span>
  </div>
  <div v-else-if="resultItems.length > 0" class="search-results-block">
    <div class="search-results-header" @click="toggleExpand">
      <n-icon size="18" :class="{ rotated: expanded }">
        <ChevronForwardOutline />
      </n-icon>
      <n-icon size="16"><SearchOutline /></n-icon>
      <span>参考来源</span>
      <span class="search-results-count">{{ resultItems.length }} 条结果</span>
    </div>
    <div v-if="expanded" class="search-results-content">
      <a
        v-for="item in resultItems"
        :key="item.url"
        :href="safeUrl(item.url)"
        :target="isSafeUrl(item.url) ? '_blank' : undefined"
        :rel="isSafeUrl(item.url) ? 'noopener noreferrer' : undefined"
        class="search-result-item"
        :class="{ 'unsafe-url': !isSafeUrl(item.url) }"
        @click.prevent="openExternal(item.url)"
      >
        <div class="search-result-title">{{ item.title }}</div>
        <div class="search-result-snippet">{{ item.snippet }}</div>
        <div class="search-result-url">{{ item.url }}</div>
      </a>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { NIcon, NSpin } from 'naive-ui'
import { ChevronForwardOutline, SearchOutline } from '@vicons/ionicons5'
import { isSafeUrl } from '../utils/lightSanitize'
// F-1.13：openExternal 抽取到 utils/externalLink.ts，消除两处重复定义
import { openExternal } from '../utils/externalLink'

interface SearchResultItem {
  title: string
  url: string
  snippet: string
}

const props = defineProps<{
  searching: boolean
  results: string
  defaultExpanded?: boolean
  query?: string
}>()

const expanded = ref(props.defaultExpanded ?? false)
const userExpanded = ref(false)
let autoCollapseTimer: ReturnType<typeof setTimeout> | null = null

/**
 * 校验搜索结果 URL 安全性：仅允许 http:// 或 https:// 协议
 * 防止 javascript: / data: 等伪协议导致的点击 XSS
 */
function safeUrl(url: string): string {
  return isSafeUrl(url) ? url : '#'
}

/* openExternal 已抽取到 utils/externalLink.ts */

const resultItems = computed<SearchResultItem[]>(() => {
  if (!props.results) return []
  try {
    let parsed: any
    if (typeof props.results === 'string') {
      parsed = JSON.parse(props.results)
    } else {
      parsed = props.results
    }
    if (Array.isArray(parsed)) return parsed
    if (parsed.results && Array.isArray(parsed.results)) return parsed.results
  } catch (e) {
    return []
  }
  return []
})

function toggleExpand() {
  expanded.value = !expanded.value
  clearAutoCollapseTimer()
  if (expanded.value) {
    // 用户手动展开，不自动收缩
    userExpanded.value = true
  } else {
    userExpanded.value = false
  }
}

function scheduleAutoCollapse() {
  clearAutoCollapseTimer()
  if (expanded.value && !props.searching && !userExpanded.value) {
    autoCollapseTimer = setTimeout(() => {
      expanded.value = false
    }, 5000)
  }
}

function clearAutoCollapseTimer() {
  if (autoCollapseTimer) {
    clearTimeout(autoCollapseTimer)
    autoCollapseTimer = null
  }
}

watch(() => props.searching, (searching) => {
  if (!searching && expanded.value && !userExpanded.value) {
    scheduleAutoCollapse()
  }
})

onMounted(() => {
  if (props.defaultExpanded) {
    expanded.value = true
    if (!props.searching) {
      scheduleAutoCollapse()
    }
  }
})

onUnmounted(() => {
  clearAutoCollapseTimer()
})
</script>

<style scoped>
.search-results-block {
  margin-bottom: 12px;
  border-radius: var(--border-radius-md);
  border: 1px solid var(--border-color);
  overflow: hidden;
  background: var(--bg-secondary);
}

.search-results-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  cursor: pointer;
  background: var(--bg-hover);
  font-size: 13px;
  color: var(--text-secondary);
  user-select: none;
  transition: all var(--transition-fast);
  font-weight: 500;
}

.search-results-header:hover {
  background: var(--bg-active);
}

.search-results-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-muted);
}

/* .n-icon 和 .n-icon.rotated 已在 style.css 全局定义 */

.search-results-content {
  padding: 12px;
  border-top: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.search-result-item {
  display: block;
  padding: 10px 12px;
  border-radius: var(--border-radius-sm);
  text-decoration: none;
  transition: all var(--transition-fast);
}

.search-result-item:hover {
  background: var(--bg-hover);
  transform: translateX(2px);
}

.search-result-item.unsafe-url {
  cursor: not-allowed;
  opacity: 0.6;
  pointer-events: none;
}

.search-result-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--accent-primary);
  margin-bottom: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.search-result-snippet {
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.6;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.search-result-url {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
