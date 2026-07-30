<template>
  <div class="input-area">
    <div class="input-wrapper">
      <!-- Token 计数器：显示在输入框上方 -->
      <TokenCounter :text="inputText" :context-size="settingsStore.config.context_size" />
      <AttachmentPreview
        v-if="attachments.length > 0"
        :attachments="attachments"
        @remove="removeAttachment"
      />
      <div v-if="isListening" class="voice-indicator-bar">
        <div class="voice-pulse">
          <div class="pulse-ring"></div>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
            <path
              d="M19 10v2a7 7 0 0 1-14 0v-2"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <line
              x1="12"
              y1="19"
              x2="12"
              y2="23"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <line
              x1="8"
              y1="23"
              x2="16"
              y2="23"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </svg>
        </div>
        <span class="voice-text">{{ voiceInterimText || '正在聆听...' }}</span>
        <button class="voice-stop-btn" @click="stopListening">停止</button>
      </div>
      <div class="chat-input-container">
        <ChatToolbar
          :is-listening="isListening"
          :speech-supported="speechSupported"
          @toggle-listening="toggleListening"
          @file-select="onFileSelect"
        />

        <div class="the-input">
          <textarea
            ref="textareaRef"
            v-model="inputText"
            placeholder="给DouYa发送消息....."
            rows="1"
            class="chat-textarea"
            @keydown="handleKeydown"
            @paste="handlePaste"
            @contextmenu="handleContextMenu"
          />
        </div>

        <div class="right-buttons">
          <button
            v-if="chatStore.isAnyGenerating"
            class="stop-btn"
            @click="chatStore.stopGeneration()"
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
              <rect x="6" y="6" width="12" height="12" rx="2"></rect>
            </svg>
          </button>
          <button v-else class="send-btn" :disabled="!canSend" @click="handleSend()">
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
              <line x1="22" y1="2" x2="11" y2="13"></line>
              <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
            </svg>
          </button>
        </div>
      </div>
      <div class="input-footer-reminder">AI也会犯错，请仔细甄别</div>
    </div>
    <div
      v-if="contextMenuVisible"
      class="ctx-menu"
      :style="{ left: contextMenuX + 'px', top: contextMenuY + 'px' }"
      @click.stop
    >
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
import { CutOutline, CopyOutline, ClipboardOutline, TextOutline } from '@vicons/ionicons5'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import type { Attachment } from '../services/wails'
import TokenCounter from './TokenCounter.vue'
import ChatToolbar from './ChatToolbar.vue'
import AttachmentPreview from './AttachmentPreview.vue'
import { useAttachments } from '../composables/useAttachments'
// 语音输入与上下文菜单逻辑抽取为 composable（基于架构优化：ChatInput.vue 1789 行→拆分独立职责）
// STT（语音输入）基于浏览器 Web Speech API 实现
import { useVoiceInput } from '../composables/useSpeech'
import { useContextMenu } from '../composables/useContextMenu'

const emit = defineEmits<{
  send: [content: string, images?: string[], attachments?: Attachment[]]
}>()
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const message = useMessage()
const inputText = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
// 使用 shallowRef：附件数组整体替换触发响应式，避免深度代理开销（任务 23）
const attachments = shallowRef<Attachment[]>([])
// 附件处理逻辑抽取到 composable（基于 F-1.8+F-3.2）：
// 包含 6 种文件类型的处理函数、文件大小校验、二进制检测、removeAttachment 等
const { processFileByType, removeAttachment } = useAttachments(attachments, message)

// 语音输入逻辑抽取到 useVoiceInput composable（基于架构优化）
// 生活类比：就像雇了一个语音速记员，说话自动转成文字填进输入框
const {
  isListening,
  voiceInterimText,
  speechSupported,
  initSpeechRecognition,
  toggleListening,
  stopListening,
  cleanup: cleanupVoiceInput
} = useVoiceInput(inputText)

const capabilities = computed(() => settingsStore.modelCapabilities)
const isSwitching = computed(() => settingsStore.isModelSwitching)
const canSend = computed(
  () => !isSwitching.value && (inputText.value.trim() || attachments.value.length > 0)
)

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

// 上下文菜单逻辑抽取到 useContextMenu composable（基于架构优化）
// 生活类比：就像在文字上右键弹出的小工具箱——剪切/复制/粘贴/全选
// 依赖注入：将 handlePaste 作为参数传入，让 composable 内部的"粘贴"操作能复用主组件的文件处理逻辑
const {
  contextMenuVisible,
  contextMenuX,
  contextMenuY,
  canCut,
  canCopy,
  handleContextMenu,
  closeContextMenu,
  ctxCut,
  ctxCopy,
  ctxPaste,
  ctxSelectAll
} = useContextMenu(textareaRef, inputText, handlePaste, message)

// isLikelyBinaryContent / checkFileSize / readFileWithErrorHandling /
// 6 个 process*File 函数 / removeAttachment 已抽取到 useAttachments composable（基于 F-1.8+F-3.2）

