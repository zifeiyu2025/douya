<template>
  <n-config-provider
    :theme="isDark ? darkTheme : undefined"
    :theme-overrides="themeOverrides"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-message-provider>
      <n-dialog-provider>
        <Transition name="main-fade">
          <div
            v-if="!showSplash"
            class="app-layout"
            :class="{ dark: isDark, 'has-background': !!settingsStore.config.chat_background }"
            :style="mainAreaStyle"
          >
            <Sidebar :collapsed="sidebarCollapsed" @toggle="sidebarCollapsed = !sidebarCollapsed" />
            <div class="main-area" :class="{ 'sidebar-collapsed': sidebarCollapsed }">
              <div class="main-header" style="--wails-draggable: drag">
                <div class="main-header-left" style="--wails-draggable: no-drag">
                  <n-button
                    quaternary
                    circle
                    size="large"
                    @click="sidebarCollapsed = !sidebarCollapsed"
                  >
                    <template #icon>
                      <n-icon size="20"><MenuOutline /></n-icon>
                    </template>
                  </n-button>
                  <div class="header-content">
                    <div class="header-title" :title="currentTitle">{{ currentTitle }}</div>
                  </div>
                </div>
                <div class="main-header-right" style="--wails-draggable: no-drag">
                  <n-select
                    :value="selectedModel"
                    :options="displayModelOptions"
                    size="small"
                    placeholder="选择模型"
                    class="model-selector"
                    :disabled="isModelSwitching || !serverStatus.running"
                    :render-label="renderModelLabel"
                    @update:value="handleModelChange"
                  />
                  <div class="server-status" :title="modelFullName">
                    <div
                      v-if="isModelSwitching && switchProgressStage !== 'idle'"
                      class="switching-animation"
                    >
                      <div class="loading-spinner"></div>
                      <span class="status-text">
                        {{ switchingModelName }} · {{ switchStageText }}{{ switchDuration }}
                      </span>
                    </div>
                    <div
                      v-else-if="modelLoadProgress && modelLoadProgress.status === 'loading'"
                      class="load-progress-animation"
                    >
                      <div class="loading-spinner"></div>
                      <div class="load-progress-info">
                        <span class="status-text">
                          {{ loadProgressModelName }} · 加载 {{ modelLoadProgress.progress }}%
                        </span>
                        <div class="load-progress-bar">
                          <div
                            class="load-progress-bar-fill"
                            :style="{ width: modelLoadProgress.progress + '%' }"
                          ></div>
                        </div>
                      </div>
                    </div>
                    <div v-else-if="modelLoadFailed" class="error-animation">
                      <span class="status-dot stopped" />
                      <span class="status-text error-text">{{ errorModelName }} · 加载失败</span>
                    </div>
                    <div
                      v-else-if="isServerLoading && switchProgressStage === 'idle' && !isFirstLoad"
                      class="loading-animation"
                    >
                      <div class="loading-spinner"></div>
                      <span class="status-text">{{ modelName || '启动中...' }}</span>
                    </div>
                    <template v-else>
                      <span
                        class="status-dot"
                        :class="serverStatus.running ? 'running' : 'stopped'"
                      />
                      <span
                        class="status-text"
                        :class="{ 'error-text': !serverStatus.running && serverStatus.error }"
                      >
                        {{ modelName }} ·
                        {{ serverStatus.running ? '已就绪' : serverStatus.error || '未运行' }}
                      </span>
                    </template>
                  </div>
                </div>
                <div class="window-controls" style="--wails-draggable: no-drag">
                  <button class="win-btn" title="服务器控制台" @click="toggleConsole">
                    <n-icon size="16">
                      <TerminalOutline />
                    </n-icon>
                  </button>
                  <button
                    class="win-btn theme-btn"
                    :title="isDark ? '切换亮色模式' : '切换暗色模式'"
                    @click="themeStore.toggleTheme()"
                  >
                    <n-icon size="16">
                      <SunnyOutline v-if="isDark" />
                      <MoonOutline v-else />
                    </n-icon>
                  </button>
                  <button class="win-btn" title="最小化" @click="handleMinimize">
                    <AppIcon name="minimize" :size="14" />
                  </button>
                  <button class="win-btn" title="最大化" @click="handleToggleMaximize">
                    <AppIcon :name="isMaximized ? 'restore' : 'maximize'" :size="12" />
                  </button>
                  <button class="win-btn win-btn-close" title="关闭" @click="handleClose">
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
            <svg
              class="switch-deco"
              width="360"
              height="360"
              viewBox="0 0 360 360"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
              aria-hidden="true"
            >
              <circle
                cx="180"
                cy="180"
                r="160"
                stroke="currentColor"
                stroke-width="1"
                opacity="0.06"
              />
              <circle
                cx="180"
                cy="180"
                r="120"
                stroke="currentColor"
                stroke-width="1"
                opacity="0.08"
              />
              <circle
                cx="180"
                cy="180"
                r="160"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-dasharray="50 320"
                class="switch-deco-outer"
                opacity="0.35"
              />
              <circle
                cx="180"
                cy="180"
                r="120"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-dasharray="35 240"
                class="switch-deco-mid"
                opacity="0.45"
              />
            </svg>
            <div class="switch-overlay-content">
              <!-- 圆形进度环 + 中心 LOGO 图片 -->
              <div class="switch-ring-wrapper">
                <svg class="switch-ring-svg" width="80" height="80" viewBox="0 0 80 80">
                  <circle
                    cx="40"
                    cy="40"
                    r="36"
                    stroke="var(--border-color)"
                    stroke-width="2"
                    fill="none"
                    opacity="0.3"
                  />
                  <circle
                    cx="40"
                    cy="40"
                    r="36"
                    stroke="var(--accent-primary)"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    fill="none"
                    stroke-dasharray="85 150"
                    class="switch-ring-arc"
                  />
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
                  :key="stage"
                  :class="[
                    'stage-item',
                    {
                      active: getSwitchStageIndex() >= idx,
                      completed: getSwitchStageIndex() > idx
                    }
                  ]"
                >
                  <span class="stage-dot"></span>
                  <span class="stage-label">{{ stage }}</span>
                </div>
              </div>
            </div>
          </div>
        </Transition>
        <Transition name="exit-overlay">
          <div v-if="showExitOverlay" class="switch-overlay switch-overlay--exit">
            <!-- 退出动效：渐变收缩消散装饰 -->
            <svg
              class="exit-deco"
              width="360"
              height="360"
              viewBox="0 0 360 360"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
              aria-hidden="true"
            >
              <!-- 三层同心圆，从外到内逐渐消失 -->
              <circle
                cx="180"
                cy="180"
                r="160"
                stroke="currentColor"
                stroke-width="1"
                opacity="0.08"
                class="exit-ring-outer"
              />
              <circle
                cx="180"
                cy="180"
                r="120"
                stroke="currentColor"
                stroke-width="1.5"
                opacity="0.15"
                class="exit-ring-mid"
              />
              <circle
                cx="180"
                cy="180"
                r="80"
                stroke="currentColor"
                stroke-width="2"
                opacity="0.25"
                class="exit-ring-inner"
              />
            </svg>
            <div class="switch-overlay-content">
              <!-- 退出动效：仅 LOGO（简洁） -->
              <div class="exit-logo-wrapper">
                <img :src="appLogo" alt="豆芽" class="exit-logo-img" />
              </div>
              <div class="switch-model-name">正在退出豆芽</div>
              <div class="switch-progress-msg">{{ exitProgress }}</div>
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
import { computed, h, onMounted, ref, watch } from 'vue'
import { darkTheme, zhCN, dateZhCN } from 'naive-ui'
import {
  NConfigProvider,
  NMessageProvider,
  NDialogProvider,
  NButton,
  NIcon,
  NSelect,
  NTooltip
} from 'naive-ui'
import {
  MenuOutline,
  SunnyOutline,
  MoonOutline,
  TerminalOutline,
  TrashOutline
} from '@vicons/ionicons5'
import Sidebar from './components/Sidebar.vue'
import AppIcon from './components/ui/AppIcon.vue'
import SplashScreen from './components/ui/SplashScreen.vue'
import ServerConsole from './components/ServerConsole.vue'
import { useChatStore } from './stores/chat'
import { fixUtf8 } from './utils/utf8'
// Task 21：抽取 mainAreaStyle 背景图逻辑为纯函数，便于单元测试双主题支持
import { buildBackgroundStyle } from './utils/backgroundStyle'
// 复用全局单例 discrete API，确保 message/dialog 跟随应用主题（任务 9）
import { discreteMessage, discreteDialog } from './utils/discrete'
import { useSettingsStore } from './stores/settings'
import { useThemeStore } from './stores/theme'
// Naive UI 全局主题覆盖：让所有组件使用项目 GitHub 蓝配色而非默认绿色（Task 2）
import { useThemeOverrides } from './composables/useThemeOverrides'
// 任务 9：抽取模型切换/窗口控制/生命周期到 composable
import { useModelSwitch } from './composables/useModelSwitch'
import { useWindowControls } from './composables/useWindowControls'
import { useAppLifecycle } from './composables/useAppLifecycle'
import { formatModelName, formatModelNameFromPath, extractQuantSuffix } from './utils/model'
import { classifyError } from './utils/errorGuidance'
import { openExternal } from './utils/externalLink'
import {
  decideSpecAdviceNotification,
  getSpecAdviceDismissedKeys,
  recordSpecAdviceDismissed
} from './utils/specAdvice'
import type { Conversation, ModelOption } from './services/wails'
import { wails } from './services/wails'
import appLogo from './assets/images/appicon.png'

