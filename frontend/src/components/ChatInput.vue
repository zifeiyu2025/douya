<template>
  <div class="input-area">
    <div class="input-wrapper">
      <!-- Token 计数器：显示在输入框上方 -->
      <TokenCounter :text="inputText" :context-size="settingsStore.config.context_size" />
      <div v-if="attachments.length > 0" class="attachment-preview-bar">
        <div
          v-for="(att, idx) in attachments"
          :key="att.name"
          class="attachment-preview-item"
          :class="att.type"
        >
          <div v-if="att.type === 'image'" class="att-thumb image-thumb">
            <img :src="att.data" alt="" />
          </div>
          <div v-else class="att-thumb file-thumb">
            <svg v-if="att.type === 'audio'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
            <svg v-else-if="att.type === 'video'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg>
            <svg v-else-if="att.type === 'pdf'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
            <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
          </div>
          <div class="att-info">
            <span class="att-name" :title="att.name">{{ att.name }}</span>
            <span class="att-type-label">{{ typeLabel(att.type) }}</span>
          </div>
          <button class="remove-att-btn" @click="removeAttachment(idx)">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
        </div>
      </div>
      <div v-if="isListening" class="voice-indicator-bar">
        <div class="voice-pulse">
          <div class="pulse-ring"></div>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><line x1="12" y1="19" x2="12" y2="23" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><line x1="8" y1="23" x2="16" y2="23" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
        </div>
        <span class="voice-text">{{ voiceInterimText || '正在聆听...' }}</span>
        <button class="voice-stop-btn" @click="stopListening">停止</button>
      </div>
      <div class="chat-input-container">
        <div class="left-buttons">
                <button class="think-btn" :class="thinkBtnClass" :disabled="!supportsThinking" @click="handleThinkClick" :title="thinkingTitle">
                  <n-icon size="22" class="think-icon"><BulbOutline /></n-icon>
                  <span v-if="thinkingMode === 'no_think'" class="think-slash"></span>
                </button>
                <button
                  v-if="supportsDeepReasoning"
                  class="deep-reason-btn"
                  :class="{ active: deepReasoningOn }"
                  @click="handleDeepReasonClick"
                  :title="deepReasoningTitle"
                >
                  <n-icon size="20" class="deep-reason-icon"><LayersOutline /></n-icon>
                </button>
                <button class="search-btn" :class="searchBtnClass" @click="handleSearchClick" :title="searchTitle">
                  <n-icon size="22"><GlobeOutline /></n-icon>
                </button>
                <div class="attach-wrapper">
                  <button class="attach-btn" @click="toggleAttachMenu" :disabled="isSwitching" title="添加附件">
                    <n-icon size="22"><AttachOutline /></n-icon>
                  </button>
            <div v-if="showAttachMenu" class="attach-menu">
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.mmproj_loaded || !capabilities.image_input }"
                @click="triggerFileUpload('image')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>
                <span>图片</span>
                <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
                <span v-else-if="!capabilities.image_input" class="unsupported-tag">不支持</span>
              </button>
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.mmproj_loaded || !capabilities.audio_input }"
                @click="triggerFileUpload('audio')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
                <span>音频</span>
                <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
                <span v-else-if="!capabilities.audio_input" class="unsupported-tag">不支持</span>
              </button>
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.mmproj_loaded || !capabilities.video_input }"
                @click="triggerFileUpload('video')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg>
                <span>视频</span>
                <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
                <span v-else-if="!capabilities.video_input" class="unsupported-tag">不支持</span>
              </button>
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.text_input }"
                @click="triggerFileUpload('text')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
                <span>文本</span>
                <span v-if="!capabilities.text_input" class="unsupported-tag">不支持</span>
              </button>
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.text_input }"
                @click="triggerFileUpload('pdf')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
                <span>PDF</span>
                <span v-if="!capabilities.text_input" class="unsupported-tag">不支持</span>
              </button>
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.text_input }"
                @click="triggerFileUpload('docx')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><path d="M9 15v-3"/><path d="M12 15v-3"/><path d="M15 15v-3"/><path d="M9 18h6"/></svg>
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
            @click="toggleListening"
            :title="speechSupported ? (isListening ? '停止语音输入' : '语音输入') : '浏览器不支持语音识别'"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"></path>
              <path d="M19 10v2a7 7 0 0 1-14 0v-2"></path>
              <line x1="12" y1="19" x2="12" y2="23"></line>
              <line x1="8" y1="23" x2="16" y2="23"></line>
            </svg>
          </button>
          <button
            v-if="settingsStore.config.slot_save_enabled"
            class="kv-btn kv-save-btn"
            :disabled="chatStore.isGenerating"
            @click="handleSaveKV"
            title="保存 KV 缓存"
          >
            <n-icon size="22"><CloudUploadOutline /></n-icon>
          </button>
          <button
            v-if="settingsStore.config.slot_save_enabled"
            class="kv-btn kv-restore-btn"
            @click="handleRestoreKV"
            title="恢复 KV 缓存"
          >
            <n-icon size="22"><CloudDownloadOutline /></n-icon>
          </button>
        </div>

        <div class="the-input">
          <textarea
            ref="textareaRef"
            v-model="inputText"
            placeholder="给DouYa发送消息....."
            @keydown="handleKeydown"
            @paste="handlePaste"
            @contextmenu="handleContextMenu"
            rows="1"
            class="chat-textarea"
          />
        </div>

        <div class="right-buttons">
          <button v-if="chatStore.isGenerating" class="stop-btn" @click="chatStore.stopGeneration()">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="6" y="6" width="12" height="12" rx="2"></rect>
            </svg>
          </button>
          <button v-else class="send-btn" :disabled="!canSend" @click="handleSend()">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="22" y1="2" x2="11" y2="13"></line>
              <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
            </svg>
          </button>
        </div>
      </div>
      <div class="input-footer-reminder">AI也会犯错，请仔细甄别</div>
    </div>
    <div v-if="contextMenuVisible" class="ctx-menu" :style="{ left: contextMenuX + 'px', top: contextMenuY + 'px' }" @click.stop>
      <button class="ctx-menu-item" :class="{ disabled: !canCut }" @click="ctxCut">
        <n-icon size="14"><CutOutline /></n-icon>
        <span>剪切</span>
      </button>
      <button class="ctx-menu-item" :class="{ disabled: !canCopy }" @click="ctxCopy">
        <n-icon size="14"><CopyOutline /></n-icon>
        <span>复制</span>
      </button>
      <button class="ctx-menu-item" @click="ctxPaste">
        <n-icon size="14"><ClipboardOutline /></n-icon>
        <span>粘贴</span>
      </button>
      <div class="ctx-menu-divider"></div>
      <button class="ctx-menu-item" @click="ctxSelectAll">
        <n-icon size="14"><TextOutline /></n-icon>
        <span>全选</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { NIcon, useMessage } from 'naive-ui'
