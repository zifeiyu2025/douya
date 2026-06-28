<template>
  <n-config-provider :theme="isDark ? darkTheme : undefined" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <Transition name="main-fade">
          <div v-if="!showSplash" class="app-layout" :class="{ dark: isDark, 'has-background': !!settingsStore.config.chat_background }" :style="mainAreaStyle">
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
                    :disabled="isModelSwitching || !serverStatus.running"
                    :render-label="renderModelLabel"
                    @update:value="handleModelChange"
                  />
                  <div class="server-status" :title="modelFullName">
                    <div v-if="isModelSwitching && switchProgressStage !== 'idle'" class="switching-animation">
                      <div class="loading-spinner"></div>
                      <span class="status-text">{{ switchingModelName }} · {{ switchStageText }}{{ switchDuration }}</span>
                    </div>
                    <div v-else-if="modelLoadProgress && modelLoadProgress.status === 'loading'" class="load-progress-animation">
                      <div class="loading-spinner"></div>
                      <div class="load-progress-info">
                        <span class="status-text">{{ loadProgressModelName }} · 加载 {{ modelLoadProgress.progress }}%</span>
                        <div class="load-progress-bar">
                          <div class="load-progress-bar-fill" :style="{ width: modelLoadProgress.progress + '%' }"></div>
                        </div>
                      </div>
                    </div>
                    <div v-else-if="modelLoadFailed" class="error-animation">
                      <span class="status-dot stopped" />
                      <span class="status-text error-text">{{ errorModelName }} · 加载失败</span>
                    </div>
                    <div v-else-if="isServerLoading && switchProgressStage === 'idle' && !isFirstLoad" class="loading-animation">
                      <div class="loading-spinner"></div>
                      <span class="status-text">{{ modelName || '启动中...' }}</span>
                    </div>
                    <template v-else>
                      <span class="status-dot" :class="serverStatus.running ? 'running' : 'stopped'" />
                      <span class="status-text" :class="{ 'error-text': !serverStatus.running && serverStatus.error }">{{ modelName }} · {{ serverStatus.running ? '已就绪' : (serverStatus.error || '未运行') }}</span>
                    </template>
                  </div>
                </div>
                <div class="window-controls" style="--wails-draggable:no-drag">
                  <button class="win-btn" @click="toggleConsole" title="服务器控制台">
                    <n-icon size="16">
                      <TerminalOutline />
                    </n-icon>
                  </button>
                  <button class="win-btn theme-btn" @click="themeStore.toggleTheme()" :title="isDark ? '切换亮色模式' : '切换暗色模式'">
                    <n-icon size="16">
                      <SunnyOutline v-if="isDark" />
                      <MoonOutline v-else />
                    </n-icon>
                  </button>
                  <button class="win-btn" @click="handleMinimize" title="最小化">
                    <AppIcon name="minimize" :size="14" />
                  </button>
                  <button class="win-btn" @click="handleToggleMaximize" title="最大化">
                    <AppIcon :name="isMaximized ? 'restore' : 'maximize'" :size="12" />
                  </button>
                  <button class="win-btn win-btn-close" @click="handleClose" title="关闭">
                    <AppIcon name="close" :size="14" />
                  </button>
                </div>
              </div>
              <router-view v-slot="{ Component }">
                <Transition name="route-fade" mode="out-in">
                  <component :is="Component" />
                </Transition>
              </router-view>
            </div>
          </div>
        </Transition>
    <Transition name="switch-overlay">
      <div v-if="showSwitchOverlay" class="switch-overlay switch-overlay--model">
        <!-- SVG 装饰层：同心圆 + 双层旋转弧线 -->
        <svg class="switch-deco" width="360" height="360" viewBox="0 0 360 360" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
          <circle cx="180" cy="180" r="160" stroke="currentColor" stroke-width="1" opacity="0.06" />
          <circle cx="180" cy="180" r="120" stroke="currentColor" stroke-width="1" opacity="0.08" />
          <circle cx="180" cy="180" r="160" stroke="currentColor" stroke-width="1.5"
            stroke-linecap="round" stroke-dasharray="50 320" class="switch-deco-outer" opacity="0.35" />
          <circle cx="180" cy="180" r="120" stroke="currentColor" stroke-width="2"
            stroke-linecap="round" stroke-dasharray="35 240" class="switch-deco-mid" opacity="0.45" />
        </svg>
        <div class="switch-overlay-content">
          <!-- 圆形进度环 + 中心 LOGO 图片 -->
          <div class="switch-ring-wrapper">
            <svg class="switch-ring-svg" width="80" height="80" viewBox="0 0 80 80">
              <circle cx="40" cy="40" r="36" stroke="var(--border-color)" stroke-width="2" fill="none" opacity="0.3" />
              <circle cx="40" cy="40" r="36" stroke="var(--accent-primary)" stroke-width="2.5"
                stroke-linecap="round" fill="none"
                stroke-dasharray="85 150"
                class="switch-ring-arc" />
            </svg>
            <div class="switch-ring-center">
              <img :src="appLogo" alt="豆芽" class="switch-ring-logo" />
            </div>
          </div>
          <div class="switch-model-name">{{ overlayModelName }}</div>
          <div class="switch-progress-msg">{{ switchStageText }}</div>
          <!-- 阶段指示器：3 阶段进度 -->
          <div class="switch-stage-indicator">
            <div
              v-for="(stage, idx) in switchStages"
              :key="idx"
              :class="['stage-item', {
                'active': getSwitchStageIndex() >= idx,
                'completed': getSwitchStageIndex() > idx
              }]"
            >
              <span class="stage-dot"></span>
              <span class="stage-label">{{ stage }}</span>
            </div>
          </div>
        </div>
      </div>
    </Transition>
    <Transition name="exit-overlay">
      <div v-if="isExiting" class="switch-overlay switch-overlay--exit">
        <!-- 退出动效：渐变收缩消散装饰 -->
        <svg class="exit-deco" width="360" height="360" viewBox="0 0 360 360" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
          <!-- 三层同心圆，从外到内逐渐消失 -->
          <circle cx="180" cy="180" r="160" stroke="currentColor" stroke-width="1" opacity="0.08" class="exit-ring-outer" />
          <circle cx="180" cy="180" r="120" stroke="currentColor" stroke-width="1.5" opacity="0.15" class="exit-ring-mid" />
          <circle cx="180" cy="180" r="80" stroke="currentColor" stroke-width="2" opacity="0.25" class="exit-ring-inner" />
        </svg>
        <div class="switch-overlay-content">
          <!-- 退出动效：仅 LOGO（简洁） -->
          <div class="exit-logo-wrapper">
            <img :src="appLogo" alt="豆芽" class="exit-logo-img" />
          </div>
          <div class="switch-model-name">正在退出豆芽</div>
          <div class="switch-progress-msg">{{ exitMessage }}</div>
        </div>
      </div>
    </Transition>
    <SplashScreen
      :visible="showSplash"
      :stage="splashStage"
      :model-name="splashModelName"
      :progress="splashProgress"
    />
    <ServerConsole ref="consoleRef" />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { darkTheme, zhCN, dateZhCN, createDiscreteApi } from 'naive-ui'
