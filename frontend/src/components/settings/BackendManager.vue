<!--
  BackendManager: 后端管理与状态展示

  从 PerformanceSettings.vue 拆分而来，负责：
    - GPU 硬件信息卡片（厂商、名称、显存）
    - 当前后端状态展示
    - 后端切换（选择 → 应用 → 重启生效）
    - 已安装后端列表
    - 后端下载（从 GitHub 下载 + 进度对话框）
-->
<template>
  <!-- ==================== GPU 检测信息卡片 ==================== -->
  <div class="backend-gpu-info">
    <n-card class="gpu-info-card" hoverable>
      <div class="gpu-info-content">
        <n-icon size="20" class="gpu-info-icon"><HardwareChipOutline /></n-icon>
        <div class="gpu-info-text">
          <span class="gpu-info-label">GPU 厂商</span>
          <span class="gpu-info-value">{{ gpuVendorLabel }}</span>
        </div>
      </div>
    </n-card>

    <n-card class="gpu-info-card" hoverable>
      <div class="gpu-info-content">
        <n-icon size="20" class="gpu-info-icon"><SpeedometerOutline /></n-icon>
        <div class="gpu-info-text">
          <span class="gpu-info-label">GPU 名称</span>
          <span class="gpu-info-value">{{ gpuNameLabel }}</span>
        </div>
      </div>
    </n-card>

    <n-card class="gpu-info-card" hoverable>
      <div class="gpu-info-content">
        <n-icon size="20" class="gpu-info-icon"><ShieldCheckmarkOutline /></n-icon>
        <div class="gpu-info-text">
          <span class="gpu-info-label">显卡驱动</span>
          <span class="gpu-info-value">{{ gpuDriverLabel }}</span>
        </div>
      </div>
    </n-card>

    <n-card class="gpu-info-card" hoverable>
      <div class="gpu-info-content">
        <n-icon size="20" class="gpu-info-icon"><ServerOutline /></n-icon>
        <div class="gpu-info-text">
          <span class="gpu-info-label">显存大小</span>
          <span class="gpu-info-value">{{ vramLabel }}</span>
        </div>
      </div>
    </n-card>
  </div>

  <!-- ==================== 当前后端状态 ==================== -->
  <n-form-item>
    <template #label>
      当前后端
      <HelpTip
        content="当前正在使用的计算后端（由启动时配置解析 + 能力预检自动匹配显卡）。auto 模式下会按能力预检结果选择：N 卡且驱动达标用 CUDA，驱动不达标或 A/I 卡用 Vulkan，无独显或缺少 Vulkan 运行时用 CPU。切换后端后需重启应用才能生效"
      />
    </template>
    <div class="backend-status-row">
      <n-tag :type="currentBackendTagType" size="small">{{ currentBackendLabel }}</n-tag>
      <span v-if="backendStatus.config_backend === 'auto'" class="backend-hint">
        （自动检测模式）
      </span>
      <span v-else class="backend-hint">
        （手动指定：{{ backendStatusLabel(backendStatus.config_backend) }}）
      </span>
    </div>
  </n-form-item>

  <!-- ==================== 后端选择与切换 ==================== -->
  <n-form-item>
    <template #label>
      切换后端
      <HelpTip
        content="选择不同的计算后端。auto 按显卡厂商自动选择：N 卡用 CUDA、AMD/Intel 用 Vulkan、无独显用 CPU（HIP/SYCL/OpenVINO 为手动高级选项）。切换后需重启应用生效"
      />
    </template>
    <div class="backend-select-row">
      <n-select
        v-model:value="selectedBackend"
        :options="backendOptions"
        :loading="loading"
        :disabled="switching"
        placeholder="选择后端类型"
        class="backend-select"
      />
      <n-button
        type="primary"
        size="small"
        ghost
        :loading="switching"
        :disabled="!canApply"
        @click="handleApply"
      >
        应用
      </n-button>
    </div>
  </n-form-item>

  <!-- 后端安装状态列表 -->
  <n-form-item>
    <template #label>
      已安装后端
      <HelpTip
        content="runtime 目录中已存在 llama-server.exe 的后端。未安装的后端可点击下方下载按钮从 GitHub 获取"
      />
    </template>
    <div class="installed-backends">
      <n-tag
        v-for="bt in backendStatus.installed_backends"
        :key="bt"
        type="success"
        size="small"
        class="backend-tag"
      >
        {{ backendStatusLabel(bt) }}
      </n-tag>
      <span v-if="backendStatus.installed_backends.length === 0" class="empty-hint">
        尚无已安装后端
      </span>
    </div>
  </n-form-item>

  <!-- 下载缺失后端 -->
  <n-form-item>
    <template #label>
      下载后端
      <HelpTip
        content="从 GitHub llama.cpp releases 下载缺失的后端 zip 包，下载后自动解压安装。需重启应用生效"
      />
    </template>
    <div class="download-row">
      <n-select
        v-model:value="selectedDownloadBackend"
        :options="downloadOptions"
        :disabled="downloading"
        placeholder="选择要下载的后端"
        class="download-select"
      />
      <n-button
        type="primary"
        size="small"
        ghost
        :loading="downloading"
        :disabled="!canDownload"
        @click="handleDownload"
      >
        下载
      </n-button>
    </div>
  </n-form-item>

  <!-- ==================== 下载进度对话框 ==================== -->
  <n-modal
    v-model:show="showProgressDialog"
    :mask-closable="false"
    :close-on-esc="false"
    preset="card"
    :title="progressDialogTitle"
    style="width: 480px; max-width: 90vw"
  >
    <div class="progress-content">
      <n-progress
        type="line"
        :percentage="Math.round(downloadProgress.Percent)"
        :status="progressStatus"
        :indicator-placement="'inside'"
        processing
      />
      <div class="progress-info">
        <span v-if="downloadProgress.Status === 'downloading'" class="progress-detail">
          {{ formatBytes(downloadProgress.Downloaded) }} /
          {{ formatBytes(downloadProgress.TotalBytes) }} （{{ downloadProgress.AssetName }}）
        </span>
        <span v-else-if="downloadProgress.Status === 'installing'" class="progress-detail">
          正在解压安装...
        </span>
        <span
          v-else-if="downloadProgress.Status === 'completed'"
          class="progress-detail success-text"
        >
          下载完成！
        </span>
        <span v-else-if="downloadProgress.Status === 'failed'" class="progress-detail error-text">
          下载失败：{{ downloadProgress.Error }}
        </span>
      </div>
      <div v-if="downloadProgress.Status === 'completed'" class="progress-hint">
        后端已下载并安装完成，请重启应用使其生效。
      </div>
    </div>
    <template #footer>
      <div class="progress-footer">
        <n-button
          v-if="downloadProgress.Status === 'completed' || downloadProgress.Status === 'failed'"
          size="small"
          @click="closeProgressDialog"
        >
          关闭
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  NFormItem,
  NSelect,
  NButton,
  NTag,
  NCard,
  NIcon,
  NModal,
  NProgress,
  useDialog,
  useMessage,
  type SelectOption
} from 'naive-ui'
import {
  HardwareChipOutline,
  SpeedometerOutline,
  ServerOutline,
  ShieldCheckmarkOutline
} from '@vicons/ionicons5'
import {
  wails,
  type BackendStatus,
  type BackendDownloadProgress,
  type BackendDownloadComplete
} from '../../services/wails'
import { logError } from '../../utils/logger'
import HelpTip from '../ui/HelpTip.vue'

