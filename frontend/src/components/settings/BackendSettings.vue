<template>
  <!-- GPU 检测信息卡片 -->
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
          <span class="gpu-info-value">{{ backendStatus.gpu_name || '未检测到 GPU' }}</span>
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

  <!-- 当前后端状态 -->
  <n-form-item>
    <template #label>
      当前后端
      <HelpTip content="当前正在使用的计算后端（由启动时配置解析）。切换后端后需重启应用才能生效" />
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

  <!-- 后端选择下拉框 -->
  <n-form-item>
    <template #label>
      切换后端
      <HelpTip
        content="选择不同的计算后端。auto 会根据硬件自动选择最合适的后端。切换后需重启应用生效"
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
        content="runtime 目录中已存在 llama-server.exe 的后端。未安装的后端在首次切换时会自动解压"
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
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { NFormItem, NSelect, NButton, NTag, NCard, NIcon, useDialog, useMessage } from 'naive-ui'
import { HardwareChipOutline, SpeedometerOutline, ServerOutline } from '@vicons/ionicons5'
import { wails, type BackendStatus } from '../../services/wails'
import { logError } from '../../utils/logger'
import HelpTip from '../ui/HelpTip.vue'

const dialog = useDialog()
const message = useMessage()

// 后端状态（从后端 GetBackendStatus 拉取）
const backendStatus = ref<BackendStatus>({
  current_backend: '',
  config_backend: 'auto',
  gpu_vendor: '',
  gpu_name: '',
  gpu_vram_mb: 0,
  installed_backends: [],
  available_backends: []
})

// 用户在下拉框中选择的值（独立于 backendStatus.config_backend，点击"应用"才提交）
const selectedBackend = ref<string>('auto')
const loading = ref(false)
const switching = ref(false)

// 后端类型 -> 中文显示名映射（对齐 Go GetBackendInfo.DisplayName）
const backendDisplayNames: Record<string, string> = {
  auto: '自动检测（推荐）',
  cuda: 'CUDA (NVIDIA)',
  hip: 'HIP (AMD)',
  sycl: 'SYCL (Intel)',
  vulkan: 'Vulkan (跨厂商)',
  openvino: 'OpenVINO (Intel)',
  cpu: 'CPU (纯 CPU)'
}

// 后端类型 -> 简短描述（用于状态提示）
const backendDescriptions: Record<string, string> = {
  auto: '自动检测',
  cuda: 'CUDA',
  hip: 'HIP',
  sycl: 'SYCL',
  vulkan: 'Vulkan',
  openvino: 'OpenVINO',
  cpu: 'CPU'
}

/** 将后端类型转为中文显示名 */
function backendStatusLabel(bt: string): string {
  return backendDisplayNames[bt] || backendDescriptions[bt] || bt
}

/** 下拉框选项：基于 available_backends 生成 */
const backendOptions = computed(() => {
  return backendStatus.value.available_backends.map(bt => ({
    label: backendStatusLabel(bt),
    value: bt
  }))
})

/** GPU 厂商中文显示 */
const gpuVendorLabel = computed(() => {
  const vendor = backendStatus.value.gpu_vendor
  if (!vendor) return '未检测到'
  const vendorMap: Record<string, string> = {
    nvidia: 'NVIDIA',
    amd: 'AMD',
    intel: 'Intel',
    vulkan: 'Vulkan'
  }
  return vendorMap[vendor] || vendor
})

/** 显存大小显示（自动转换为 GB） */
const vramLabel = computed(() => {
  const vram = backendStatus.value.gpu_vram_mb
  if (!vram) return '无'
  if (vram >= 1024) {
    return `${(vram / 1024).toFixed(1)} GB`
  }
  return `${vram} MB`
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

/** 加载后端状态 */
async function loadBackendStatus() {
  loading.value = true
  try {
    const status = await wails.getBackendStatus()
    backendStatus.value = status
    // 同步下拉框选中值为当前配置值
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

  // 弹出确认对话框：切换后端需要重启推理服务
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
    // 刷新状态
    await loadBackendStatus()
  } catch (e) {
    message.destroyAll()
    logError('Failed to switch backend', e)
    const errMsg = e instanceof Error ? e.message : String(e)
    message.error(`后端切换失败：${errMsg}`, { duration: 5000 })
    // 切换失败也刷新状态，恢复下拉框选中值
    await loadBackendStatus()
  } finally {
    switching.value = false
  }
}

// 监听后端切换事件（后端推送状态更新时刷新前端显示）
let unsubscribeBackendSwitched: (() => void) | null = null

onMounted(() => {
  loadBackendStatus()
  unsubscribeBackendSwitched = wails.subscribeBackendSwitched((status: BackendStatus) => {
    backendStatus.value = status
    selectedBackend.value = status.config_backend || 'auto'
  })
})

onUnmounted(() => {
  if (unsubscribeBackendSwitched) {
    unsubscribeBackendSwitched()
    unsubscribeBackendSwitched = null
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

/* 响应式：窄屏单列 */
@media (max-width: 480px) {
  .backend-gpu-info {
    grid-template-columns: 1fr;
  }
}
</style>
