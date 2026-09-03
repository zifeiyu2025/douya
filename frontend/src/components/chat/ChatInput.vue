<template>
  <div class="input-area">
    <div
      class="input-wrapper"
      @dragenter="onDragEnter"
      @dragover.prevent
      @dragleave="onDragLeave"
      @drop.prevent="onDrop"
    >
      <!-- 拖拽上传高亮遮罩：文件拖入输入框时浮现，松开释放即添加附件 -->
      <div v-if="isDragging" class="drop-overlay" aria-hidden="true">
        <svg
          class="drop-icon"
          width="30"
          height="30"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
          <polyline points="17 8 12 3 7 8"></polyline>
          <line x1="12" y1="3" x2="12" y2="15"></line>
        </svg>
        <span class="drop-title">松开以添加文件</span>
        <span class="drop-sub">支持上传图片 / 音频 / 视频 / PDF / Word / 文本</span>
      </div>
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
            :placeholder="inputPlaceholder"
            :disabled="chatUnavailable"
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
import { useChatStore } from '../../stores/chat'
import { useSettingsStore } from '../../stores/settings'
import type { Attachment } from '../../services/wails'
import TokenCounter from './TokenCounter.vue'
import ChatToolbar from './ChatToolbar.vue'
import AttachmentPreview from './AttachmentPreview.vue'
import { useAttachments } from '../../composables/useAttachments'
import {
  checkUploadCapability,
  IMAGE_ACCEPT,
  AUDIO_ACCEPT,
  TEXT_ACCEPT,
  VIDEO_ACCEPT
} from '../../utils/attachments'
// 语音输入与上下文菜单逻辑抽取为 composable（基于架构优化：ChatInput.vue 1789 行→拆分独立职责）
// STT（语音输入）基于浏览器 Web Speech API 实现
import { useVoiceInput } from '../../composables/useSpeech'
import { useContextMenu } from '../../composables/useContextMenu'
import { isEmbeddingModelName } from '../../utils/model'
import { discreteDialog } from '../../utils/discrete'

const emit = defineEmits<{
  send: [content: string, images?: string[], attachments?: Attachment[]]
}>()
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const message = useMessage()
const inputText = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
// 使用 shallowRef：附件数组整体替换触发响应式，避免深度代理开销
const attachments = shallowRef<Attachment[]>([])
// 附件处理逻辑抽取到 composable：
// 包含 6 种文件类型的处理函数、文件大小校验、二进制检测、removeAttachment 等
const { processFileByType, removeAttachment } = useAttachments(attachments, message)

// 语音输入逻辑抽取到 useVoiceInput composable（基于架构优化）
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

// isEmbeddingOnly：当前模型是否为嵌入模型（只能做向量化/检索、不能聊天）。
// 两层信号，取其一即命中：
//   1. 后端权威信号 text_generation === false（llama-server 报告仅嵌入能力）
//   2. 模型名兜底匹配（isEmbeddingModelName），防止后端信号缺失时误选
const isEmbeddingOnly = computed(() => {
  if (capabilities.value.text_generation === false) return true
  return isEmbeddingModelName(settingsStore.currentModel)
})
// chatUnavailable：主界面进入对话的前提条件不满足时禁用输入框——
//   1. 模型目录为空（missingModels）：没有模型可加载，引导用户先去设置下载
//   2. 正在加载/切换模型（isSwitching）：模型未就绪，不可对话
//   3. 服务器未运行或模型未加载完成（serverStatus.running/model_ready）：未达到对话条件
// 生活类比：聊天舱的"话筒"在引擎没着车、或者还没挂上挡位之前是锁死的，
// 只有发动机运转（running）且挡位挂好（model_ready）时才能踩油门说话。
const chatUnavailable = computed(() => {
  if (settingsStore.missingModels) return true
  if (isSwitching.value) return true
  const s = settingsStore.serverStatus
  return !(s.running && s.model_ready)
})

// inputPlaceholder：输入框禁用时给出明确的中文原因提示，引导用户如何恢复对话。
const inputPlaceholder = computed(() => {
  const s = settingsStore.serverStatus
  // missingModels 且引擎未运行才是真正的"没装模型"；引擎已运行说明模型正在加载
  // （内置下载器装完模型后端会自动加载，无需重启），此时显示加载中而非下载引导
  if (settingsStore.missingModels && !s.running) return '尚未安装模型，请先到「设置」中下载模型'
  if (isSwitching.value) return '模型加载中，请稍候…'
  if (s.error) return '引擎异常，请查看顶部状态提示'
  if (!s.running) return '引擎启动中，请稍候…'
  if (!s.model_ready) return '模型加载中，请稍候…'
  if (isEmbeddingOnly.value) return '当前为嵌入模型，仅支持检索，请切换对话模型'
  return '向豆芽提问……'
})

