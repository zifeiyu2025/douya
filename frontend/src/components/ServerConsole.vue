<template>
  <n-drawer v-model:show="visible" :width="560" placement="right">
    <n-drawer-content title="llama-server 控制台" closable>
      <n-tabs v-model:value="activeTab" type="line" size="small">
        <n-tab-pane name="logs" tab="日志">
          <div class="console-toolbar">
            <n-button size="small" type="tertiary" @click="clearLogs">清空</n-button>
            <n-button size="small" type="tertiary" @click="copyLogs">复制</n-button>
            <n-checkbox v-model:checked="paused" size="small" @update:checked="onPauseChange">
              暂停
            </n-checkbox>
            <span class="mode-tag" :class="{ 'mode-native': isConPTY, 'mode-fallback': !isConPTY }">
              {{ isConPTY ? '原生终端' : '文本日志' }}
            </span>
          </div>
          <div class="terminal-area">
            <TerminalConsole ref="terminalRef" />
          </div>
        </n-tab-pane>
      </n-tabs>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { NDrawer, NDrawerContent, NButton, NCheckbox, NTabs, NTabPane, useMessage } from 'naive-ui'
import { wails } from '../services/wails'
import TerminalConsole from './TerminalConsole.vue'

const visible = ref(false)
const activeTab = ref<'logs'>('logs')
const paused = ref(false)
const isConPTY = ref(false)
const terminalRef = ref<InstanceType<typeof TerminalConsole>>()
const message = useMessage()

// 清空终端
const clearLogs = () => {
  terminalRef.value?.clear()
}

// 复制终端内容到剪贴板
const copyLogs = async () => {
  try {
    const text = await terminalRef.value?.copy()
    if (text) {
      await navigator.clipboard.writeText(text)
      message.success('已复制到剪贴板')
    } else {
      message.info('终端无内容可复制')
    }
  } catch {
    message.error('复制失败')
  }
}

// 暂停状态变化时同步到终端组件
const onPauseChange = (value: boolean) => {
  terminalRef.value?.setPaused(value)
}

const toggle = () => {
  visible.value = !visible.value
}

// 抽屉打开时调整终端尺寸（容器从 0 变为可见，需要重新 fit）
watch(visible, val => {
  if (val) {
    // 延迟一帧等待 DOM 渲染完成
    requestAnimationFrame(() => {
      terminalRef.value?.fit()
    })
  }
})

defineExpose({ toggle })

onMounted(async () => {
  try {
    isConPTY.value = await wails.isConPTYMode()
  } catch {
    isConPTY.value = false
  }
})

onUnmounted(() => {
  // TerminalConsole 内部会清理终端数据订阅（unsubscribe）
})
</script>

<style scoped>
/* 控制台标题强排版：字号 15px，字重 600 */
:deep(.n-drawer-header__title) {
  font-size: 15px;
  font-weight: 600;
}
.console-toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--border-color);
  flex-wrap: wrap;
}
.mode-tag {
  margin-left: auto;
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 500;
}
/* 原生终端：成功语义色（绿） */
.mode-native {
  color: var(--accent-success);
  background: var(--accent-g-soft);
}
/* 文本日志：警告语义色（黄） */
.mode-fallback {
  color: var(--accent-warning);
  background: var(--accent-y-soft);
}
.terminal-area {
  height: calc(100vh - 180px);
  overflow: hidden;
}
</style>
