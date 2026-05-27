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
                  <n-button size="small" text @click.stop="clearBackground">清除</n-button>
                </div>
              </div>
            </n-upload>
          </div>
        </n-form-item>

        <n-form-item label="背景透明度" v-if="formConfig.chat_background">
          <n-slider v-model:value="formConfig.chat_background_opacity" :min="0.2" :max="1" :step="0.05" />
          <span class="slider-value">{{ Math.round(formConfig.chat_background_opacity * 100) }}%</span>
        </n-form-item>

        <n-form-item label="用户头像">
          <div class="avatar-upload-wrapper">
            <div class="avatar-preview">
              <img :src="formConfig.user_avatar || defaultUserAvatar" class="avatar-img" />
            </div>
            <div class="avatar-buttons">
              <n-upload
                :show-file-list="false"
                :custom-request="handleUserAvatarUpload"
                accept="image/*"
              >
                <n-button size="small" quaternary>上传</n-button>
              </n-upload>
              <n-button size="small" text @click="clearUserAvatar" v-if="formConfig.user_avatar">清除</n-button>
            </div>
          </div>
        </n-form-item>

        <n-form-item label="AI头像">
          <div class="avatar-upload-wrapper">
            <div class="avatar-preview ai-avatar">
              <img :src="formConfig.ai_avatar || defaultAiAvatar" class="avatar-img" />
            </div>
            <div class="avatar-buttons">
              <n-upload
                :show-file-list="false"
                :custom-request="handleAIAvatarUpload"
                accept="image/*"
              >
                <n-button size="small" quaternary>上传</n-button>
              </n-upload>
              <n-button size="small" text @click="clearAIAvatar" v-if="formConfig.ai_avatar">清除</n-button>
            </div>
          </div>
        </n-form-item>

        <n-divider>搜索 API KEY</n-divider>

        <n-form-item label="Ollama API Key">
          <n-input
            v-model:value="searchKeys.ollama_api_key"
            type="password"
            show-password-on="click"
            placeholder="输入 Ollama API Key"
            @blur="saveSearchKeys"
          />
        </n-form-item>

        <n-form-item label="Tavily API Key">
          <n-input
            v-model:value="searchKeys.tavily_api_key"
            type="password"
            show-password-on="click"
            placeholder="输入 Tavily API Key"
            @blur="saveSearchKeys"
          />
        </n-form-item>

        <n-form-item label="GitHub API Key">
          <n-input
            v-model:value="searchKeys.github_api_key"
            type="password"
            show-password-on="click"
            placeholder="输入 GitHub API Key"
            @blur="saveSearchKeys"
          />
        </n-form-item>

        <n-divider>系统提示词</n-divider>
        <n-form-item>
          <n-input v-model:value="formConfig.system_prompt" type="textarea" placeholder="自定义提示词将追加在豆芽默认提示词后面，用于补充角色设定和行为指令..." :autosize="{ minRows: 6, maxRows: 20 }" class="rounded-textarea" style="width: 100%;" />
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
          <n-slider v-model:value="contextSizeIndex" :min="0" :max="contextSizeSteps.length - 1" :step="1" :marks="contextSizeMarks" />
          <span class="slider-value">{{ formatContextSize(formConfig.context_size) }}</span>
          <n-button v-if="currentModelRef" quaternary size="tiny" class="reset-btn" @click="applyContextSizeRef">{{ formatContextSize(currentModelRef.raw.context_size) }}</n-button>
        </n-form-item>
        <n-form-item label="重复惩罚">
          <n-slider v-model:value="formConfig.repeat_penalty" :min="0" :max="2" :step="0.01" />
          <span class="slider-value">{{ formConfig.repeat_penalty }}</span>
        </n-form-item>

        <div class="gen-params-save-row">
          <span class="gen-params-status" v-if="genParamsDirty">设置已修改，自动保存中...</span>
          <span class="gen-params-status saved" v-else-if="formConfig.context_size > 0">✓ 已保存</span>
        </div>
      </n-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, onUnmounted } from 'vue'
import {
  NButton, NIcon, NForm, NFormItem, NInput,
  NSlider, NDivider, useMessage, NUpload,
} from 'naive-ui'
import { ArrowBackOutline } from '@vicons/ionicons5'
import { useSettingsStore } from '../stores/settings'
import { matchModelRef } from '../stores/settings'
import { type Config, type SearchAPIKeys } from '../services/wails'
import { wails } from '../services/wails'
import defaultUserAvatar from '../assets/images/user-avatar.svg'
import defaultAiAvatar from '../assets/images/appicon.png'

const settingsStore = useSettingsStore()
const message = useMessage()
const saving = ref(false)
const genParamsDirty = ref(false)
let genParamsSaveTimer: ReturnType<typeof setTimeout> | null = null

