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
        <n-collapse :default-expanded-names="['basic']" display-directive="show">

          <!-- ==================== 基础设置 ==================== -->
          <n-collapse-item name="basic">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">基础设置</span>
                <span class="settings-group-desc">常用设置，影响对话体验</span>
              </div>
            </template>
            <BasicSettings />
          </n-collapse-item>

          <!-- ==================== 高级设置 ==================== -->
          <n-collapse-item name="advanced">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">高级设置</span>
                <span class="settings-group-desc">需要一定技术背景的配置项</span>
              </div>
            </template>
            <AdvancedSettings />
          </n-collapse-item>

          <!-- ==================== 实验设置 ==================== -->
          <n-collapse-item name="experimental">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">实验设置</span>
                <span class="settings-group-desc">实验性功能，可能影响稳定性</span>
              </div>
            </template>
            <ExperimentalSettings />
          </n-collapse-item>

          <!-- ==================== 关于 ==================== -->
          <n-collapse-item name="about">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">关于</span>
                <span class="settings-group-desc">版本信息与更新</span>
              </div>
            </template>
            <AboutSettings />
          </n-collapse-item>

        </n-collapse>
      </n-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, onUnmounted, provide } from 'vue'
import {
  NButton, NIcon, NForm,
  NCollapse, NCollapseItem, useMessage,
} from 'naive-ui'
import { ArrowBackOutline } from '@vicons/ionicons5'
import { useSettingsStore } from '../stores/settings'
import { matchModelRef } from '../stores/settings'
import { MODEL_REFS } from '../utils/modelRefs'
import { showSuccess } from '../utils/showError'
import { type Config, type SearchAPIKeys } from '../services/wails'
import { wails } from '../services/wails'
import defaultUserAvatar from '../assets/images/user-avatar.svg'
import defaultAiAvatar from '../assets/images/appicon.png'
import BasicSettings from '../components/settings/BasicSettings.vue'
import AdvancedSettings from '../components/settings/AdvancedSettings.vue'
import ExperimentalSettings from '../components/settings/ExperimentalSettings.vue'
import AboutSettings from '../components/settings/AboutSettings.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from '../components/settings/settingsContext'

const settingsStore = useSettingsStore()
const message = useMessage()
const saving = ref(false)
const genParamsDirty = ref(false)
let genParamsSaveTimer: ReturnType<typeof setTimeout> | null = null

// GPU 检测结果：默认 true（显示全部选项），onMounted 时通过 getSmartParams 更新
const hasGPUInfo = ref(true)

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
const refShowThinking = ref(false)

watch(contextSizeIndex, (idx) => {
  formConfig.value.context_size = contextSizeSteps[idx]
})

function applyContextSizeRef() {
  const raw = activeModelRefRaw.value
  const idx = findClosestStepIndex(raw.context_size)
  contextSizeIndex.value = idx
  formConfig.value.context_size = contextSizeSteps[idx]
}

const cacheTypeKOptions = computed(() => {
  const hasGPU = hasGPUInfo.value
  const baseOptions = [
    { label: '自动', value: '' },
    { label: 'f32 (32bit)', value: 'f32' },
    { label: 'f16 (16bit)', value: 'f16' },
    { label: 'q8_0 (8bit)', value: 'q8_0' },
    { label: 'q5_1 (5bit)', value: 'q5_1' },
    { label: 'q5_0 (5bit)', value: 'q5_0' },
    { label: 'q4_1 (4bit)', value: 'q4_1' },
    { label: 'q4_0 (4bit)', value: 'q4_0' },
  ]
  if (hasGPU) {
    // GPU 模式：在 f16 后插入 bf16，在 q4_0 后追加 iq4_nl
    baseOptions.splice(3, 0, { label: 'bf16 (16bit)', value: 'bf16' })
    baseOptions.push({ label: 'iq4_nl (4bit)', value: 'iq4_nl' })
  }
  return baseOptions
})

