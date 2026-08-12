<!--
  GpuAccelerationSettings: GPU 加速设置
  生活类比：像汽车的驱动方式设置——前驱/后驱/四驱（GPU层数）、涡轮增压（Flash Attention）

  从 PerformanceSettings.vue 拆分而来，负责：
    - GPU 状态检测展示
    - GPU 层数配置（gpu_layers）
    - Flash Attention 三态开关（auto/on/off）
-->
<template>
  <!-- ==================== GPU 加速设置 ==================== -->
  <n-divider style="margin: 24px 0 16px" />
  <div class="section-header">
    <span class="section-icon">🚀</span>
    <span class="section-title">GPU 加速</span>
  </div>

  <!-- GPU 状态 -->
  <n-form-item>
    <template #label>
      GPU 状态
      <HelpTip
        content="自动检测 GPU 信息。Vulkan 兜底时显存由 llama.cpp 在推理时自动探测，此处显示「运行时自动探测」属正常现象"
      />
    </template>
    <div class="gpu-status-row">
      <!-- Vulkan 兜底：友好提示运行时自动探测 -->
      <n-tag v-if="isVulkanFallback" type="info" size="small">
        Vulkan 运行时（显存由 llama.cpp 自动探测）
      </n-tag>
      <n-tag v-else-if="gpuInfo.has_gpu" type="success" size="small">
        {{ gpuInfo.gpu_name }} ({{ gpuInfo.vram_gb }}GB)
      </n-tag>
      <n-tag v-else-if="gpuInfo.has_cuda_backend" type="warning" size="small">
        CUDA 驱动已检测
      </n-tag>
      <n-tag v-else type="error" size="small">未检测到 GPU</n-tag>
    </div>
  </n-form-item>

  <!-- GPU 层数 -->
  <n-form-item>
    <template #label>
      GPU 层数
      <HelpTip
        content="加载到 GPU 的模型层数。0=自动（有 N 卡时全部卸载），99=全部卸载到 GPU，1=仅第一层在 GPU。检测到 N 卡但显示 CPU 运行时，手动设为 99 可强制使用 GPU"
      />
    </template>
    <n-input-number
      v-model:value="formConfig.gpu_layers"
      :min="0"
      :max="99"
      :step="1"
      placeholder="0 = 自动"
      style="width: 100%"
      @blur="autoSave"
    />
  </n-form-item>

  <!-- Flash Attention -->
  <n-form-item>
    <template #label>
      Flash Attention
      <HelpTip
        content="GPU 上的注意力加速技术，大幅提升推理速度并降低显存占用。自动=有 GPU 时开启，无 GPU 时关闭。CPU 模式下强制开启会报错"
      />
    </template>
    <n-select v-model:value="flashAttnValue" :options="flashAttnOptions" @update:value="autoSave" />
  </n-form-item>
</template>

<script setup lang="ts">
import { inject, ref, computed, onMounted } from 'vue'
import { NFormItem, NSelect, NInputNumber, NTag, NDivider } from 'naive-ui'
import { wails } from '../../services/wails'
import HelpTip from '../ui/HelpTip.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'

defineOptions({ name: 'GpuAccelerationSettings' })

// 从父级注入配置上下文（formConfig、autoSave 等共享状态）
const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)!
const { formConfig, autoSave } = ctx

// ===== GPU 状态信息（从后端 getBackendStatus 获取） =====
const gpuInfo = ref({ has_gpu: false, has_cuda_backend: false, gpu_name: '', vram_gb: 0, gpu_vendor: '' })

/** 是否为 Vulkan 兜底检测（vendor=vulkan 且无真实显存） */
const isVulkanFallback = computed(() => gpuInfo.value.gpu_vendor === 'vulkan')

// ===== Flash Attention 三态选项 =====
const flashAttnOptions = [
  { label: '自动（有 GPU 时开启）', value: 'auto' },
  { label: '开启', value: 'on' },
  { label: '关闭', value: 'off' }
]

// Flash Attention 值的 getter/setter：
// 后端用 null 表示"自动"，前端用 'auto' 字符串表示，需要双向转换
const flashAttnValue = computed({
  get: () => {
    if (formConfig.value.flash_attn === null) return 'auto'
    return formConfig.value.flash_attn ? 'on' : 'off'
  },
  set: (val: string) => {
    if (val === 'auto') formConfig.value.flash_attn = null
    else formConfig.value.flash_attn = val === 'on'
  }
})

/**
 * 加载 GPU 状态信息（从后端 getBackendStatus 获取）。
 * 原 smart-params 已移除，改为直接读取后端已检测好的显卡状态。
 */
async function loadGpuInfo() {
  try {
    const status = await wails.getBackendStatus()
    const hasGpu = !!status.gpu_name || status.gpu_vram_mb > 0 || !!status.gpu_vendor
    gpuInfo.value = {
      has_gpu: hasGpu,
      has_cuda_backend: status.gpu_vendor === 'nvidia',
      gpu_name: status.gpu_name,
      vram_gb: Math.round(status.gpu_vram_mb / 1024),
      gpu_vendor: status.gpu_vendor
    }
  } catch {
    // 获取失败时保持默认值（无 GPU），不阻塞设置页渲染
  }
}

onMounted(() => {
  loadGpuInfo()
})
</script>

<style scoped>
/* GPU 状态行 */
.gpu-status-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 区块标题 */
.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.section-icon {
  font-size: 18px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}
</style>
