<template>
  <div class="think-block" :class="{ 'is-thinking': isThinking }">
    <div class="think-block-header" @click="toggle">
      <!-- 大脑图标：位于行首，与工具栏思考开关同源 -->
      <BrainIcon :size="15" class="think-icon" />
      <span v-if="isThinking" class="think-status thinking">
        思考中
        <span class="thinking-dots">
          <span>.</span>
          <span>.</span>
          <span>.</span>
        </span>
      </span>
      <span v-else-if="safeDuration > 0" class="think-status done">
        思考完成（持续了{{ formattedDuration }}）
      </span>
      <span v-else>思考完成</span>
      <!--
        折叠态单行滚动直播（参考 ZCode/DeepSeek 的折中交互）：
        流式期间即使用户收起思考块，标题栏右侧仍单行滚动展示当前思考行，
        两端渐隐遮罩 + 按行刷新，让"实时可见"与"收起克制"并存
      -->
      <span v-if="isThinking && !expanded && previewLine" class="think-live-preview">
        <span
          ref="previewTextRef"
          class="think-live-preview-text"
          :class="{ rolling: !!rollState }"
          :style="rollStyle"
        >
          {{ previewLine }}
        </span>
      </span>
      <!-- 折叠指示箭头：置于行尾（参考 ZCode），展开时顺时针转 90° 指向下方 -->
      <n-icon size="15" class="think-chevron" :class="{ rotated: expanded }">
        <ChevronForwardOutline />
      </n-icon>
    </div>
    <div v-if="expanded" class="think-block-content">
      <!--
        实时格式化渲染（与正文 useMorphRender 一致）：
        - 流式中（isThinking=true）：stable/unstable 分块缓存 + 实时 markdown 格式
        - 流式结束（isThinking=false）：finalizeRender 全量渲染确保完整
        - 历史消息：bind 后自动渲染一次
      -->
      <div ref="scrollRef" class="think-scroll" @scroll="onScroll">
        <div ref="containerRef" class="think-block-content-inner markdown-body"></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { NIcon } from 'naive-ui'
import { ChevronForwardOutline } from '@vicons/ionicons5'
import BrainIcon from '../ui/BrainIcon.vue'
import { useMorphRender } from '../../composables/useMorphRender'

const props = defineProps<{
  content: string
  defaultExpanded?: boolean
  isThinking?: boolean
  duration?: number
}>()

const expanded = ref(props.defaultExpanded ?? false)
// 用户手动点击过折叠/展开后，不再自动收起（尊重用户意图）
const userToggled = ref(false)

function toggle() {
  userToggled.value = true
  expanded.value = !expanded.value
}

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

// isThinking 从 true 切到 false（正文开始输出）时：
// 1. 全量渲染确保完整 2. 自动收起思考块（用户手动操作过则尊重用户）
watch(
  () => props.isThinking,
  (thinking, prev) => {
    if (!thinking) {
      finalizeRender()
      scrollToBottomIfStuck()
      if (prev && cleanedContent.value && !userToggled.value) {
        expanded.value = false
      }
    }
  }
)

// ============ 折叠态单行滚动直播 ============
// 取思考内容最后一个非空行，去掉行首 markdown 记号后作为直播文本
const previewLine = computed(() => {
  if (!props.isThinking) return ''
  const lines = cleanedContent.value.split('\n')
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i]
      .trim()
      .replace(/^[#>*\-+`•\d.)]+\s*/, '')
      .trim()
    if (line) return line.slice(0, 200)
  }
  return ''
})

const previewTextRef = ref<HTMLElement | null>(null)
const rollState = ref<{ shift: number; duration: number } | null>(null)

const rollStyle = computed(() => {
  if (!rollState.value) return undefined
  return {
    '--roll-shift': `${-rollState.value.shift}px`,
    '--roll-duration': `${rollState.value.duration}s`
  }
})

// 超出容器宽度才滚动；速度按文本长度折算（约 24px/s），限幅 3~12s
async function measureRoll() {
  rollState.value = null
  if (!props.isThinking || expanded.value) return
  await nextTick()
  const el = previewTextRef.value
  const parent = el?.parentElement
  if (!el || !parent) return
  const overflow = el.offsetWidth - parent.clientWidth
  if (overflow > 8) {
    const distance = overflow + 16
    rollState.value = {
      shift: distance,
      duration: Math.min(12, Math.max(3, distance / 24))
    }
  }
}

watch(previewLine, measureRoll)
watch(expanded, measureRoll)

// ============ 粘底滚动 + 用户回翻感知 ============
const scrollRef = ref<HTMLElement | null>(null)
const stickBottom = ref(true)

function onScroll() {
  const el = scrollRef.value
  if (!el) return
  // 用户往上翻离底部超过 24px 即视为主动回看，暂停跟随；拉回底部自动恢复
  stickBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 24
}