const cacheTypeVOptions = computed(() => {
  const hasGPU = hasGPUInfo.value
  const baseOptions = [
    { label: '自动', value: '' },
    { label: 'f32 (32bit)', value: 'f32' },
    { label: 'f16 (16bit)', value: 'f16' },
    { label: 'q8_0 (8bit)', value: 'q8_0' },
    { label: 'q5_1 (5bit)', value: 'q5_1' },
    { label: 'q5_0 (5bit)', value: 'q5_0' },
    { label: 'q4_1 (4bit)', value: 'q4_1' },
    { label: 'q4_0 (4bit)', value: 'q4_0' },
  ]
  if (hasGPU) {
    baseOptions.splice(3, 0, { label: 'bf16 (16bit)', value: 'bf16' })
    baseOptions.push({ label: 'iq4_nl (4bit)', value: 'iq4_nl' })
  }
  return baseOptions
})

const reasoningOptions = [
  { label: '开启', value: 'on' },
  { label: '关闭', value: 'off' },
  { label: '自动', value: 'auto' },
]

const specTypeOptions = computed(() => {
  const caps = settingsStore.modelCapabilities
  const options = [
    { label: '自动检测', value: '' },
  ]
  // 仅当模型支持 MTP 时才显示 draft-mtp 选项
  if (caps.has_mtp) {
    options.push({ label: 'MTP 推测解码 🔥', value: 'draft-mtp' })
  }
  options.push(
    { label: 'Eagle3 推测解码', value: 'draft-eagle3' },
    { label: 'Draft-Simple 推测解码', value: 'draft-simple' },
    { label: 'Ngram-Mod 推测解码', value: 'ngram-mod' },
    { label: 'Ngram-Simple 推测解码', value: 'ngram-simple' },
    { label: 'Ngram-Map-K 推测解码', value: 'ngram-map-k' },
    { label: 'Ngram-Map-K4V 推测解码', value: 'ngram-map-k4v' },
    { label: 'Ngram-Cache 推测解码', value: 'ngram-cache' },
    { label: '关闭', value: 'none' },
  )
  return options
})

const supportsReasoning = computed(() => settingsStore.modelCapabilities.reasoning)

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
  cache_idle_slots: true,
  cache_reuse: 0,
  cache_ram: 8192,
  image_min_tokens: 0,
  image_max_tokens: 0,
  fit_target: 0,
  fit_ctx: 0,
  system_prompt: '',
  chat_background: '',
  chat_background_opacity: 0.8,
  user_avatar: '',
  ai_avatar: '',
  search_mode: 'off',
  thinking_enabled: true,
  thinking_soft_switch: 'auto',
  sleep_idle_seconds: 120,
  models_max: 1,
  rag_enabled: false,
  rag_active_kb: 'default',
  rag_top_k: 3,
  rag_min_score: 0.3,
  rag_chunk_size: 512,
  rag_chunk_overlap: 64,
  embedding_model: '',
  mmap: true,
  kv_offload: true,
  context_shift: false,
  min_p: 0.05,
  dry_multiplier: 0,
  dry_base: 1.75,
  dry_allowed_length: 2,
  dry_sequence_breaker: '',
  dry_penalty_last_n: 0,
  grp_attn_n: 0,
  grp_attn_w: 0,
  jinja: false,
  cache_prompt: false,
  metrics: false,
  verbose: false,
  spec_draft_threads: 0,
  spec_draft_threads_batch: 0,
  spec_default: false,
  device: '',
  parallel: 0,
  cache_type_k: '',
  cache_type_v: '',
  spec_type: '',
  spec_draft_n_max: 0,
  spec_draft_n_min: 0,
  spec_ngram_mod_n_min: 0,
  spec_ngram_mod_n_max: 0,
  spec_ngram_mod_n_match: 0,
  spec_ngram_simple_size_n: 0,
  spec_ngram_simple_size_m: 0,
  spec_ngram_simple_min_hits: 0,
  spec_ngram_map_k_size_n: 0,
  spec_ngram_map_k_size_m: 0,
  spec_ngram_map_k_min_hits: 0,
  spec_ngram_map_k4v_size_n: 0,
  spec_ngram_map_k4v_size_m: 0,
  spec_ngram_map_k4v_min_hits: 0,
  lookup_cache_static: '',
  lookup_cache_dynamic: '',
  spec_draft_model: '',
  cache_type_k_draft: '',
  cache_type_v_draft: '',
  server_api_key_enabled: true,
  expose_server: false,
  swa_full: false,
  ctx_checkpoints: 32,
  checkpoint_min_step: 256,
  tools: '',
  prefill_assistant: true,
  slot_prompt_similarity: 0.1,
  skip_chat_parsing: false,
  api_prefix: '',
  simple_io: false,
  agent: false,
  ui_mcp_proxy: false,
  backend_sampling: false,
  lora_paths: '',
  gpu_layers: 0,
  flash_attn: null,
  mlock: null,
  threads: 0,
  batch_size: 0,
  close_action: 'ask',
  // 推理配置
  reasoning: 'off',
  reasoning_budget: 0,
  reasoning_budget_message: '',
  reasoning_format: '',
  // RAG 重排序配置
  reranker_model_path: '',
  rerank_top_n: 5,
  // KV 缓存持久化配置
  slot_save_path: '',
  slot_save_enabled: false,
  // Draft 模型 GPU 配置
  spec_draft_ngl: 0,
  spec_draft_device: '',
  // 请求级采样配置
  samplers: '',
  ignore_eos: false,
  adaptive_target: 0,
  adaptive_decay: 0,
})

