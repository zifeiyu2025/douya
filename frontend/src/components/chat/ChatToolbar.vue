<template>
  <div class="left-buttons">
    <button
      class="think-btn"
      :class="thinkBtnClass"
      :disabled="!supportsThinking"
      :title="thinkingTitle"
      @click="handleThinkClick"
    >
      <BrainIcon :size="20" class="think-icon" />
      <span v-if="thinkingMode === 'no_think'" class="think-slash"></span>
    </button>
    <button
      v-if="supportsDeepReasoning"
      class="deep-reason-btn"
      :class="{ active: deepReasoningOn }"
      :title="deepReasoningTitle"
      @click="handleDeepReasonClick"
    >
      <n-icon size="20" class="deep-reason-icon"><LayersOutline /></n-icon>
    </button>
    <button
      class="search-btn"
      :class="searchBtnClass"
      :title="searchTitle"
      @click="handleSearchClick"
    >
      <n-icon size="22"><GlobeOutline /></n-icon>
    </button>
    <!-- 采样参数快捷抽屉：直调 composable 导出的 open，免 emit 链 -->
    <button class="params-btn" title="生成参数调节" @click="openParamsPanel">
      <n-icon size="22"><OptionsOutline /></n-icon>
    </button>
    <div class="attach-wrapper">
      <button class="attach-btn" :disabled="isSwitching" title="添加附件" @click="toggleAttachMenu">
        <n-icon size="22"><AttachOutline /></n-icon>
      </button>
      <div v-if="showAttachMenu" class="attach-menu">
        <button
          class="attach-menu-item"
          :class="{ disabled: !capabilities.mmproj_loaded || !capabilities.image_input }"
          @click="triggerFileUpload('image')"
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
            <circle cx="8.5" cy="8.5" r="1.5" />
            <polyline points="21 15 16 10 5 21" />
          </svg>
          <span>图片</span>
          <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
          <span v-else-if="!capabilities.image_input" class="unsupported-tag">不支持</span>
        </button>
        <button
          class="attach-menu-item"
          :class="{ disabled: !capabilities.mmproj_loaded || !capabilities.audio_input }"
          @click="triggerFileUpload('audio')"
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M9 18V5l12-2v13" />
            <circle cx="6" cy="18" r="3" />
            <circle cx="18" cy="16" r="3" />
          </svg>
          <span>音频</span>
          <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
          <span v-else-if="!capabilities.audio_input" class="unsupported-tag">不支持</span>
        </button>
        <button
          class="attach-menu-item"
          :class="{ disabled: !capabilities.mmproj_loaded || !capabilities.video_input }"
          @click="triggerFileUpload('video')"
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <polygon points="23 7 16 12 23 17 23 7" />
            <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
          </svg>
          <span>视频</span>
          <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
          <span v-else-if="!capabilities.video_input" class="unsupported-tag">不支持</span>
        </button>
        <button
          class="attach-menu-item"
          :class="{ disabled: !capabilities.text_input }"
          @click="triggerFileUpload('text')"
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
            <polyline points="14 2 14 8 20 8" />
            <line x1="16" y1="13" x2="8" y2="13" />
            <line x1="16" y1="17" x2="8" y2="17" />
          </svg>
          <span>文本</span>
          <span v-if="!capabilities.text_input" class="unsupported-tag">不支持</span>
        </button>
        <button
          class="attach-menu-item"
          :class="{ disabled: !capabilities.text_input }"
          @click="triggerFileUpload('pdf')"
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
            <polyline points="14 2 14 8 20 8" />
            <line x1="16" y1="13" x2="8" y2="13" />
            <line x1="16" y1="17" x2="8" y2="17" />
            <polyline points="10 9 9 9 8 9" />
          </svg>
          <span>PDF</span>
          <span v-if="!capabilities.text_input" class="unsupported-tag">不支持</span>
        </button>
        <button
          class="attach-menu-item"
          :class="{ disabled: !capabilities.text_input }"
          @click="triggerFileUpload('docx')"
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
            <polyline points="14 2 14 8 20 8" />
            <path d="M9 15v-3" />
            <path d="M12 15v-3" />
            <path d="M15 15v-3" />
            <path d="M9 18h6" />
          </svg>
          <span>Word</span>
          <span v-if="!capabilities.text_input" class="unsupported-tag">不支持</span>
        </button>
      </div>
    </div>
    <input ref="fileInputRef" type="file" class="hidden-file-input" @change="handleFileSelect" />
    <button
      class="voice-btn"
      :class="{ active: isListening, unsupported: !speechSupported }"
      :disabled="!speechSupported"
      :title="
        speechSupported ? (isListening ? '停止语音输入' : '语音输入') : '浏览器不支持语音识别'
      "
      @click="emit('toggleListening')"
    >
      <svg
        width="20"
        height="20"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"></path>
        <path d="M19 10v2a7 7 0 0 1-14 0v-2"></path>
        <line x1="12" y1="19" x2="12" y2="23"></line>
        <line x1="8" y1="23" x2="16" y2="23"></line>
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { NIcon, useMessage } from 'naive-ui'
import { GlobeOutline, AttachOutline, LayersOutline, OptionsOutline } from '@vicons/ionicons5'
import BrainIcon from '../ui/BrainIcon.vue'
import { checkUploadCapability, getAcceptForType } from '../../utils/attachments'
import { useSettingsStore } from '../../stores/settings'
import { wails } from '../../services/wails'
import { openParamsPanel } from '../../composables/useSamplingSettings'