defineOptions({ name: 'BackendManager' })

const dialog = useDialog()
const message = useMessage()

// ===== 后端状态（从后端 GetBackendStatus 拉取） =====
const backendStatus = ref<BackendStatus>({
  current_backend: '',
  config_backend: 'auto',
  gpu_vendor: '',
  gpu_name: '',
  gpu_vram_mb: 0,
  gpu_driver_version: '',
  installed_backends: [],
  available_backends: []
})

// 用户在下拉框中选择的值（独立于 backendStatus.config_backend，点击"应用"才提交）
const selectedBackend = ref<string>('auto')
const loading = ref(false)
const switching = ref(false)

// ===== 下载后端相关状态 =====
const selectedDownloadBackend = ref<string>('')
const downloading = ref(false)
const showProgressDialog = ref(false)
const downloadProgress = ref<BackendDownloadProgress>({
  Backend: '',
  AssetName: '',
  TagName: '',
  TotalBytes: 0,
  Downloaded: 0,
  Percent: 0,
  Status: '',
  Error: '',
  Label: ''
})

// ===== 后端类型 -> 中文显示名映射 =====
const backendDisplayNames: Record<string, string> = {
  auto: '自动检测（推荐）',
  cuda: 'CUDA (NVIDIA)',
  hip: 'ROCm (AMD)',
  sycl: 'SYCL (Intel)',
  vulkan: 'Vulkan (跨厂商)',
  openvino: 'OpenVINO (Intel)',
  cpu: 'CPU (纯 CPU)'
}