import { NConfigProvider, NMessageProvider, NDialogProvider, NButton, NIcon, NSelect, NTooltip } from 'naive-ui'
import { MenuOutline, SunnyOutline, MoonOutline, TerminalOutline, TrashOutline } from '@vicons/ionicons5'
import Sidebar from './components/Sidebar.vue'
import AppIcon from './components/ui/AppIcon.vue'
import SplashScreen from './components/ui/SplashScreen.vue'
import ServerConsole from './components/ServerConsole.vue'
import { useChatStore } from './stores/chat'
import { fixUtf8 } from './utils/utf8'
import { useSettingsStore } from './stores/settings'
import { useThemeStore } from './stores/theme'
import { formatModelName, formatModelNameFromPath, extractQuantSuffix } from './utils/model'
import { classifyError } from './utils/errorGuidance'
import type { Conversation, ModelOption } from './services/wails'
import { wails } from './services/wails'
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised, WindowHide } from '../wailsjs/runtime/runtime'
import appLogo from './assets/images/appicon.png'

const { message: discreteMessage, dialog: discreteDialog } = createDiscreteApi(['message', 'dialog'])

const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()

const isDark = computed(() => themeStore.isDark)
const serverStatus = computed(() => settingsStore.serverStatus)
const sidebarCollapsed = defineModel<boolean>('sidebarCollapsed', { default: false })

