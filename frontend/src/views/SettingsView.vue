<template>
  <div class="settings-container">
    <div class="settings-header">
      <button class="back-btn" type="button" aria-label="返回" @click="$router.push('/')">
        <svg width="20" height="20" viewBox="0 0 512 512" fill="none" aria-hidden="true">
          <path
            d="M244 400L100 256l144-144M120 256h292"
            stroke="currentColor"
            stroke-width="48"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
      <span class="settings-title">设置</span>
    </div>
    <div class="settings-content">
      <n-form label-placement="left" label-width="120" :model="formConfig">
        <n-collapse :default-expanded-names="['appearance']" display-directive="show">
          <!-- ==================== 外观 ==================== -->
          <n-collapse-item name="appearance">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">外观</span>
                <span class="settings-group-desc">主题、背景、头像</span>
              </div>
            </template>
            <AppearanceSettings />
          </n-collapse-item>

          <!-- ==================== AI 对话 ==================== -->
          <n-collapse-item name="chat">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">AI 对话</span>
                <span class="settings-group-desc">提示词、推理、生成参数、朗读</span>
              </div>
            </template>
            <AIChatSettings />
          </n-collapse-item>

          <!-- ==================== 联网搜索 ==================== -->
          <n-collapse-item name="search">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">联网搜索</span>
                <span class="settings-group-desc">搜索引擎、搜索密钥</span>
              </div>
            </template>
            <WebSearchSettings />
          </n-collapse-item>

          <!-- ==================== 性能 ==================== -->
          <n-collapse-item name="performance">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">性能</span>
                <span class="settings-group-desc">GPU、后端、KV 缓存、推测解码</span>
              </div>
            </template>
            <PerformanceSettings />
          </n-collapse-item>

          <!-- ==================== API 服务 ==================== -->
          <n-collapse-item name="api">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">API 服务</span>
                <span class="settings-group-desc">端点、密钥、局域网访问</span>
              </div>
            </template>
            <APIServiceSettings />
          </n-collapse-item>

          <!-- ==================== 高级工具 ==================== -->
          <n-collapse-item name="advanced">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">高级</span>
                <span class="settings-group-desc">MCP 工具、RAG、LoRA、实验功能</span>
              </div>
            </template>
            <AdvancedExperimentalSettings />
          </n-collapse-item>

          <!-- ==================== 关于 ==================== -->
          <n-collapse-item name="about">
            <template #header>
              <div class="settings-group-header">
                <span class="settings-group-title">关于</span>
                <span class="settings-group-desc">版本与更新</span>
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
import { NForm, NCollapse, NCollapseItem, useMessage, type UploadCustomRequestOptions } from 'naive-ui'
import { useSettingsStore } from '../stores/settings'
import { matchModelRef } from '../stores/settings'
import { MODEL_REFS } from '../utils/modelRefs'
import { showSuccess } from '../utils/showError'
import { logError } from '../utils/logger' // 安全实践（#34）：用 logError 替代 console.error，避免泄漏后端细节
import { type Config, type SearchAPIKeys, DEFAULT_CONFIG } from '../services/wails'
import { wails } from '../services/wails'
import defaultUserAvatar from '../assets/images/user-avatar.svg'
import defaultAiAvatar from '../assets/images/appicon.png'
import AppearanceSettings from '../components/settings/AppearanceSettings.vue'
import AIChatSettings from '../components/settings/AIChatSettings.vue'
import WebSearchSettings from '../components/settings/WebSearchSettings.vue'
import PerformanceSettings from '../components/settings/PerformanceSettings.vue'
import APIServiceSettings from '../components/settings/APIServiceSettings.vue'
import AdvancedExperimentalSettings from '../components/settings/AdvancedExperimentalSettings.vue'
import AboutSettings from '../components/settings/AboutSettings.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from '../components/settings/settingsContext'
// F-1.17：readFileAsDataURL 抽取到 imageProcess.ts 统一导出，消除两处重复定义
import { readFileAsDataURL } from '../utils/imageProcess'

const settingsStore = useSettingsStore()
const message = useMessage()
const saving = ref(false)
const genParamsDirty = ref(false)
let genParamsSaveTimer: ReturnType<typeof setTimeout> | null = null