const backendDescriptions: Record<string, string> = {
  auto: '自动检测',
  cuda: 'CUDA',
  hip: 'ROCm',
  sycl: 'SYCL',
  vulkan: 'Vulkan',
  openvino: 'OpenVINO',
  cpu: 'CPU'
}

/** 将后端类型转为中文显示名 */
function backendStatusLabel(bt: string): string {
  return backendDisplayNames[bt] || backendDescriptions[bt] || bt
}

/** auto 模式不会自动选中的后端，仅作为设置页手动高级选项 */
const advancedBackends = new Set<string>(['hip', 'sycl', 'openvino'])

/** 将后端列表按「常用 / 高级」分组；auto 解析到的 CUDA/Vulkan/CPU 归入常用组 */
function groupBackendOptions(list: string[]) {
  const primary = list.filter(bt => !advancedBackends.has(bt))
  const advanced = list.filter(bt => advancedBackends.has(bt))
  const groups: SelectOption[] = []
  if (primary.length) {
    groups.push({
      type: 'group',
      label: '常用后端',
      children: primary.map(bt => ({ label: backendStatusLabel(bt), value: bt }))
    })
  }
  if (advanced.length) {
    groups.push({
      type: 'group',
      label: '高级后端（手动）',
      children: advanced.map(bt => ({ label: backendStatusLabel(bt), value: bt }))
    })
  }
  return groups
}

/** 下拉框选项：基于 available_backends 生成，按常用/高级分组 */
const backendOptions = computed(() => groupBackendOptions(backendStatus.value.available_backends))

/** GPU 厂商中文显示 */
const gpuVendorLabel = computed(() => {
  const vendor = backendStatus.value.gpu_vendor
  if (!vendor) return '未检测到'
  const vendorMap: Record<string, string> = {
    nvidia: 'NVIDIA',
    amd: 'AMD',
    intel: 'Intel',
    vulkan: 'Vulkan (跨厂商)'
  }
  return vendorMap[vendor] || vendor
})

/** GPU 名称显示（Vulkan 兜底时给出友好提示） */
const gpuNameLabel = computed(() => {
  const vendor = backendStatus.value.gpu_vendor
  const name = backendStatus.value.gpu_name
  if (vendor === 'vulkan') return '由 llama.cpp 运行时自动探测'
  if (name) return name
  return '未检测到 GPU'
})

/** 显存大小显示（自动转换为 GB；Vulkan 兜底时提示运行时探测） */
const vramLabel = computed(() => {
  const vram = backendStatus.value.gpu_vram_mb
  const vendor = backendStatus.value.gpu_vendor
  if (vendor === 'vulkan' && !vram) return '运行时自动探测'
  if (!vram) return '无'
  if (vram >= 1024) {
    return `${(vram / 1024).toFixed(1)} GB`
  }
  return `${vram} MB`
})

/** 显卡驱动版本显示（仅 N 卡有值；驱动是 CUDA 能力预检的关键依据） */
const gpuDriverLabel = computed(() => {
  if (backendStatus.value.gpu_driver_version) return backendStatus.value.gpu_driver_version
  if (backendStatus.value.gpu_vendor === 'nvidia') return '未知（驱动检测失败）'
  return '—'
})

/** 当前后端显示标签 */
const currentBackendLabel = computed(() => {
  const cur = backendStatus.value.current_backend
  if (!cur || cur === 'auto') return '自动检测'
  return backendStatusLabel(cur)
})