import { GlobeOutline, AttachOutline, BulbOutline, CloudUploadOutline, CloudDownloadOutline, LayersOutline } from '@vicons/ionicons5'
import { CutOutline, CopyOutline, ClipboardOutline, TextOutline } from '@vicons/ionicons5'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { wails } from '../services/wails'
import type { Attachment } from '../services/wails'
import TokenCounter from './TokenCounter.vue'
import { showSuccess } from '../utils/showError'
import { useAttachments } from '../composables/useAttachments'

interface SpeechRecognitionEvent {
  results: SpeechRecognitionResultList
  resultIndex: number
}

interface SpeechRecognitionErrorEvent {
  error: string
  message: string
}

interface SpeechRecognitionInstance {
  lang: string
  continuous: boolean
  interimResults: boolean
  maxAlternatives: number
  onresult: ((event: SpeechRecognitionEvent) => void) | null
  onerror: ((event: SpeechRecognitionErrorEvent) => void) | null
  onend: (() => void) | null
  onstart: (() => void) | null
  start: () => void
  stop: () => void
  abort: () => void
}

declare global {
  interface Window {
    SpeechRecognition?: new () => SpeechRecognitionInstance
    webkitSpeechRecognition?: new () => SpeechRecognitionInstance
  }
}

const emit = defineEmits<{ send: [content: string, images?: string[], attachments?: Attachment[]] }>()
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const message = useMessage()
const inputText = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
// 使用 shallowRef：附件数组整体替换触发响应式，避免深度代理开销（任务 23）
const attachments = shallowRef<Attachment[]>([])
// 附件处理逻辑抽取到 composable（基于 F-1.8+F-3.2）：
// 包含 6 种文件类型的处理函数、文件大小校验、二进制检测、removeAttachment 等
const { processFileByType, removeAttachment } = useAttachments(attachments, message)
const showAttachMenu = ref(false)
const pendingUploadType = ref<string>('image')