function detectFileType(file: File): string | null {
  // 优先按 MIME type 判断
  if (file.type.startsWith('image/')) return 'image'
  if (file.type.startsWith('audio/')) return 'audio'
  if (file.type.startsWith('video/')) return 'video'
  if (file.type === 'application/pdf') return 'pdf'
  // Word 文档 MIME（Officedocument.wordprocessingml.document）
  if (file.type === 'application/vnd.openxmlformats-officedocument.wordprocessingml.document')
    return 'docx'

  // 文本类型：按 MIME type 判断
  const textMimes = [
    'text/plain',
    'text/markdown',
    'text/csv',
    'application/json',
    'text/html',
    'text/css',
    'text/javascript',
    'application/xml'
  ]
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
  if (
    (type === 'image' || type === 'audio' || type === 'video') &&
    !capabilities.value.mmproj_loaded
  ) {
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

const IMAGE_ACCEPT = '.jpg,.jpeg,.png,.gif,.webp,.bmp,.svg,.heic,.heif'
const AUDIO_ACCEPT = '.wav,.mp3,.ogg,.flac,.aac,.m4a,.wma'
const TEXT_ACCEPT =
  '.txt,.md,.csv,.json,.xml,.html,.htm,.css,.js,.jsx,.ts,.tsx,.vue,.svelte,.py,.go,.java,.c,.cpp,.h,.hpp,.rs,.sh,.bat,.yaml,.yml,.toml,.ini,.cfg,.log,.sql,.adoc,.tex,.bib,.cs,.kt,.swift,.dart,.r,.scala,.hs,.cu,.cuh,.comp,.properties'
const VIDEO_ACCEPT = '.mp4,.webm,.avi,.mov,.mkv,.wmv,.flv'

// MAX_*_SIZE 常量 / checkFileSize / readFileWithErrorHandling
// 已抽取到 useAttachments composable（基于 F-1.8+F-3.2）

// processImageFile / processAudioFile / processPdfFile / processDocxFile /
// processVideoFile / processTextFile / removeAttachment
// 已抽取到 useAttachments composable（基于 F-1.8+F-3.2）：
//   - processImageFile 保留独立实现（异步流水线）
//   - 其他 5 个通过 processFileCommon 高阶函数 + FILE_CONFIGS 表驱动统一处理
//   - 原约 130 行重复代码缩减为 composable 中的 1 个通用函数 + 1 个配置表

// initSpeechRecognition / toggleListening / startListening / stopListening
// 已抽取到 useVoiceInput composable（基于架构优化：ChatInput.vue 1789 行→拆分独立职责）

// ChatToolbar 选中文件后通过 emit 通知父组件处理（useAttachments composable 留在此处）
function onFileSelect(type: string, file: File) {
  processFileByType(type, file)
}

function handleSend() {
  const text = inputText.value.trim()
  if (!text && attachments.value.length === 0) return
  if (chatStore.isAnyGenerating) return
  if (isSwitching.value) return

  if (isListening.value) {
    stopListening()
  }

  const imageAttachments = attachments.value.filter(a => a.type === 'image')
  const images = imageAttachments.length > 0 ? imageAttachments.map(a => a.data) : undefined

  const allAttachments =
    attachments.value.length > 0 ? attachments.value.map(a => ({ ...a })) : undefined

  emit('send', text, images, allAttachments)
  inputText.value = ''
  attachments.value = []
  nextTick(() => {
    if (textareaRef.value) {
      textareaRef.value.style.height = 'auto'
    }
  })
}

onMounted(() => {
  document.addEventListener('click', closeContextMenu)
  document.addEventListener('contextmenu', closeContextMenu, true)
  if (speechSupported.value) {
    initSpeechRecognition()
  }
})

onUnmounted(() => {
  document.removeEventListener('click', closeContextMenu)
  document.removeEventListener('contextmenu', closeContextMenu, true)
  // 释放语音识别资源（composable 内部清理）
  cleanupVoiceInput()
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

.voice-indicator-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  padding: 0 14px;
  margin: 0 0 4px;
  background: var(--bg-secondary);
  border-radius: var(--border-radius-md);
  animation: chat-input-fade-in 0.2s ease;
}

@keyframes chat-input-fade-in {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
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
  animation: pulse-ring 1.5s ease-in-out infinite;
}

/* Q10: 重命名为 pulse-ring 避免与全局 style.css 的 @keyframes pulse（透明度脉冲）冲突
 * Vue scoped 不隔离 @keyframes，同名定义会覆盖全局，导致全局 animation: pulse 行为被污染 */
@keyframes pulse-ring {
  0% {
    transform: scale(1);
    opacity: 0.6;
  }
  50% {
    transform: scale(1.4);
    opacity: 0;
  }
  100% {
    transform: scale(1);
    opacity: 0;
  }
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
  transition:
    border-color 0.2s,
    box-shadow 0.2s;
  position: relative;
}

.chat-input-container:focus-within {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-primary) 20%, transparent);
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
  from {
    opacity: 0;
    transform: scale(0.96);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
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
  transition:
    transform 0.2s,
    box-shadow 0.2s;
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
  animation: chat-input-ripple 0.5s ease-out;
  pointer-events: none;
}

@keyframes chat-input-ripple {
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
  transition:
    transform 0.2s,
    box-shadow 0.2s;
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
