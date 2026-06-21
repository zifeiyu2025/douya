<template>
  <n-drawer v-model:show="visible" :width="560" placement="right">
    <n-drawer-content title="llama-server 控制台" closable>
      <n-tabs v-model:value="activeTab" type="line" size="small">
        <n-tab-pane name="logs" tab="日志">
          <div class="console-toolbar">
            <n-button size="small" @click="clearLogs" type="tertiary">
              清空
            </n-button>
            <n-button size="small" @click="copyLogs" type="tertiary">
              复制
            </n-button>
            <n-checkbox v-model:checked="autoScroll" size="small">
              自动滚动
            </n-checkbox>
            <n-checkbox v-model:checked="pauseStream" size="small">
              暂停
            </n-checkbox>
            <span class="log-count">{{ logs.length }} 行</span>
          </div>
          <div class="console-content" ref="consoleRef" @scroll="handleScroll">
            <div
              v-for="(line, idx) in displayLogs"
              :key="idx"
              class="console-line"
              :class="getLineClass(line)"
            >{{ line }}</div>
          </div>
        </n-tab-pane>
        <n-tab-pane name="slots" tab="Slot 状态">
          <SlotStatus />
        </n-tab-pane>
      </n-tabs>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { NDrawer, NDrawerContent, NButton, NCheckbox, NTabs, NTabPane, useMessage } from 'naive-ui'
import { wails } from '../services/wails'
import SlotStatus from './SlotStatus.vue'

const visible = ref(false)
const activeTab = ref<'logs' | 'slots'>('logs')
const logs = ref<string[]>([])
const pausedLogs = ref<string[]>([])
const autoScroll = ref(true)
const pauseStream = ref(false)
const consoleRef = ref<HTMLElement>()
const MAX_LOGS = 1000 // 最多保留 1000 行，避免内存占用过大
const message = useMessage()

// 暂停时显示的日志快照，恢复时合并
const displayLogs = computed(() => {
  if (pauseStream.value) {
    return pausedLogs.value
  }
  return logs.value
})

const getLineClass = (line: string) => {
  const lower = line.toLowerCase()
  if (lower.includes('error') || lower.includes('failed') || lower.includes('exception')) {
    return 'line-error'
  }
  if (lower.includes('warn')) {
    return 'line-warn'
  }
  if (lower.includes('loaded') || lower.includes('ready') || lower.includes('listening')) {
    return 'line-success'
  }
  return ''
}

const scrollToBottom = async () => {
  if (!autoScroll.value || !consoleRef.value) return
  await nextTick()
  consoleRef.value.scrollTop = consoleRef.value.scrollHeight
}

const handleScroll = () => {
  if (!consoleRef.value) return
  const { scrollTop, scrollHeight, clientHeight } = consoleRef.value
  // 用户手动滚动到底部时恢复自动滚动，向上滚动超过 10px 时停止自动滚动
  const atBottom = scrollHeight - scrollTop - clientHeight < 10
  autoScroll.value = atBottom
}

const clearLogs = () => {
  logs.value = []
  pausedLogs.value = []
}

const copyLogs = async () => {
  const text = logs.value.join('\n')
  try {
    await navigator.clipboard.writeText(text)
    message.success('已复制到剪贴板')
  } catch {
    message.error('复制失败')
  }
}

const toggle = () => {
  visible.value = !visible.value
}

// 暂停状态变化时快照/恢复
const handlePauseChange = (paused: boolean) => {
  if (paused) {
    // 进入暂停：快照当前日志
    pausedLogs.value = [...logs.value]
  } else {
    // 恢复：清空快照，自动滚动到底部
    pausedLogs.value = []
    nextTick(() => scrollToBottom())
  }
}

watch(pauseStream, handlePauseChange)

defineExpose({ toggle })

onMounted(() => {
  wails.onServerLog((line: string) => {
    if (pauseStream.value) {
      // 暂停时仍记录到 logs，但不更新显示
      logs.value.push(line)
      if (logs.value.length > MAX_LOGS) {
        logs.value = logs.value.slice(-MAX_LOGS)
      }
      return
    }
    logs.value.push(line)
    if (logs.value.length > MAX_LOGS) {
      logs.value = logs.value.slice(-MAX_LOGS)
    }
    scrollToBottom()
  })
})

onUnmounted(() => {
  wails.offServerLog()
})
</script>

<style scoped>
.console-toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--n-border-color);
  flex-wrap: wrap;
}
.log-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--n-text-color-3);
}
.console-content {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
  padding: 8px;
  height: calc(100vh - 140px);
  overflow-y: auto;
  background: var(--n-color);
  white-space: pre-wrap;
  word-break: break-all;
}
.console-line {
  padding: 1px 0;
  white-space: pre-wrap;
  word-break: break-all;
}
.line-error {
  color: #ff6b6b;
}
.line-warn {
  color: #ffd93d;
}
.line-success {
  color: #6bcf7f;
}
</style>
