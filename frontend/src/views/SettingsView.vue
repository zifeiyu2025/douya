<template>
  <div class="settings-container">
    <div class="settings-header">
      <n-button quaternary circle @click="$router.push('/')">
        <template #icon>
          <n-icon><ArrowBackOutline /></n-icon>
        </template>
      </n-button>
      <span class="settings-title">设置</span>
    </div>
    <div class="settings-content">
      <n-form label-placement="left" label-width="120" :model="formConfig">
        <n-divider>外观设置</n-divider>

        <n-form-item label="聊天背景">
          <div class="upload-wrapper">
            <n-upload
              :show-file-list="false"
              :custom-request="handleBackgroundUpload"
              accept="image/*"
            >
              <div class="upload-placeholder" v-if="!formConfig.chat_background">
                <div class="upload-icon">🖼️</div>
                <div class="upload-text">点击上传背景图片</div>
              </div>
              <div class="upload-preview" v-else>
                <img :src="formConfig.chat_background" class="background-preview" />
                <div class="upload-actions">
                  <n-button size="small" text @click="clearBackground">清除</n-button>
                </div>
              </div>
            </n-upload>
          </div>
        </n-form-item>

        <n-form-item label="用户头像">
          <div class="avatar-upload-wrapper">
            <div class="avatar-preview">
              <img :src="formConfig.user_avatar || defaultUserAvatar" class="avatar-img" />
            </div>
            <n-upload
              :show-file-list="false"
              :custom-request="handleUserAvatarUpload"
              accept="image/*"
            >
              <n-button size="small" quaternary>上传</n-button>
            </n-upload>
            <n-button size="small" text @click="clearUserAvatar" v-if="formConfig.user_avatar">清除</n-button>
          </div>
        </n-form-item>

        <n-form-item label="AI头像">
          <div class="avatar-upload-wrapper">
            <div class="avatar-preview ai-avatar">
              <img :src="formConfig.ai_avatar || defaultAiAvatar" class="avatar-img" />
            </div>
            <n-upload
              :show-file-list="false"
              :custom-request="handleAIAvatarUpload"
              accept="image/*"
            >
              <n-button size="small" quaternary>上传</n-button>
            </n-upload>
            <n-button size="small" text @click="clearAIAvatar" v-if="formConfig.ai_avatar">清除</n-button>
          </div>
        </n-form-item>

        <n-divider>系统提示词</n-divider>
        <n-form-item label="系统提示词">
          <n-input v-model:value="formConfig.system_prompt" type="textarea" placeholder="设置 AI 的角色和行为指令..." :autosize="{ minRows: 4, maxRows: 12 }" class="rounded-textarea" style="width: 100%;" />
        </n-form-item>

        <n-divider>生成参数</n-divider>

        <div v-if="currentModelRef" class="model-ref-card">
          <div class="model-ref-header">
            <span class="model-ref-icon">📋</span>
            <span class="model-ref-title">{{ currentModelRef.name }} 官方参考参数</span>
            <span class="model-ref-current" v-if="settingsStore.currentModel">当前: {{ settingsStore.currentModel }}</span>
          </div>
          <div class="model-ref-body">
            <div class="model-ref-row" v-for="item in currentModelRef.params" :key="item.label">
              <span class="model-ref-label">{{ item.label }}</span>
              <span class="model-ref-value">{{ item.value }}</span>
            </div>
            <div v-if="currentModelRef.note" class="model-ref-note">{{ currentModelRef.note }}</div>
          </div>
          <n-button size="tiny" quaternary class="model-ref-apply" @click="applyModelRef">
            应用参考参数
          </n-button>
        </div>

        <n-form-item label="温度">
          <n-slider v-model:value="formConfig.temperature" :min="0" :max="2" :step="0.01" />
          <span class="slider-value">{{ formConfig.temperature }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="formConfig.temperature = currentModelRef.raw.temperature">{{ currentModelRef.raw.temperature }}</n-button>
        </n-form-item>
        <n-form-item label="Top P">
          <n-slider v-model:value="formConfig.top_p" :min="0" :max="1" :step="0.01" />
          <span class="slider-value">{{ formConfig.top_p }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="formConfig.top_p = currentModelRef.raw.top_p">{{ currentModelRef.raw.top_p }}</n-button>
        </n-form-item>
        <n-form-item label="Top K">
          <n-slider v-model:value="formConfig.top_k" :min="0" :max="100" :step="1" />
          <span class="slider-value">{{ formConfig.top_k }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="formConfig.top_k = currentModelRef.raw.top_k">{{ currentModelRef.raw.top_k }}</n-button>
        </n-form-item>
        <n-form-item label="上下文长度">
          <n-input-number v-model:value="formConfig.context_size" :min="256" :max="131072" :step="256" />
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="formConfig.context_size = currentModelRef.raw.context_size">{{ currentModelRef.raw.context_size }}</n-button>
        </n-form-item>
        <n-form-item label="重复惩罚">
          <n-slider v-model:value="formConfig.repeat_penalty" :min="0" :max="2" :step="0.01" />
          <span class="slider-value">{{ formConfig.repeat_penalty }}</span>
        </n-form-item>

        <div class="gen-params-save-row">
          <span class="gen-params-status" v-if="genParamsDirty">参数已修改，2秒后自动保存</span>
          <span class="gen-params-status saved" v-else-if="formConfig.context_size > 0">✓ 已保存</span>
          <n-button size="small" type="primary" @click="saveGenParams" :loading="saving" :disabled="!genParamsDirty">
            保存参数
          </n-button>
        </div>

        <div class="settings-actions">
          <n-button type="primary" @click="handleSave" :loading="saving">保存设置</n-button>
        </div>
      </n-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import {
  NButton, NIcon, NForm, NFormItem, NInput, NInputNumber,
  NSlider, NDivider, useMessage, NUpload,
} from 'naive-ui'
import { ArrowBackOutline } from '@vicons/ionicons5'
import { useSettingsStore } from '../stores/settings'
import { matchModelRef } from '../stores/settings'
import { type Config } from '../services/wails'
import { wails } from '../services/wails'
import defaultUserAvatar from '../assets/images/user-avatar.svg'
import defaultAiAvatar from '../assets/images/ai-avatar.svg'

const settingsStore = useSettingsStore()
const message = useMessage()
const saving = ref(false)
const genParamsDirty = ref(false)
let genParamsSaveTimer: ReturnType<typeof setTimeout> | null = null

const formConfig = ref<Config>({
  model_path: '',
  llama_server_path: '',
  api_base: '',
  port: 8080,
  context_size: 32768,
  temperature: 0.8,
  top_p: 0.95,
  top_k: 20,
  repeat_penalty: 1.0,
  mmproj_auto: true,
  mmproj_offload: true,
  kv_unified: false,
  cache_idle_slots: false,
  cache_ram: 0,
  image_min_tokens: 0,
  image_max_tokens: 0,
  fit_target: 0,
  fit_ctx: 0,
  system_prompt: '',
  chat_background: '',
  user_avatar: '',
  ai_avatar: '',
  search_enabled: false,
  sleep_idle_seconds: 120,
  models_max: 1,
})

interface ModelRefConfig {
  name: string
  raw: { temperature: number; top_p: number; top_k: number; context_size: number; repeat_penalty: number }
  params: { label: string; value: string }[]
  note?: string
}

const MODEL_REFS: Record<string, ModelRefConfig> = {
  'qwen3.5-9b': {
    name: 'Qwen3.5U-9B',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 8192, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '8,192 (推荐) / 最大 131,072' },
      { label: '温度', value: '0.6 (非思考) / 0.8 (思考模式)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.4GB VRAM)' },
      { label: '参数量', value: '~9B' },
    ],
    note: 'Qwen3.5U 支持思考/非思考模式，非思考模式建议 temperature=0.6，思考模式建议 temperature=0.8',
  },
  'gemma-4-e4b': {
    name: 'Gemma4-E4B',
    raw: { temperature: 0.8, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '32,768 (推荐) / 最大 131,072' },
      { label: '温度', value: '0.8' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.1GB VRAM)' },
      { label: '参数量', value: '~7.5B (E4B 架构)' },
    ],
    note: 'Gemma4 支持多模态输入（图片），建议 context_size=32768 以获得最佳体验',
  },
  'qwen3.5-9b-deepseek': {
    name: 'Qwen3.5-9B-DeepSeek-V4-Flash',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 8192, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '8,192 (推荐) / 最大 262,144' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.4GB VRAM)' },
      { label: '参数量', value: '~9B' },
    ],
    note: 'DeepSeek-V4-Flash 蒸馏版，支持图片和音频输入',
  },
  'qwen3.5-9b-glm': {
    name: 'Qwen3.5-9B-GLM5.1-Distill-v1',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 8192, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '8,192 (推荐) / 最大 262,144' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.4GB VRAM)' },
      { label: '参数量', value: '~9B' },
    ],
    note: 'GLM5.1 蒸馏版，纯文本模型，不支持多模态输入',
  },
}

