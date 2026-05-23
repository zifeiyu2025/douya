<template>
  <n-config-provider :theme="isDark ? darkTheme : undefined" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <div class="app-layout" :class="{ dark: isDark }">
          <Sidebar :collapsed="sidebarCollapsed" @toggle="sidebarCollapsed = !sidebarCollapsed" />
          <div class="main-area">
            <div class="main-header">
              <div class="main-header-left">
                <n-button quaternary circle @click="sidebarCollapsed = !sidebarCollapsed" size="large">
                  <template #icon>
                    <n-icon size="20"><MenuOutline /></n-icon>
                  </template>
                </n-button>
                <div class="header-content">
                  <div class="header-title">{{ currentTitle }}</div>
                </div>
              </div>
              <div class="main-header-right">
                <n-select
                  :value="selectedModel"
                  :options="modelOptions"
                  size="small"
                  placeholder="选择模型"
                  class="model-selector"
                  :disabled="isModelSwitching"
                  :render-label="renderModelLabel"
                  @update:value="handleModelChange"
                />
                <div class="server-status" :title="modelFullName">
                  <div v-if="isModelSwitching" class="switching-animation">
                    <div class="loading-spinner"></div>
                    <span class="status-text">{{ switchingModelName }} · 加载中{{ switchDuration }}</span>
                  </div>
                  <div v-else-if="modelLoadFailed" class="error-animation">
                    <span class="status-dot stopped" />
                    <span class="status-text error-text">{{ errorModelName }} · 加载失败</span>
                  </div>
                  <div v-else-if="isServerLoading" class="loading-animation">
                    <div class="loading-spinner"></div>
                    <span class="status-text">{{ modelName || '启动中...' }}</span>
                  </div>
                  <template v-else>
                    <span class="status-dot" :class="serverStatus.running ? 'running' : 'stopped'" />
                    <span class="status-text">{{ modelName }} · {{ serverStatus.running ? '已就绪' : '未运行' }}</span>
                  </template>
                </div>
              </div>
            </div>
            <router-view />
          </div>
        </div>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { darkTheme, zhCN, dateZhCN, createDiscreteApi, NTooltip } from 'naive-ui'
import { NConfigProvider, NMessageProvider, NDialogProvider, NButton, NIcon, NSelect } from 'naive-ui'
import { MenuOutline } from '@vicons/ionicons5'
import Sidebar from './components/Sidebar.vue'
import { useChatStore } from './stores/chat'
import { fixUtf8 } from './utils/utf8'
import { useSettingsStore } from './stores/settings'
import { useThemeStore } from './stores/theme'
import { formatModelName, formatModelNameFromPath, extractQuantSuffix } from './utils/model'
import type { Conversation, ModelOption } from './services/wails'
import { wails } from './services/wails'

const { message: discreteMessage } = createDiscreteApi(['message'])

const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()

const isDark = computed(() => themeStore.isDark)
const serverStatus = computed(() => settingsStore.serverStatus)
const sidebarCollapsed = defineModel<boolean>('sidebarCollapsed', { default: false })

const isServerLoading = computed(() => {
  return serverStatus.value.switching || (!serverStatus.value.running && !serverStatus.value.error)
})

const isModelSwitching = computed(() => settingsStore.isModelSwitching)
const modelLoadFailed = computed(() => settingsStore.modelLoadFailed)
const switchingModelDisplay = computed(() => settingsStore.switchingModelDisplay)
const switchStartedAt = computed(() => settingsStore.switchStartedAt)
const previousModelBeforeSwitch = computed(() => settingsStore.previousModelBeforeSwitch)

const switchingModelName = computed(() => {
  if (switchingModelDisplay.value) return switchingModelDisplay.value
  if (serverStatus.value.switching_to) return formatModelName(serverStatus.value.switching_to).display
  return ''
})

const errorModelName = computed(() => {
  if (switchingModelDisplay.value) return switchingModelDisplay.value
  if (serverStatus.value.switching_to) return formatModelName(serverStatus.value.switching_to).display
  return modelName.value || ''
})

const switchDuration = ref('')
let switchDurationTimer: ReturnType<typeof setInterval> | null = null

watch(() => settingsStore.isModelSwitching, (isSwitching) => {
  if (isSwitching) {
    stopSwitchDurationTimer()
    switchDurationTimer = setInterval(() => {
      if (settingsStore.switchStartedAt > 0) {
        const elapsed = Math.floor((Date.now() - settingsStore.switchStartedAt) / 1000)
        if (elapsed > 0) {
          switchDuration.value = ` · 已等待 ${elapsed}s`
        }
      }
    }, 1000)
  } else {
    stopSwitchDurationTimer()
  }
})

function stopSwitchDurationTimer() {
  if (switchDurationTimer) {
    clearInterval(switchDurationTimer)
    switchDurationTimer = null
  }
  switchDuration.value = ''
}

watch(() => settingsStore.currentModel, (newModel) => {
  if (isModelSwitching.value) return
  if (newModel && newModel !== selectedModel.value) {
    const match = modelOptions.value.find(m => m.value === newModel)
    if (match) {
      selectedModel.value = newModel
    }
  }
})

const modelOptions = ref<{ label: string; value: string; fullName: string; quantSuffix: string; isLoaded: boolean; mmprojVision: boolean; mmprojAudio: boolean; status: string }[]>([])
const availableModels = ref<ModelOption[]>([])
const selectedModel = ref('')

const modelName = computed(() => {
  if (selectedModel.value) {
    return formatModelName(selectedModel.value).display
  }
  const path = settingsStore.config.model_path
  if (!path) return ''
  return formatModelNameFromPath(path).display
})