// 内容渲染发生在 useMorphRender 的 RAF 回调里（晚于本组件的 nextTick），
// 因此用 RAF 排队，保证读到的是本次渲染完成后的高度
function scrollToBottomIfStuck() {
  if (!expanded.value || !stickBottom.value) return
  requestAnimationFrame(() => {
    const el = scrollRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

watch(cleanedContent, () => {
  if (props.isThinking) scrollToBottomIfStuck()
})

watch(expanded, value => {
  if (value && props.isThinking) {
    stickBottom.value = true
    scrollToBottomIfStuck()
  }
})

const safeDuration = computed(() => props.duration ?? 0)

// 耗时始终精确显示，不用"几秒"这类模糊表述
const formattedDuration = computed(() => {
  const d = safeDuration.value
  if (d <= 0) return '0秒'
  if (d < 60) return `${Math.round(d)}秒`
  const min = Math.floor(d / 60)
  const sec = Math.round(d % 60)
  return sec > 0 ? `${min}分${sec}秒` : `${min}分`
})
</script>

<style scoped>
/* 书房风·朱砂折页：
 * 折叠标题以 衬线体 + 朱砂印色呈现；思考正文直接落纸无底色，
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
  flex-shrink: 0;
}

.think-status.thinking {
  color: var(--seal-color);
}

.think-status.done {
  color: var(--text-secondary);
}

/* 折叠指示箭头：位于行尾，展开时常显；收起时 hover 才浮现（降噪） */
.think-chevron {
  flex-shrink: 0;
  margin-left: auto;
  color: var(--text-muted);
  opacity: 0;
  transition:
    transform 0.2s ease,
    color 0.2s ease,
    opacity 0.2s ease;
}

.think-chevron.rotated,
.think-block-header:hover .think-chevron {
  opacity: 1;
}

.think-chevron.rotated {
  transform: rotate(90deg);
}

.think-block-header:hover .think-chevron {
  color: var(--accent-primary);
}

/* 大脑图标：朱砂印色点题；思考中与折页线同呼吸（同步 2.2s 透明度脉动） */
.think-icon {
  flex-shrink: 0;
  color: var(--seal-color);
}

.think-block.is-thinking .think-icon {
  animation: think-icon-breathe 2.2s ease-in-out infinite;
}

@keyframes think-icon-breathe {
  0%,
  100% {
    opacity: 0.45;
  }
  50% {
    opacity: 1;
  }
}

/* 折叠态滚动直播：占满标题剩余宽度，两端渐隐遮罩，单行不换行 */
.think-live-preview {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  -webkit-mask-image: linear-gradient(90deg, transparent, #000 8%, #000 92%, transparent);
  mask-image: linear-gradient(90deg, transparent, #000 8%, #000 92%, transparent);
}

.think-live-preview-text {
  display: inline-block;
  white-space: nowrap;
  color: var(--text-muted);
  font-size: 12px;
  padding-right: 16px;
}

.think-live-preview-text.rolling {
  animation: think-roll var(--roll-duration, 6s) linear infinite alternate;
}

@keyframes think-roll {
  from {
    transform: translateX(0);
  }
  to {
    transform: translateX(var(--roll-shift, -100%));
  }
}

/* 折页内容区：透明落纸 + 左缘朱砂折页线（常态半透，克制） */
.think-block-content {
  margin-top: 4px;
  background: transparent;
  border-left: 2px solid color-mix(in srgb, var(--seal-color) 35%, transparent);
}

/* 滚动区：限高防长思考把正文推出视口；粘底跟随由脚本驱动 */
.think-scroll {
  max-height: 280px;
  overflow-y: auto;
  scrollbar-width: thin;
  padding: 2px 0 2px 14px;
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
  .think-block.is-thinking .think-icon {
    animation: none;
  }
  .think-live-preview-text.rolling {
    animation: none;
  }
}

/* 思考中省略号：三点依次上浮的波浪，柔和有韵律 */
.thinking-dots {
  display: inline-flex;
  gap: 2px;
  margin-left: 2px;
}

.thinking-dots span {
  display: inline-block;
  animation: thinkingDot 1.4s ease-in-out infinite;
  opacity: 0.25;
  transform: translateY(0);
}
.thinking-dots span:nth-child(1) {
  animation-delay: 0s;
}
.thinking-dots span:nth-child(2) {
  animation-delay: 0.18s;
}
.thinking-dots span:nth-child(3) {
  animation-delay: 0.36s;
}

@keyframes thinkingDot {
  0%,
  55%,
  100% {
    opacity: 0.25;
    transform: translateY(0);
  }
  25% {
    opacity: 1;
    transform: translateY(-3px);
  }
}
</style>
