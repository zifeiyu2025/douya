<template>
  <Transition name="trim-fade">
    <div
      v-if="data && data.reason === 'exceed_context_size'"
      class="context-trimmed-notice"
      @click="expanded = !expanded"
    >
      <div class="trimmed-header">
        <span class="trimmed-dot" aria-hidden="true"></span>
        <span class="trimmed-text">上下文已自动裁剪</span>
        <n-icon size="14" :class="{ rotated: expanded }">
          <ChevronForwardOutline />
        </n-icon>
      </div>
      <div v-if="expanded" class="trimmed-detail">
        <span>
          对话内容超出模型上下文长度（{{ data.promptTokens }} /
          {{ data.contextSize }} tokens），已自动裁剪早期对话以继续生成
        </span>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline } from '@vicons/ionicons5'

interface TrimmedData {
  reason: string
  promptTokens?: number
  contextSize?: number
  messagesAfter?: number
}

defineProps<{
  data: TrimmedData | null
}>()

const expanded = ref(false)
</script>

<style scoped>
/* 书房风·信息行：左缘赭石细线 + 印章方点，无色块底
 * 注：语义别名 --warning-color 在 tokens.css 中指向未定义变量而失效，
 * 且原 fallback 为硬编码 hex，故直接使用真实令牌 --accent-warning（赭石） */
.context-trimmed-notice {
  margin-bottom: 10px;
  border-left: 2px solid color-mix(in srgb, var(--accent-warning) 55%, transparent);
  background: transparent;
  cursor: pointer;
  user-select: none;
}

.trimmed-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px 4px 10px;
  font-size: 12.5px;
  color: var(--text-secondary);
  transition: background 0.2s;
}

/* 悬浮反馈走背景色阶 */
.trimmed-header:hover {
  background: color-mix(in srgb, var(--text-primary) 4%, transparent);
}

.trimmed-dot {
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  background: var(--accent-warning);
}

.trimmed-text {
  flex: 1;
}

/* .n-icon 和 .n-icon.rotated 已在 style.css 全局定义 */

/* 展开详情：与标题文字左缘对齐，上缘 hairline 分隔 */
.trimmed-detail {
  padding: 6px 8px 8px 21px;
  margin-left: 10px;
  border-top: 1px solid var(--border-light);
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.6;
}

.trim-fade-enter-active {
  transition:
    opacity 0.4s ease,
    transform 0.4s ease;
}

.trim-fade-leave-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s ease;
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