// 工具栏按钮组：思考模式、搜索模式、深度推理、附件、语音
//（KV 缓存已改为自动保存/恢复，无需手动按钮）
// 通过 props 接收语音输入状态，通过 emit 向上传递语音切换与文件选择事件
defineProps<{
  isListening: boolean
  speechSupported: boolean
}>()

const emit = defineEmits<{
  toggleListening: []
  fileSelect: [type: string, file: File]
}>()

const settingsStore = useSettingsStore()
const message = useMessage()

const searchMode = computed(() => settingsStore.searchMode)
const thinkingMode = computed(() => settingsStore.thinkingSoftSwitch)
const capabilities = computed(() => settingsStore.modelCapabilities)
const isSwitching = computed(() => settingsStore.isModelSwitching)
const supportsThinking = computed(() => settingsStore.modelCapabilities.thinking_mode !== 'none')
const thinkingTitle = computed(() => {
  if (!supportsThinking.value) return '当前模型不支持深度思考'
  switch (thinkingMode.value) {
    case 'think':
      return '深度思考：回答前先进行深度推理分析'
    default:
      return '深度思考已关闭（默认不思考，点击开启）'
  }
})
const thinkBtnClass = computed(() => ({
  active: thinkingMode.value === 'think',
  'no-think-mode': thinkingMode.value === 'no_think',
  unsupported: !supportsThinking.value
}))
const supportsDeepReasoning = computed(() => capabilities.value.supports_preserve_reasoning)
const deepReasoningOn = computed(() => !!settingsStore.config.reasoning_preserve)
const deepReasoningTitle = computed(() =>
  deepReasoningOn.value
    ? '深度推理：保留完整思考历史（每条消息都保留推理过程）'
    : '深度推理：仅保留最近一次思考（点击开启完整历史保留）'
)
async function handleDeepReasonClick() {
  const newVal = !deepReasoningOn.value
  // 先拉取后端最新配置，避免用过期的 settingsStore.config 覆盖其他路径的修改
  const cfg = await wails.getConfig()
  cfg.reasoning_preserve = newVal
  await settingsStore.updateConfig(cfg)
  message.destroyAll()
  if (newVal) {
    message.success('已开启深度推理，将保留完整思考历史', { duration: 2000 })
  } else {
    message.info('已关闭深度推理，仅保留最近一次思考', { duration: 2000 })
  }
}
const searchTitle = computed(() => {
  switch (searchMode.value) {
    case 'on':
      return '强制搜索（所有消息都搜索）'
    case 'auto':
      return '智能搜索（按需自动搜索）'
    default:
      return '联网搜索已关闭'
  }
})
const searchBtnClass = computed(() => ({
  active: searchMode.value === 'on',
  'auto-mode': searchMode.value === 'auto'
}))

async function handleSearchClick() {
  const prevMode = searchMode.value
  // 即将开启搜索（当前是 off），检查是否配置了搜索 API Key
  if (prevMode === 'off') {
    await settingsStore.loadSearchAPIKeys()
    const keys = settingsStore.searchAPIKeys
    // 已无兜底搜索（Bing 已移除），未配置 API Key 时阻止开启并提示用户
    if (!keys.tavily_api_key_set && !keys.ollama_api_key_set) {
      message.destroyAll()
      message.warning(
        '未配置搜索 API Key，无法使用联网搜索。请在「设置 → 联网搜索」中配置 Tavily 或 Ollama API Key',
        { duration: 5000 }
      )
      return
    }
  }
  await settingsStore.cycleSearchMode()
  const curMode = searchMode.value
  if (curMode === prevMode) return
  message.destroyAll()
  switch (curMode) {
    case 'off':
      message.info('联网搜索已关闭', { duration: 2000 })
      break
    case 'auto':
      message.success('智能搜索已开启，按需自动搜索', { duration: 2500 })
      break
    case 'on':
      message.success('强制搜索已开启，所有消息都将搜索', { duration: 2500 })
      break
  }
}

async function handleThinkClick() {
  if (!supportsThinking.value) return
  const prevMode = thinkingMode.value
  await settingsStore.cycleThinkingMode()
  const curMode = thinkingMode.value
  if (curMode === prevMode) return
  message.destroyAll()
  switch (curMode) {
    case 'think':
      message.success('已开启深度思考，回答前将进行深度推理', { duration: 2000 })
      break
    case 'no_think':
      message.info('已关闭深度思考，快速回答（不思考）', { duration: 2000 })
      break
  }
}

