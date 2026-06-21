<template>
  <!-- 仅在服务器就绪时显示 token 计数 -->
  <div v-if="visible" class="token-counter" :class="{ warning: isWarning }">
    {{ tokenCount }} tokens
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { wails } from '../services/wails'
import type { ServerStatus } from '../services/wails'

const props = withDefaults(defineProps<{
  /** 要计数的文本 */
  text: string
  /** 上下文长度（用于警告阈值） */
  contextSize?: number
}>(), {
  contextSize: 0,
})

// token 计数，默认 0
const tokenCount = ref(0)
// 是否可见：服务器未就绪时隐藏
const visible = ref(false)
// 服务器是否就绪
const serverReady = ref(false)

// 防抖定时器句柄
let debounceTimer: ReturnType<typeof setTimeout> | null = null

// 警告阈值：token 数超过上下文长度的 80%
const isWarning = computed(() => {
  if (!props.contextSize || props.contextSize <= 0) return false
  return tokenCount.value >= props.contextSize * 0.8
})

// 计算文本的 token 数（带防抖）
function scheduleCountTokens(text: string) {
  // 清除上一次未触发的防抖
  if (debounceTimer) {
    clearTimeout(debounceTimer)
    debounceTimer = null
  }
  // 空文本直接归零，不发起请求
  if (!text) {
    tokenCount.value = 0
    return
  }
  // 服务器未就绪时不请求
  if (!serverReady.value) return
  // 300ms 防抖
  debounceTimer = setTimeout(async () => {
    try {
      // countTokens 接收 ChatMessage 数组，构造一条 user 消息
      const count = await wails.countTokens([{ role: 'user', content: text }])
      tokenCount.value = count
    } catch {
      // 错误时静默处理，不显示错误提示
    }
  }, 300)
}

// 监听文本变化，触发防抖计数
watch(() => props.text, (newText) => {
  scheduleCountTokens(newText)
})

// 监听服务器状态：只有 running 为 true 时才显示和请求
function handleServerStatus(status: ServerStatus) {
  serverReady.value = !!status.running
  visible.value = serverReady.value
}

onMounted(() => {
  wails.onServerStatus(handleServerStatus)
})

onUnmounted(() => {
  if (debounceTimer) {
    clearTimeout(debounceTimer)
    debounceTimer = null
  }
  wails.offServerStatus()
})
</script>

<style scoped>
.token-counter {
  position: absolute;
  right: 14px;
  bottom: 4px;
  font-size: 12px;
  color: #999;
  pointer-events: none;
  user-select: none;
  z-index: 1;
}

.token-counter.warning {
  color: #f0a020;
}
</style>