const contextSizeSteps = [2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144]
const contextSizeMarks: Record<number, string> = {
  0: '2K',
  1: '4K',
  2: '8K',
  3: '16K',
  4: '32K',
  5: '64K',
  6: '128K',
  7: '256K',
}

function formatContextSize(size: number): string {
  if (size >= 1024) {
    const k = size / 1024
    return k >= 1024 ? `${(k / 1024).toFixed(0)}M` : `${k >= 100 ? Math.round(k) : k}K`
  }
  return `${size}`
}

function findClosestStepIndex(size: number): number {
  let closest = 0
  let minDiff = Math.abs(contextSizeSteps[0] - size)
  for (let i = 1; i < contextSizeSteps.length; i++) {
    const diff = Math.abs(contextSizeSteps[i] - size)
    if (diff < minDiff) {
      minDiff = diff
      closest = i
    }
  }
  return closest
}

const contextSizeIndex = ref(2)

watch(contextSizeIndex, (idx) => {
  formConfig.value.context_size = contextSizeSteps[idx]
})

function applyContextSizeRef() {
  const ref = currentModelRef.value
  if (!ref) return
  const idx = findClosestStepIndex(ref.raw.context_size)
  contextSizeIndex.value = idx
  formConfig.value.context_size = contextSizeSteps[idx]
}

const formConfig = ref<Config>({
  model_path: '',
  llama_server_path: '',
  api_base: '',
  port: 8080,
  context_size: 8192,
  temperature: 0.6,
  top_p: 0.95,
  top_k: 20,
  repeat_penalty: 1,
  mmproj_auto: true,
  mmproj_offload: true,
  kv_unified: false,
  cache_idle_slots: false,
  cache_ram: 0,
  image_min_tokens: 0,
  image_max_tokens: 0,
  fit_target: 0,
  fit_ctx: 0,
  reasoning: 'auto',
  reasoning_budget: 0,
  reasoning_format: '',
  system_prompt: '',
  chat_background: '',
  chat_background_opacity: 0.8,
  user_avatar: '',
  ai_avatar: '',
  search_enabled: false,
  sleep_idle_seconds: 120,
  models_max: 1,
  rag_enabled: false,
  rag_active_kb: 'default',
  rag_top_k: 3,
  rag_min_score: 0.3,
  rag_chunk_size: 512,
  rag_chunk_overlap: 64,
})

const searchKeys = ref<SearchAPIKeys>({
    ollama_api_key: '',
    tavily_api_key: '',
    github_api_key: '',
})

function saveSearchKeys() {
    settingsStore.saveSearchAPIKeys(searchKeys.value)
}

interface ModelRefConfig {
  name: string
  raw: { temperature: number; top_p: number; top_k: number; context_size: number; repeat_penalty: number }
  params: { label: string; value: string }[]
  note?: string
}

const MODEL_REFS: Record<string, ModelRefConfig> = {
  'qwen3.5-9b': {
    name: 'Qwen3.5U-9B',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K (YaRN)' },
      { label: '温度', value: '0.6 (非思考) / 0.8 (思考模式)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.4GB VRAM)' },
      { label: '参数量', value: '~9B' },
    ],
    note: 'Qwen3.5U 原生上下文 32K，通过 YaRN 扩展可达 128K。非思考模式建议 temperature=0.6，思考模式建议 temperature=0.8',
  },
  'gemma-4-e4b': {
    name: 'Gemma4-E4B',
    raw: { temperature: 1.0, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.1GB VRAM)' },
      { label: '参数量', value: '~7.5B (E4B 架构)' },
    ],
    note: 'Gemma4 E4B 最大支持 128K 上下文，Google 官方推荐 temperature=1.0，建议 context_size=32K 以获得最佳体验',
  },
  'qwen3.5-9b-deepseek': {
    name: 'Qwen3.5-9B-DeepSeek-V4-Flash',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 1M' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.4GB VRAM)' },
      { label: '参数量', value: '~9B' },
    ],
    note: 'DeepSeek-V4-Flash 蒸馏版，原生支持 1M 上下文，本地受限于显存推荐 32K',
  },
  'qwen3.5-9b-glm': {
    name: 'Qwen3.5-9B-GLM5.1-Distill-v1',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
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
  formConfig.value.repeat_penalty = ref.raw.repeat_penalty
  applyContextSizeRef()
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
  if (file.size > 2 * 1024 * 1024) {
    message.error('背景图片大小不能超过 2MB')
    return
  }
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.chat_background = base64
  } catch {
    message.error('上传失败')
  }
}

function clearBackground() {
  formConfig.value.chat_background = ''
  formConfig.value.chat_background_opacity = 0.8
}