const isListening = ref(false)
const voiceInterimText = ref('')
let recognition: SpeechRecognitionInstance | null = null
let voiceFinalBuffer = ''

const speechSupported = computed(() => {
  return !!(window.SpeechRecognition || window.webkitSpeechRecognition)
})

const searchMode = computed(() => settingsStore.searchMode)
const thinkingMode = computed(() => settingsStore.thinkingSoftSwitch)
const capabilities = computed(() => settingsStore.modelCapabilities)
const isSwitching = computed(() => settingsStore.isModelSwitching)
const supportsThinking = computed(() => settingsStore.modelCapabilities.thinking_mode !== 'none')
const thinkingTitle = computed(() => {
    if (!supportsThinking.value) return '当前模型不支持思考'
    switch (thinkingMode.value) {
        case 'think': return '强制深度思考'
        case 'no_think': return '快速回答（不思考）'
        default: return '自动思考'
    }
})
const thinkBtnClass = computed(() => ({
    active: thinkingMode.value === 'think',
    'auto-mode': thinkingMode.value === 'auto',
    'no-think-mode': thinkingMode.value === 'no_think',
    unsupported: !supportsThinking.value,
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
        case 'on': return '强制搜索（所有消息都搜索）'
        case 'auto': return '智能搜索（按需自动搜索）'
        default: return '联网搜索已关闭'
    }
})
const searchBtnClass = computed(() => ({
    active: searchMode.value === 'on',
    'auto-mode': searchMode.value === 'auto',
}))
const canSend = computed(() => !isSwitching.value && (inputText.value.trim() || attachments.value.length > 0))

async function handleSearchClick() {
    const prevMode = searchMode.value
    // 即将开启搜索（当前不是 on），检查是否配置了搜索 API Key
    if (prevMode !== 'on') {
        await settingsStore.loadSearchAPIKeys()
        const keys = settingsStore.searchAPIKeys
        // 未配置 API Key 时，不再拦截，改为提示将使用 Bing 兜底搜索
        // Bing 是免 Key 兜底引擎，确保用户始终有搜索能力
        if (!keys.tavily_api_key_set && !keys.ollama_api_key_set) {
            message.destroyAll()
            message.info('未配置搜索 API Key，将使用 Bing 免费搜索（可在设置中配置 Tavily/Ollama 获得更佳体验）', { duration: 3500 })
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
        case 'auto':
            message.info('已切换为自动思考', { duration: 2000 })
            break
        case 'think':
            message.success('已开启强制深度思考', { duration: 2000 })
            break
        case 'no_think':
            message.info('已切换为快速回答（不思考）', { duration: 2000 })
            break
    }
}

async function handleSaveKV() {
    if (chatStore.isGenerating) return
    try {
        await wails.saveSlot(0)
        showSuccess(message, 'KV 缓存已保存')
    } catch (e) {
        message.destroyAll()
        message.error(`保存 KV 缓存失败: ${e}`, { duration: 3000 })
    }
}

async function handleRestoreKV() {
    try {
        await wails.restoreSlot(0)
        showSuccess(message, 'KV 缓存已恢复')
    } catch (e) {
        message.destroyAll()
        message.error(`恢复 KV 缓存失败: ${e}`, { duration: 3000 })
    }
}

function adjustHeight() {
  if (textareaRef.value) {
    textareaRef.value.style.height = 'auto'
    textareaRef.value.style.height = Math.min(textareaRef.value.scrollHeight, 180) + 'px'
  }
}

watch(inputText, () => {
  nextTick(adjustHeight)
})

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}

const contextMenuVisible = ref(false)
const contextMenuX = ref(0)
const contextMenuY = ref(0)
const canCut = ref(false)
const canCopy = ref(false)

