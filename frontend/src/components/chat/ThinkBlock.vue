<template>
  <div class="think-block" :class="{ 'is-thinking': isThinking }">
    <div class="think-block-header" @click="expanded = !expanded">
      <n-icon size="15" class="think-chevron" :class="{ rotated: expanded }">
        <ChevronForwardOutline />
      </n-icon>
      <n-icon size="14" class="think-icon"><BulbOutline /></n-icon>
      <span v-if="isThinking" class="think-status thinking">
        深度思考中
        <span class="thinking-dots">
          <span>.</span>
          <span>.</span>
          <span>.</span>
        </span>
      </span>
      <span v-else-if="safeDuration > 0" class="think-status done">
        深度思考完成(用时{{ formattedDuration }})
      </span>
      <span v-else>深度思考过程</span>
    </div>
    <div v-if="expanded" class="think-block-content">
      <!-- 左边缘脉络流光：仅思考中显示，沿边缘上下流动 -->
      <div class="think-vein" aria-hidden="true"></div>
      <!--
        实时格式化渲染（与正文 useMorphRender 一致）：
        - 流式中（isThinking=true）：stable/unstable 分块缓存 + 实时 markdown 格式
        - 流式结束（isThinking=false）：finalizeRender 全量渲染确保完整
        - 历史消息：bind 后自动渲染一次
      -->
      <div ref="containerRef" class="think-block-content-inner markdown-body"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline, BulbOutline } from '@vicons/ionicons5'
import { useMorphRender } from '../../composables/useMorphRender'

const props = defineProps<{
  content: string
  defaultExpanded?: boolean
  isThinking?: boolean
  duration?: number
}>()

const expanded = ref(props.defaultExpanded ?? false)

/**
 * 清理内容：过滤思考内容里偶发的工具调用标签行
 * （这些是模型误输出，不应展示给用户）
 */
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

// 使用 useMorphRender 实现实时格式化渲染（stable/unstable 分块缓存）
const { containerRef, bind, finalizeRender } = useMorphRender()
bind(() => cleanedContent.value)

// isThinking 从 true 切到 false 时触发最终渲染（全量渲染确保完整）
watch(
  () => props.isThinking,
  thinking => {
    if (!thinking) {
      finalizeRender()
    }
  }
)

const safeDuration = computed(() => props.duration ?? 0)

const formattedDuration = computed(() => {
  const d = safeDuration.value
  if (d <= 0) return '0秒'
  if (d < 60) return `${d.toFixed(1)}秒`
  const min = Math.floor(d / 60)
  const sec = Math.round(d % 60)
  return sec > 0 ? `${min}分${sec}秒` : `${min}分`
})
</script>

<style scoped>
/* 书房风·朱砂折页：
 * 折叠标题以 § 章节号 + 衬线体呈现；思考正文直接落纸无底色，
 * 仅以左缘一道朱砂细线作"折页线"划分层级 */

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
  transition: color 0.2s ease;
}

.think-block-header:hover {
  color: var(--text-primary);
}

/* 状态文字：衬线体呼应书页标题气质 */
.think-status {
  font-family: var(--font-display);
  font-size: 13px;
  letter-spacing: 0.02em;
}

.think-status.thinking {
  color: var(--seal-color);
}

.think-status.done {
  color: var(--text-secondary);
}

/* 折叠指示箭头：位于行首，展开时顺时针转 90° 指向下方 */
.think-chevron {
  flex-shrink: 0;
  color: var(--text-muted);
  transition:
    transform 0.2s ease,
    color 0.2s ease;
}

.think-chevron.rotated {
  transform: rotate(90deg);
}

.think-block-header:hover .think-chevron {
  color: var(--accent-primary);
}

/* 灯泡图标：朱砂印色点题 */
.think-icon {
  flex-shrink: 0;
  color: var(--seal-color);
}

/* 折页内容区：透明落纸 + 左缘朱砂折页线（常态半透，克制） */
.think-block-content {
  margin-top: 4px;
  padding: 2px 0 2px 14px;
  background: transparent;
  border-left: 2px solid color-mix(in srgb, var(--seal-color) 35%, transparent);
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.65;
}

/* 思考中：折页线呼吸明灭（纯色透明度脉动，无渐变、无光晕、无发光阴影） */
.think-block.is-thinking .think-block-content {
  border-left-color: var(--seal-color);
  animation: think-seal-breathe 2.2s ease-in-out infinite;
}

@keyframes think-seal-breathe {
  0%,
  100% {
    border-left-color: color-mix(in srgb, var(--seal-color) 30%, transparent);
  }
  50% {
    border-left-color: var(--seal-color);
  }
}

@media (prefers-reduced-motion: reduce) {
  .think-block.is-thinking .think-block-content {
    animation: none;
    border-left-color: var(--seal-color);
  }
  .think-chevron {
    transition: none;
  }
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
  0%,
  20% {
    opacity: 0;
  }
  50% {
    opacity: 1;
  }
  80%,
  100% {
    opacity: 0;
  }
}
</style>
