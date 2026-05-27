<template>
  <n-config-provider :theme="isDark ? darkTheme : undefined" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <div class="app-layout" :class="{ dark: isDark, 'has-background': !!settingsStore.config.chat_background }" :style="mainAreaStyle">
          <Sidebar :collapsed="sidebarCollapsed" @toggle="sidebarCollapsed = !sidebarCollapsed" />
          <div class="main-area" :class="{ 'sidebar-collapsed': sidebarCollapsed }">
            <div class="main-header" style="--wails-draggable:drag">
              <div class="main-header-left" style="--wails-draggable:no-drag">
                <n-button quaternary circle @click="sidebarCollapsed = !sidebarCollapsed" size="large">
                  <template #icon>
                    <n-icon size="20"><MenuOutline /></n-icon>
                  </template>
                </n-button>
                <div class="header-content">
                  <div class="header-title" :title="currentTitle">{{ currentTitle }}</div>
                </div>
              </div>
              <div class="main-header-right" style="--wails-draggable:no-drag">
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
              <div class="window-controls" style="--wails-draggable:no-drag">
                <button class="win-btn theme-btn" @click="themeStore.toggleTheme()" :title="isDark ? '切换亮色模式' : '切换暗色模式'">
                  <n-icon size="16">
                    <SunnyOutline v-if="isDark" />
                    <MoonOutline v-else />
                  </n-icon>
                </button>
                <button class="win-btn" @click="handleMinimize" title="最小化">
                  <svg width="12" height="12" viewBox="0 0 12 12"><rect y="5" width="12" height="1.5" fill="currentColor"/></svg>
                </button>
                <button class="win-btn" @click="handleToggleMaximize" title="最大化">
                  <svg v-if="isMaximized" width="12" height="12" viewBox="0 0 12 12">
                    <rect x="2.5" y="0" width="9.5" height="9.5" fill="none" stroke="currentColor" stroke-width="1.2"/>
                    <rect x="0" y="2.5" width="9.5" height="9.5" fill="var(--bg-primary)" stroke="currentColor" stroke-width="1.2"/>
                  </svg>
                  <svg v-else width="12" height="12" viewBox="0 0 12 12">
                    <rect x="0.5" y="0.5" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.2"/>
                  </svg>
                </button>
                <button class="win-btn win-btn-close" @click="handleClose" title="关闭">
                  <svg width="12" height="12" viewBox="0 0 12 12">
                    <line x1="1" y1="1" x2="11" y2="11" stroke="currentColor" stroke-width="1.4"/>
                    <line x1="11" y1="1" x2="1" y2="11" stroke="currentColor" stroke-width="1.4"/>
                  </svg>
                </button>
              </div>
            </div>
            <router-view />
          </div>
        </div>
    <Transition name="switch-overlay">
      <div v-if="isExiting" class="switch-overlay">
        <div class="switch-overlay-content">
          <div class="switch-spinner"></div>
          <div class="switch-model-name">正在退出豆芽</div>
          <div class="switch-progress-msg">{{ exitMessage }}</div>
        </div>
      </div>
    </Transition>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { darkTheme, zhCN, dateZhCN, createDiscreteApi } from 'naive-ui'
import { NConfigProvider, NMessageProvider, NDialogProvider, NButton, NIcon, NSelect, NTooltip } from 'naive-ui'
import { MenuOutline, SunnyOutline, MoonOutline } from '@vicons/ionicons5'
import Sidebar from './components/Sidebar.vue'
import { useChatStore } from './stores/chat'
import { fixUtf8 } from './utils/utf8'
import { useSettingsStore } from './stores/settings'
import { useThemeStore } from './stores/theme'
import { formatModelName, formatModelNameFromPath, extractQuantSuffix } from './utils/model'
import type { Conversation, ModelOption } from './services/wails'
import { wails } from './services/wails'
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised, WindowHide } from '../wailsjs/runtime/runtime'

const { message: discreteMessage } = createDiscreteApi(['message'])

const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()

const isDark = computed(() => themeStore.isDark)
const serverStatus = computed(() => settingsStore.serverStatus)
const sidebarCollapsed = defineModel<boolean>('sidebarCollapsed', { default: false })

const mainAreaStyle = computed(() => {
  if (settingsStore.config.chat_background) {
    const opacity = settingsStore.config.chat_background_opacity ?? 0.8
    return {
      '--chat-background': `url(${settingsStore.config.chat_background})`,
      '--chat-background-opacity': String(opacity)
    } as Record<string, string>
  }
  return {}
})

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