function handleContextMenu(e: MouseEvent) {
  e.preventDefault()
  const ta = textareaRef.value
  if (!ta) return
  // 检查是否有选中文本
  const start = ta.selectionStart
  const end = ta.selectionEnd
  canCut.value = start !== end
  canCopy.value = start !== end
  // 边界检测：确保菜单不超出视窗
  const menuWidth = 160
  const menuHeight = 180
  let x = e.clientX
  let y = e.clientY
  if (x + menuWidth > window.innerWidth) x = window.innerWidth - menuWidth - 4
  if (y + menuHeight > window.innerHeight) y = window.innerHeight - menuHeight - 4
  contextMenuX.value = x
  contextMenuY.value = y
  contextMenuVisible.value = true
}

function closeContextMenu() {
  contextMenuVisible.value = false
}

function ctxCut() {
  if (!canCut.value) return
  document.execCommand('cut')
  closeContextMenu()
}

function ctxCopy() {
  if (!canCopy.value) return
  document.execCommand('copy')
  closeContextMenu()
}

async function ctxPaste() {
  closeContextMenu()
  try {
    // 优先尝试读取剪贴板文件
    const clipboardItems = await navigator.clipboard.read()
    for (const item of clipboardItems) {
      for (const type of item.types) {
        if (type.startsWith('image/') || type.startsWith('audio/') || type.startsWith('video/') || type === 'application/pdf') {
          const blob = await item.getType(type)
          const file = new File([blob], `pasted-${Date.now()}.${type.split('/')[1]}`, { type })
          // 模拟 paste 事件处理
          const fakeEvent = {
            clipboardData: { items: [{ kind: 'file', type, getAsFile: () => file }] },
            preventDefault: () => {}
          } as unknown as ClipboardEvent
          handlePaste(fakeEvent)
          return
        }
      }
    }
    // 没有文件，读取文本
    const text = await navigator.clipboard.readText()
    if (text) {
      const ta = textareaRef.value
      if (ta) {
        const start = ta.selectionStart
        const end = ta.selectionEnd
        inputText.value = inputText.value.slice(0, start) + text + inputText.value.slice(end)
        nextTick(() => {
          ta.selectionStart = ta.selectionEnd = start + text.length
          ta.focus()
        })
      }
    }
  } catch {
    message.warning('粘贴失败，请使用 Ctrl+V')
  }
}

function ctxSelectAll() {
  const ta = textareaRef.value
  if (ta) {
    ta.select()
    ta.focus()
  }
  closeContextMenu()
}

function handlePaste(e: ClipboardEvent) {
  const clipboardData = e.clipboardData
  if (!clipboardData) return

  const items = clipboardData.items
  if (!items || items.length === 0) return

  // 收集剪贴板中的文件
  const files: File[] = []
  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    if (item.kind === 'file') {
      const file = item.getAsFile()
      if (file) files.push(file)
    }
  }

  if (files.length === 0) return

  // 有文件时阻止默认行为（避免图片被插入 textarea）
  e.preventDefault()

  for (const file of files) {
    const fileType = detectFileType(file)
    if (!fileType) {
      message.warning(`不支持的文件类型: ${file.name}`)
      continue
    }
    if (!checkCapability(fileType)) continue
    processFileByType(fileType, file)
  }
}

// isLikelyBinaryContent / checkFileSize / readFileWithErrorHandling /
// 6 个 process*File 函数 / removeAttachment 已抽取到 useAttachments composable（基于 F-1.8+F-3.2）

function detectFileType(file: File): string | null {
  // 优先按 MIME type 判断
  if (file.type.startsWith('image/')) return 'image'
  if (file.type.startsWith('audio/')) return 'audio'
  if (file.type.startsWith('video/')) return 'video'
  if (file.type === 'application/pdf') return 'pdf'
  // Word 文档 MIME（Officedocument.wordprocessingml.document）
  if (file.type === 'application/vnd.openxmlformats-officedocument.wordprocessingml.document') return 'docx'

  // 文本类型：按 MIME type 判断
  const textMimes = ['text/plain', 'text/markdown', 'text/csv', 'application/json', 'text/html', 'text/css', 'text/javascript', 'application/xml']
  if (textMimes.includes(file.type)) return 'text'

  // 按扩展名兜底
  const ext = file.name.split('.').pop()?.toLowerCase() || ''
  if (IMAGE_ACCEPT.includes(`.${ext}`)) return 'image'
  if (AUDIO_ACCEPT.includes(`.${ext}`)) return 'audio'
  if (VIDEO_ACCEPT.includes(`.${ext}`)) return 'video'
  if (ext === 'pdf') return 'pdf'
  if (ext === 'docx') return 'docx'
  if (TEXT_ACCEPT.includes(`.${ext}`)) return 'text'

  return null
}