// ----- Store / 主题 -----
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()
// themeOverrides 是 ComputedRef<GlobalThemeOverrides>，会随 isDark 自动切换
// n-config-provider 接受 ref，会自动 unwrap，模板里直接传 ref 即可
const themeOverrides = useThemeOverrides()

const isDark = computed(() => themeStore.isDark)
const serverStatus = computed(() => settingsStore.serverStatus)
const sidebarCollapsed = defineModel<boolean>('sidebarCollapsed', { default: false })

const mainAreaStyle = computed(() => {
  // 双主题都支持背景图：逻辑抽取到 utils/backgroundStyle.ts（Task 21）
  // isDark 不再作为限制条件，亮色与深色都会注入 --chat-background 变量
  return buildBackgroundStyle(
    settingsStore.config.chat_background ?? '',
    settingsStore.config.chat_background_opacity
  )
})

// ----- 调用三个 composable（任务 9：抽取模型切换/窗口控制/生命周期）-----
const {
  switchProgressStage,
  switchingModelName,
  switchDuration,
  switchStageText,
  showSwitchOverlay,
  overlayModelName,
  switchStages,
  isModelSwitching,
  switchingModelDisplay,
  getSwitchStageIndex
} = useModelSwitch()

const { isMaximized, handleMinimize, handleToggleMaximize, handleClose } = useWindowControls()