const currentModelRef = computed(() => {
  return matchModelRef(settingsStore.currentModel, MODEL_REFS)
})

function applyModelRef() {
  const ref = currentModelRef.value
  if (!ref) return
  formConfig.value.temperature = ref.raw.temperature
  formConfig.value.top_p = ref.raw.top_p
  formConfig.value.top_k = ref.raw.top_k
  formConfig.value.context_size = ref.raw.context_size
  formConfig.value.repeat_penalty = ref.raw.repeat_penalty
  message.success(`已应用 ${ref.name} 参考参数`)
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}

async function handleBackgroundUpload(data: any) {
  const file = data.file.file as File
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.chat_background = base64
    message.success('背景图片已设置')
  } catch {
    message.error('上传失败')
  }
}

function clearBackground() {
  formConfig.value.chat_background = ''
  message.success('背景图片已清除')
}

async function handleUserAvatarUpload(data: any) {
  const file = data.file.file as File
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.user_avatar = base64
    message.success('用户头像已设置')
  } catch {
    message.error('上传失败')
  }
}

function clearUserAvatar() {
  formConfig.value.user_avatar = ''
  message.success('用户头像已清除')
}

async function handleAIAvatarUpload(data: any) {
  const file = data.file.file as File
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.ai_avatar = base64
    message.success('AI头像已设置')
  } catch {
    message.error('上传失败')
  }
}