const mainAreaStyle = computed(() => {
  if (settingsStore.config.chat_background) {
    const opacity = settingsStore.config.chat_background_opacity ?? 0.8
    const bg = settingsStore.config.chat_background
    let bgUrl: string
    if (bg.startsWith('data:')) {
      bgUrl = bg
    } else {
      bgUrl = '/local-file/' + encodeURIComponent(bg)
    }
    return {
      '--chat-background': `url(${bgUrl})`,
      '--chat-background-opacity': String(opacity)
    } as Record<string, string>
  }
  return {}
})

const isServerLoading = computed(() => {
  return serverStatus.value.switching || (!serverStatus.value.model_ready && !serverStatus.value.error)
})

const isModelSwitching = computed(() => settingsStore.isModelSwitching)
const isFirstLoad = computed(() => settingsStore.isFirstLoad)
const modelLoadFailed = computed(() => settingsStore.modelLoadFailed)
const modelLoadProgress = computed(() => settingsStore.modelLoadProgress)
const switchingModelDisplay = computed(() => settingsStore.switchingModelDisplay)
const switchStartedAt = computed(() => settingsStore.switchStartedAt)
const previousModelBeforeSwitch = computed(() => settingsStore.previousModelBeforeSwitch)

const switchingModelName = computed(() => {
  if (switchingModelDisplay.value) return switchingModelDisplay.value
  if (serverStatus.value.switching_to) return formatModelName(serverStatus.value.switching_to).display
  return ''
})

const loadProgressModelName = computed(() => {
  if (!modelLoadProgress.value) return ''
  return formatModelName(modelLoadProgress.value.model).display
})

const errorModelName = computed(() => {
  if (switchingModelDisplay.value) return switchingModelDisplay.value
  if (serverStatus.value.switching_to) return formatModelName(serverStatus.value.switching_to).display
  return modelName.value || ''
})

const switchDuration = ref('')
let switchDurationTimer: ReturnType<typeof setInterval> | null = null

// 合并触发条件：切换进行中（isModelSwitching）或切换后反馈（stage 非 idle）都显示 overlay
// 这样 MessageList.vue 不再需要自己的切换 overlay，避免重复
const showSwitchOverlay = computed(() =>
  isModelSwitching.value || (settingsStore.switchProgress.stage !== 'idle' && settingsStore.hasEverBeenReady)
)

const switchProgressStage = computed(() => settingsStore.switchProgress.stage)

const switchStageText = computed(() => {
  // 切换进行中（前端发起，后端还未推送 stage）
  if (isModelSwitching.value && settingsStore.switchProgress.stage === 'idle') {
    return '正在切换模型...'
  }
  const stage = settingsStore.switchProgress.stage
  const texts: Record<string, string> = {
    'preparing': '准备切换模型...',
    'loading': '加载模型中...',
    'waiting': '初始化模型...',
    'detecting': '检测模型能力...',
    'done': '加载完成',
    'failed': '模型加载失败',
    'vram-warning': 'VRAM 不足警告，可能影响性能...',
    'spec-warning': '推测解码兼容性警告...',
  }
  return texts[stage] || '加载中...'
})

const overlayModelName = computed(() => {
  if (settingsStore.switchProgress.targetModel) return settingsStore.switchProgress.targetModel
  if (switchingModelDisplay.value) return switchingModelDisplay.value
  return ''
})