/** 当前后端标签颜色 */
const currentBackendTagType = computed<'success' | 'warning' | 'info'>(() => {
  const cur = backendStatus.value.current_backend
  if (cur === 'cpu') return 'warning'
  if (cur === 'auto' || !cur) return 'info'
  return 'success'
})

/** 是否可以点击应用：选择值与当前配置值不同时才可应用 */
const canApply = computed(() => {
  return selectedBackend.value !== backendStatus.value.config_backend && !switching.value
})

/** 下载选项：排除 auto（auto 由当前后端驱动下载），同样按常用/高级分组 */
const downloadOptions = computed(() =>
  groupBackendOptions(backendStatus.value.available_backends.filter(bt => bt !== 'auto'))
)

/** 是否可以点击下载：选择了后端且不在下载中 */
const canDownload = computed(() => {
  return selectedDownloadBackend.value !== '' && !downloading.value
})

/** 进度对话框标题 */
const progressDialogTitle = computed(() => {
  const bt = downloadProgress.value.Backend
  return bt ? `下载 ${backendStatusLabel(bt)} 后端` : '下载后端'
})

/** n-progress 状态 */
const progressStatus = computed<'success' | 'error' | 'default'>(() => {
  const s = downloadProgress.value.Status
  if (s === 'completed') return 'success'
  if (s === 'failed') return 'error'
  return 'default'
})

/** 格式化字节数为可读字符串 */
function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

/** 处理下载按钮点击 */
function handleDownload() {
  const target = selectedDownloadBackend.value
  if (!target) return

  dialog.warning({
    title: '下载显卡后端',
    content: `即将从 GitHub 下载 ${backendStatusLabel(target)} 后端 zip 包，\n下载完成后自动解压安装。\n\n是否继续？`,
    positiveText: '确认下载',
    negativeText: '取消',
    onPositiveClick: () => {
      doDownloadBackend(target)
    }
  })
}

/** 执行后端下载 */
async function doDownloadBackend(target: string) {
  downloading.value = true
  showProgressDialog.value = true
  downloadProgress.value = {
    Backend: target,
    AssetName: '',
    TagName: '',
    TotalBytes: 0,
    Downloaded: 0,
    Percent: 0,
    Status: 'downloading',
    Error: '',
    Label: ''
  }
  try {
    await wails.downloadBackend(target)
  } catch (e) {
    logError('Failed to start backend download', e)
    message.error(`下载启动失败：${e instanceof Error ? e.message : String(e)}`)
    downloading.value = false
    showProgressDialog.value = false
  }
}

/** 关闭进度对话框 */
function closeProgressDialog() {
  showProgressDialog.value = false
  downloading.value = false
  loadBackendStatus()
}

/** 加载后端状态 */
async function loadBackendStatus() {
  loading.value = true
  try {
    const status = await wails.getBackendStatus()
    backendStatus.value = status
    selectedBackend.value = status.config_backend || 'auto'
  } catch (e) {
    logError('Failed to load backend status', e)
    message.error('加载后端状态失败')
  } finally {
    loading.value = false
  }
}

/** 处理应用按钮点击 */
function handleApply() {
  const target = selectedBackend.value
  if (target === backendStatus.value.config_backend) return

  dialog.warning({
    title: '切换显卡后端',
    content: `切换后端需要重启应用才能生效。\n\n即将切换到：${backendStatusLabel(target)}\n\n是否继续？`,
    positiveText: '确认切换',
    negativeText: '取消',
    onPositiveClick: () => {
      doSwitchBackend(target)
    }
  })
}

/** 执行后端切换 */
async function doSwitchBackend(target: string) {
  switching.value = true
  message.loading('正在切换后端...', { duration: 0 })
  try {
    await wails.switchBackend(target)
    message.destroyAll()
    message.success('后端配置已保存，重启应用后生效', { duration: 5000 })
    await loadBackendStatus()
  } catch (e) {
    message.destroyAll()
    logError('Failed to switch backend', e)
    const errMsg = e instanceof Error ? e.message : String(e)
    message.error(`后端切换失败：${errMsg}`, { duration: 5000 })
    await loadBackendStatus()
  } finally {
    switching.value = false
  }
}

