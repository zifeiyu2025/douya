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
        <n-tab-pane name="metrics" tab="指标">
          <div class="metrics-toolbar">
            <n-button
              size="small"
              type="tertiary"
              :loading="metricsLoading"
              @click="refreshMetrics"
            >
              刷新
            </n-button>
            <n-checkbox
              v-model:checked="autoRefresh"
              size="small"
              @update:checked="onAutoRefreshChange"
            >
              自动刷新（5秒）
            </n-checkbox>
            <span v-if="lastUpdateTime" class="metrics-update-time">
              更新于 {{ lastUpdateTime }}
            </span>
          </div>
          <div v-if="metricsError" class="metrics-error">
            <n-alert type="warning" :title="metricsErrorTitle" closable @close="metricsError = ''">
              {{ metricsError }}
            </n-alert>
          </div>
          <div v-else-if="metrics" class="metrics-content">
            <div class="metrics-grid">
              <div class="metric-card metric-card-primary">
                <div class="metric-label">生成速度</div>
                <div class="metric-value">
                  {{ formatTokenSpeed(metrics.predict_tokens_per_second) }}
                </div>
                <div class="metric-unit">token/s</div>
              </div>
              <div class="metric-card">
                <div class="metric-label">Prompt 处理速度</div>
                <div class="metric-value">
                  {{ formatTokenSpeed(metrics.prompt_tokens_per_second) }}
                </div>
                <div class="metric-unit">token/s</div>
              </div>
              <div class="metric-card">
                <div class="metric-label">处理中请求</div>
                <div class="metric-value">{{ metrics.processing_requests }}</div>
              </div>
              <div class="metric-card">
                <div class="metric-label">排队请求</div>
                <div class="metric-value" :class="{ 'metric-warn': metrics.deferred_requests > 0 }">
                  {{ metrics.deferred_requests }}
                </div>
              </div>
              <div class="metric-card">
                <div class="metric-label">繁忙 Slot 比</div>
                <div class="metric-value">{{ metrics.busy_slots_per_decode.toFixed(2) }}</div>
                <div class="metric-unit">slot/decode</div>
              </div>
              <div class="metric-card">
                <div class="metric-label">最大 token 数</div>
                <div class="metric-value">{{ formatNumber(metrics.n_tokens_max) }}</div>
              </div>
            </div>
            <div class="metrics-detail">
              <div class="detail-title">累计统计</div>
              <div class="detail-row">
                <span class="detail-label">已生成 token</span>
                <span class="detail-value">{{ formatNumber(metrics.tokens_predicted_total) }}</span>
              </div>
              <div class="detail-row">
                <span class="detail-label">已处理 prompt token</span>
                <span class="detail-value">{{ formatNumber(metrics.tokens_prompt_total) }}</span>
              </div>
              <div class="detail-row">
                <span class="detail-label">生成总耗时</span>
                <span class="detail-value">
                  {{ formatSeconds(metrics.predicted_seconds_total) }}
                </span>
              </div>
              <div class="detail-row">
                <span class="detail-label">Prompt 处理总耗时</span>
                <span class="detail-value">{{ formatSeconds(metrics.prompt_seconds_total) }}</span>
              </div>
              <div class="detail-row">
                <span class="detail-label">llama_decode() 调用次数</span>
                <span class="detail-value">{{ formatNumber(metrics.n_decode_total) }}</span>
              </div>
            </div>
          </div>
          <div v-else class="metrics-empty">
            <n-empty description="点击「刷新」加载指标数据" />
          </div>
        </n-tab-pane>
      </n-tabs>
    </n-drawer-content>
  </n-drawer>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import {
  NDrawer,
  NDrawerContent,
  NButton,
  NCheckbox,
  NTabs,
  NTabPane,
  NAlert,
  NEmpty,
  useMessage
} from 'naive-ui'
import { wails } from '../services/wails'
import type { MetricsSummary } from '../types/chat'
import TerminalConsole from './TerminalConsole.vue'

const visible = ref(false)
const activeTab = ref<'logs' | 'metrics'>('logs')
const paused = ref(false)
const isConPTY = ref(false)
const terminalRef = ref<InstanceType<typeof TerminalConsole>>()
const message = useMessage()

