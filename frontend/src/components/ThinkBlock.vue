<template>
  <div class="think-block" :class="{ 'is-thinking': isThinking }">
    <div class="think-block-header" @click="expanded = !expanded">
      <n-icon size="18" :class="{ rotated: expanded }">
        <ChevronForwardOutline />
      </n-icon>
      <n-icon size="16" class="think-icon"><BulbOutline /></n-icon>
      <span v-if="isThinking" class="think-status thinking">正在思考<span class="thinking-dots"><span>.</span><span>.</span><span>.</span></span></span>
      <span v-else-if="safeDuration > 0" class="think-status done">已思考(用时{{ formattedDuration }})</span>
      <span v-else>思考过程</span>
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
      <div
        ref="containerRef"
        class="think-block-content-inner markdown-body"
      ></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline, BulbOutline } from '@vicons/ionicons5'
import { useMorphRender } from '../composables/useMorphRender'

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
watch(() => props.isThinking, (thinking) => {
  if (!thinking) {
    finalizeRender()
  }
})

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
/* .n-icon 和 .n-icon.rotated 已在 style.css 全局定义 */

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
  color: var(--accent-think);
}

.think-status {
  font-size: 13px;
}

.think-status.thinking {
  color: var(--accent-think);
}

.think-status.done {
  color: var(--text-secondary);
}

.think-block-content {
  margin-top: 8px;
  padding: 12px 16px 12px 18px;
  background: var(--bg-tertiary);
  border-radius: var(--border-radius-md);
  border: 1px solid var(--border-color);
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.65;
  position: relative;
  overflow: hidden;
  transition: border-color 0.3s ease, box-shadow 0.3s ease, background 0.3s ease;
}

/* 内容层提升至伪元素之上，确保文字始终可读 */
.think-block-content-inner {
  position: relative;
  z-index: 2;
}

/* 思考中：整块能量场
 * - 左侧背景渐隐出能量底色
 * - 边框微微偏绿 + 多层外辉光，营造科幻 HUD 感
 */
.think-block.is-thinking .think-block-content {
  border-color: color-mix(in srgb, var(--accent-think) 38%, var(--border-color));
  background:
    linear-gradient(
      90deg,
      color-mix(in srgb, var(--accent-think) 7%, var(--bg-tertiary)) 0%,
      var(--bg-tertiary) 55%
    );
  box-shadow:
    inset 1px 0 0 color-mix(in srgb, var(--accent-think) 55%, transparent),
    0 0 0 1px color-mix(in srgb, var(--accent-think) 10%, transparent),
    0 0 18px color-mix(in srgb, var(--accent-think) 14%, transparent);
}

/* 注册可动画的角度变量（Chromium 85+ / 现代 WebView2 支持）
 * 不支持时 conic-gradient 仍以初始值渲染为静态光带，不影响可见性 */
@property --think-angle {
  syntax: '<angle>';
  initial-value: 0deg;
  inherits: false;
}

/* 左边缘脉络流光条
 * - 默认（思考完成）：静态淡色条作为视觉装饰
 * - 思考中（is-thinking）：化为环绕整块边框的能量光带，沿边框顺时针流动
 */
.think-vein {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 2px;
  background: var(--border-color);
  opacity: 0.6;
  transition: opacity 0.3s ease;
  z-index: 1;
}

.think-block.is-thinking .think-vein {
  /* 由左侧脉络条展开为覆盖整块的边框光带 */
  left: 0;
  right: 0;
  top: 0;
  bottom: 0;
  width: auto;
  padding: 1px; /* 光带宽度 */
  background: conic-gradient(
    from var(--think-angle),
    transparent 0deg,
    transparent 210deg,
    color-mix(in srgb, var(--accent-think-glow) 18%, transparent) 270deg,
    color-mix(in srgb, var(--accent-think-glow) 70%, transparent) 335deg,
    color-mix(in srgb, var(--accent-think-glow) 55%, white) 352deg,
    transparent 360deg
  );
  /* mask 镂空技巧：只显示 padding 圈（即边框那一圈），内部镂空不遮挡内容 */
  -webkit-mask:
    linear-gradient(#fff 0 0) content-box,
    linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
          mask-composite: exclude;
  border-radius: inherit;
  opacity: 1;
  filter: drop-shadow(0 0 3px color-mix(in srgb, var(--accent-think-glow) 45%, transparent))
          drop-shadow(0 0 7px color-mix(in srgb, var(--accent-think-glow) 20%, transparent));
  animation: think-rotate 5s linear infinite;
  will-change: --think-angle;
}

/* 左侧辉光散射场：从光柱向右衰减的绿色辉光，呼吸式脉动 */
.think-block.is-thinking .think-block-content::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 64px;
  background: radial-gradient(
    ellipse at left center,
    color-mix(in srgb, var(--accent-think) 20%, transparent) 0%,
    color-mix(in srgb, var(--accent-think) 7%, transparent) 40%,
    transparent 80%
  );
  pointer-events: none;
  animation: vein-breathe 2.4s ease-in-out infinite;
  z-index: 0;
}

@keyframes think-rotate {
  to { --think-angle: 360deg; }
}

@keyframes vein-breathe {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

@media (prefers-reduced-motion: reduce) {
  .think-block.is-thinking .think-vein,
  .think-block.is-thinking .think-block-content::before {
    animation: none;
  }
  .think-block.is-thinking .think-vein {
    background: var(--accent-think);
    -webkit-mask: none;
            mask: none;
    filter: none;
  }
  .think-block.is-thinking .think-block-content::before {
    opacity: 0.8;
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
  0%, 20% { opacity: 0; }
  50% { opacity: 1; }
  80%, 100% { opacity: 0; }
}
</style>