function clearAIAvatar() {
  formConfig.value.ai_avatar = ''
  message.success('AI头像已清除')
}

onMounted(async () => {
  await settingsStore.loadConfig()
  formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
  genParamsDirty.value = false
})

watch(() => settingsStore.currentModel, async () => {
  if (!genParamsDirty.value) {
    await settingsStore.loadConfig()
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
  }
})

const GEN_PARAMS_KEYS: (keyof Config)[] = [
  'temperature', 'top_p', 'top_k', 'context_size', 'repeat_penalty',
]

watch(
  () => GEN_PARAMS_KEYS.map(k => formConfig.value[k]),
  () => {
    const savedConfig = settingsStore.config
    const dirty = GEN_PARAMS_KEYS.some(k => formConfig.value[k] !== savedConfig[k])
    genParamsDirty.value = dirty
    if (dirty) {
      scheduleGenParamsSave()
    }
  }
)

function scheduleGenParamsSave() {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
  }
  genParamsSaveTimer = setTimeout(() => {
    saveGenParams()
  }, 2000)
}

async function saveGenParams() {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
    genParamsSaveTimer = null
  }
  saving.value = true
  try {
    await settingsStore.updateConfig(formConfig.value)
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
    genParamsDirty.value = false
    message.success('生成参数已保存')
  } catch {
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    await settingsStore.updateConfig(formConfig.value)
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
    message.success('设置已保存')
  } catch {
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}


</script>

<style scoped>
.settings-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
}

.settings-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-color);
}

.settings-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  max-width: 640px;
  width: 100%;
  margin: 0 auto;
}

.upload-wrapper {
  width: 100%;
}

.upload-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px;
  border: 2px dashed var(--border-color);
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.upload-placeholder:hover {
  border-color: #07c160;
  background: rgba(7, 193, 96, 0.04);
}

.upload-icon {
  font-size: 32px;
}

.upload-text {
  color: var(--text-secondary);
}

.upload-preview {
  position: relative;
  border-radius: 12px;
  overflow: hidden;
  cursor: pointer;
}

.background-preview {
  width: 100%;
  height: 160px;
  object-fit: cover;
  border-radius: 12px;
}

.upload-actions {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 8px;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.6));
  display: flex;
  justify-content: center;
}

.avatar-upload-wrapper {
  display: flex;
  align-items: center;
  gap: 12px;
}

.avatar-preview {
  width: 64px;
  height: 64px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.avatar-preview.ai-avatar {
}

.avatar-preview .avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.settings-actions {
  padding-top: 20px;
  display: flex;
  justify-content: flex-end;
}

.slider-value {
  min-width: 48px;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  margin-left: 12px;
}

.rounded-textarea :deep(.n-input__textarea-wrapper) {
  border-radius: 12px;
}

.reset-btn {
  margin-left: 4px;
  font-size: 11px;
  color: var(--text-muted);
  min-width: 32px;
}

.model-ref-card {
  margin-bottom: 16px;
  border-radius: 10px;
  border: 1px solid var(--border-color);
  background: var(--bg-secondary);
  overflow: hidden;
}

.model-ref-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-tertiary, var(--bg-primary));
}

.model-ref-icon {
  font-size: 14px;
}

.model-ref-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.model-ref-current {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-muted);
  font-weight: 400;
}

.model-ref-body {
  padding: 10px 14px;
}

.model-ref-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 12px;
}

.model-ref-label {
  color: var(--text-secondary);
}

.model-ref-value {
  color: var(--text-primary);
  font-weight: 500;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
}

.model-ref-note {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--border-color);
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.5;
}

.model-ref-apply {
  width: 100%;
  border-top: 1px solid var(--border-color);
  border-radius: 0 0 10px 10px;
}

.gen-params-save-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  padding: 12px 0 4px;
}

.gen-params-status {
  font-size: 12px;
  color: var(--accent-warning, #ff976a);
}

.gen-params-status.saved {
  color: var(--accent-success, #07c160);
}
</style>