const isMaximized = ref(false)
const isExiting = ref(false)
const exitMessage = ref('')

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

// 保存模型切换结果，等动效结束后再显示提示
let pendingModelSwitchResult: any = null

// 监听模型切换状态变化，当切换结束时显示提示
watch(() => settingsStore.isModelSwitching, (newVal, oldVal) => {
  if (oldVal && !newVal && pendingModelSwitchResult) {
    // 动效结束了，现在显示提示消息
    const result = pendingModelSwitchResult
    pendingModelSwitchResult = null
    // 等待 0.3s 过渡动画完全结束
    setTimeout(() => {
      if (result.success) {
        const caps = result.capabilities || settingsStore.modelCapabilities
        const features: string[] = []
        if (caps.image_input) features.push('图片')
        if (caps.audio_input) features.push('音频')
        if (caps.reasoning) features.push('推理')
        const featureText = features.length > 0 ? ` · 支持${features.join('、')}` : ' · 仅文本'
        discreteMessage.success(`${formatModelName(result.current_model).display}${featureText} 已就绪`, { duration: 3000 })
        loadAvailableModels()
      } else {
        discreteMessage.error(result.error || '模型加载失败', { duration: 5000 })
      }
    }, 300)
  }
})

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
    pendingModelSwitchResult = result
    if (result.success) {
      selectedModel.value = result.current_model || value
    } else {
      selectedModel.value = result.rolled_back
        ? (result.current_model || previousModel)
        : previousModel
    }
  } catch (e) {
    console.error('Failed to switch model:', e)
    selectedModel.value = previousModel
    discreteMessage.error(`切换模型失败: ${e}`, { duration: 5000 })
  }
}

function handleMinimize() {
  WindowMinimise()
}

function handleToggleMaximize() {
  WindowToggleMaximise()
  updateMaximizedState()
}

function handleClose() {
  WindowHide()
}

async function updateMaximizedState() {
  try {
    isMaximized.value = await WindowIsMaximised()
  } catch {
    isMaximized.value = false
  }
}

onMounted(async () => {
  chatStore.initStreamListener()
  settingsStore.initStatusListener()
  settingsStore.initSwitchProgressListener()
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
  updateMaximizedState()

  wails.onShutdownProgress((progress: { stage: string, message: string }) => {
    isExiting.value = true
    exitMessage.value = progress.message
  })

  window.addEventListener('resize', updateMaximizedState)
})

onUnmounted(() => {
  chatStore.cleanupStreamListener()
  settingsStore.cleanupStatusListener()
  settingsStore.cleanupSwitchProgressListener()
  wails.offAbnormalCleanup()
  wails.offSwitchProgress()
  wails.offShutdownProgress()
  window.removeEventListener('resize', updateMaximizedState)
})
</script>

<style scoped>
.model-selector {
  min-width: 120px;
  max-width: 260px;
  flex-shrink: 0;
}

.loading-animation,
.switching-animation,
.error-animation {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.switching-animation {
  color: var(--accent-warning);
  font-weight: 500;
  animation: switchingPulse 1.5s ease-in-out infinite;
}

.error-text {
  color: var(--accent-danger);
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

.window-controls {
  display: flex;
  align-items: center;
  height: 100%;
  -webkit-app-region: no-drag;
  flex-shrink: 0;
  z-index: 100;
}

.win-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 100%;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
  flex-shrink: 0;
}

.win-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.win-btn:active {
  background: var(--bg-active);
}

.win-btn-close:hover {
  background: #e81123;
  color: #ffffff;
}

.win-btn-close:active {
  background: #bf0f1d;
  color: #ffffff;
}

:global(.dark) .theme-btn {
  background: transparent;
}

:global(.dark) .theme-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

:global(.dark) .win-btn-close:hover {
  background: #e81123;
  color: #ffffff;
}

:global(.dark) .win-btn-close:active {
  background: #bf0f1d;
  color: #ffffff;
}

.switch-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: var(--bg-primary);
  opacity: 0.85;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  pointer-events: auto;
}

.switch-overlay-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}

.switch-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--border-color);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: exit-spin 0.8s linear infinite;
}

.switch-model-name {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.switch-progress-msg {
  font-size: 13px;
  color: var(--text-secondary);
}

.switch-overlay-enter-active,
.switch-overlay-leave-active {
  transition: opacity 0.3s ease;
}

.switch-overlay-enter-from,
.switch-overlay-leave-to {
  opacity: 0;
}

@keyframes exit-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