// GPU 检测结果：默认 true（显示全部选项），onMounted 时通过 getBackendStatus 更新
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
  7: '256K'
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

watch(contextSizeIndex, idx => {
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
    { label: 'q4_0 (4bit)', value: 'q4_0' }
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
    { label: 'q4_0 (4bit)', value: 'q4_0' }
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
  { label: '自动', value: 'auto' }
]

const specTypeOptions = computed(() => {
  const caps = settingsStore.modelCapabilities
  const options = [{ label: '自动检测', value: '' }]
  // 仅当模型支持 MTP 时才显示 draft-mtp 选项
  if (caps.has_mtp) {
    options.push({ label: 'MTP 推测解码 🔥', value: 'draft-mtp' })
  }
  options.push(
    { label: 'Eagle3 推测解码', value: 'draft-eagle3' },
    { label: 'DFlash 推测解码', value: 'draft-dflash' },
    { label: 'Draft-Simple 推测解码', value: 'draft-simple' },
    { label: 'DSpark 推测解码', value: 'draft-dspark' },
    { label: 'Ngram-Mod 推测解码', value: 'ngram-mod' },
    { label: 'Ngram-Simple 推测解码', value: 'ngram-simple' },
    { label: 'Ngram-Map-K 推测解码', value: 'ngram-map-k' },
    { label: 'Ngram-Map-K4V 推测解码', value: 'ngram-map-k4v' },
    { label: 'Ngram-Cache 推测解码', value: 'ngram-cache' },
    { label: '关闭', value: 'none' }
  )
  return options
})

const supportsReasoning = computed(() => settingsStore.modelCapabilities.reasoning)

// 初始值复用 DEFAULT_CONFIG，避免 80+ 字段硬编码（任务 21）
// onMounted 时会从 settingsStore 加载实际配置覆盖此默认值
const formConfig = ref<Config>({ ...DEFAULT_CONFIG })

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
  tavily_api_key_set: false
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
    logError('Failed to save search API keys', e)
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
    logError('Failed to save server API key', e)
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
  message.destroyAll()
  if (formConfig.value.expose_server) {
    // 开启局域网访问前检查：必须已启用 API Key 并设置密钥
    // 生活类比：开门营业前先检查门锁装好了没，没锁好就不让开门
    if (!formConfig.value.server_api_key_enabled || !hasServerApiKey.value) {
      formConfig.value.expose_server = false
      message.error(
        '开启局域网访问前，必须先启用服务端 API Key 并设置密钥。请先在下方设置 API Key 后再开启局域网访问。',
        { duration: 6000 }
      )
      return
    }
    await autoSave()
    message.warning('已开启局域网访问，重启服务后生效。同一局域网内的设备可通过本机 IP 访问 API。', {
      duration: 5000
    })
  } else {
    await autoSave()
    message.info('已关闭局域网访问，重启服务后仅本机可访问。', { duration: 3000 })
  }
}

async function onEnableWebUIToggle() {
  await autoSave()
  message.destroyAll()
  if (formConfig.value.enable_web_ui) {
    message.warning(
      '已启用原生 Web UI，重启服务后可通过浏览器访问 http://127.0.0.1:' +
        formConfig.value.port +
        ' 。该 UI 与豆芽前端独立，仅供高级用户调试。',
      { duration: 5000 }
    )
  } else {
    message.info('已关闭原生 Web UI，重启服务后生效。', { duration: 3000 })
  }
}

const currentModelRef = computed(() => {
  return matchModelRef(settingsStore.currentModel, MODEL_REFS)
})

