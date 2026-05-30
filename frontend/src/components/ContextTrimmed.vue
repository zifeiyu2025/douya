<template>
  <Transition name="trim-fade">
    <div v-if="data" class="context-trimmed-notice" @click="expanded = !expanded">
      <div class="trimmed-header">
        <span class="trimmed-icon">✂️</span>
        <span class="trimmed-text">{{ headerText }}</span>
        <n-icon size="16" :class="{ rotated: expanded }">
          <ChevronForwardOutline />
        </n-icon>
      </div>
      <div v-if="expanded" class="trimmed-detail">
        <span v-if="data.reason === 'exceed_context_size'">
          对话内容超出模型上下文长度（{{ data.promptTokens }} / {{ data.contextSize }} tokens），已自动裁剪早期对话以继续生成
        </span>
        <span v-else>
          对话历史较长，已自动裁剪早期内容以确保当前对话正常进行
        </span>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline } from '@vicons/ionicons5'

interface TrimmedData {
  reason: string
  promptTokens?: number
  contextSize?: number
  messagesAfter?: number
}

const props = defineProps<{
  data: TrimmedData | null
}>()

const expanded = ref(false)

const headerText = computed(() => {
  if (!props.data) return ''
  if (props.data.reason === 'exceed_context_size') {
    return '上下文已自动裁剪'
  }
  return '已裁剪早期对话'
})
</script>

<style scoped>
.context-trimmed-notice {
  margin-bottom: 10px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--warning-color, #f0a020) 30%, transparent);
  overflow: hidden;
  background: color-mix(in srgb, var(--warning-color, #f0a020) 8%, var(--bg-secondary, #f5f5f5));
  cursor: pointer;
  user-select: none;
}

.trimmed-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  font-size: 12.5px;
  color: var(--text-secondary);
  font-weight: 500;
  transition: background 0.2s;
}

.trimmed-header:hover {
  background: color-mix(in srgb, var(--warning-color, #f0a020) 12%, transparent);
}

.trimmed-icon {
  font-size: 13px;
  flex-shrink: 0;
}

.trimmed-text {
  flex: 1;
}

.n-icon.rotated {
  transform: rotate(90deg);
}

.n-icon {
  transition: transform 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  flex-shrink: 0;
}

.trimmed-detail {
  padding: 8px 12px 10px;
  border-top: 1px solid color-mix(in srgb, var(--warning-color, #f0a020) 20%, transparent);
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.6;
}

.trim-fade-enter-active {
  transition: opacity 0.4s ease, transform 0.4s ease;
}

.trim-fade-leave-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}

.trim-fade-enter-from {
  opacity: 0;
  transform: translateY(-6px);
}

.trim-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