// 切换阶段指示器（3 阶段，与原 MessageList 逻辑一致）
const switchStages = ['准备切换', '加载新模型', '初始化完成']

function getSwitchStageIndex(): number {
  // 切换进行中但后端未推送 stage 时，显示第一阶段
  if (isModelSwitching.value && settingsStore.switchProgress.stage === 'idle') return 0
  const stage = settingsStore.switchProgress.stage
  switch (stage) {
    case 'preparing': return 0
    case 'loading': return 1
    case 'done': return 2
    default: return 0
  }
}

const isMaximized = ref(false)
const isExiting = ref(false)
const exitMessage = ref('')

// 服务器控制台
const consoleRef = ref()
const toggleConsole = () => {
  consoleRef.value?.toggle()
}

// SplashScreen 逻辑
const modelLoadTimeout = computed(() => settingsStore.switchState.phase === 'timeout')
const showSplash = computed(() => {
  // 首次加载未就绪时显示 splash（无论 failed 还是 timeout 都仍显示）
  // SplashScreen 组件会根据 stage 决定是否转圈（timeout/failed 均映射为 'failed' stage，停止转圈）
  if (!settingsStore.hasEverBeenReady) return true
  // 已就绪后无论是否 timeout 都不显示 splash
  if (modelLoadTimeout.value) return false
  return false
})

const splashStage = computed(() => settingsStore.switchProgress.stage)

const splashModelName = computed(() => {
  const name = settingsStore.switchProgress.targetModel || settingsStore.currentModel
  if (!name) return ''
  return formatModelName(name).display
})

const splashProgress = computed(() => {
  // 优先使用后端推送的真实加载进度
  if (modelLoadProgress.value && modelLoadProgress.value.status === 'loading') {
    return Math.max(5, Math.min(99, Math.round(modelLoadProgress.value.progress)))
  }
  // 无真实进度时使用粗略阶段映射（仅作为兜底）
  const stageMap: Record<string, number> = {
    idle: 0, preparing: 5, loading: 10,
    waiting: 10, detecting: 90, done: 100,
    failed: 100, rolling_back: 50,
  }
  return stageMap[settingsStore.switchProgress.stage] ?? 0
})

// 切换 overlay 进度条百分比（复用 splashProgress 逻辑）
const switchProgressPercent = computed(() => {
  if (modelLoadProgress.value && modelLoadProgress.value.status === 'loading') {
    return Math.max(5, Math.min(99, Math.round(modelLoadProgress.value.progress)))
  }
  const stageMap: Record<string, number> = {
    idle: 0, preparing: 5, loading: 10,
    waiting: 10, detecting: 90, done: 100,
    failed: 100, rolling_back: 50,
  }
  return stageMap[settingsStore.switchProgress.stage] ?? 0
})

