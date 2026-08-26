<template>
  <Transition name="trunc-fade">
    <div v-if="visible" class="output-truncated-notice">
      <span class="trunc-dot" aria-hidden="true"></span>
      <span class="trunc-text">回复因达到最大输出长度被截断，可发送「继续」让 AI 接着生成</span>
    </div>
  </Transition>
</template>

<script setup lang="ts">
defineProps<{
  visible: boolean
}>()
</script>

<style scoped>
/* 视觉风格与 ContextTrimmed 保持一致：左缘赭石细线 + 印章方点信息行
 * 注：语义别名 --warning-color 在 tokens.css 中指向未定义变量而失效，
 * 且原 fallback 为硬编码 hex，故直接使用真实令牌 --accent-warning（赭石） */
.output-truncated-notice {
  margin-bottom: 10px;
  border-left: 2px solid color-mix(in srgb, var(--accent-warning) 55%, transparent);
  background: transparent;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px 5px 10px;
  font-size: 12.5px;
  color: var(--text-secondary);
  user-select: none;
}

.trunc-dot {
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  background: var(--accent-warning);
}

.trunc-text {
  flex: 1;
  line-height: 1.5;
}

.trunc-fade-enter-active {
  transition:
    opacity 0.4s ease,
    transform 0.4s ease;
}

.trunc-fade-leave-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s ease;
}

.trunc-fade-enter-from {
  opacity: 0;
  transform: translateY(-6px);
}

.trunc-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