const backgroundImageUrl = computed(() => {
    const bg = formConfig.value.chat_background
    if (!bg) return ''
    if (bg.startsWith('data:')) return bg
    return '/local-file/' + encodeURIComponent(bg)
})

// 搜索 API Key 设置状态（后端不再返回实际密钥，仅返回是否已设置）
const searchKeys = ref<SearchAPIKeys>({
    ollama_api_key: '',
    tavily_api_key: '',
    ollama_api_key_set: false,
    tavily_api_key_set: false,
})

// 用户输入的新 API Key（不在状态中保存真实密钥）
const newOllamaApiKey = ref('')
const newTavilyApiKey = ref('')
const savingSearchKeys = ref(false)

async function saveSearchKeys() {
    // 只发送非空的 key，空值表示不更新
    const keysToUpdate: Partial<SearchAPIKeys> = {}
    if (newOllamaApiKey.value) {
        keysToUpdate.ollama_api_key = newOllamaApiKey.value
    }
    if (newTavilyApiKey.value) {
        keysToUpdate.tavily_api_key = newTavilyApiKey.value
    }
    if (Object.keys(keysToUpdate).length === 0) return

    // 构建提示文案：区分保存了哪些 key
    const savedNames: string[] = []
    if (keysToUpdate.ollama_api_key) savedNames.push('Ollama')
    if (keysToUpdate.tavily_api_key) savedNames.push('Tavily')

    savingSearchKeys.value = true
    try {
        await settingsStore.saveSearchAPIKeys(keysToUpdate)
        // 保存成功后清空输入框
        newOllamaApiKey.value = ''
        newTavilyApiKey.value = ''
        showSuccess(message, `${savedNames.join(' + ')} API Key 已保存`)
    } catch (e) {
        console.error('Failed to save search API keys:', e)
        message.destroyAll()
        message.error('API Key 保存失败，请重试', { duration: 4000 })
    } finally {
        savingSearchKeys.value = false
    }
}

const serverApiKey = ref('')
const hasServerApiKey = ref(false)
const savingServerApiKey = ref(false)

async function saveServerApiKey() {
    if (!serverApiKey.value) return
    savingServerApiKey.value = true
    try {
        await settingsStore.saveServerAPIKey(serverApiKey.value)
        hasServerApiKey.value = true
        serverApiKey.value = ''
        showSuccess(message, '服务 API Key 已保存')
    } catch (e) {
        console.error('Failed to save server API key:', e)
        message.destroyAll()
        message.error('服务 API Key 保存失败，请重试', { duration: 4000 })
    } finally {
        savingServerApiKey.value = false
    }
}