watch(switchProgressStage, (stage) => {
  if (stage !== 'idle') {
    stopSwitchDurationTimer()
    switchDurationTimer = setInterval(() => {
      const startTime = settingsStore.switchStartedAt || settingsStore.switchProgress.startTime
      if (startTime > 0) {
        const elapsed = Math.floor((Date.now() - startTime) / 1000)
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

const modelOptions = ref<{ label: string; value: string; fullName: string; quantSuffix: string; isLoaded: boolean; mmprojVision: boolean; mmprojAudio: boolean; mmprojVideo: boolean; status: string }[]>([])
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

function renderModelLabel(option: { label: string; value: string; fullName?: string; quantSuffix?: string; isLoaded?: boolean; mmprojVision?: boolean; mmprojAudio?: boolean; mmprojVideo?: boolean; status?: string }) {
  const children = [h('span', option.label)]
  if (option.quantSuffix) {
    children.push(h('span', {
      style: 'color: var(--text-muted); font-size: 11px; margin-left: 4px; font-weight: 400;'
    }, option.quantSuffix))
  }
  const tags: string[] = []
  if (option.mmprojVision) tags.push('📷')
  if (option.mmprojAudio) tags.push('🎤')
  if (option.mmprojVideo) tags.push('🎬')
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
  // 删除按钮：点击时阻止冒泡，避免触发模型切换
  const deleteBtn = h(NButton, {
    type: 'error',
    size: 'small',
    quaternary: true,
    style: 'margin-left: 8px; flex-shrink: 0;',
    onClick: (e: Event) => {
      e.stopPropagation()
      confirmDeleteModel(option.value, option.label)
    }
  }, {
    icon: () => h(NIcon, { size: 14 }, { default: () => h(TrashOutline) })
  })
  const content = h('span', {
    style: 'display: inline-flex; align-items: center; width: 100%; justify-content: space-between'
  }, [
    h('span', { style: 'display: inline-flex; align-items: center; min-width: 0; overflow: hidden' }, children),
    deleteBtn
  ])
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
        mmprojVideo: m.mmproj_video,
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

// 确认删除模型：当前使用模型需二次确认
function confirmDeleteModel(modelName: string, displayLabel: string) {
  const isCurrentModel = modelName === selectedModel.value
  if (isCurrentModel) {
    // 删除当前使用的模型：先显示额外警告，再二次确认
    discreteDialog.warning({
      title: '警告：删除当前模型',
      content: '您正在删除当前使用的模型，删除后将切换到默认模型。确定要继续吗？',
      positiveText: '继续',
      negativeText: '取消',
      onPositiveClick: () => {
        // 二次确认
        discreteDialog.warning({
          title: '再次确认',
          content: `请再次确认要删除模型 "${displayLabel}"。此操作不可撤销。`,
          positiveText: '确认删除',
          negativeText: '取消',
          onPositiveClick: async () => {
            await doDeleteModel(modelName)
          }
        })
      }
    })
  } else {
    // 删除非当前模型：单次确认
    discreteDialog.warning({
      title: '删除模型',
      content: `确认删除模型 "${displayLabel}" 吗？`,
      positiveText: '确认删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        await doDeleteModel(modelName)
      }
    })
  }
}

// 执行删除模型：调用后端删除并刷新列表
async function doDeleteModel(modelName: string) {
  try {
    await wails.deleteModel(modelName)
    await wails.reloadModels()
    await loadAvailableModels()
    discreteMessage.success('模型删除成功')
  } catch (e) {
    console.error('删除模型失败:', e)
    discreteMessage.error(`删除模型失败: ${e}`, { duration: 5000 })
  }
}

// 模型切换结果提示（在 handleModelChange 中 await 返回后直接处理，避免 watch 微任务竞态）

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
    // 直接处理结果，不再依赖 watch，避免微任务调度竞态导致提示延迟
    if (result.success) {
      selectedModel.value = result.current_model || value
      // 等待 0.3s 过渡动画完全结束后显示成功提示
      setTimeout(() => {
        const caps = result.capabilities || settingsStore.modelCapabilities
        const features: string[] = []
        if (caps.image_input) features.push('图片')
        if (caps.audio_input) features.push('音频')
        if (caps.reasoning) features.push('推理')
        const featureText = features.length > 0 ? ` · 支持${features.join('、')}` : ' · 仅文本'
        discreteMessage.success(`${formatModelName(result.current_model || value).display}${featureText} 已就绪`, { duration: 3000 })
        loadAvailableModels()
      }, 300)
    } else {
      selectedModel.value = result.rolled_back
        ? (result.current_model || previousModel)
        : previousModel
      // 等待 0.3s 过渡动画完全结束后显示失败提示
      setTimeout(() => {
        const errorText = result.error || '模型加载失败'
        const guidance = classifyError(errorText)
        if (guidance) {
          // 有修复指引的错误使用 dialog 展示详细信息
          const suggestions = guidance.suggestions.map((s, i) => `${i + 1}. ${s}`).join('\n')
          discreteDialog.error({
            title: guidance.title,
            content: `${guidance.description}\n\n修复建议：\n${suggestions}`,
            positiveText: '知道了',
            style: { whiteSpace: 'pre-wrap' },
          })
        } else {
          discreteMessage.error(errorText, { duration: 5000 })
        }
      }, 300)
    }
  } catch (e) {
    console.error('Failed to switch model:', e)
    selectedModel.value = previousModel
    const errorText = `切换模型失败: ${e}`
    const guidance = classifyError(String(e))
    if (guidance) {
      const suggestions = guidance.suggestions.map((s, i) => `${i + 1}. ${s}`).join('\n')
      discreteDialog.error({
        title: guidance.title,
        content: `${guidance.description}\n\n修复建议：\n${suggestions}`,
        positiveText: '知道了',
        style: { whiteSpace: 'pre-wrap' },
      })
    } else {
      discreteMessage.error(errorText, { duration: 5000 })
    }
  }
}

function handleMinimize() {
  WindowMinimise()
}

function handleToggleMaximize() {
  WindowToggleMaximise()
  updateMaximizedState()
}

async function handleClose() {
  const action = await wails.handleCloseRequest()
  if (action === 'exit') {
    wails.gracefulExit()
    return
  }
  if (action === 'tray') {
    WindowHide()
    return
  }
  // action === 'ask'：首次关闭时询问
  discreteDialog.warning({
    title: '关闭窗口',
    content: '你希望将豆芽最小化到系统托盘后台运行，还是直接退出程序？',
    positiveText: '最小化到托盘',
    negativeText: '直接退出',
    onPositiveClick: async () => {
      await wails.setCloseAction('tray')
      WindowHide()
    },
    onNegativeClick: async () => {
      await wails.setCloseAction('exit')
      wails.gracefulExit()
    },
  })
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
  settingsStore.initMmprojUnavailableListener()
  settingsStore.initModelLoadProgressListener()
  chatStore.loadConversations()
  await settingsStore.loadConfig()

  // 首次启动：当后端 ready 标志位置位（server:status 推送 running=true）后重新加载会话列表
  // 原因：onMounted 同步调用 loadConversations 时 a.ready 可能仍为 false，会被后端拒掉
  let hasLoadedOnReady = false
  watch(() => settingsStore.serverStatus.running, (running) => {
    if (running && !hasLoadedOnReady) {
      hasLoadedOnReady = true
      chatStore.loadConversations()
    }
  }, { immediate: true })

  // 首次启动失败时弹出修复建议对话框（而非仅在状态栏显示文字）
  // 生活类比：就像开店时设备出故障，不只挂个"暂停营业"牌子，还要告诉顾客具体出了什么问题、怎么修
  let hasShownStartupError = false
  watch(() => settingsStore.serverStatus.error, (errorVal) => {
    if (!errorVal || hasShownStartupError) return
    // 仅在首次加载阶段（从未就绪过）弹出 dialog，避免与手动切换模型的提示重复
    if (settingsStore.hasEverBeenReady) return
    hasShownStartupError = true
    const guidance = classifyError(errorVal)
    if (guidance) {
      const suggestions = guidance.suggestions.map((s, i) => `${i + 1}. ${s}`).join('\n')
      discreteDialog.error({
        title: guidance.title,
        content: `${guidance.description}\n\n错误详情：${errorVal}\n\n修复建议：\n${suggestions}`,
        positiveText: '知道了',
        style: { whiteSpace: 'pre-wrap' },
      })
    }
  })

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

  await Promise.all([loadAvailableModels(), updateMaximizedState()])

  wails.onShutdownProgress((progress: { stage: string, message: string }) => {
    isExiting.value = true
    exitMessage.value = progress.message
  })

  window.addEventListener('resize', updateMaximizedState)
})

onUnmounted(() => {
  stopSwitchDurationTimer()
  chatStore.cleanupStreamListener()
  settingsStore.cleanupStatusListener()
  settingsStore.cleanupSwitchProgressListener()
  settingsStore.cleanupMmprojUnavailableListener()
  settingsStore.cleanupModelLoadProgressListener()
  wails.offAbnormalCleanup()
  wails.offSwitchProgress()
  wails.offShutdownProgress()
  window.removeEventListener('resize', updateMaximizedState)
})
</script>

<style scoped>
.main-fade-enter-active {
  transition: opacity 0.5s ease 0.3s, transform 0.5s cubic-bezier(0.4, 0, 0.2, 1) 0.3s;
}

.main-fade-leave-active {
  transition: opacity 0.3s ease, transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.main-fade-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

.main-fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.model-selector {
  min-width: 120px;
  max-width: 260px;
  flex-shrink: 0;
}

.loading-animation,
.switching-animation,
.error-animation,
.load-progress-animation {
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

.load-progress-animation {
  color: var(--accent-primary);
  font-weight: 500;
}

.load-progress-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.load-progress-bar {
  width: 120px;
  height: 4px;
  background: var(--bg-secondary);
  border-radius: 2px;
  overflow: hidden;
}

.load-progress-bar-fill {
  height: 100%;
  background: var(--accent-primary);
  border-radius: 2px;
  transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
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
  opacity: 0.92;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  pointer-events: auto;
  /* 径向渐变营造氛围 */
  background-image: radial-gradient(circle at 50% 50%, rgba(7, 193, 96, 0.04) 0%, transparent 70%);
}

.switch-overlay-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  position: relative;
  z-index: 1;
}

/* ===== SVG 装饰层（切换动效）===== */
.switch-deco {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: var(--accent-primary);
  pointer-events: none;
  z-index: 0;
}

.switch-deco-outer {
  transform-origin: 180px 180px;
  animation: spin 30s linear infinite;
  will-change: transform;
}

.switch-deco-mid {
  transform-origin: 180px 180px;
  animation: spin-reverse 20s linear infinite;
  will-change: transform;
}

@keyframes spin-reverse {
  to { transform: rotate(-360deg); }
}

/* ===== 圆形进度环（与 MessageList 一致） ===== */
.switch-ring-wrapper {
  position: relative;
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 用纯色 rgba 替代 color-mix，避免 WebView2 兼容性问题 */
  filter: drop-shadow(0 0 8px rgba(7, 193, 96, 0.4));
}

.switch-ring-svg {
  display: block;
}

.switch-ring-arc {
  transform-origin: 40px 40px;
  animation: spin 1.4s linear infinite;
}

.switch-ring-center {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 圆环中心 LOGO 图片（替代原 pulse 点） */
.switch-ring-logo {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  object-fit: cover;
  /* 轻微呼吸缩放，提示正在加载 */
  animation: logo-breath 1.8s ease-in-out infinite;
}

@keyframes logo-breath {
  0%, 100% {
    transform: scale(1);
    opacity: 0.85;
  }
  50% {
    transform: scale(1.08);
    opacity: 1;
  }
}

.switch-model-name {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
  text-align: center;
  max-width: 320px;
  word-break: break-word;
}

.switch-progress-msg {
  font-size: 13px;
  color: var(--text-secondary);
}

/* ===== 阶段指示器（3 阶段）=====
 * 实用性：让用户知道当前进度到了哪一步
 */
.switch-stage-indicator {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-top: 8px;
}

.stage-item {
  display: flex;
  align-items: center;
  gap: 6px;
  opacity: 0.4;
  transition: opacity 0.3s ease;
}

.stage-item.active {
  opacity: 1;
}

.stage-item.completed {
  opacity: 0.8;
}

.stage-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--border-color);
  transition: background 0.3s ease, box-shadow 0.3s ease;
}

.stage-item.active .stage-dot {
  background: var(--accent-primary);
  box-shadow: 0 0 8px rgba(7, 193, 96, 0.6);
  animation: stage-dot-pulse 1.5s ease-in-out infinite;
}

.stage-item.completed .stage-dot {
  background: var(--accent-primary);
}

@keyframes stage-dot-pulse {
  0%, 100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.3);
  }
}

.stage-label {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.stage-item.active .stage-label {
  color: var(--accent-primary);
  font-weight: 500;
}

/* ===== 切换 overlay 过渡：入场缩放 + 出场模糊 ===== */
.switch-overlay-enter-active {
  transition: opacity 0.3s ease, transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.switch-overlay-leave-active {
  transition: opacity 0.4s ease, transform 0.4s cubic-bezier(0.4, 0, 0.2, 1), filter 0.4s ease;
}

.switch-overlay-enter-from {
  opacity: 0;
  transform: scale(1.08);
}
.switch-overlay-leave-to {
  opacity: 0;
  transform: scale(0.96);
  filter: blur(4px);
}

/* ===== 退出动效（exit-overlay）独特设计 =====
 * 与切换动效区分：装饰层向中心收缩 + 图标 + 文字渐变色
 */
.switch-overlay--exit {
  background-image: radial-gradient(circle at 50% 50%, rgba(250, 81, 81, 0.04) 0%, transparent 70%);
}

.exit-deco {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: var(--accent-primary);
  pointer-events: none;
  z-index: 0;
  /* 整体缓慢收缩 */
  animation: exit-deco-shrink 1.2s ease-out forwards;
}

@keyframes exit-deco-shrink {
  from {
    transform: translate(-50%, -50%) scale(1.4);
    opacity: 0;
  }
  30% {
    opacity: 1;
  }
  to {
    transform: translate(-50%, -50%) scale(0.6);
    opacity: 0.3;
  }
}

/* 三层圆环从外到内依次消失 */
.exit-ring-outer {
  animation: exit-ring-fade 0.8s ease-out 0.2s forwards;
}
.exit-ring-mid {
  animation: exit-ring-fade 0.8s ease-out 0.4s forwards;
}
.exit-ring-inner {
  animation: exit-ring-fade 0.8s ease-out 0.6s forwards;
}

@keyframes exit-ring-fade {
  to {
    opacity: 0;
  }
}

.exit-center-dot {
  animation: exit-dot-pulse 1s ease-in-out infinite;
}

@keyframes exit-dot-pulse {
  0%, 100% {
    opacity: 0.4;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.5);
  }
}

/* ===== 退出动效：仅 LOGO（简洁） =====
 * LOGO 居中显示，缓慢变淡传递离开感
 */
.exit-logo-wrapper {
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: exit-logo-enter 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}

@keyframes exit-logo-enter {
  from {
    opacity: 0;
    transform: scale(0.8);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.exit-logo-img {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  object-fit: cover;
  /* 退出时 LOGO 缓慢变淡 */
  animation: exit-logo-fade 1.2s ease-out 0.3s forwards;
}

@keyframes exit-logo-fade {
  from {
    opacity: 1;
    transform: scale(1);
  }
  to {
    opacity: 0.6;
    transform: scale(0.92);
  }
}

/* ===== 退出 overlay 过渡：入场从右滑入 + 出场向下消散 ===== */
.exit-overlay-enter-active {
  transition: opacity 0.4s ease, transform 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.exit-overlay-leave-active {
  transition: opacity 0.6s ease, transform 0.6s cubic-bezier(0.4, 0, 0.2, 1), filter 0.6s ease;
}

.exit-overlay-enter-from {
  opacity: 0;
  transform: translateY(-12px);
}
.exit-overlay-leave-to {
  opacity: 0;
  transform: translateY(20px) scale(0.98);
  filter: blur(6px);
}

/* 尊重用户的减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .switch-deco-outer,
  .switch-deco-mid,
  .switch-ring-arc,
  .switch-ring-logo,
  .stage-item.active .stage-dot,
  .exit-deco,
  .exit-ring-outer,
  .exit-ring-mid,
  .exit-ring-inner,
  .exit-center-dot,
  .exit-logo-wrapper,
  .exit-logo-img {
    animation: none;
  }
}

/* ===== 路由切换过渡 =====
 * out-in 模式：先淡出当前，再淡入下一个，避免重叠
 * translateY(6px) 轻微上滑，配合 opacity 营造层次感
 */
.route-fade-enter-active {
  transition: opacity 0.3s ease, transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity, transform;
}

.route-fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity, transform;
}

.route-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.route-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}</style>
