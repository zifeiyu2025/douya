<template>
  <!-- 后端下载确认框：runtime 推理后端缺失时弹出，让用户决定"是否立即下载"。 -->
  <n-modal
    :show="show"
    :mask-closable="false"
    :close-on-esc="false"
    preset="card"
    title="需要下载推理后端"
    style="width: 560px; max-width: 92vw"
    :bordered="false"
  >
    <!-- GPU 检测信息 -->
    <div v-if="payload?.gpu_name" class="info-row">
      <span class="label">检测到显卡</span>
      <span class="value">{{ payload.gpu_name }}</span>
    </div>

    <div class="info-row">
      <span class="label">推荐后端</span>
      <span class="value">{{ payload?.backend_name || payload?.backend_type || '未知' }}</span>
    </div>

    <!-- 缺失文件清单 -->
    <div v-if="payload?.missing_files" class="missing-section">
      <div class="section-title">缺少的依赖文件：</div>
      <pre class="missing-files">{{ payload.missing_files }}</pre>
    </div>

    <n-alert type="warning" :show-icon="true" class="explain-alert">
      首次使用需要下载并安装推理后端，下载完成后会自动重启应用使其生效。
      如果网络不佳，您也可以稍后在「设置 → 后端」中手动下载。
    </n-alert>

    <div class="source-tip">
      <n-icon size="13" color="#888">
        <svg viewBox="0 0 24 24" fill="currentColor">
          <path
            d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17h-2v-6h2v6zm0-8h-2V7h2v4z"
          />
        </svg>
      </n-icon>
      <span>下载来源：{{ payload?.source_url || 'GitHub 官方发布' }}</span>
    </div>

    <!-- 超时倒计时：超时后后端默认继续下载，保证开箱即用 -->
    <div v-if="countdown > 0" class="countdown-tip">
      若未在 {{ countdown }} 秒内选择，将默认继续下载
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button @click="onExit">退出</n-button>
        <n-button type="primary" @click="onDownload">立即下载</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { NAlert, NButton, NIcon, NModal, NSpace } from 'naive-ui'

export interface BackendDownloadRequestPayload {
  gpu_name: string
  backend_name: string
  backend_type: string
  missing_files: string
  timeout_seconds: number
  source_url: string
}

const props = defineProps<{
  show: boolean
  payload: BackendDownloadRequestPayload | null
}>()

const emit = defineEmits<{
  (e: 'download'): void
  (e: 'exit'): void
}>()

// 超时倒计时（秒）：向后端承诺的等待时间同步展示，超时后后端自动继续下载
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

function stopTimer() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

watch(
  () => props.show,
  show => {
    stopTimer()
    if (show) {
      countdown.value = Math.max(0, props.payload?.timeout_seconds ?? 60)
      timer = setInterval(() => {
        countdown.value -= 1
        if (countdown.value <= 0) stopTimer()
      }, 1000)
    }
  },
  { immediate: true }
)

onUnmounted(stopTimer)

function onDownload() {
  stopTimer()
  emit('download')
}

function onExit() {
  stopTimer()
  emit('exit')
}
</script>

<style scoped>
.info-row {
  display: flex;
  align-items: baseline;
  margin-bottom: 8px;
  font-size: 14px;
}
.info-row:last-of-type {
  margin-bottom: 4px;
}
.label {
  color: var(--n-text-color-3, #999);
  min-width: 84px;
  flex-shrink: 0;
}
.value {
  color: var(--n-text-color, #fff);
  word-break: break-all;
}
.missing-section {
  margin: 12px 0;
}
.section-title {
  font-size: 13px;
  color: var(--n-text-color-2, #ccc);
  margin-bottom: 6px;
}
.missing-files {
  background: var(--n-color-target, rgba(255, 255, 255, 0.04));
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--n-text-color-2, #bbb);
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  max-height: 24vh;
  overflow-y: auto;
}
.explain-alert {
  margin: 6px 0 10px;
}
.source-tip {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--n-text-color-3, #888);
}
.countdown-tip {
  margin-top: 10px;
  font-size: 12px;
  color: var(--warning-color, #f0a020);
}
</style>