const { showSplash, splashStage, splashModelName, splashProgress, showExitOverlay, exitProgress } =
  useAppLifecycle()

// ----- 服务器状态显示 -----
const isServerLoading = computed(() => {
  return (
    serverStatus.value.switching || (!serverStatus.value.model_ready && !serverStatus.value.error)
  )
})
const isFirstLoad = computed(() => settingsStore.isFirstLoad)
const modelLoadFailed = computed(() => settingsStore.modelLoadFailed)
const modelLoadProgress = computed(() => settingsStore.modelLoadProgress)

const loadProgressModelName = computed(() => {
  if (!modelLoadProgress.value) return ''
  return formatModelName(modelLoadProgress.value.model).display
})

// ----- 模型选择 -----
const modelOptions = ref<
  {
    label: string
    value: string
    fullName: string
    quantSuffix: string
    isLoaded: boolean
    mmprojVision: boolean
    mmprojAudio: boolean
    mmprojVideo: boolean
    status: string
  }[]
>([])
const availableModels = ref<ModelOption[]>([])
const selectedModel = ref('')

// 模型下拉为空时显示引导提示：用一个不可选的提示项占位，引导用户放置 .gguf 文件
const displayModelOptions = computed(() => {
  if (modelOptions.value.length === 0) {
    return [
      {
        label: '未找到模型，请将 .gguf 文件放入 models/ 目录',
        value: '__no_models__',
        disabled: true
      }
    ]
  }
  return modelOptions.value
})

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