const canSend = computed(
  () => !chatUnavailable.value && (inputText.value.trim() || attachments.value.length > 0)
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
    const warn = checkUploadCapability(fileType, capabilities.value)
    if (warn) {
      message.warning(warn)
      continue
    }
    processFileByType(fileType, file)
  }
}

// ===== 拖拽上传 =====
// 拖入文件时输入框浮现高亮遮罩，松开后逐个处理（类型识别 → 模型能力判断 → 添加）。
// dragDepth 深计数：防止鼠标在输入框内子元素间移动时 dragleave 误关闭遮罩，
// 只在真正拖出整个输入框后才隐藏。
const isDragging = ref(false)
let dragDepth = 0

function hasFiles(e: DragEvent): boolean {
  return !!e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')
}

function onDragEnter(e: DragEvent) {
  // 仅当拖入的是文件（而非纯文本/链接）才处理，避免误弹遮罩
  if (!hasFiles(e)) return
  e.preventDefault()
  dragDepth++
  if (dragDepth === 1) isDragging.value = true
}

function onDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) isDragging.value = false
}

// drop 分发：复用粘贴文件的同一套「识别类型 → 校验模型能力 → 添加附件」逻辑，
// 保证拖拽与粘贴、选择的行为完全一致
function onDrop(e: DragEvent) {
  const files = e.dataTransfer?.files
  dragDepth = 0
  isDragging.value = false
  if (!files || files.length === 0) return
  if (chatUnavailable.value) {
    message.warning('引擎尚未就绪，暂时无法添加附件')
    return
  }
  for (const file of Array.from(files)) {
    const fileType = detectFileType(file)
    if (!fileType) {
      message.warning(`不支持的文件类型: ${file.name}，可上传图片、音频、视频、PDF、Word 或文本`)
      continue
    }
    const warn = checkUploadCapability(fileType, capabilities.value)
    if (warn) {
      message.warning(warn)
      continue
    }
    processFileByType(fileType, file)
  }
}

// 上下文菜单逻辑抽取到 useContextMenu composable（基于架构优化）
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
// 6 个 process*File 函数 / removeAttachment 已抽取到 useAttachments composable

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

// checkCapability 抽取到 utils/attachments（与 ChatToolbar 共用），调用点改为 checkUploadCapability

// processFileByType 已抽取到 useAttachments composable

// MAX_*_SIZE 常量 / checkFileSize / readFileWithErrorHandling
// 已抽取到 useAttachments composable

// processImageFile / processAudioFile / processPdfFile / processDocxFile /
// processVideoFile / processTextFile / removeAttachment
// 已抽取到 useAttachments composable：
//   - processImageFile 保留独立实现（异步流水线）
//   - 其他 5 个通过 processFileCommon 高阶函数 + FILE_CONFIGS 表驱动统一处理
//   - 原约 130 行重复代码缩减为 composable 中的 1 个通用函数 + 1 个配置表

// initSpeechRecognition / toggleListening / startListening / stopListening
// 已抽取到 useVoiceInput composable（基于架构优化：ChatInput.vue 1789 行→拆分独立职责）

// ChatToolbar 选中文件后通过 emit 通知父组件处理（useAttachments composable 留在此处）
function onFileSelect(type: string, file: File) {
  processFileByType(type, file)
}

// showEmbeddingModelWarning：点击发送时若当前是嵌入模型，弹出明确提示并阻止发送，
// 引导用户切换到对话模型，避免再出现"logits"这类无法生成回复的报错。
function showEmbeddingModelWarning() {
  const modelName = settingsStore.currentModel || '当前模型'
  discreteDialog.warning({
    title: '当前模型不能聊天',
    content: `「${modelName}」是嵌入模型，只能做文本向量化/检索（如知识库问答），无法进行对话回复。\n\n请点击左上角模型名称，切换到对话类模型后再发送消息。`,
    positiveText: '知道了',
    style: { whiteSpace: 'pre-wrap' }
  })
}