function checkCapability(type: string): boolean {
  if ((type === 'image' || type === 'audio' || type === 'video') && !capabilities.value.mmproj_loaded) {
    message.warning('多模态投影未加载，无法处理此类型文件')
    return false
  }
  if (type === 'image' && !capabilities.value.image_input) {
    message.warning('当前模型不支持图片输入')
    return false
  }
  if (type === 'audio' && !capabilities.value.audio_input) {
    message.warning('当前模型不支持音频输入')
    return false
  }
  if (type === 'video' && !capabilities.value.video_input) {
    message.warning('当前模型不支持视频输入')
    return false
  }
  if ((type === 'text' || type === 'pdf' || type === 'docx') && !capabilities.value.text_input) {
    message.warning('当前模型不支持文本文件输入')
    return false
  }
  return true
}

// processFileByType 已抽取到 useAttachments composable（基于 F-1.8+F-3.2）

function toggleAttachMenu() {
  showAttachMenu.value = !showAttachMenu.value
}

function closeAttachMenu() {
  showAttachMenu.value = false
}

function typeLabel(type: string): string {
  switch (type) {
    case 'image': return '图片'
    case 'audio': return '音频'
    case 'text': return '文本'
    case 'video': return '视频'
    case 'pdf': return 'PDF'
    case 'docx': return 'DOCX'
    default: return type
  }
}

const IMAGE_ACCEPT = '.jpg,.jpeg,.png,.gif,.webp,.bmp,.svg,.heic,.heif'
const AUDIO_ACCEPT = '.wav,.mp3,.ogg,.flac,.aac,.m4a,.wma'
const TEXT_ACCEPT = '.txt,.md,.csv,.json,.xml,.html,.htm,.css,.js,.jsx,.ts,.tsx,.vue,.svelte,.py,.go,.java,.c,.cpp,.h,.hpp,.rs,.sh,.bat,.yaml,.yml,.toml,.ini,.cfg,.log,.sql,.adoc,.tex,.bib,.cs,.kt,.swift,.dart,.r,.scala,.hs,.cu,.cuh,.comp,.properties'
const PDF_ACCEPT = '.pdf'
const DOCX_ACCEPT = '.docx'
const VIDEO_ACCEPT = '.mp4,.webm,.avi,.mov,.mkv,.wmv,.flv'

// MAX_*_SIZE 常量 / checkFileSize / readFileWithErrorHandling
// 已抽取到 useAttachments composable（基于 F-1.8+F-3.2）

function getAcceptForType(type: string): string {
  switch (type) {
    case 'image': return IMAGE_ACCEPT
    case 'audio': return AUDIO_ACCEPT
    case 'text': return TEXT_ACCEPT
    case 'pdf': return PDF_ACCEPT
    case 'docx': return DOCX_ACCEPT
    case 'video': return VIDEO_ACCEPT
    default: return ''
  }
}

function triggerFileUpload(type: string) {
  if (!checkCapability(type)) return

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

function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files || input.files.length === 0) return

  const type = pendingUploadType.value
  // 安全实践（基于 F-1.8）：统一调用 composable 的 processFileByType，避免重复 if/else 链
  for (const file of Array.from(input.files)) {
    processFileByType(type, file)
  }
  input.value = ''
}

// processImageFile / processAudioFile / processPdfFile / processDocxFile /
// processVideoFile / processTextFile / removeAttachment
// 已抽取到 useAttachments composable（基于 F-1.8+F-3.2）：
//   - processImageFile 保留独立实现（异步流水线）
//   - 其他 5 个通过 processFileCommon 高阶函数 + FILE_CONFIGS 表驱动统一处理
//   - 原约 130 行重复代码缩减为 composable 中的 1 个通用函数 + 1 个配置表

