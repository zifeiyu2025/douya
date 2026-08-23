<template>
  <div class="main-header" style="--wails-draggable: drag" @dblclick="onHeaderDoubleClick">
    <div class="main-header-left" style="--wails-draggable: no-drag">
      <n-button quaternary circle size="large" @click="emit('toggle-sidebar')">
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
      <!-- C-3: 状态区可点击，弹出当前模型的真实 GGUF 元数据详情卡 -->
      <n-popover trigger="click" placement="bottom-end" :show-arrow="false" raw>
        <template #trigger>
          <button type="button" class="server-status" aria-label="查看模型详情">
            <span v-if="switchProgressStage !== 'idle'" class="switching-animation">
              <span class="loading-spinner"></span>
              <span class="status-text">
                {{ switchingModelName }} · {{ switchStageText }}{{ switchDuration }}
              </span>
            </span>
            <span
              v-else-if="modelLoadProgress && modelLoadProgress.status === 'loading'"
              class="load-progress-animation"
            >
              <span class="loading-spinner"></span>
              <span class="load-progress-info">
                <span class="status-text">
                  {{ loadProgressModelName }} · 加载 {{ modelLoadProgress.progress }}%
                </span>
                <span class="load-progress-bar">
                  <span
                    class="load-progress-bar-fill"
                    :style="{ transform: 'scaleX(' + modelLoadProgress.progress / 100 + ')' }"
                  ></span>
                </span>
              </span>
            </span>
            <span v-else-if="modelLoadFailed" class="error-animation">
              <span class="status-dot stopped" />
              <span class="status-text error-text">{{ errorModelName }} · 加载失败</span>
            </span>
            <span
              v-else-if="isServerLoading && switchProgressStage === 'idle' && !isFirstLoad"
              class="loading-animation"
            >
              <span class="loading-spinner"></span>
              <span class="status-text">{{ modelName || '启动中...' }}</span>
            </span>
            <span v-else class="status-idle">
              <span class="status-dot" :class="serverStatus.running ? 'running' : 'stopped'" />
              <span
                class="status-text"
                :class="{ 'error-text': !serverStatus.running && serverStatus.error }"
              >
                {{ modelName }} ·
                {{ serverStatus.running ? '已就绪' : serverStatus.error || '未运行' }}
              </span>
            </span>
          </button>
        </template>
        <ModelDetailCard :model="currentModelDetail" />
      </n-popover>
    </div>
    <div class="window-controls" style="--wails-draggable: no-drag">
      <button class="win-btn" title="服务器控制台" @click="emit('toggle-console')">
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
      <button class="win-btn" title="最小化" @click="emit('minimize')">
        <AppIcon name="minimize" :size="14" />
      </button>
      <button class="win-btn" title="最大化" @click="emit('toggle-maximize')">
        <AppIcon :name="isMaximized ? 'restore' : 'maximize'" :size="12" />
      </button>
      <button class="win-btn win-btn-close" title="关闭" @click="emit('close')">
        <AppIcon name="close" :size="14" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { NButton, NIcon, NPopover, NSelect, NTooltip } from 'naive-ui'
import {
  MenuOutline,
  SunnyOutline,
  MoonOutline,
  TerminalOutline,
  TrashOutline
} from '@vicons/ionicons5'
import AppIcon from '../ui/AppIcon.vue'
import ModelDetailCard from '../models/ModelDetailCard.vue'
// C-3 模型详情卡：点击状态区弹出，展示 B-5 真实 GGUF 元数据
import { useChatStore } from '../../stores/chat'
import { fixUtf8 } from '../../utils/utf8'
import { discreteMessage, discreteDialog } from '../../utils/discrete'
import { useSettingsStore } from '../../stores/settings'
import { useThemeStore } from '../../stores/theme'
import { formatModelName, formatModelNameFromPath, extractQuantSuffix } from '../../utils/model'
import { classifyError } from '../../utils/errorGuidance'
import { logError } from '../../utils/logger'
import type { Conversation, ModelOption } from '../../services/wails'
import { wails } from '../../services/wails'

