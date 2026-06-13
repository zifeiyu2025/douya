<template>
  <div class="think-block">
    <div class="think-block-header" @click="expanded = !expanded">
      <n-icon size="18" :class="{ rotated: expanded }">
        <ChevronForwardOutline />
      </n-icon>
      <n-icon size="16" class="think-icon"><BulbOutline /></n-icon>
      <span v-if="isThinking" class="think-status thinking">正在思考<span class="thinking-dots"><span>.</span><span>.</span><span>.</span></span></span>
      <span v-else-if="duration > 0" class="think-status done">已思考(用时{{ formattedDuration }})</span>
      <span v-else>思考过程</span>
    </div>
    <div v-if="expanded" class="think-block-content">
      <div class="think-block-content-inner markdown-body" v-html="renderedContent" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline, BulbOutline } from '@vicons/ionicons5'
import { renderMarkdown } from '../utils/markdown'
import { cleanStreamingContent } from '../utils/streaming'

const props = defineProps<{
  content: string
  defaultExpanded?: boolean
  isThinking?: boolean
  duration?: number
}>()

const expanded = ref(props.defaultExpanded ?? false)

const cleanedContent = computed(() => {
  if (!props.content) return ''
  return props.content
    .split('\n')
    .filter((line: string) => {
      const trimmed = line.trim()
      return !trimmed.startsWith('<tool_call') && !trimmed.startsWith('</tool_call')
    })
    .join('\n')
    .trim()
})

// remark 是异步的，使用 ref + watch 模式
const renderedContent = ref('')

watch(cleanedContent, async (newVal) => {
  if (!newVal) {
    renderedContent.value = ''
    return
  }
  try {
    renderedContent.value = await renderMarkdown(cleanStreamingContent(newVal))
  } catch (_) {
    renderedContent.value = newVal
  }
}, { immediate: true })

const duration = computed(() => props.duration ?? 0)

const formattedDuration = computed(() => {
  const d = duration.value
  if (d <= 0) return '0秒'
  if (d < 60) return `${d.toFixed(1)}秒`
  const min = Math.floor(d / 60)
  const sec = Math.round(d % 60)
  return sec > 0 ? `${min}分${sec}秒` : `${min}分`
})
</script>

<style scoped>
.n-icon.rotated {
  transform: rotate(90deg);
  transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}
.n-icon {
  transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.think-block {
  margin-bottom: 12px;
}

.think-block-header {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  user-select: none;
  padding: 4px 0;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 500;
}

.think-block-header:hover {
  color: var(--text-primary);
}

.think-icon {
  color: var(--accent-warning);
}

.think-status {
  font-size: 13px;
}

.think-status.thinking {
  color: var(--accent-warning);
}

.think-status.done {
  color: var(--text-secondary);
}

.think-block-content {
  margin-top: 8px;
  padding: 12px 16px;
  background: var(--bg-tertiary);
  border-radius: var(--border-radius-md);
  border: 1px solid var(--border-color);
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.65;
}

.thinking-dots span {
  animation: thinkingDot 1.4s infinite;
  opacity: 0;
}
.thinking-dots span:nth-child(1) {
  animation-delay: 0s;
}
.thinking-dots span:nth-child(2) {
  animation-delay: 0.2s;
}
.thinking-dots span:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes thinkingDot {
  0%, 20% { opacity: 0; }
  50% { opacity: 1; }
  80%, 100% { opacity: 0; }
}
</style>