function initSpeechRecognition() {
  const SpeechRecognitionCtor = window.SpeechRecognition || window.webkitSpeechRecognition
  if (!SpeechRecognitionCtor) return

  recognition = new SpeechRecognitionCtor()
  recognition.lang = 'zh-CN'
  recognition.continuous = true
  recognition.interimResults = true
  recognition.maxAlternatives = 1

  recognition.onresult = (event: SpeechRecognitionEvent) => {
    let interim = ''
    let finalTranscript = ''
    for (let i = event.resultIndex; i < event.results.length; i++) {
      const result = event.results[i]
      if (result.isFinal) {
        finalTranscript += result[0].transcript
      } else {
        interim += result[0].transcript
      }
    }
    if (finalTranscript) {
      voiceFinalBuffer += finalTranscript
      inputText.value = voiceFinalBuffer + interim
    } else {
      inputText.value = voiceFinalBuffer + interim
    }
    voiceInterimText.value = interim
  }

  recognition.onerror = (event: SpeechRecognitionErrorEvent) => {
    console.warn('语音识别错误:', event.error)
    if (event.error === 'not-allowed') {
      isListening.value = false
      voiceInterimText.value = ''
    }
  }

  recognition.onend = () => {
    if (isListening.value) {
      try {
        recognition?.start()
      } catch {
        isListening.value = false
        voiceInterimText.value = ''
      }
    } else {
      voiceInterimText.value = ''
    }
  }

  recognition.onstart = () => {
    isListening.value = true
  }
}

function toggleListening() {
  if (!speechSupported.value) return
  if (isListening.value) {
    stopListening()
  } else {
    startListening()
  }
}

function doStartRecognition(rec: SpeechRecognitionInstance) {
  try {
    rec.start()
  } catch {
    console.warn('无法启动语音识别')
  }
}

function startListening() {
  if (!recognition) {
    initSpeechRecognition()
  }
  if (!recognition) return

  voiceFinalBuffer = inputText.value
  voiceInterimText.value = ''

  try {
    recognition.start()
  } catch {
    recognition = null
    initSpeechRecognition()
    if (recognition) {
      doStartRecognition(recognition)
    }
  }
}

function stopListening() {
  if (recognition) {
    isListening.value = false
    recognition.stop()
  }
  voiceInterimText.value = ''
  voiceFinalBuffer = ''
}

function handleSend() {
  const text = inputText.value.trim()
  if (!text && attachments.value.length === 0) return
  if (chatStore.isGenerating) return
  if (isSwitching.value) return

  if (isListening.value) {
    stopListening()
  }

  const imageAttachments = attachments.value.filter(a => a.type === 'image')
  const images = imageAttachments.length > 0
    ? imageAttachments.map(a => a.data)
    : undefined

  const allAttachments = attachments.value.length > 0
    ? attachments.value.map(a => ({ ...a }))
    : undefined

  emit('send', text, images, allAttachments)
  inputText.value = ''
  attachments.value = []
  nextTick(() => {
    if (textareaRef.value) {
      textareaRef.value.style.height = 'auto'
    }
  })
}

function handleClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.attach-wrapper')) {
    closeAttachMenu()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('click', closeContextMenu)
  document.addEventListener('contextmenu', closeContextMenu, true)
  if (speechSupported.value) {
    initSpeechRecognition()
  }
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('click', closeContextMenu)
  document.removeEventListener('contextmenu', closeContextMenu, true)
  if (recognition) {
    isListening.value = false
    recognition.abort()
    recognition = null
  }
})
</script>

<style scoped>
.input-area {
  padding: 10px 24px 10px;
  border-top: 1px solid var(--border-color);
  background: var(--bg-secondary);
  position: relative;
  z-index: 1;
}

.input-wrapper {
  max-width: var(--msg-max-width);
  margin: 0 auto;
  width: 100%;
}

.input-footer-reminder {
  max-width: var(--msg-max-width);
  margin: 6px auto 0;
  text-align: center;
  font-size: 11px;
  line-height: 1.5;
  color: var(--text-muted);
  letter-spacing: 0.2px;
  user-select: none;
}

