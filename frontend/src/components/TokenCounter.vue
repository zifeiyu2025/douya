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
    <!-- 空闲：显示已用 token 数 / 上下文上限 -->
    <template v-else>
      <span class="token-label" :class="statusClass">{{ displayCount }}</span>
      <span v-if="contextSize > 0" class="token-sep">/</span>
      <span v-if="contextSize > 0" class="token-total">{{ formatCtx(contextSize) }}</span>
      <div v-if="contextSize > 0" class="token-bar">
        <div class="token-bar-fill" :class="statusClass" :style="{ width: pct + '%' }"></div>
      </div>
      <!-- P2-A2: 上下文使用率提示文案 -->
      <span v-if="statusText" class="status-text" :class="statusClass">{{ statusText }}</span>
      <!-- P2-A3: 手动压缩按钮（仅当使用率 >= 50% 且有会话时显示） -->
      <button
        v-if="pct >= 50 && conversationId"
        class="compress-btn"
        :class="{ loading: isCompressing }"
        :disabled="isCompressing"
        title="立即压缩早期对话"
        @click="handleCompress"
      >
        {{ isCompressing ? '压缩中…' : '压缩' }}
      </button>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useMessage } from 'naive-ui'
import { wails } from '../services/wails'
import { useSettingsStore } from '../stores/settings'
import { useChatStore } from '../stores/chat'
import { showSuccess, showError } from '../utils/showError'
import { usePromptProgress } from '../composables/usePromptProgress'

const props = withDefaults(
  defineProps<{
    text: string
    contextSize?: number
  }>(),
  {
    contextSize: 0
  }
)

const settings = useSettingsStore()
const chatStore = useChatStore()
const message = useMessage()
// 输入框文本的 token 数（实时增量）
const inputTokens = ref(0)
let timer: ReturnType<typeof setTimeout> | null = null

// 使用 model_ready 判断可见性（比 running 更精确：表示模型已加载可使用）
const show = computed(() => !!settings.serverStatus.model_ready)

// 从 chat store 获取流式状态
const isGenerating = computed(() => chatStore.isGenerating)
const tokensPerSecond = computed(() => chatStore.tokensPerSecond)
const predictedN = computed(() => chatStore.predictedN)
const promptProgress = computed(() => chatStore.promptProgress)
// 最近一次请求的 prompt_tokens（来自 llama-server usage），持久化显示总上下文已用 token 数
const lastPromptTokens = computed(() => chatStore.lastPromptTokens)

// 生成速度显示（保留 1 位小数）
const speedDisplay = computed(() => {
  const spd = tokensPerSecond.value
  if (spd >= 100) return Math.round(spd).toString()
  return spd.toFixed(1)
})

// Prompt 处理进度百分比与 ETA
// 安全实践（基于 F-1.3+F-3.11）：抽取到 usePromptProgress composable，
// 与 MessageList.vue 共享同一计算逻辑，避免一处改漏导致两处显示不一致
const { percent: promptPercent, eta: promptEta } = usePromptProgress(() => promptProgress.value)

// ---- 上下文 token 计数逻辑（空闲状态使用） ----

// 总已用 token = 上次请求的 prompt_tokens + 当前输入框文本的 token 增量
// lastPromptTokens 来自 llama-server usage，包含系统提示词+历史消息+RAG+搜索结果等
// inputTokens 是当前正在输入的新文本，发送后会并入下次请求的 prompt_tokens
const totalTokens = computed(() => lastPromptTokens.value + inputTokens.value)

// 百分比
const pct = computed(() => {
  if (!props.contextSize || props.contextSize <= 0) return 0
  return Math.min(100, Math.round((totalTokens.value / props.contextSize) * 100))
})

// 状态样式
const statusClass = computed(() => {
  if (!props.contextSize || props.contextSize <= 0) return ''
  const r = totalTokens.value / props.contextSize
  if (r >= 0.95) return 'danger'
  if (r >= 0.8) return 'warn'
  if (r >= 0.6) return 'notice' // P2-A2: 新增 60% 提示档
  return ''
})

// P2-A2: 状态提示文案
// 60% 提示用户上下文紧张，80% 提示已自动压缩，95% 警告即将溢出
const statusText = computed(() => {
  if (!props.contextSize || props.contextSize <= 0) return ''
  const r = totalTokens.value / props.contextSize
  if (r >= 0.95) return '即将超出上下文，请开启新对话'
  if (r >= 0.8) return '上下文紧张，已自动压缩早期对话'
  if (r >= 0.6) return '上下文较紧张'
  return ''
})

// 显示数字（总已用 token 数）
// < 1024 直接显示数字，>= 1024 用 K 单位（与右侧 contextSize 的 K 格式一致）
const displayCount = computed(() => formatCtx(totalTokens.value))