const errorModelName = computed(() => {
  if (switchingModelDisplay.value) return switchingModelDisplay.value
  if (serverStatus.value.switching_to)
    return formatModelName(serverStatus.value.switching_to).display
  return modelName.value || ''
})

const currentTitle = computed(() => {
  const conv = chatStore.conversations.find(
    (c: Conversation) => c.id === chatStore.currentConversationId
  )
  return fixUtf8(conv?.title || '豆芽 AI')
})

watch(
  () => settingsStore.currentModel,
  newModel => {
    if (isModelSwitching.value) return
    if (newModel && newModel !== selectedModel.value) {
      const match = modelOptions.value.find(m => m.value === newModel)
      if (match) {
        selectedModel.value = newModel
      }
    }
  }
)

function renderModelLabel(option: {
  label: string
  value: string
  fullName?: string
  quantSuffix?: string
  isLoaded?: boolean
  mmprojVision?: boolean
  mmprojAudio?: boolean
  mmprojVideo?: boolean
  status?: string
}) {
  const children = [h('span', option.label)]
  if (option.quantSuffix) {
    children.push(
      h(
        'span',
        {
          style: 'color: var(--text-muted); font-size: 11px; margin-left: 4px; font-weight: 400;'
        },
        option.quantSuffix
      )
    )
  }
  const tags: string[] = []
  if (option.mmprojVision) tags.push('📷')
  if (option.mmprojAudio) tags.push('🎤')
  if (option.mmprojVideo) tags.push('🎬')
  if (tags.length > 0) {
    children.push(
      h(
        'span',
        {
          style: 'margin-left: 6px; font-size: 11px;'
        },
        tags.join(' ')
      )
    )
  }
  if (option.status === 'sleeping') {
    children.push(
      h(
        'span',
        {
          style: 'color: #f0a020; margin-left: 6px; font-size: 10px;'
        },
        '💤'
      )
    )
  } else if (option.status === 'loading') {
    children.push(
      h(
        'span',
        {
          style: 'color: var(--accent-primary); margin-left: 6px; font-size: 10px;'
        },
        '⏳'
      )
    )
  } else if (option.isLoaded) {
    children.push(
      h(
        'span',
        {
          style: 'color: var(--accent-primary); margin-left: 6px; font-size: 10px;'
        },
        '●'
      )
    )
  }
  // 删除按钮：点击时阻止冒泡，避免触发模型切换
  const deleteBtn = h(
    NButton,
    {
      type: 'error',
      size: 'small',
      quaternary: true,
      style: 'margin-left: 8px; flex-shrink: 0;',
      onClick: (e: Event) => {
        e.stopPropagation()
        confirmDeleteModel(option.value, option.label)
      }
    },
    {
      icon: () => h(NIcon, { size: 14 }, { default: () => h(TrashOutline) })
    }
  )
  const content = h(
    'span',
    {
      style:
        'display: inline-flex; align-items: center; width: 100%; justify-content: space-between'
    },
    [
      h(
        'span',
        { style: 'display: inline-flex; align-items: center; min-width: 0; overflow: hidden' },
        children
      ),
      deleteBtn
    ]
  )
  return h(
    NTooltip,
    { placement: 'right', delay: 300 },
    {
      trigger: () => content,
      default: () => option.fullName || option.value
    }
  )
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
        status: m.status
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
        discreteMessage.success(
          `${formatModelName(result.current_model || value).display}${featureText} 已就绪`,
          { duration: 3000 }
        )
        loadAvailableModels()
      }, 300)
    } else {
      selectedModel.value = result.rolled_back
        ? result.current_model || previousModel
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
            style: { whiteSpace: 'pre-wrap' }
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
        style: { whiteSpace: 'pre-wrap' }
      })
    } else {
      discreteMessage.error(errorText, { duration: 5000 })
    }
  }
}