// 顶部标题栏：模型选择器、服务器状态显示（含模型详情卡）、窗口控制按钮
// - store（settings/theme/chat）为 pinia 单例，本组件直接 import 使用
// - switchDuration / switchStageText：来自 useModelSwitch 的本地计时器状态与文字映射，
//   由 App.vue 调用 useModelSwitch 后通过 props 传入（避免本组件重复调用 composable
//   导致 wails 事件监听重复注册；C-7 可重入化后本组件可直接消费 composable）
// - 窗口控制（最小化/最大化/关闭/双击标题栏）：通过 emit 交由 App.vue 调用 useWindowControls
// - isMaximized：来自 useWindowControls 的本地状态，通过 prop 传入
defineProps<{
  switchDuration: string
  switchStageText: string
  isMaximized: boolean
}>()

const emit = defineEmits<{
  'toggle-sidebar': []
  'toggle-console': []
  minimize: []
  'toggle-maximize': []
  close: []
  'header-double-click': [event: MouseEvent]
}>()

// ----- Store / 主题 -----
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()

const isDark = computed(() => themeStore.isDark)
const serverStatus = computed(() => settingsStore.serverStatus)

// ----- 从 store 直接派生模型切换状态（与 useModelSwitch 中逻辑一致）-----
// 说明：useModelSwitch 不可在本组件重复调用（会重复注册 wails 事件监听），
//       这里直接从 store 派生等价的纯 computed，数据来源完全一致。
const switchProgressStage = computed(() => settingsStore.switchProgress.stage)
const isModelSwitching = computed(() => settingsStore.isModelSwitching)
const switchingModelDisplay = computed(() => settingsStore.switchingModelDisplay)

const switchingModelName = computed(() => {
  if (switchingModelDisplay.value) return switchingModelDisplay.value
  if (settingsStore.serverStatus.switching_to) {
    return formatModelName(settingsStore.serverStatus.switching_to).display
  }
  return ''
})

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
interface ModelOptionView {
  label: string
  value: string
  fullName: string
  /** 从文件名猜测的量化后缀（回退用） */
  quantSuffix: string
  /** GGUF 解析出的真实量化类型名（B-5），可能为空 */
  quantType?: string
  /** GGUF 参数量规模标签（B-5，如 "4B"），可能为空 */
  sizeLabel?: string
  isLoaded: boolean
  mmprojVision: boolean
  mmprojAudio: boolean
  mmprojVideo: boolean
  status: string
}
const modelOptions = ref<ModelOptionView[]>([])
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

// 当前选中模型的完整信息（详情卡数据源；找不到时为 null，卡片显示空态由父级隐藏）
const currentModelDetail = computed<ModelOption | null>(() => {
  const found = availableModels.value.find(m => m.name === selectedModel.value)
  return found ?? null
})

