<template>
  <div v-if="show" class="token-info">
    <!-- 生成中：显示速度和 token 数 -->
    <template v-if="isGenerating && tokensPerSecond > 0">
      <span class="gen-speed">⚡ {{ speedDisplay }} t/s</span>
      <span class="gen-sep">·</span>
      <span class="gen-count">{{ predictedN.toLocaleString() }} tokens</span>
    </template>
    <!-- Prompt 处理中：显示进度 -->
    <template v-else-if="isGenerating && promptProgress && promptPercent > 0">
      <span class="prompt-progress-text">
        正在处理提示词 {{ promptPercent }}%
        <span v-if="promptEta" class="prompt-eta">(ETA: {{ promptEta }}s)</span>
      </span>
      <div class="prompt-bar">
        <div class="prompt-bar-fill" :style="{ width: promptPercent + '%' }"></div>
      </div>
    </template>
    <!-- 空闲：显示输入 token 计数 -->
    <template v-else>
      <span class="token-label" :class="statusClass">{{ displayCount }}</span>
      <span v-if="contextSize > 0" class="token-sep">/</span>
      <span v-if="contextSize > 0" class="token-total">{{ formatCtx(contextSize) }}</span>
      <div v-if="contextSize > 0" class="token-bar">
        <div class="token-bar-fill" :class="statusClass" :style="{ width: pct + '%' }"></div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { wails } from '../services/wails'
import { useSettingsStore } from '../stores/settings'
import { useChatStore } from '../stores/chat'

const props = withDefaults(defineProps<{
  text: string
  contextSize?: number
}>(), {
  contextSize: 0,
})

const settings = useSettingsStore()
const chatStore = useChatStore()
const tokenCount = ref(0)
let timer: ReturnType<typeof setTimeout> | null = null

// 使用 model_ready 判断可见性（比 running 更精确：表示模型已加载可使用）
const show = computed(() => !!settings.serverStatus.model_ready)

// 从 chat store 获取流式状态
const isGenerating = computed(() => chatStore.isGenerating)
const tokensPerSecond = computed(() => chatStore.tokensPerSecond)
const predictedN = computed(() => chatStore.predictedN)
const promptProgress = computed(() => chatStore.promptProgress)

// 生成速度显示（保留 1 位小数）
const speedDisplay = computed(() => {
  const spd = tokensPerSecond.value
  if (spd >= 100) return Math.round(spd).toString()
  return spd.toFixed(1)
})

// Prompt 处理进度百分比
const promptPercent = computed(() => {
  const pp = promptProgress.value
  if (!pp || pp.total <= 0) return 0
  const actualTotal = pp.total - pp.cache
  const actualProcessed = pp.processed - pp.cache
  if (actualTotal <= 0) return 0
  return Math.min(100, Math.round((actualProcessed / actualTotal) * 100))
})

// Prompt 处理 ETA（秒）
const promptEta = computed(() => {
  const pp = promptProgress.value
  if (!pp || pp.processed <= 0 || pp.timeMs <= 0) return null
  const actualProcessed = pp.processed - pp.cache
  const actualTotal = pp.total - pp.cache
  if (actualProcessed <= 0 || actualTotal <= 0) return null
  const elapsedSec = pp.timeMs / 1000
  const eta = elapsedSec * (actualTotal / actualProcessed - 1)
  if (eta < 1) return null
  return Math.ceil(eta)
})

// ---- 输入 token 计数逻辑（空闲状态使用） ----

// 百分比
const pct = computed(() => {
  if (!props.contextSize || props.contextSize <= 0) return 0
  return Math.min(100, Math.round((tokenCount.value / props.contextSize) * 100))
})

// 状态样式
const statusClass = computed(() => {
  if (!props.contextSize || props.contextSize <= 0) return ''
  const r = tokenCount.value / props.contextSize
  if (r >= 0.95) return 'danger'
  if (r >= 0.8) return 'warn'
  return ''
})

// 显示数字
const displayCount = computed(() => tokenCount.value.toLocaleString())

// 格式化上下文大小
function formatCtx(n: number): string {
  if (n >= 1024 && n % 1024 === 0) return `${n / 1024}K`
  if (n >= 1024) return `${(n / 1024).toFixed(1)}K`
  return String(n)
}

// 使用 llama.cpp 原生 /tokenize API 实时计算 token 数
function scheduleCount(text: string) {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
  if (!text) {
    tokenCount.value = 0
    return
  }
  if (!show.value) return
  timer = setTimeout(async () => {
    try {
      const tokens = await wails.tokenize(text)
      tokenCount.value = tokens.length
    } catch {
      // 静默
    }
  }, 150)
}

watch(() => props.text, (t) => scheduleCount(t))

// 服务器就绪时立即触发一次计数
watch(show, (ready) => {
  if (ready && props.text) scheduleCount(props.text)
})

onUnmounted(() => {
  if (timer) clearTimeout(timer)
})
</script>

<style scoped>
.token-info {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 16px 4px;
  font-size: 11px;
  line-height: 1;
  color: var(--text-muted);
  user-select: none;
  font-variant-numeric: tabular-nums;
}

/* ---- 生成速度样式 ---- */
.gen-speed {
  font-weight: 600;
  color: var(--accent-primary, #10b981);
  letter-spacing: 0.02em;
}

.gen-sep {
  opacity: 0.3;
}

.gen-count {
  opacity: 0.7;
}

/* ---- Prompt 处理进度样式 ---- */
.prompt-progress-text {
  color: var(--accent-primary, #10b981);
  font-weight: 500;
}

.prompt-eta {
  opacity: 0.6;
  margin-left: 2px;
}

.prompt-bar {
  width: 60px;
  height: 3px;
  background: var(--bg-hover, rgba(0, 0, 0, 0.06));
  border-radius: 1.5px;
  overflow: hidden;
  margin-left: 6px;
}

.prompt-bar-fill {
  height: 100%;
  background: var(--accent-primary, #10b981);
  border-radius: 1.5px;
  transition: width 0.3s ease;
}

/* ---- 输入 token 计数样式 ---- */
.token-label {
  font-weight: 600;
  letter-spacing: 0.02em;
  transition: color 0.2s;
}

.token-label.warn {
  color: var(--accent-warning, #f59e0b);
}

.token-label.danger {
  color: var(--accent-danger, #ef4444);
}

.token-sep {
  opacity: 0.4;
}

.token-total {
  opacity: 0.6;
}

.token-bar {
  width: 48px;
  height: 2px;
  background: var(--bg-hover, rgba(0, 0, 0, 0.06));
  border-radius: 1px;
  overflow: hidden;
  margin-left: 2px;
}

.token-bar-fill {
  height: 100%;
  background: var(--accent-primary, #10b981);
  border-radius: 1px;
  transition: width 0.25s ease, background 0.25s;
}

.token-bar-fill.warn {
  background: var(--accent-warning, #f59e0b);
}

.token-bar-fill.danger {
  background: var(--accent-danger, #ef4444);
}
</style>