.attachment-preview-bar {
  display: flex;
  gap: 10px;
  padding: 8px 12px 6px;
  flex-wrap: wrap;
}

.attachment-preview-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-radius: var(--border-radius-sm);
  background: var(--bg-secondary);
  max-width: 220px;
  transition: all 0.2s;
}

.attachment-preview-item:hover {
  background: var(--bg-hover);
}

.attachment-preview-item.image {
  padding: 4px;
  max-width: none;
}

.att-thumb {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.att-thumb.image-thumb {
  width: 64px;
  height: 64px;
  border-radius: var(--border-radius-sm);
  overflow: hidden;
}

.att-thumb.image-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.att-thumb.file-thumb {
  width: 40px;
  height: 40px;
  border-radius: var(--border-radius-sm);
  background: var(--bg-hover);
  color: var(--text-secondary);
}

.att-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 3px;
}

.att-name {
  font-size: 12px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 130px;
  font-weight: 500;
}

.att-type-label {
  font-size: 11px;
  color: var(--text-muted);
}

.remove-att-btn {
  position: absolute;
  top: -6px;
  right: -6px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--accent-danger);
  color: white;
  border: 2px solid var(--bg-primary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  line-height: 1;
  opacity: 0;
  transition: opacity 0.2s, transform 0.2s;
}

.attachment-preview-item:hover .remove-att-btn {
  opacity: 1;
}

.remove-att-btn:hover {
  transform: scale(1.1);
}

.voice-indicator-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  padding: 0 14px;
  margin: 0 0 4px;
  background: var(--bg-secondary);
  border-radius: var(--border-radius-md);
  animation: fadeIn 0.2s ease;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}

.voice-pulse {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  color: var(--accent-danger);
}

.pulse-ring {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  background: var(--accent-r-soft);
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0% { transform: scale(1); opacity: 0.6; }
  50% { transform: scale(1.4); opacity: 0; }
  100% { transform: scale(1); opacity: 0; }
}

.voice-text {
  flex: 1;
  font-size: 13px;
  color: var(--text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.voice-stop-btn {
  font-size: 12px;
  padding: 4px 12px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s;
  font-weight: 500;
}

.voice-stop-btn:hover {
  background: var(--accent-r-soft);
  border-color: var(--accent-danger);
  color: var(--accent-danger);
}

.chat-input-container {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  /* 容器与 textarea 同色，避免双层底色撕裂 */
  background: var(--bg-input);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-lg);
  padding: 12px 14px;
  width: 100%;
  box-sizing: border-box;
  transition: border-color 0.2s, box-shadow 0.2s;
  position: relative;
}

.chat-input-container:focus-within {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-primary) 20%, transparent);
}

.left-buttons {
  flex-shrink: 0;
  display: flex;
  gap: 4px;
}

.search-btn, .think-btn, .attach-btn, .voice-btn, .kv-btn {
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
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1), opacity 0.3s ease;
  will-change: transform;
}

.deep-reason-btn:hover .deep-reason-icon {
  transform: scale(1.08) rotate(-8deg);
}

.deep-reason-btn.active {
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
  animation: deep-reason-pulse 3.5s ease-in-out infinite;
}

.deep-reason-btn.active .deep-reason-icon {
  animation: deep-reason-shimmer 2s ease-in-out infinite;
}

@keyframes deep-reason-pulse {
  0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-primary) 0%, transparent); }
  50% { box-shadow: 0 0 8px 1px color-mix(in srgb, var(--accent-primary) 22%, transparent); }
}

@keyframes deep-reason-shimmer {
  0%, 100% { filter: drop-shadow(0 0 0 transparent); transform: scale(1); }
  50% { filter: drop-shadow(0 0 4px color-mix(in srgb, var(--accent-primary) 40%, transparent)); transform: scale(1.05); }
}

.attach-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.search-btn:hover, .think-btn:hover, .attach-btn:hover:not(:disabled), .voice-btn:hover, .kv-btn:hover:not(:disabled) {
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
  color: var(--accent-g-primary);
  background: var(--accent-g-soft);
}