// 附件菜单
const showAttachMenu = ref(false)
const pendingUploadType = ref<string>('image')
const fileInputRef = ref<HTMLInputElement | null>(null)

function toggleAttachMenu() {
  showAttachMenu.value = !showAttachMenu.value
}

function closeAttachMenu() {
  showAttachMenu.value = false
}

function triggerFileUpload(type: string) {
  const warn = checkUploadCapability(type, capabilities.value)
  if (warn) {
    message.warning(warn)
    return
  }

  pendingUploadType.value = type
  if (fileInputRef.value) {
    fileInputRef.value.accept = getAcceptForType(type)
    if (type === 'image') {
      fileInputRef.value.multiple = true
    } else {
      fileInputRef.value.multiple = false
    }
    fileInputRef.value.click()
  }
  closeAttachMenu()
}

// 文件选中后，通过 emit 把文件交给父组件处理（useAttachments composable 留在 ChatInput）
function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files || input.files.length === 0) return

  const type = pendingUploadType.value
  for (const file of Array.from(input.files)) {
    emit('fileSelect', type, file)
  }
  input.value = ''
}

function handleClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.attach-wrapper')) {
    closeAttachMenu()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.left-buttons {
  flex-shrink: 0;
  display: flex;
  gap: 4px;
}

.search-btn,
.params-btn,
.attach-btn,
.voice-btn,
.kv-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: var(--border-radius-md);
  border: none;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
  color: var(--text-secondary);
}

.deep-reason-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: var(--border-radius-md);
  border: none;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
  color: var(--text-secondary);
  position: relative;
}

.deep-reason-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.deep-reason-btn .deep-reason-icon {
  /* 书房风：图标不再缩放旋转，仅保留淡入过渡 */
  transition: opacity 0.2s ease;
}

.deep-reason-btn.active {
  /* 书房风：激活态保持静态苔绿落印，不做光晕呼吸 */
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
}

.attach-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.search-btn:hover,
.params-btn:hover,
.attach-btn:hover:not(:disabled),
.voice-btn:hover,
.kv-btn:hover:not(:disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.kv-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.kv-save-btn:hover:not(:disabled) {
  color: var(--accent-primary);
}

.kv-restore-btn:hover:not(:disabled) {
  color: var(--accent-warning);
}

.search-btn.active {
  /* 书房风：激活态使用苔绿真实令牌，淡底由 color-mix 现调 */
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
}

.search-btn.auto-mode {
  color: var(--accent-warning);
  background: color-mix(in srgb, var(--accent-warning) 8%, transparent);
}

.search-btn.auto-mode:hover {
  background: color-mix(in srgb, var(--accent-warning) 14%, transparent);
}

.think-btn {
  position: relative;
}

.think-btn,
.think-btn.unsupported,
.think-btn:disabled {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: var(--border-radius-md);
  border: none;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
  color: var(--text-secondary);
}

.think-btn:hover:not(:disabled) {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.think-btn.unsupported,
.think-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.think-btn .think-icon {
  /* 图标不再缩放，仅保留淡入过渡 */
  transition: opacity 0.2s ease;
}

.think-btn.no-think-mode {
  color: var(--text-muted);
}

.think-btn .think-slash {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%) rotate(-45deg);
  width: 24px;
  height: 3px;
  background: var(--accent-danger);
  border-radius: var(--border-radius-xs);
  pointer-events: none;
  opacity: 0.9;
}

.think-btn.active {
  /* 与深思按钮同语汇：静态激活态，去光晕脉冲 */
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
}

.voice-btn.active {
  color: var(--accent-danger);
  background: var(--accent-r-soft);
}

.voice-btn.unsupported {
  opacity: 0.3;
  cursor: not-allowed;
}

.attach-wrapper {
  position: relative;
}

.attach-menu {
  position: absolute;
  bottom: 52px;
  left: 0;
  /* 阅读层表面：与右键菜单/会话菜单同语汇，背景图模式下自动适配 */
  background: var(--surface-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-md);
  box-shadow: var(--shadow-lg);
  padding: 6px 0;
  min-width: 160px;
  z-index: 100;
}

.attach-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 10px 14px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  color: var(--text-primary);
  transition: background 0.15s;
}

.attach-menu-item:hover:not(.disabled) {
  background: var(--bg-hover);
}

.attach-menu-item.disabled {
  color: var(--text-tertiary);
  cursor: not-allowed;
}

.unsupported-tag {
  margin-left: auto;
  font-size: 10px;
  color: var(--text-muted);
  background: var(--bg-hover);
  padding: 2px 8px;
  border-radius: var(--border-radius-xs);
}

.hidden-file-input {
  display: none;
}
</style>