const activeModelRefRaw = computed(() => {
  const ref = currentModelRef.value
  if (!ref) {
    // 无推荐参数时回退到全局默认采样参数（与 DEFAULT_CONFIG 一致，避免"点推荐 vs 默认"行为差异）
    return {
      temperature: DEFAULT_CONFIG.temperature,
      top_p: DEFAULT_CONFIG.top_p,
      top_k: DEFAULT_CONFIG.top_k,
      context_size: DEFAULT_CONFIG.context_size,
      repeat_penalty: DEFAULT_CONFIG.repeat_penalty
    }
  }
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

// ===== 模型专属参数预设 =====
// 每个模型保存各自的生成参数，切换模型时自动恢复用户习惯
// 生活类比：每个员工有各自的"办公偏好卡片"，换工位时自动按卡片调整
const hasModelPreset = ref(false)
const savingModelPreset = ref(false)

// 加载当前模型的预设状态（切换模型后调用）
async function loadModelPresetStatus() {
  const model = settingsStore.currentModel
  if (!model) {
    hasModelPreset.value = false
    return
  }
  try {
    hasModelPreset.value = await wails.hasModelParams(model)
  } catch {
    hasModelPreset.value = false
  }
}

// 保存当前参数为该模型的预设
async function saveModelPreset() {
  const model = settingsStore.currentModel
  if (!model) {
    message.warning('请先加载模型再保存预设')
    return
  }
  savingModelPreset.value = true
  try {
    // 先保存当前配置到后端，确保保存的是最新参数
    await autoSave()
    await wails.saveModelParams(model)
    hasModelPreset.value = true
    showSuccess(message, `已保存 ${model} 的参数预设，下次切换到此模型将自动恢复`)
  } catch (e) {
    logError('保存模型预设失败', e)
    message.error('保存模型预设失败')
  } finally {
    savingModelPreset.value = false
  }
}

// 清除该模型的预设
async function clearModelPreset() {
  const model = settingsStore.currentModel
  if (!model) return
  savingModelPreset.value = true
  try {
    await wails.clearModelParams(model)
    hasModelPreset.value = false
    showSuccess(message, `已清除 ${model} 的参数预设，切换到此模型将使用全局默认参数`)
  } catch (e) {
    logError('清除模型预设失败', e)
    message.error('清除模型预设失败')
  } finally {
    savingModelPreset.value = false
  }
}

/* F-1.17：fileToBase64 已抽取到 utils/imageProcess.ts（readFileAsDataURL） */

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
  formConfig.value.chat_background_opacity = 0.85
}

// maxAvatarSize 头像文件最大大小（1MB）
const maxAvatarSize = 1024 * 1024

/**
 * 处理头像上传（用户头像和 AI 头像共用）。
 * 生活类比：像照相机拍照，无论拍用户还是拍 AI 形象，相机操作都一样，只是存到不同相册。
 * @param data n-upload custom-request 回调传入的数据
 * @param fieldName 要写入 formConfig 的字段名（'user_avatar' 或 'ai_avatar'）
 */
async function handleAvatarUpload(data: UploadCustomRequestOptions, fieldName: 'user_avatar' | 'ai_avatar') {
  const file = data.file.file as File
  if (file.size > maxAvatarSize) {
    message.destroyAll()
    message.error('头像图片大小不能超过 1MB')
    return
  }
  try {
    const base64 = await readFileAsDataURL(file)
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

let savingPromise: Promise<void> | null = null // 进行中的保存 Promise，防止重入（任务 13）

async function doAutoSave() {
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
    genParamsSaveTimer = null
  }
  saving.value = true
  try {
    // 先拉取后端最新配置，避免覆盖其他路径（如 RAG 开启、模型切换）的修改
    const latest = await wails.getConfig()
    // 仅合并用户实际修改过的字段（formConfig 相对于 settingsStore.config 的 diff）
    // 用户未改的字段保留 latest 的值（后端最新），避免用过期值覆盖后端改动
    const merged: Config = { ...latest }
    for (const k of ALL_CONFIG_KEYS) {
      if (formConfig.value[k] !== settingsStore.config[k]) {
        // 字段名和值均来自同一 Config 对象，类型必然匹配
        ;(merged as any)[k] = formConfig.value[k]
      }
    }
    await settingsStore.updateConfig(merged)
    // 浅拷贝替代 JSON.parse(JSON.stringify)，Config 字段均为原始类型（任务 13）
    formConfig.value = { ...settingsStore.config }
    genParamsDirty.value = false
  } catch (e) {
    logError('自动保存配置失败', e)
    message.destroyAll()
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

async function autoSave() {
  // 重入保护：进行中的保存复用同一 Promise，避免并发覆盖（任务 13）
  if (savingPromise) return savingPromise
  savingPromise = doAutoSave()
  try {
    await savingPromise
  } finally {
    savingPromise = null
  }
}

onMounted(async () => {
  await settingsStore.loadConfig()
  // 浅拷贝替代 JSON.parse(JSON.stringify)，Config 字段均为原始类型（任务 22）
  formConfig.value = { ...settingsStore.config }
  contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
  genParamsDirty.value = false
  await settingsStore.loadSearchAPIKeys()
  searchKeys.value = { ...settingsStore.searchAPIKeys }
  hasServerApiKey.value = await settingsStore.hasServerAPIKey()
  // 加载当前模型的预设状态
  await loadModelPresetStatus()
  // 获取硬件信息以判断是否有 GPU（影响 KV cache 类型可选项），改用 getBackendStatus
  try {
    const status = await wails.getBackendStatus()
    hasGPUInfo.value = !!status.gpu_name || status.gpu_vram_mb > 0 || status.gpu_vendor !== ''
  } catch {
    // 获取失败时保持默认值（true），显示全部选项
  }
})

watch(
  () => settingsStore.currentModel,
  async () => {
    if (!genParamsDirty.value) {
      await settingsStore.loadConfig()
      // 浅拷贝替代 JSON.parse(JSON.stringify)，Config 字段均为原始类型（任务 22）
      formConfig.value = { ...settingsStore.config }
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
    // 加载新模型的预设状态（显示"已保存/未保存"标记）
    loadModelPresetStatus()
  }
)

// port 变化时自动同步 api_base 中的端口，保持两者一致
// 生活类比：改了门牌号，快递单上的地址也跟着自动更新
watch(
  () => formConfig.value.port,
  (newPort, oldPort) => {
    if (newPort == null || newPort === oldPort) return
    const base = formConfig.value.api_base || ''
    // 用正则匹配 URL 末尾的 :端口，替换为新端口
    // 匹配 http://host:port 或 http://host:port/path 中的端口部分
    const portPattern = /^(https?:\/\/[^/:]+):\d+/
    if (portPattern.test(base)) {
      formConfig.value.api_base = base.replace(portPattern, `$1:${newPort}`)
    } else if (/^https?:\/\/[^/:]+$/.test(base)) {
      // 地址不含端口（如 http://127.0.0.1），追加端口
      formConfig.value.api_base = `${base}:${newPort}`
    }
    // 如果 api_base 格式异常（不以 http 开头），不做自动修改，让后端校验拦截
  }
)

// GPU 状态变化时，自动重置不兼容的 KV cache 类型选中值（bf16/iq4_nl 仅 GPU 可用）
watch(hasGPUInfo, hasGPU => {
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
  'model_path',
  'llama_server_path',
  'backend_type',
  'api_base',
  'port',
  'context_size',
  'temperature',
  'top_p',
  'top_k',
  'repeat_penalty',
  'mmproj_auto',
  'mmproj_offload',
  'kv_unified',
  'cache_idle_slots',
  'cache_reuse',
  'cache_ram',
  'image_min_tokens',
  'image_max_tokens',
  'fit_target',
  'fit_ctx',
  'system_prompt',
  'system_prompt_mode',
  'programming_mode',
  'chat_background',
  'chat_background_opacity',
  'user_avatar',
  'ai_avatar',
  'search_mode',
  'sleep_idle_seconds',
  'models_max',
  'rag_enabled',
  'rag_active_kb',
  'rag_top_k',
  'rag_min_score',
  'rag_chunk_size',
  'rag_chunk_overlap',
  'embedding_model',
  'mmap',
  'kv_offload',
  'context_shift',
  'min_p',
  'dry_multiplier',
  'dry_base',
  'dry_allowed_length',
  'dry_sequence_breaker',
  'dry_penalty_last_n',
  'grp_attn_n',
  'grp_attn_w',
  'jinja',
  'cache_prompt',
  'metrics',
  'verbose',
  'spec_draft_threads',
  'spec_draft_threads_batch',
  'spec_default',
  'device',
  'parallel',
  'split_mode',
  'tensor_split',
  'main_gpu',
  'cache_type_k',
  'cache_type_v',
  'spec_type',
  'spec_draft_n_max',
  'spec_draft_n_min',
  'spec_ngram_mod_n_min',
  'spec_ngram_mod_n_max',
  'spec_ngram_mod_n_match',
  'spec_ngram_simple_size_n',
  'spec_ngram_simple_size_m',
  'spec_ngram_simple_min_hits',
  'spec_ngram_map_k_size_n',
  'spec_ngram_map_k_size_m',
  'spec_ngram_map_k_min_hits',
  'spec_ngram_map_k4v_size_n',
  'spec_ngram_map_k4v_size_m',
  'spec_ngram_map_k4v_min_hits',
  'lookup_cache_static',
  'lookup_cache_dynamic',
  'spec_draft_model',
  'cache_type_k_draft',
  'cache_type_v_draft',
  'chat_template_file',
  'server_api_key_enabled',
  'expose_server',
  'enable_web_ui',
  'swa_full',
  'ctx_checkpoints',
  'checkpoint_min_step',
  'tools',
  'prefill_assistant',
  'slot_prompt_similarity',
  'skip_chat_parsing',
  'api_prefix',
  'simple_io',
  'agent',
  'ui_mcp_proxy',
  'backend_sampling',
  'gpu_layers',
  'flash_attn',
  'mlock',
  'threads',
  'threads_http',
  'batch_size',
  // 原生能力：直接 I/O / MoE CPU 卸载 / 算子卸载
  'direct_io',
  'cpu_moe',
  'n_cpu_moe',
  'op_offload',
  // 推理配置
  'reasoning',
  'reasoning_budget',
  'reasoning_budget_message',
  'reasoning_format',
  'reasoning_effort',
  'reasoning_preserve',
  // RAG 重排序配置
  'reranker_model_path',
  'rerank_top_n',
  // KV 缓存持久化配置
  'slot_save_path',
  'slot_save_enabled',
  'lora_paths',
  // Draft 模型 GPU 配置
  'spec_draft_ngl',
  'spec_draft_device',
  // 请求级采样配置
  'samplers',
  'ignore_eos',
  'adaptive_target',
  'adaptive_decay',
  // TTS 朗读配置
  'tts_enabled',
  'tts_voice',
  'tts_rate',
  'tts_pitch',
  'tts_volume'
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
  // 注意：组件已卸载，message 实例可能失效，静默保存并 catch 错误
  if (genParamsSaveTimer) {
    clearTimeout(genParamsSaveTimer)
    genParamsSaveTimer = null
    if (genParamsDirty.value) {
      autoSave().catch(e => {
        // 静默处理：组件已卸载，无法弹 UI 提示，仅控制台记录
        logError('[settings] onUnmounted autoSave failed:', e)
      })
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
  hasModelPreset,
  saveModelPreset,
  clearModelPreset,
  savingModelPreset,
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
  onEnableWebUIToggle,
  cacheTypeKOptions,
  cacheTypeVOptions,
  specTypeOptions,
  handleAgentChange,
  handleBackendSamplingChange,
  settingsStore
}

provide(SETTINGS_CONTEXT_KEY, settingsContext)
</script>

<style scoped>
.settings-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary);
}

.settings-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 24px;
  border-bottom: 1px solid var(--border-color);
}

/* .back-btn 样式已抽取到 style.css 全局（F-1.15），此处不再重复 */

.settings-title {
  font-size: 22px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: -0.2px;
}

.settings-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px 24px 80px;
  max-width: 640px;
  width: 100%;
  margin: 0 auto;
  /* 自定义滚动条，避免占用内容空间 */
  scrollbar-width: thin;
  scrollbar-color: var(--border-color) transparent;
}

/* Q10: 设置面板的滚动条与全局 ::-webkit-scrollbar 有意不同
 * 全局用 --scrollbar-thumb + 4px 圆角 + hover 时变主题色（显眼）
 * 此处用 --border-color + 3px 圆角 + hover 时变 --text-tertiary（低调，不抢设置内容焦点）
 * 不要强行统一，避免设置面板滚动条变得过于显眼影响阅读 */
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
  background-color: var(--text-tertiary);
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

/* 折叠面板密度优化：项之间间距适中，避免过于拥挤 */
.settings-content :deep(.n-collapse-item) {
  margin-bottom: 4px;
}

.settings-content :deep(.n-collapse-item__header) {
  padding: 12px 8px;
  border-radius: var(--border-radius-sm);
  transition: background-color var(--transition-fast);
}

.settings-content :deep(.n-collapse-item__header:hover) {
  background: var(--bg-hover);
}

.settings-content :deep(.n-collapse-item__content-inner) {
  padding-top: 8px;
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