async function handleUserAvatarUpload(data: any) {
  const file = data.file.file as File
  if (file.size > 1024 * 1024) {
    message.error('头像图片大小不能超过 1MB')
    return
  }
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.user_avatar = base64
  } catch {
    message.error('上传失败')
  }
}

function clearUserAvatar() {
  formConfig.value.user_avatar = ''
}

async function handleAIAvatarUpload(data: any) {
  const file = data.file.file as File
  if (file.size > 1024 * 1024) {
    message.error('头像图片大小不能超过 1MB')
    return
  }
  try {
    const base64 = await fileToBase64(file)
    formConfig.value.ai_avatar = base64
  } catch {
    message.error('上传失败')
  }
}

function clearAIAvatar() {
  formConfig.value.ai_avatar = ''
}

onMounted(async () => {
  await settingsStore.loadConfig()
  formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
  contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
  genParamsDirty.value = false
  await settingsStore.loadSearchAPIKeys()
  searchKeys.value = { ...settingsStore.searchAPIKeys }
})

watch(() => settingsStore.currentModel, async () => {
  if (!genParamsDirty.value) {
    await settingsStore.loadConfig()
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
    contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
  }
})

const ALL_CONFIG_KEYS: (keyof Config)[] = [
  'model_path', 'temperature', 'top_p', 'top_k', 'context_size', 'repeat_penalty',
  'system_prompt', 'chat_background', 'chat_background_opacity', 'user_avatar', 'ai_avatar',
  'search_enabled', 'sleep_idle_seconds', 'models_max',
  'mmproj_auto', 'mmproj_offload', 'kv_unified', 'cache_idle_slots',
  'cache_ram', 'image_min_tokens', 'image_max_tokens',
  'fit_target', 'fit_ctx', 'reasoning', 'reasoning_budget', 'reasoning_format',
  'rag_enabled', 'rag_active_kb', 'rag_top_k', 'rag_min_score', 'rag_chunk_size', 'rag_chunk_overlap',
]

watch(
  () => ALL_CONFIG_KEYS.map(k => formConfig.value[k]),
  () => {
    const savedConfig = settingsStore.config
    const dirty = ALL_CONFIG_KEYS.some(k => formConfig.value[k] !== savedConfig[k])
    genParamsDirty.value = dirty
    if (dirty) {
      scheduleAutoSave()
    }
  }
)

function scheduleAutoSave() {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
  }
  genParamsSaveTimer = setTimeout(() => {
    autoSave()
  }, 1500)
}

async function autoSave() {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
    genParamsSaveTimer = null
  }
  saving.value = true
  try {
    await settingsStore.updateConfig(formConfig.value)
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
    genParamsDirty.value = false
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
  border-color: var(--accent-primary);
  background: var(--accent-tertiary);
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
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.5));
  display: flex;
  justify-content: center;
}

:deep(.upload-actions .n-button.n-button--text) {
  color: #ffffff;
}

:deep(.upload-actions .n-button.n-button--text:hover) {
  color: var(--accent-primary);
  background: rgba(255, 255, 255, 0.15);
}

.avatar-upload-wrapper {
  display: flex;
  align-items: center;
  gap: 16px;
}

.avatar-buttons {
  display: flex;
  align-items: center;
  gap: 8px;
}

.avatar-preview {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  box-shadow: var(--shadow-md);
  transition: all 0.2s;
  flex-shrink: 0;
}

.avatar-preview:hover {
  transform: scale(1.05);
  box-shadow: var(--shadow-lg);
}

.avatar-preview.ai-avatar {
}

.avatar-preview .avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 50%;
  aspect-ratio: 1;
}

.slider-value {
  min-width: 48px;
  text-align: right;
  font-size: 13px;
  color: var(--text-secondary);
  margin-left: 12px;
}

.rounded-textarea :deep(.n-input__textarea-wrapper) {
  border-radius: 16px;
}

.rounded-textarea :deep(.n-input__textarea) {
  border-radius: 16px;
}

.rounded-textarea :deep(.n-input__border) {
  border-radius: 16px;
}

.rounded-textarea :deep(.n-input__state-border) {
  border-radius: 16px;
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
  color: var(--accent-warning);
}

.gen-params-status.saved {
  color: var(--accent-success);
}
</style>

<style>
.has-background .settings-container {
  background: color-mix(in srgb, var(--bg-primary) 88%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border-radius: 12px;
}

.has-background .settings-header {
  background: color-mix(in srgb, var(--bg-primary) 85%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.has-background .model-ref-card {
  background: color-mix(in srgb, var(--bg-secondary) 85%, transparent);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.has-background .model-ref-header {
  background: color-mix(in srgb, var(--bg-tertiary, var(--bg-primary)) 85%, transparent);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}
</style>