const modelName = computed(() => {
  if (selectedModel.value) {
    return formatModelName(selectedModel.value).display
  }
  const path = settingsStore.config.model_path
  if (!path) return ''
  return formatModelNameFromPath(path).display
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

function renderModelLabel(option: ModelOptionView) {
  // 下拉项元数据展示：参数量规模（accent 强调）+ 量化类型（弱化小字）
  // 真实 GGUF 数据（quantType）优先于文件名猜测（quantSuffix）
  const quantText = option.quantType || option.quantSuffix
  const childSpans = [
    h('span', option.label),
    ...(option.sizeLabel
      ? [
          h(
            'span',
            {
              style:
                'color: var(--accent-primary); font-size: 11px; margin-left: 6px; font-weight: 600;'
            },
            option.sizeLabel
          )
        ]
      : []),
    ...(quantText
      ? [
          h(
            'span',
            {
              style:
                'color: var(--text-muted); font-size: 11px; margin-left: 4px; font-weight: 400;'
            },
            quantText
          )
        ]
      : [])
  ]
  const tags: string[] = []
  if (option.mmprojVision) tags.push('📷')
  if (option.mmprojAudio) tags.push('🎤')
  if (option.mmprojVideo) tags.push('🎬')
  if (tags.length > 0) {
    childSpans.push(
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
    childSpans.push(
      h(
        'span',
        {
          style: 'color: #f0a020; margin-left: 6px; font-size: 10px;'
        },
        '💤'
      )
    )
  } else if (option.status === 'loading') {
    childSpans.push(
      h(
        'span',
        {
          style: 'color: var(--accent-primary); margin-left: 6px; font-size: 10px;'
        },
        '⏳'
      )
    )
  } else if (option.isLoaded) {
    childSpans.push(
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
        childSpans
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
      return {
        label: display,
        value: m.name,
        fullName: m.file_name || m.name,
        quantSuffix: extractQuantSuffix(m.file_name || ''),
        quantType: m.quant_type || undefined,
        sizeLabel: m.size_label || undefined,
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
    logError('Failed to load available models:', e)
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
    logError('删除模型失败:', e)
    discreteMessage.error(`删除模型失败: ${e}`, { duration: 5000 })
  }
}

// 模型切换结果提示（在 handleModelChange 中 await 返回后直接处理，避免 watch 微任务竞态）

async function handleModelChange(value: string) {
  if (isModelSwitching.value) return

  const targetModel = availableModels.value.find(m => m.name === value)
  if (!targetModel) {
    logError('Unknown model:', value)
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
        // 如果后端恢复了该模型的专属参数预设，在提示中追加说明
        const restoredText = result.params_restored ? ' · 已恢复专属参数' : ''
        discreteMessage.success(
          `${formatModelName(result.current_model || value).display}${featureText}${restoredText} 已就绪`,
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
    logError('切换模型失败:', e)
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
  ready => {
    if (ready) {
      debouncedRefreshModels()
    }
  }
)

// ----- 启动时加载可用模型列表 -----
// 注：其他生命周期事件（监听器注册、loadConfig、异常清理、退出进度）由 useAppLifecycle 负责；
//     窗口控制（resize、maximize）由 useWindowControls 负责；
//     模型切换监听（switchProgress、modelLoadProgress）由 useModelSwitch 负责。
//     loadAvailableModels 仅调用 wails.getAvailableModels()，不依赖 config，可独立调用。
onMounted(async () => {
  await loadAvailableModels()
})

// 卸载时清理防抖定时器，避免组件销毁后仍触发 loadAvailableModels
onUnmounted(() => {
  if (refreshModelsTimer) {
    clearTimeout(refreshModelsTimer)
    refreshModelsTimer = null
  }
})

// 窗口控制事件转发：交由 App.vue 调用 useWindowControls 的对应方法处理
function onHeaderDoubleClick(e: MouseEvent) {
  emit('header-double-click', e)
}
</script>

<style scoped>
.model-selector {
  min-width: 120px;
  max-width: 260px;
  flex-shrink: 0;
}

.loading-animation,
.switching-animation,
.error-animation,
.load-progress-animation,
.status-idle {
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
  display: block;
  width: 100%;
  height: 100%;
  background: var(--accent-primary);
  border-radius: 2px;
  transform-origin: left;
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
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

/* ===== 状态区按钮化（C-3）=====
 * 由 div 改为 button：可点击弹出模型详情卡；
 * 视觉与原 div 完全一致（透明底、无边框），仅增加 hover 反馈与 cursor:pointer
 */
.server-status {
  display: flex;
  align-items: center;
  padding: 4px 10px;
  background: transparent;
  border: none;
  border-radius: var(--border-radius-sm);
  font-family: inherit;
  font-size: 13px;
  line-height: 1;
  cursor: pointer;
  transition: background var(--transition-fast);
  appearance: none;
  -webkit-appearance: none;
}

.server-status:hover {
  background: var(--bg-hover);
}

.server-status:focus-visible {
  outline: 2px solid var(--accent-primary);
  outline-offset: -2px;
}
</style>
