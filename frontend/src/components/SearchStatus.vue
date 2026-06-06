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
        v-for="(item, idx) in resultItems"
        :key="idx"
        :href="item.url"
        target="_blank"
        rel="noopener noreferrer"
        class="search-result-item"
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

.n-icon.rotated {
  transform: rotate(90deg);
}

.n-icon {
  transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

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

.search-result-title {
  font-size: 13px;
  font-weight: 500;
  color: #576b95;
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
