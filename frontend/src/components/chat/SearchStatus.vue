<template>
  <!-- 搜索中：印章方点脉冲 + 一行小注，收敛为 hairline 信息行 -->
  <div v-if="searching" class="search-status">
    <span class="search-pulse-dot" aria-hidden="true"></span>
    <span class="search-status-text">{{ query ? `正在搜索: ${query}` : '正在搜索...' }}</span>
  </div>
  <div v-else-if="error" class="search-error-block">
    <span class="search-error-dot" aria-hidden="true"></span>
    <span class="search-error-text">{{ error }}</span>
  </div>
  <div v-else-if="resultItems.length > 0" class="search-results-block">
    <!-- 折叠头：§ 章节号 + 衬线体标签，如目录条目 -->
    <div class="search-results-header" @click="toggleExpand">
      <n-icon size="14" :class="{ rotated: expanded }">
        <ChevronForwardOutline />
      </n-icon>
      <span class="search-results-mark" aria-hidden="true">§</span>
      <span class="search-results-label">参考来源</span>
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
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline } from '@vicons/ionicons5'
import { isSafeUrl } from '../../utils/lightSanitize'
// openExternal 抽取到 utils/externalLink.ts，消除两处重复定义
import { openExternal } from '../../utils/externalLink'
import type { SearchResultItem } from '../../types/search'

const props = defineProps<{
  searching: boolean
  results: string
  defaultExpanded?: boolean
  query?: string
  error?: string
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
  } catch {
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

watch(
  () => props.searching,
  searching => {
    if (!searching && expanded.value && !userExpanded.value) {
      scheduleAutoCollapse()
    }
  }
)

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
/* 书房风·信息行语汇：
 * 参考来源块以上下两条 hairline 细线圈定范围，无卡片底色；
 * 条目走左缘细线缩进列，hover 苔绿落点 */
.search-results-block {
  margin-bottom: 12px;
  border-top: 1px solid var(--border-light);
  border-bottom: 1px solid var(--border-light);
}

/* 搜索中状态行：朱砂方点脉冲 + 小字注记 */
.search-status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding: 4px 0;
  font-size: 13px;
  color: var(--text-secondary);
}

.search-status-text {
  font-family: var(--font-display);
  letter-spacing: 0.02em;
}

.search-pulse-dot {
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  background: var(--seal-color);
  animation: search-dot-pulse 1.4s ease-in-out infinite;
}

@keyframes search-dot-pulse {
  0%,
  100% {
    opacity: 0.3;
  }
  50% {
    opacity: 1;
  }
}

/* 搜索错误提示行：左缘朱砂细线 + 错误色文字，无色块底
 * 注：语义别名 --error-color 在 tokens.css 中指向未定义变量而失效，
 * 故此处直接使用真实令牌 --accent-danger（朱砂） */
.search-error-block {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding: 6px 0 6px 12px;
  border-left: 2px solid var(--accent-danger);
  font-size: 13px;
  color: var(--accent-danger);
}

.search-error-dot {
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  background: var(--accent-danger);
}

.search-error-text {
  line-height: 1.5;
  word-break: break-word;
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
  padding: 0 0 8px;
  display: flex;
  flex-direction: column;
}

/* 条目：左缘 hairline 缩进列，相邻条目以细分隔线区隔 */
.search-result-item {
  display: block;
  padding: 8px 4px 8px 14px;
  border-left: 1px solid var(--border-light);
  text-decoration: none;
  transition:
    background-color var(--transition-fast),
    border-color var(--transition-fast);
}

.search-result-item + .search-result-item {
  border-top: 1px solid var(--border-light);
}

/* hover：淡背景阶 + 左缘线转苔绿，示意"落笔在此" */
.search-result-item:hover {
  background: color-mix(in srgb, var(--text-primary) 4%, transparent);
  border-left-color: var(--accent-primary);
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
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.search-result-url {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