// 格式化 token 数：< 1024 显示原数，>= 1024 显示 K 单位（按 1024 进制，与 contextSize 一致）
function formatCtx(n: number): string {
  if (n >= 1024) {
    const k = n / 1024
    // 整数 K（如 5120 → 5K），小数 K 保留 1 位（如 1536 → 1.5K）
    return k % 1 === 0 ? `${k}K` : `${k.toFixed(1)}K`
  }
  return String(n)
}

// 使用 llama.cpp 原生 /tokenize API 实时计算 token 数
let requestVersion = 0 // IPC 请求版本号，防止快速输入时旧结果覆盖新结果

function scheduleCount(text: string) {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
  if (!text) {
    inputTokens.value = 0
    return
  }
  if (!show.value) return
  timer = setTimeout(async () => {
    const version = ++requestVersion
    try {
      const tokens = await wails.tokenize(text)
      // 丢弃过期结果：用户已输入新内容，旧请求的结果不再适用
      if (version !== requestVersion) return
      inputTokens.value = tokens.length
    } catch {
      // 静默
    }
  }, 150)
}

watch(
  () => props.text,
  t => scheduleCount(t)
)

// 服务器就绪时立即触发一次计数
watch(show, ready => {
  if (ready && props.text) scheduleCount(props.text)
})

// ---- P2-A3: 手动压缩逻辑 ----
const isCompressing = ref(false)
const conversationId = computed(() => chatStore.currentConversationId)

async function handleCompress() {
  const convId = conversationId.value
  if (!convId || isCompressing.value) return
  isCompressing.value = true
  try {
    const result = await wails.compressConversation(convId)
    // 显示压缩结果提示
    if (result.trimmedCount > 0) {
      showSuccess(message, `${result.message}：已压缩 ${result.trimmedCount} 条早期消息`)
    } else {
      showSuccess(message, result.message || '无需压缩')
    }
    // 压缩后重新计算 token（下次输入会自动触发 scheduleCount）
    if (props.text) scheduleCount(props.text)
  } catch (err) {
    showError(message, '压缩失败', err)
  } finally {
    isCompressing.value = false
  }
}

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
  color: var(--accent-success);
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
  color: var(--accent-success);
  font-weight: 500;
}

.prompt-eta {
  opacity: 0.6;
  margin-left: 2px;
}

.prompt-bar {
  width: 60px;
  height: 3px;
  background: var(--bg-tertiary);
  border-radius: 1.5px;
  overflow: hidden;
  margin-left: 6px;
}

.prompt-bar-fill {
  height: 100%;
  background: var(--accent-success);
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
  color: var(--accent-warning);
}

.token-label.danger {
  color: var(--accent-danger);
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
  background: var(--bg-tertiary);
  border-radius: 1px;
  overflow: hidden;
  margin-left: 2px;
}

.token-bar-fill {
  height: 100%;
  background: var(--accent-primary);
  border-radius: 1px;
  transition:
    width 0.25s ease,
    background 0.25s;
}

.token-bar-fill.warn {
  background: var(--accent-warning);
}

.token-bar-fill.danger {
  background: var(--accent-danger);
}

/* ---- P2-A2: 状态文案样式 ---- */
/* 60% 档 notice 用主色（蓝绿），80% 档 warn 用警告色（橙），95% 档 danger 用危险色（红） */
.status-text {
  margin-left: 6px;
  font-size: 10px;
  opacity: 0.85;
  transition:
    color 0.2s,
    opacity 0.2s;
  white-space: nowrap;
}

.status-text.notice {
  color: var(--accent-primary);
}

.status-text.warn {
  color: var(--accent-warning);
  font-weight: 500;
}

.status-text.danger {
  color: var(--accent-danger);
  font-weight: 600;
}

/* 60% 档 token-label 和进度条也用 notice 色，与文案保持一致 */
.token-label.notice {
  color: var(--accent-primary);
}

.token-bar-fill.notice {
  background: var(--accent-primary);
}

/* ---- P2-A3: 手动压缩按钮样式 ----
 * 统一风格：透明底色 + 字体颜色（与气泡操作按钮一致） */
.compress-btn {
  margin-left: 8px;
  padding: 1px 8px;
  font-size: 10px;
  line-height: 1.4;
  color: var(--text-muted);
  background: transparent;
  border: 1px solid color-mix(in srgb, var(--text-muted) 30%, transparent);
  border-radius: 8px;
  cursor: pointer;
  transition:
    color 0.15s ease,
    border-color 0.15s ease,
    background-color 0.15s ease;
  user-select: none;
}

.compress-btn:hover:not(:disabled) {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 8%, transparent);
}

.compress-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.compress-btn.loading {
  color: var(--accent-warning);
  border-color: var(--accent-warning);
}
</style>