// ===== 事件监听 =====
let unsubscribeBackendSwitched: (() => void) | null = null
let unsubscribeDownloadProgress: (() => void) | null = null
let unsubscribeDownloadComplete: (() => void) | null = null

onMounted(() => {
  loadBackendStatus()
  unsubscribeBackendSwitched = wails.subscribeBackendSwitched((status: BackendStatus) => {
    backendStatus.value = status
    selectedBackend.value = status.config_backend || 'auto'
  })
  unsubscribeDownloadProgress = wails.subscribeBackendDownloadProgress(
    (progress: BackendDownloadProgress) => {
      downloadProgress.value = progress
      if (progress.Status === 'completed' || progress.Status === 'failed') {
        downloading.value = false
      }
    }
  )
  unsubscribeDownloadComplete = wails.subscribeBackendDownloadComplete(
    (result: BackendDownloadComplete) => {
      if (result.success) {
        downloadProgress.value = {
          ...downloadProgress.value,
          Backend: result.backend,
          Status: 'completed',
          Percent: 100,
          Label: '下载完成'
        }
        downloading.value = false
        message.success(`${backendStatusLabel(result.backend)} 后端下载安装完成，请重启应用生效`, {
          duration: 5000
        })
      } else {
        downloadProgress.value = {
          ...downloadProgress.value,
          Backend: result.backend,
          Status: 'failed',
          Error: result.error || '未知错误'
        }
        downloading.value = false
        message.error(
          `${backendStatusLabel(result.backend)} 后端安装失败：${result.error || '未知错误'}`,
          { duration: 8000 }
        )
      }
      loadBackendStatus()
    }
  )
})

onUnmounted(() => {
  if (unsubscribeBackendSwitched) {
    unsubscribeBackendSwitched()
    unsubscribeBackendSwitched = null
  }
  if (unsubscribeDownloadProgress) {
    unsubscribeDownloadProgress()
    unsubscribeDownloadProgress = null
  }
  if (unsubscribeDownloadComplete) {
    unsubscribeDownloadComplete()
    unsubscribeDownloadComplete = null
  }
})
</script>

<style scoped>
/* GPU 信息卡片网格布局 */
.backend-gpu-info {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 10px;
  margin-bottom: 16px;
}

.gpu-info-card {
  cursor: default;
  transition:
    transform 0.15s ease,
    box-shadow 0.15s ease;
}

.gpu-info-card:hover {
  transform: translateY(-1px);
}

.gpu-info-card :deep(.n-card__content) {
  padding: 12px 14px;
}

.gpu-info-content {
  display: flex;
  align-items: center;
  gap: 10px;
}

.gpu-info-icon {
  color: var(--text-secondary);
  flex-shrink: 0;
}

.gpu-info-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.gpu-info-label {
  font-size: 11px;
  color: var(--text-muted);
}

.gpu-info-value {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 后端状态行 */
.backend-status-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.backend-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* 后端选择行 */
.backend-select-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.backend-select {
  flex: 1;
  max-width: 320px;
}

/* 已安装后端标签列表 */
.installed-backends {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.backend-tag {
  margin: 0;
}

.empty-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* 下载后端选择行 */
.download-row {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.download-select {
  flex: 1;
  max-width: 320px;
}

/* 响应式：窄屏单列 */
@media (max-width: 480px) {
  .backend-gpu-info {
    grid-template-columns: 1fr;
  }
}

/* 下载进度对话框 */
.progress-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.progress-info {
  font-size: 13px;
  color: var(--text-secondary);
}

.progress-detail {
  word-break: break-all;
}

.progress-detail.success-text {
  color: var(--n-color-success, #18a058);
}

.progress-detail.error-text {
  color: var(--n-color-error, #d03050);
}

.progress-hint {
  font-size: 12px;
  color: var(--text-muted);
  padding: 8px 12px;
  background: var(--n-color-target, rgba(255, 255, 255, 0.06));
  border-radius: 4px;
}

.progress-footer {
  display: flex;
  justify-content: flex-end;
}
</style>