async function onServerAPIKeyToggle() {
    await autoSave()
    // 切换开关后需要重新创建 client 以更新 API Key 设置
    if (formConfig.value.server_api_key_enabled) {
        hasServerApiKey.value = await settingsStore.hasServerAPIKey()
    }
}

async function onExposeServerToggle() {
    await autoSave()
    message.destroyAll()
    if (formConfig.value.expose_server) {
        message.warning('已开启局域网访问，重启服务后生效。请确保已设置 API Key 防止未授权访问。', { duration: 5000 })
    } else {
        message.info('已关闭局域网访问，重启服务后仅本机可访问。', { duration: 3000 })
    }
}

const currentModelRef = computed(() => {
  return matchModelRef(settingsStore.currentModel, MODEL_REFS)
})

const activeModelRefRaw = computed(() => {
  const ref = currentModelRef.value
  if (!ref) return { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 8192, repeat_penalty: 1.0 }
  const useThinking = settingsStore.thinkingEnabled && ref.raw_thinking
  return useThinking ? ref.raw_thinking! : ref.raw
})

function applyModelRef() {
  const ref = currentModelRef.value
  if (!ref) return
  const useThinking = settingsStore.thinkingEnabled && ref.raw_thinking
  const raw = useThinking ? ref.raw_thinking! : ref.raw
  formConfig.value.temperature = raw.temperature
  formConfig.value.top_p = raw.top_p
  formConfig.value.top_k = raw.top_k
  formConfig.value.repeat_penalty = raw.repeat_penalty
  const idx = findClosestStepIndex(raw.context_size)
  contextSizeIndex.value = idx
  formConfig.value.context_size = contextSizeSteps[idx]
  const modeLabel = useThinking ? '思考模式' : '非思考模式'
  showSuccess(message, `已应用 ${ref.name} ${modeLabel}参考参数`)
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}

async function selectBackgroundImage() {
  try {
    const filePath = await wails.selectImageFile()
    if (filePath) {
      formConfig.value.chat_background = filePath
    }
  } catch {
    message.destroyAll()
    message.error('选择图片失败')
  }
}

function clearBackground() {
  formConfig.value.chat_background = ''
  formConfig.value.chat_background_opacity = 0.8
}

// maxAvatarSize 头像文件最大大小（1MB）
const maxAvatarSize = 1024 * 1024

/**
 * 处理头像上传（用户头像和 AI 头像共用）。
 * 生活类比：像照相机拍照，无论拍用户还是拍 AI 形象，相机操作都一样，只是存到不同相册。
 * @param data n-upload custom-request 回调传入的数据
 * @param fieldName 要写入 formConfig 的字段名（'user_avatar' 或 'ai_avatar'）
 */
async function handleAvatarUpload(
  data: any,
  fieldName: 'user_avatar' | 'ai_avatar',
) {
  const file = data.file.file as File
  if (file.size > maxAvatarSize) {
    message.destroyAll()
    message.error('头像图片大小不能超过 1MB')
    return
  }
  try {
    const base64 = await fileToBase64(file)
    formConfig.value[fieldName] = base64
  } catch {
    message.destroyAll()
    message.error('上传失败')
  }
}

function clearUserAvatar() {
  formConfig.value.user_avatar = ''
}

function clearAIAvatar() {
  formConfig.value.ai_avatar = ''
}

// Agent 模式切换处理：启用 Agent 时自动关闭 UIMcpProxy（互斥）
function handleAgentChange() {
  if (formConfig.value.agent) {
    formConfig.value.ui_mcp_proxy = false
  }
  autoSave()
}