function handleSend() {
  const text = inputText.value.trim()
  if (!text && attachments.value.length === 0) return
  if (chatStore.isAnyGenerating) return
  if (isSwitching.value) return
  if (chatUnavailable.value) return
  // 嵌入模型不能聊天：拦截发送并给出引导提示
  if (isEmbeddingOnly.value) {
    showEmbeddingModelWarning()
    return
  }

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
  /* 悬浮输入舱：舱体悬浮于消息流之上并与窗口边缘留缝，
   * 不设 border-top 分隔线；设置背景图时可从舱体四周透出 */
  padding: 6px 24px 16px;
  position: relative;
  z-index: 1;
}

.input-wrapper {
  /* 微信式铺满：不再居中限宽，直接吃满 .input-area 已有的 24px 水平内边距，
   * 左右边缘与上方消息列（同样 24px）保持对齐 */
  width: 100%;
  position: relative; /* 作为拖拽高亮遮罩（绝对定位）的定位上下文 */
}

/* 拖拽上传高亮遮罩：文件拖入输入框时浮现的"放下我"提示
 * pointer-events:none 不拦截拖放事件，事件穿透到下方 .input-wrapper 的 drop 处理 */
.drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  pointer-events: none;
  /* 书房风：半透明纸面 + 苔绿虚线描边，悬停感但贴合纸面原理，不做玻璃拟态 */
  background: color-mix(in srgb, var(--surface-panel) 96%, transparent);
  border: 1.5px dashed var(--accent-primary);
  border-radius: var(--border-radius-md);
  color: var(--accent-primary);
  text-align: center;
  animation: drop-overlay-in 0.18s ease;
}

@keyframes drop-overlay-in {
  from {
    opacity: 0;
    transform: scale(0.985);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.drop-icon {
  opacity: 0.9;
}

.drop-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: 0.2px;
}

.drop-sub {
  font-size: 12px;
  color: var(--text-muted);
}

.input-footer-reminder {
  margin-top: 6px;
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
  /* 书房风：panel 纸面 + hairline 细线描边，禁用玻璃拟态 */
  background: var(--surface-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-sm);
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
  border-radius: var(--border-radius-md);
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
  /* 书房风书写面：panel 纸面 + hairline 细线 + 单层低透明投影，
   * 禁用玻璃拟态；圆角收锐为 md 阶梯，与全屋利落感一致 */
  background: var(--surface-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-md);
  padding: 12px 14px;
  width: 100%;
  box-sizing: border-box;
  box-shadow: var(--shadow-sm);
  transition:
    border-color 0.2s,
    box-shadow 0.2s;
  position: relative;
}

/* 聚焦：苔绿细线落笔 + 环形描边提示"正在输入"，不加重投影 */
.chat-input-container:focus-within {
  border-color: var(--accent-primary);
  box-shadow:
    0 0 0 2px color-mix(in srgb, var(--accent-primary) 14%, transparent),
    var(--shadow-sm);
}

.ctx-menu {
  position: fixed;
  /* 阅读层表面：与附件菜单/会话菜单同语汇，背景图模式下自动适配 */
  background: var(--surface-panel);
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

/* 未达到对话条件时的禁用态：输入框变淡并禁止交互，静默提示用户"还不能说话" */
.chat-textarea:disabled {
  opacity: 0.55;
  cursor: not-allowed;
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
  /* 书房风：强调色底上以纸面底色作字色（浅色下米纸白、夜读下深褐） */
  color: var(--bg-primary);
  cursor: pointer;
  transition:
    transform 0.2s,
    box-shadow 0.2s,
    background-color 0.2s;
  flex-shrink: 0;
  position: relative;
  overflow: hidden;
  /* 至多一层低透明投影，不做彩色发光 */
  box-shadow: var(--shadow-sm);
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  box-shadow: none;
}

/* 悬浮反馈走背景色阶，不放大、不加发光 */
.send-btn:not(:disabled):hover {
  background: var(--accent-secondary);
  box-shadow: var(--shadow-sm);
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
  /* 原引用的 --accent-r-primary 为未定义死变量，背景整条失效；
   * 现改用真实令牌 --accent-danger（朱砂） */
  background: var(--accent-danger);
  color: var(--bg-primary);
  cursor: pointer;
  transition:
    transform 0.2s,
    box-shadow 0.2s,
    background-color 0.2s;
  flex-shrink: 0;
  position: relative;
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}

.stop-btn:hover {
  /* 朱砂加深一档，用 color-mix 调制，明暗主题通用 */
  background: color-mix(in srgb, var(--accent-danger) 86%, black);
  box-shadow: var(--shadow-sm);
}
</style>