// ----- 服务器控制台 -----
const consoleRef = ref()
const toggleConsole = () => {
  consoleRef.value?.toggle()
}

// ----- 服务器就绪后自动刷新模型列表（带防抖保护）-----
// 防抖计时器：避免短时间内重复调用 loadAvailableModels（如 running 和 model_ready 几乎同时变化）
let refreshModelsTimer: ReturnType<typeof setTimeout> | null = null
function debouncedRefreshModels() {
  if (refreshModelsTimer) clearTimeout(refreshModelsTimer)
  refreshModelsTimer = setTimeout(() => {
    loadAvailableModels()
    refreshModelsTimer = null
  }, 500)
}

// 当服务器从停止变为运行时，刷新模型列表
watch(
  () => serverStatus.value.running,
  (running, prev) => {
    if (running && !prev) {
      debouncedRefreshModels()
    }
  }
)

// 当模型准备就绪时，刷新模型列表以更新各模型的 is_loaded 状态
watch(
  () => serverStatus.value.model_ready,
  (ready, prev) => {
    if (ready) {
      debouncedRefreshModels()
      // 推测解码智能提醒：模型加载完成时检测是否需要引导用户下载 sidecar 模型
      // 仅在 false → true 的边沿触发，避免每次轮询都弹窗
      void tryShowSpecAdviceNotification(prev ?? false, ready)
    }
  }
)

/**
 * 推测解码智能提醒：模型加载完成后弹通知引导用户下载 sidecar 模型
 *
 * 决策逻辑见 utils/specAdvice.ts 的 decideSpecAdviceNotification（已单元测试覆盖），
 * 此处只负责：获取 smartParams → 调用决策函数 → 弹 dialog → 记录 dismissKey。
 *
 * 生活类比：像导购在顾客提着商品出门时叫住他——
 *  - 先看顾客是不是刚提完货（model_ready 边沿）
 *  - 看顾客有没有遗漏配件（spec_advice 是否非空）
 *  - 看顾客是不是已经被告知过（dismissedKeys 是否包含）
 *  - 全部条件满足，才会上前提醒"先生/女士，您还需要配一个 XX"
 */
async function tryShowSpecAdviceNotification(prevReady: boolean, currReady: boolean) {
  const cfg = settingsStore.config
  if (!cfg) return
  try {
    const smartParams = await wails.getSmartParams()
    const decision = decideSpecAdviceNotification({
      prevReady,
      currReady,
      specAdvice: smartParams.spec_advice,
      adviceEnabled: cfg.spec_advice_enabled,
      dismissedKeys: getSpecAdviceDismissedKeys()
    })
    if (!decision.shouldShow) return
    const advice = smartParams.spec_advice!
    discreteDialog.info({
      title: `${advice.desc} 推测解码可用`,
      content: `${advice.reason}。是否前往 hf-mirror.com（国内镜像）下载对应的 ${advice.desc} 草稿模型？下载后在「设置 → 推测解码」中配置 Draft 模型路径即可启用。`,
      positiveText: '前往下载',
      negativeText: '以后再说',
      onPositiveClick: () => {
        openExternal(advice.download_url)
        recordSpecAdviceDismissed(decision.dismissKey)
      },
      onNegativeClick: () => {
        // 用户已知晓，记录 dismiss 避免重复打扰
        recordSpecAdviceDismissed(decision.dismissKey)
      },
      onClose: () => {
        // 关闭按钮（X）或点击遮罩关闭：同样记录，避免反复弹出
        recordSpecAdviceDismissed(decision.dismissKey)
      }
    })
  } catch (e) {
    // 通知失败不影响主流程，仅记录日志
    console.warn('[specAdvice] 获取智能参数失败，跳过推测解码通知', e)
  }
}