.search-btn.auto-mode {
  color: var(--accent-warning);
  background: color-mix(in srgb, var(--accent-warning) 8%, transparent);
}

.search-btn.auto-mode:hover {
  background: color-mix(in srgb, var(--accent-warning) 14%, transparent);
}

.think-btn.active {
    color: var(--accent-primary);
    background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
    animation: think-pulse 3s ease-in-out infinite;
}

@keyframes think-pulse {
    0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-primary) 0%, transparent); }
    50% { box-shadow: 0 0 6px 1px color-mix(in srgb, var(--accent-primary) 18%, transparent); }
}

.think-btn.unsupported {
    opacity: 0.3;
    cursor: not-allowed;
}

.think-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
}

.think-btn {
    position: relative;
}

.think-btn .think-icon {
    transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1), opacity 0.3s ease;
    will-change: transform;
}

.think-btn.auto-mode {
    color: var(--text-secondary);
}

.think-btn.auto-mode:hover .think-icon {
    transform: scale(1.08);
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
    border-radius: 2px;
    pointer-events: none;
    opacity: 0.9;
}

.think-btn.active .think-icon {
    transform: scale(1.05);
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

.ctx-menu {
  position: fixed;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-md);
  box-shadow: var(--shadow-lg);
  padding: 6px 0;
  min-width: 140px;
  z-index: 1000;
  animation: ctxMenuIn 0.12s ease;
}

@keyframes ctxMenuIn {
  from { opacity: 0; transform: scale(0.96); }
  to { opacity: 1; transform: scale(1); }
}

.ctx-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 14px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  color: var(--text-primary);
  transition: background 0.15s;
}

.ctx-menu-item:hover:not(.disabled) {
  background: var(--bg-hover);
}

.ctx-menu-item.disabled {
  color: var(--text-tertiary);
  cursor: not-allowed;
}

.ctx-menu-divider {
  height: 1px;
  background: var(--border-color);
  margin: 4px 0;
}

.attach-menu {
  position: absolute;
  bottom: 52px;
  left: 0;
  background: var(--bg-primary);
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

.the-input {
  flex: 1;
  width: 100%;
  display: flex;
  align-items: center;
}

.chat-textarea {
  width: 100%;
  border: none;
  /* 透明背景，让容器 .chat-input-container 的底色透出，避免双底色 */
  background: transparent;
  font-family: inherit;
  font-size: 15px;
  line-height: 1.65;
  resize: none;
  outline: none;
  color: var(--text-primary);
  max-height: 180px;
  padding: 8px 0;
}

.chat-textarea::placeholder {
  color: var(--text-muted);
}

.right-buttons {
  flex-shrink: 0;
}

.send-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: var(--border-radius-md);
  border: none;
  background: var(--accent-primary);
  color: white;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
  flex-shrink: 0;
  position: relative;
  overflow: hidden;
  /* 持续微发光，强化"可点击"感知 */
  box-shadow: 0 2px 8px color-mix(in srgb, var(--accent-primary) 25%, transparent);
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  box-shadow: none;
}

.send-btn:not(:disabled):hover {
  background: var(--accent-secondary);
  transform: scale(1.05);
  box-shadow: 0 4px 14px color-mix(in srgb, var(--accent-primary) 40%, transparent);
}

/* 点击涟漪效果（伪元素，不影响布局） */
.send-btn:not(:disabled):active::after,
.stop-btn:active::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: var(--bg-tertiary);
  animation: ripple 0.5s ease-out;
  pointer-events: none;
}

@keyframes ripple {
  0% {
    transform: scale(0);
    opacity: 1;
  }
  100% {
    transform: scale(2);
    opacity: 0;
  }
}

.stop-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: var(--border-radius-md);
  border: none;
  background: var(--accent-r-primary);
  color: white;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
  flex-shrink: 0;
  position: relative;
  overflow: hidden;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--accent-r-primary) 25%, transparent);
}

.stop-btn:hover {
  background: var(--accent-r-strong);
  transform: scale(1.05);
  box-shadow: 0 4px 14px color-mix(in srgb, var(--accent-r-primary) 40%, transparent);
}
</style>

<style>
.has-background .input-area {
  background: transparent;
  border-top-color: transparent;
}
</style>