// 后端采样切换处理：启用时重置推理预算（互斥）
function handleBackendSamplingChange() {
  if (formConfig.value.backend_sampling) {
    formConfig.value.reasoning_budget = -1
  }
  autoSave()
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
    message.destroyAll()
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await settingsStore.loadConfig()
  formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
  contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
  genParamsDirty.value = false
  await settingsStore.loadSearchAPIKeys()
  searchKeys.value = { ...settingsStore.searchAPIKeys }
  hasServerApiKey.value = await settingsStore.hasServerAPIKey()
  // 获取硬件信息以判断是否有 GPU（影响 KV cache 类型可选项）
  try {
    const smartParams = await wails.getSmartParams()
    hasGPUInfo.value = smartParams.hardware.has_gpu
  } catch {
    // 获取失败时保持默认值（true），显示全部选项
  }
})

watch(() => settingsStore.currentModel, async () => {
  if (!genParamsDirty.value) {
    await settingsStore.loadConfig()
    formConfig.value = JSON.parse(JSON.stringify(settingsStore.config))
    contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
    if (currentModelRef.value) {
      applyModelRef()
    }
    // 如果当前 spec_type 为 draft-mtp 但模型不支持 MTP，自动重置为空（自动检测）
    if (formConfig.value.spec_type === 'draft-mtp' && !settingsStore.modelCapabilities.has_mtp) {
      formConfig.value.spec_type = ''
    }
    // 非推理模型：自动重置 reasoning 为 off
    if (!settingsStore.modelCapabilities.reasoning && formConfig.value.reasoning !== 'off') {
      formConfig.value.reasoning = 'off'
      formConfig.value.reasoning_budget = -1
    }
  }
})

// GPU 状态变化时，自动重置不兼容的 KV cache 类型选中值（bf16/iq4_nl 仅 GPU 可用）
watch(hasGPUInfo, (hasGPU) => {
  if (!hasGPU) {
    const kVal = formConfig.value.cache_type_k
    const vVal = formConfig.value.cache_type_v
    if (kVal === 'bf16' || kVal === 'iq4_nl') {
      formConfig.value.cache_type_k = ''
    }
    if (vVal === 'bf16' || vVal === 'iq4_nl') {
      formConfig.value.cache_type_v = ''
    }
  }
})

const ALL_CONFIG_KEYS: (keyof Config)[] = [
  'model_path', 'llama_server_path', 'api_base', 'port', 'context_size',
  'temperature', 'top_p', 'top_k', 'repeat_penalty',
  'mmproj_auto', 'mmproj_offload', 'kv_unified', 'cache_idle_slots', 'cache_reuse', 'cache_ram',
  'image_min_tokens', 'image_max_tokens', 'fit_target', 'fit_ctx',
  'system_prompt', 'chat_background', 'chat_background_opacity', 'user_avatar', 'ai_avatar',
  'search_mode', 'thinking_enabled', 'thinking_soft_switch', 'sleep_idle_seconds', 'models_max',
  'rag_enabled', 'rag_active_kb', 'rag_top_k', 'rag_min_score', 'rag_chunk_size', 'rag_chunk_overlap', 'embedding_model',
  'mmap', 'kv_offload', 'context_shift', 'min_p',
  'dry_multiplier', 'dry_base', 'dry_allowed_length',
  'dry_sequence_breaker', 'dry_penalty_last_n',
  'grp_attn_n', 'grp_attn_w',
  'jinja', 'cache_prompt', 'metrics', 'verbose',
  'spec_draft_threads', 'spec_draft_threads_batch', 'spec_default',
  'device', 'parallel', 'cache_type_k', 'cache_type_v', 'spec_type',
  'spec_draft_n_max', 'spec_draft_n_min',
  'spec_ngram_mod_n_min', 'spec_ngram_mod_n_max', 'spec_ngram_mod_n_match',
  'spec_ngram_simple_size_n', 'spec_ngram_simple_size_m', 'spec_ngram_simple_min_hits',
  'spec_ngram_map_k_size_n', 'spec_ngram_map_k_size_m', 'spec_ngram_map_k_min_hits',
  'spec_ngram_map_k4v_size_n', 'spec_ngram_map_k4v_size_m', 'spec_ngram_map_k4v_min_hits',
  'lookup_cache_static', 'lookup_cache_dynamic', 'spec_draft_model',
  'cache_type_k_draft', 'cache_type_v_draft',
  'server_api_key_enabled', 'expose_server', 'swa_full',
  'ctx_checkpoints', 'checkpoint_min_step', 'tools', 'prefill_assistant',
  'slot_prompt_similarity', 'skip_chat_parsing', 'api_prefix', 'simple_io',
  'agent', 'ui_mcp_proxy', 'backend_sampling',
  'gpu_layers', 'flash_attn', 'mlock', 'threads', 'batch_size',
  // 推理配置
  'reasoning', 'reasoning_budget', 'reasoning_budget_message', 'reasoning_format',
  // RAG 重排序配置
  'reranker_model_path', 'rerank_top_n',
  // KV 缓存持久化配置
  'slot_save_path', 'slot_save_enabled',
  // Draft 模型 GPU 配置
  'spec_draft_ngl', 'spec_draft_device',
  // 请求级采样配置
  'samplers', 'ignore_eos', 'adaptive_target', 'adaptive_decay',
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
  // 300ms 防抖：用户感知为即时保存，同时避免快速输入时频繁写盘
  genParamsSaveTimer = setTimeout(() => {
    autoSave()
  }, 300)
}

