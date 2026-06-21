<template>
  <div class="slot-status">
    <div class="slot-toolbar">
      <n-button size="small" @click="refresh" :loading="loading" type="tertiary">
        刷新
      </n-button>
      <n-switch v-model:value="autoRefresh" size="small">
        <template #checked>自动刷新</template>
        <template #unchecked>已暂停</template>
      </n-switch>
      <span class="slot-count">共 {{ slots.length }} 个 slot</span>
    </div>
    <n-data-table
      :columns="columns"
      :data="slots"
      :bordered="false"
      :single-line="false"
      size="small"
      :pagination="false"
      flex-height
      style="height: calc(100vh - 200px)"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, onUnmounted, watch } from 'vue'
import { NButton, NSwitch, NDataTable, NTag, type DataTableColumns } from 'naive-ui'
import { wails, type SlotInfo } from '../services/wails'

const slots = ref<SlotInfo[]>([])
const autoRefresh = ref(true)
const loading = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

// 判断 slot 是否空闲：task 为空或为 "0" 时视为 idle
const isIdle = (row: SlotInfo) => !row.task || row.task === '0'

// 表格列定义
const columns: DataTableColumns<SlotInfo> = [
  {
    title: 'ID',
    key: 'id',
    width: 60,
    align: 'center',
  },
  {
    title: '状态',
    key: 'task',
    width: 110,
    align: 'center',
    render(row) {
      if (isIdle(row)) {
        return h(NTag, { type: 'default', size: 'small', bordered: false }, { default: () => 'idle' })
      }
      return h(NTag, { type: 'info', size: 'small', bordered: false }, { default: () => 'processing' })
    },
  },
  {
    title: 'Prompt',
    key: 'n_prompt',
    width: 90,
    align: 'right',
  },
  {
    title: 'Predicted',
    key: 'n_predicted',
    width: 100,
    align: 'right',
  },
  {
    title: 'GPU层',
    key: 'n_gpu_layers',
    width: 80,
    align: 'right',
  },
  {
    title: '模型',
    key: 'model',
    ellipsis: { tooltip: true },
    render(row) {
      // 仅显示文件名，避免过长
      const name = row.model || '-'
      const parts = name.replace(/\\/g, '/').split('/')
      return parts[parts.length - 1] || name
    },
  },
  {
    title: '缓存',
    key: 'n_cache_tokens',
    width: 90,
    align: 'right',
  },
]

// 拉取 slot 数据，错误时静默处理
const refresh = async () => {
  loading.value = true
  try {
    const data = await wails.getSlots()
    slots.value = data || []
  } catch {
    // 静默处理：保留已有数据
  } finally {
    loading.value = false
  }
}

// 设置自动刷新定时器
const startTimer = () => {
  stopTimer()
  timer = setInterval(refresh, 2000)
}

// 清除定时器
const stopTimer = () => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

// 自动刷新开关变化：暂停时清除定时器，继续时重新设置
watch(autoRefresh, (val) => {
  if (val) {
    startTimer()
  } else {
    stopTimer()
  }
})

onMounted(() => {
  refresh()
  if (autoRefresh.value) {
    startTimer()
  }
})

onUnmounted(() => {
  stopTimer()
})
</script>

<style scoped>
.slot-status {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.slot-toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--n-border-color);
}
.slot-count {
  margin-left: auto;
  font-size: 12px;
  color: var(--n-text-color-3);
}
</style>