// 指标相关状态
const metrics = ref<MetricsSummary | null>(null)
const metricsLoading = ref(false)
const metricsError = ref('')
const metricsErrorTitle = ref('获取指标失败')
const autoRefresh = ref(false)
const lastUpdateTime = ref('')
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

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

// 刷新指标数据
const refreshMetrics = async () => {
  metricsLoading.value = true
  try {
    metrics.value = await wails.getMetrics()
    metricsError.value = ''
    lastUpdateTime.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  } catch (err) {
    const errMsg = err instanceof Error ? err.message : String(err)
    metricsError.value = errMsg
    // 友好的错误提示：常见原因分类
    if (errMsg.includes('status 400')) {
      metricsErrorTitle.value = '请求参数错误（400）'
      metricsError.value = 'router 模式下 /metrics 需要 model 参数。请确认已加载模型后再刷新指标。'
    } else if (errMsg.includes('status 404') || errMsg.toLowerCase().includes('not found')) {
      metricsErrorTitle.value = '指标端点未启用'
      metricsError.value = '请在设置中开启「服务器指标端点（--metrics）」后重启 llama-server。'
    } else if (errMsg.includes('客户端未初始化')) {
      metricsErrorTitle.value = '服务器未启动'
    } else if (errMsg.includes('当前无已加载模型')) {
      metricsErrorTitle.value = '无已加载模型'
    } else {
      metricsErrorTitle.value = '获取指标失败'
    }
  } finally {
    metricsLoading.value = false
  }
}

// 自动刷新开关
const onAutoRefreshChange = (checked: boolean) => {
  if (checked) {
    // 立即触发一次刷新，再启动定时器
    refreshMetrics()
    autoRefreshTimer = setInterval(refreshMetrics, 5000)
  } else {
    if (autoRefreshTimer) {
      clearInterval(autoRefreshTimer)
      autoRefreshTimer = null
    }
  }
}

// 格式化辅助函数
const formatTokenSpeed = (v: number) => (v > 0 ? v.toFixed(1) : '0.0')
const formatNumber = (v: number) => Math.floor(v).toLocaleString('zh-CN')
const formatSeconds = (v: number) => v.toFixed(2) + 's'

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

// 切换到指标 tab 时自动加载一次
watch(activeTab, val => {
  if (val === 'metrics' && !metrics.value && !metricsLoading.value) {
    refreshMetrics()
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
  // 清理自动刷新定时器
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
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

/* 指标 tab 样式 */
.metrics-toolbar {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--border-color);
  flex-wrap: wrap;
}
.metrics-update-time {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-color-3);
}
.metrics-error {
  padding: 16px 0;
}
.metrics-content {
  padding: 12px 0;
}
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
  margin-bottom: 20px;
}
.metric-card {
  background: var(--bg-color-secondary, rgba(0, 0, 0, 0.03));
  border-radius: 8px;
  padding: 12px;
  border: 1px solid var(--border-color);
}
.metric-card-primary {
  grid-column: span 2;
  background: var(--accent-p-soft, rgba(99, 102, 241, 0.1));
  border-color: var(--accent-primary, #6366f1);
}
.metric-label {
  font-size: 12px;
  color: var(--text-color-3);
  margin-bottom: 6px;
}
.metric-value {
  font-size: 24px;
  font-weight: 600;
  color: var(--text-color);
  line-height: 1.2;
}
.metric-card-primary .metric-value {
  font-size: 32px;
  color: var(--accent-primary, #6366f1);
}
.metric-unit {
  font-size: 12px;
  color: var(--text-color-3);
  margin-top: 2px;
}
.metric-progress {
  margin-top: 8px;
}
.metric-warn {
  color: var(--accent-warning);
}
.metrics-detail {
  border-top: 1px solid var(--border-color);
  padding-top: 12px;
}
.detail-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-color-2);
  margin-bottom: 8px;
}
.detail-row {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
  font-size: 13px;
}
.detail-label {
  color: var(--text-color-3);
}
.detail-value {
  color: var(--text-color);
  font-variant-numeric: tabular-nums;
}
.metrics-empty {
  padding: 40px 0;
}
</style>
