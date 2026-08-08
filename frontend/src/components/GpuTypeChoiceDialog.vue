<template>
  <n-modal
    :show="show"
    :mask-closable="false"
    :close-on-esc="false"
    preset="card"
    title="请选择推理后端"
    style="width: 580px; max-width: 90vw"
    :bordered="false"
  >
    <!-- GPU 检测信息卡片 -->
    <div class="gpu-info-card">
      <div class="card-title">检测到的显卡信息</div>
      <div class="info-row">
        <span class="label">显卡名称</span>
        <span class="value">{{ payload?.gpu_name || '未识别' }}</span>
      </div>
      <div class="info-row">
        <span class="label">厂商</span>
        <span class="value">{{ vendorLabel }}</span>
      </div>
      <div class="info-row">
        <span class="label">专用显存</span>
        <span class="value">{{ vramLabel }}</span>
      </div>
    </div>

    <!-- 说明 -->
    <n-alert type="info" :show-icon="true" class="explain-alert">
      程序检测到未知的显卡状态，无法自动选择最合适的推理后端。请根据您的了解选择；
      如果不确定，选择「我不清楚」即可，程序会使用 CPU 进行推理（最稳定）。
    </n-alert>

    <!-- 选项列表 -->
    <div class="options-title">请选择推理后端：</div>
    <n-radio-group v-model:value="selected" class="radio-group">
      <n-space vertical :size="10">
        <div class="radio-item radio-recommended" :class="{ active: selected === 'cpu' }">
          <n-radio value="cpu">
            <div class="opt-content">
              <div class="opt-header">
                <span class="opt-title">我不清楚</span>
                <n-tag size="small" type="success" round>推荐</n-tag>
              </div>
              <div class="opt-desc">使用 CPU 推理，兼容性最好，适合核显或不确定的设备</div>
            </div>
          </n-radio>
        </div>

        <div class="radio-item" :class="{ active: selected === 'vulkan' }">
          <n-radio value="vulkan">
            <div class="opt-content">
              <div class="opt-header">
                <span class="opt-title">Vulkan</span>
                <span class="opt-tag">跨厂商</span>
              </div>
              <div class="opt-desc">支持 NVIDIA / AMD / Intel 显卡的 GPU 加速</div>
            </div>
          </n-radio>
        </div>

        <div class="radio-item" :class="{ active: selected === 'cuda' }">
          <n-radio value="cuda">
            <div class="opt-content">
              <div class="opt-header">
                <span class="opt-title">CUDA</span>
                <span class="opt-tag">NVIDIA 专属</span>
              </div>
              <div class="opt-desc">仅适用于 NVIDIA 独立显卡（如 RTX / GTX 系列）</div>
            </div>
          </n-radio>
        </div>

        <div class="radio-item" :class="{ active: selected === 'hip' }">
          <n-radio value="hip">
            <div class="opt-content">
              <div class="opt-header">
                <span class="opt-title">HIP</span>
                <span class="opt-tag">AMD 专属</span>
              </div>
              <div class="opt-desc">仅适用于 AMD 独立显卡（如 Radeon RX 系列）</div>
            </div>
          </n-radio>
        </div>

        <div class="radio-item" :class="{ active: selected === 'sycl' }">
          <n-radio value="sycl">
            <div class="opt-content">
              <div class="opt-header">
                <span class="opt-title">SYCL</span>
                <span class="opt-tag">Intel 专属</span>
              </div>
              <div class="opt-desc">仅适用于 Intel 独立显卡（如 Arc A 系列）</div>
            </div>
          </n-radio>
        </div>
      </n-space>
    </n-radio-group>

    <!-- 底部提示 -->
    <div class="footer-tip">
      <n-icon size="14" color="#888">
        <svg viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 17h-2v-6h2v6zm0-8h-2V7h2v4z" />
        </svg>
      </n-icon>
      <span>选择后如果无法启动，程序会询问您是否切换到 CPU</span>
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button @click="onCancel">取消</n-button>
        <n-button type="primary" :loading="submitting" @click="onConfirm">
          确认选择
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NIcon, NModal, NRadio, NRadioGroup, NSpace, NTag } from 'naive-ui'

interface GpuTypeChoicePayload {
  gpu_name: string
  gpu_vendor: string
  gpu_vram_mb: number
  gpu_type: string
  timeout_seconds: number
}

const props = defineProps<{
  show: boolean
  payload: GpuTypeChoicePayload | null
}>()

const emit = defineEmits<{
  (e: 'choose', backend: string): void
  (e: 'cancel'): void
}>()

// 默认选择"我不清楚"（CPU），降低用户决策压力
const selected = ref<string>('cpu')
const submitting = ref(false)

// 每次打开对话框时重置为默认选择
watch(
  () => props.show,
  show => {
    if (show) {
      selected.value = 'cpu'
      submitting.value = false
    }
  }
)

const vendorLabel = computed(() => {
  const v = props.payload?.gpu_vendor
  if (v === 'amd') return 'AMD'
  if (v === 'intel') return 'Intel'
  if (v === 'nvidia') return 'NVIDIA'
  return v || '未知'
})

const vramLabel = computed(() => {
  const mb = props.payload?.gpu_vram_mb || 0
  if (mb === 0) return '未检测到'
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`
  return `${mb} MB`
})

function onConfirm() {
  submitting.value = true
  emit('choose', selected.value)
}

function onCancel() {
  // 取消等同于选择"我不清楚"（CPU），保证启动流程不卡住
  emit('choose', 'cpu')
}
</script>

<style scoped>
.gpu-info-card {
  background: var(--n-color-target, rgba(255, 255, 255, 0.04));
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
}
.card-title {
  font-size: 13px;
  color: var(--n-text-color-3, #999);
  margin-bottom: 8px;
}
.info-row {
  display: flex;
  margin-bottom: 6px;
  font-size: 14px;
  align-items: baseline;
}
.info-row:last-child {
  margin-bottom: 0;
}
.label {
  color: var(--n-text-color-3, #999);
  min-width: 80px;
  flex-shrink: 0;
}
.value {
  color: var(--n-text-color, #fff);
  word-break: break-all;
}
.explain-alert {
  margin-bottom: 16px;
}
.options-title {
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 12px;
}
.radio-group {
  width: 100%;
}
.radio-item {
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid transparent;
  transition: all 0.2s ease;
}
.radio-item.active {
  background: var(--n-color-target, rgba(99, 179, 237, 0.08));
  border-color: var(--n-color-target, rgba(99, 179, 237, 0.3));
}
.radio-recommended.active {
  background: rgba(56, 178, 113, 0.08);
  border-color: rgba(56, 178, 113, 0.3);
}
.opt-content {
  display: inline-block;
  vertical-align: top;
  margin-left: 4px;
}
.opt-header {
  display: flex;
  align-items: center;
  gap: 8px;
}
.opt-title {
  font-weight: 500;
  font-size: 14px;
}
.opt-tag {
  font-size: 11px;
  color: var(--n-text-color-3, #999);
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--n-color-target, rgba(255, 255, 255, 0.06));
}
.opt-desc {
  color: var(--n-text-color-3, #999);
  font-size: 12px;
  margin-top: 2px;
}
.footer-tip {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--n-divider-color, rgba(255, 255, 255, 0.06));
  font-size: 12px;
  color: var(--n-text-color-3, #888);
}
</style>