// ----- 启动时加载可用模型列表 -----
// 注：其他生命周期事件（监听器注册、loadConfig、异常清理、退出进度）由 useAppLifecycle 负责；
//     窗口控制（resize、maximize）由 useWindowControls 负责；
//     模型切换监听（switchProgress、modelLoadProgress）由 useModelSwitch 负责。
//     loadAvailableModels 仅调用 wails.getAvailableModels()，不依赖 config，可独立调用。
onMounted(async () => {
  await loadAvailableModels()
})
</script>

<style scoped>
.main-fade-enter-active {
  transition:
    opacity 0.5s ease 0.3s,
    transform 0.5s cubic-bezier(0.4, 0, 0.2, 1) 0.3s;
  will-change: transform, opacity;
}

.main-fade-leave-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: transform, opacity;
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
  0%,
  100% {
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
  transition:
    background 0.15s,
    color 0.15s;
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

.switch-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  /* 实色背景：主题对齐 */
  background: var(--bg-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  pointer-events: auto;
  /* 径向渐变营造氛围 */
  background-image: radial-gradient(
    circle at 50% 50%,
    color-mix(in srgb, var(--accent-primary) 4%, transparent) 0%,
    transparent 70%
  );
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
  to {
    transform: rotate(-360deg);
  }
}

/* ===== 圆形进度环（与 MessageList 一致） ===== */
.switch-ring-wrapper {
  position: relative;
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 使用 color-mix 跟随主色调，filter 支持 color-mix */
  filter: drop-shadow(0 0 8px color-mix(in srgb, var(--accent-primary) 40%, transparent));
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
  0%,
  100% {
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
  transition:
    background 0.3s ease,
    box-shadow 0.3s ease;
}

.stage-item.active .stage-dot {
  background: var(--accent-primary);
  box-shadow: 0 0 8px color-mix(in srgb, var(--accent-primary) 60%, transparent);
  animation: stage-dot-pulse 1.5s ease-in-out infinite;
}

.stage-item.completed .stage-dot {
  background: var(--accent-primary);
}

@keyframes stage-dot-pulse {
  0%,
  100% {
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
  transition:
    opacity 0.3s ease,
    transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.switch-overlay-leave-active {
  transition:
    opacity 0.4s ease,
    transform 0.4s cubic-bezier(0.4, 0, 0.2, 1),
    filter 0.4s ease;
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
  /* 退出语义色用 --accent-danger 派生，移除硬编码 rgba */
  background-image: radial-gradient(
    circle at 50% 50%,
    color-mix(in srgb, var(--accent-danger) 4%, transparent) 0%,
    transparent 70%
  );
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
  0%,
  100% {
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
  transition:
    opacity 0.4s ease,
    transform 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.exit-overlay-leave-active {
  transition:
    opacity 0.6s ease,
    transform 0.6s cubic-bezier(0.4, 0, 0.2, 1),
    filter 0.6s ease;
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
  transition:
    opacity 0.3s ease,
    transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity, transform;
}

.route-fade-leave-active {
  transition:
    opacity 0.2s ease,
    transform 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity, transform;
}

.route-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.route-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