const modelFullName = computed(() => {
  const model = availableModels.value.find(m => m.name === selectedModel.value)
  if (model?.file_name) return model.file_name
  if (selectedModel.value) {
    return selectedModel.value
  }
  const path = settingsStore.config.model_path
  return path || ''
})

const currentTitle = computed(() => {
  const conv = chatStore.conversations.find((c: Conversation) => c.id === chatStore.currentConversationId)
  return fixUtf8(conv?.title || '豆芽 AI')
})

function renderModelLabel(option: { label: string; value: string; fullName?: string; quantSuffix?: string; isLoaded?: boolean; mmprojVision?: boolean; mmprojAudio?: boolean; status?: string }) {
  const children = [h('span', option.label)]
  if (option.quantSuffix) {
    children.push(h('span', {
      style: 'color: var(--text-muted); font-size: 11px; margin-left: 4px; font-weight: 400;'
    }, option.quantSuffix))
  }
  const tags: string[] = []
  if (option.mmprojVision) tags.push('📷')
  if (option.mmprojAudio) tags.push('🎤')
  if (tags.length > 0) {
    children.push(h('span', {
      style: 'margin-left: 6px; font-size: 11px;'
    }, tags.join(' ')))
  }
  if (option.status === 'sleeping') {
    children.push(h('span', {
      style: 'color: #f0a020; margin-left: 6px; font-size: 10px;'
    }, '💤'))
  } else if (option.status === 'loading') {
    children.push(h('span', {
      style: 'color: var(--accent-primary); margin-left: 6px; font-size: 10px;'
    }, '⏳'))
  } else if (option.isLoaded) {
    children.push(h('span', {
      style: 'color: var(--accent-primary); margin-left: 6px; font-size: 10px;'
    }, '●'))
  }
  const content = h('span', { style: 'display: inline-flex; align-items: center' }, children)
  return h(NTooltip, { placement: 'right', delay: 300 }, {
    trigger: () => content,
    default: () => option.fullName || option.value,
  })
}

async function loadAvailableModels() {
  try {
    const models = await wails.getAvailableModels()
    availableModels.value = models
    modelOptions.value = models.map(m => {
      const { display } = formatModelName(m.name)
      const quantSuffix = extractQuantSuffix(m.file_name || '')
      return {
        label: display,
        value: m.name,
        fullName: m.file_name || m.name,
        quantSuffix,
        isLoaded: m.is_loaded,
        mmprojVision: m.mmproj_vision,
        mmprojAudio: m.mmproj_audio,
        status: m.status,
      }
    })

    const defaultModel = models.find(m => m.is_default)
    if (defaultModel && !selectedModel.value) {
      selectedModel.value = defaultModel.name
    }
  } catch (e) {
    console.error('Failed to load available models:', e)
  }
}

async function handleModelChange(value: string) {
  if (isModelSwitching.value) return

  const targetModel = availableModels.value.find(m => m.name === value)
  if (!targetModel) {
    console.error('Unknown model:', value)
    return
  }

  const previousModel = settingsStore.currentModel || selectedModel.value
  selectedModel.value = value

  try {
    const result = await settingsStore.switchModel(value, previousModel)
    if (result.success) {
      selectedModel.value = result.current_model || value
      const caps = result.capabilities || settingsStore.modelCapabilities
      const features: string[] = []
      if (caps.image_input) features.push('图片')
      if (caps.audio_input) features.push('音频')
      if (caps.reasoning) features.push('推理')
      const featureText = features.length > 0 ? ` · 支持${features.join('、')}` : ' · 仅文本'
      discreteMessage.success(`${formatModelName(result.current_model || value).display}${featureText} 已就绪`, { duration: 3000 })
      loadAvailableModels()
    } else {
      selectedModel.value = result.rolled_back
        ? (result.current_model || previousModel)
        : previousModel
      discreteMessage.error(result.error || '模型加载失败', { duration: 5000 })
    }
  } catch (e) {
    console.error('Failed to switch model:', e)
    selectedModel.value = previousModel
    discreteMessage.error(`切换模型失败: ${e}`, { duration: 5000 })
  }
}

function handleBeforeUnload() {
    wails.prepareShutdown()
}

onMounted(async () => {
  chatStore.initStreamListener()
  settingsStore.initStatusListener()
  chatStore.loadConversations()
  await settingsStore.loadConfig()

  wails.onAbnormalCleanup((data) => {
    chatStore.loadConversations()
    discreteMessage.info(`已自动清理 ${data.count} 个异常会话（无有效消息）`, { duration: 5000 })
  })

  try {
    const result = await wails.getCleanupResult()
    if (result && result.length > 0) {
      chatStore.loadConversations()
      discreteMessage.info(`已自动清理 ${result.length} 个异常会话（无有效消息）`, { duration: 5000 })
    }
  } catch (e) {
    console.error('检查清理结果失败:', e)
  }

  loadAvailableModels()

  window.addEventListener('beforeunload', handleBeforeUnload)
})

onUnmounted(() => {
  chatStore.cleanupStreamListener()
  settingsStore.cleanupStatusListener()
  wails.offAbnormalCleanup()
  window.removeEventListener('beforeunload', handleBeforeUnload)
})
</script>

<style scoped>
.model-selector {
  min-width: 140px;
  max-width: 200px;
}

.server-status {
  min-width: 180px;
  margin-left: 8px;
  justify-content: flex-end;
}

.loading-animation {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-secondary);
  font-size: 14px;
}

.switching-animation {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--accent-warning);
  font-size: 14px;
  font-weight: 500;
  animation: switchingPulse 1.5s ease-in-out infinite;
}

.error-animation {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}

.error-text {
  color: #f04444;
}

@keyframes switchingPulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.6;
  }
}

.loading-spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--border-color);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