onUnmounted(() => {
  // 离开设置页时如有未保存的修改，立即同步保存，避免丢失
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
    genParamsSaveTimer = null
    if (genParamsDirty.value) {
      autoSave()
    }
  }
})

// 通过 provide 向子组件注入共享状态
const settingsContext: SettingsContext = {
  formConfig,
  autoSave,
  genParamsDirty,
  backgroundImageUrl,
  selectBackgroundImage,
  clearBackground,
  handleAvatarUpload,
  clearUserAvatar,
  clearAIAvatar,
  defaultUserAvatar,
  defaultAiAvatar,
  reasoningOptions,
  supportsReasoning,
  currentModelRef,
  activeModelRefRaw,
  refShowThinking,
  applyModelRef,
  contextSizeIndex,
  contextSizeSteps,
  contextSizeMarks,
  formatContextSize,
  applyContextSizeRef,
  newOllamaApiKey,
  newTavilyApiKey,
  searchKeys,
  saveSearchKeys,
  savingSearchKeys,
  serverApiKey,
  hasServerApiKey,
  saveServerApiKey,
  savingServerApiKey,
  onServerAPIKeyToggle,
  onExposeServerToggle,
  cacheTypeKOptions,
  cacheTypeVOptions,
  specTypeOptions,
  handleAgentChange,
  handleBackendSamplingChange,
  settingsStore,
}

provide(SETTINGS_CONTEXT_KEY, settingsContext)

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
  padding: 20px 20px 80px;
  max-width: 640px;
  width: 100%;
  margin: 0 auto;
  /* 自定义滚动条，避免占用内容空间 */
  scrollbar-width: thin;
  scrollbar-color: var(--border-color) transparent;
}

/* WebKit 滚动条优化 */
.settings-content::-webkit-scrollbar {
  width: 6px;
}

.settings-content::-webkit-scrollbar-track {
  background: transparent;
}

.settings-content::-webkit-scrollbar-thumb {
  background-color: var(--border-color);
  border-radius: 3px;
}

.settings-content::-webkit-scrollbar-thumb:hover {
  background-color: var(--text-tertiary, rgba(0, 0, 0, 0.3));
}

/* 分隔线间距优化，避免配置区域过于拥挤 */
.settings-content :deep(.n-divider) {
  margin-top: 24px;
  margin-bottom: 16px;
}

.settings-content :deep(.n-divider:first-child) {
  margin-top: 0;
}

/* 表单项间距优化 */
.settings-content :deep(.n-form-item) {
  margin-bottom: 16px;
}

.settings-group-header {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.settings-group-title {
  font-size: 16px;
  font-weight: 600;
}

.settings-group-desc {
  font-size: 12px;
  color: var(--n-text-color-3);
}
</style>
