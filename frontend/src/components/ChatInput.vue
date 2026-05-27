<template>
  <div class="input-area">
    <div class="input-wrapper">
      <div v-if="attachments.length > 0" class="attachment-preview-bar">
        <div
          v-for="(att, idx) in attachments"
          :key="idx"
          class="attachment-preview-item"
          :class="att.type"
        >
          <div v-if="att.type === 'image'" class="att-thumb image-thumb">
            <img :src="att.data" alt="" />
          </div>
          <div v-else class="att-thumb file-thumb">
            <svg v-if="att.type === 'audio'" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
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
                <button class="search-btn" :class="{ active: searchEnabled }" @click="settingsStore.toggleSearch()" :title="searchEnabled ? '联网搜索已开启' : '开启联网搜索'">
                  <n-icon size="22"><GlobeOutline /></n-icon>
                </button>
                <button class="search-btn rag-btn" :class="{ active: ragEnabled }" @click="toggleRAG" :title="ragEnabled ? '知识库检索已开启' : '开启知识库检索'">
                  <n-icon size="22"><BookOutline /></n-icon>
                </button>
                <div class="attach-wrapper">
                  <button class="attach-btn" @click="toggleAttachMenu" :disabled="isSwitching" title="添加附件">
                    <n-icon size="22"><AttachOutline /></n-icon>
                  </button>
            <div v-if="showAttachMenu" class="attach-menu">
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.image_input }"
                @click="triggerFileUpload('image')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>
                <span>图片</span>
                <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
                <span v-else-if="!capabilities.image_input" class="unsupported-tag">不支持</span>
              </button>
              <button
                class="attach-menu-item"
                :class="{ disabled: !capabilities.audio_input }"
                @click="triggerFileUpload('audio')"
              >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
                <span>音频</span>
                <span v-if="!capabilities.mmproj_loaded" class="unsupported-tag">未加载mmproj</span>
                <span v-else-if="!capabilities.audio_input" class="unsupported-tag">不支持</span>
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
        </div>

        <div class="the-input">
          <textarea
            ref="textareaRef"
            v-model="inputText"
            placeholder="给DouYa发送消息....."
            @keydown="handleKeydown"
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
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { NIcon } from 'naive-ui'
import { GlobeOutline, AttachOutline, BookOutline } from '@vicons/ionicons5'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { wails } from '../services/wails'
import type { Attachment } from '../services/wails'

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
const inputText = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const attachments = ref<Attachment[]>([])
const showAttachMenu = ref(false)
const pendingUploadType = ref<string>('image')

const isListening = ref(false)
const voiceInterimText = ref('')
let recognition: SpeechRecognitionInstance | null = null
let voiceFinalBuffer = ''

const speechSupported = computed(() => {
  return !!(window.SpeechRecognition || window.webkitSpeechRecognition)
})

const searchEnabled = computed(() => settingsStore.searchEnabled)
const ragEnabled = ref(true)
const capabilities = computed(() => settingsStore.modelCapabilities)
const isSwitching = computed(() => settingsStore.isModelSwitching)
const canSend = computed(() => !isSwitching.value && (inputText.value.trim() || attachments.value.length > 0))

async function toggleRAG() {
  const newVal = !ragEnabled.value
  try {
    await wails.setRAGEnabled(newVal)
    ragEnabled.value = newVal
  } catch (e) {
    console.warn('toggle RAG failed:', e)
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
    case 'pdf': return 'PDF'
    default: return type
  }
}

const IMAGE_ACCEPT = '.jpg,.jpeg,.png,.gif,.webp,.bmp,.svg'
const AUDIO_ACCEPT = '.wav,.mp3,.ogg,.flac,.aac,.m4a,.wma'
const TEXT_ACCEPT = '.txt,.md,.csv,.json,.xml,.html,.css,.js,.ts,.py,.go,.java,.c,.cpp,.h,.rs,.sh,.yaml,.yml,.toml,.ini,.cfg,.log,.sql'
const PDF_ACCEPT = '.pdf'

function getAcceptForType(type: string): string {
  switch (type) {
    case 'image': return IMAGE_ACCEPT
    case 'audio': return AUDIO_ACCEPT
    case 'text': return TEXT_ACCEPT
    case 'pdf': return PDF_ACCEPT
    default: return ''
  }
}

function triggerFileUpload(type: string) {
  if (type === 'image' && !capabilities.value.image_input) return
  if (type === 'audio' && !capabilities.value.audio_input) return
  if ((type === 'text' || type === 'pdf') && !capabilities.value.text_input) return

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
  for (const file of Array.from(input.files)) {
    if (type === 'image') {
      processImageFile(file)
    } else if (type === 'audio') {
      processAudioFile(file)
    } else if (type === 'pdf') {
      processPdfFile(file)
    } else {
      processTextFile(file)
    }
  }
  input.value = ''
}

function processImageFile(file: File) {
  if (!file.type.startsWith('image/')) return
  if (attachments.value.filter(a => a.type === 'image').length >= 4) return
  const reader = new FileReader()
  reader.onload = () => {
    const dataUrl = reader.result as string
    attachments.value.push({
      type: 'image',
      name: file.name,
      mime_type: file.type,
      data: dataUrl,
    })
  }
  reader.readAsDataURL(file)
}

function processAudioFile(file: File) {
  const ext = file.name.split('.').pop()?.toLowerCase() || 'wav'
  const reader = new FileReader()
  reader.onload = () => {
    const base64 = (reader.result as string).split(',')[1]
    attachments.value.push({
      type: 'audio',
      name: file.name,
      mime_type: file.type || `audio/${ext}`,
      data: base64,
      format: ext,
    })
  }
  reader.readAsDataURL(file)
}

function processPdfFile(file: File) {
  const reader = new FileReader()
  reader.onload = () => {
    const base64 = (reader.result as string).split(',')[1]
    attachments.value.push({
      type: 'pdf',
      name: file.name,
      mime_type: 'application/pdf',
      data: base64,
    })
  }
  reader.readAsDataURL(file)
}

function processTextFile(file: File) {
  const reader = new FileReader()
  reader.onload = () => {
    const text = reader.result as string
    attachments.value.push({
      type: 'text',
      name: file.name,
      mime_type: file.type || 'text/plain',
      data: text,
    })
  }
  reader.readAsText(file)
}

function removeAttachment(idx: number) {
  attachments.value.splice(idx, 1)
}

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
  if (speechSupported.value) {
    initSpeechRecognition()
  }
  wails.isRAGEnabled().then(v => { ragEnabled.value = v }).catch(() => {})
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (recognition) {
    isListening.value = false
    recognition.abort()
    recognition = null
  }
})
</script>

<style scoped>
.input-area {
  padding: 16px 24px 20px;
  border-top: 1px solid var(--border-color);
  background: transparent;
  position: relative;
  z-index: 1;
}

.input-wrapper {
  max-width: var(--msg-max-width);
  margin: 0 auto;
  width: 100%;
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
  padding: 8px 12px;
  border-radius: 12px;
  border: 1px solid var(--border-color);
  background: var(--bg-secondary);
  max-width: 220px;
  transition: all 0.2s;
}

.attachment-preview-item:hover {
  border-color: var(--accent-primary);
  box-shadow: var(--shadow-sm);
}

.attachment-preview-item.image {
  padding: 6px;
  max-width: none;
}

.att-thumb {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.att-thumb.image-thumb {
  width: 52px;
  height: 52px;
  border-radius: 10px;
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
  border-radius: 10px;
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
  padding: 8px 14px;
  margin: 0 0 4px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 12px;
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
  background: rgba(239, 68, 68, 0.15);
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
  background: rgba(239, 68, 68, 0.1);
  border-color: var(--accent-danger);
  color: var(--accent-danger);
}

.chat-input-container {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 16px;
  padding: 12px 14px;
  width: 100%;
  box-sizing: border-box;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.chat-input-container:focus-within {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-tertiary);
}

.left-buttons {
  flex-shrink: 0;
  display: flex;
  gap: 4px;
}

.search-btn, .attach-btn, .voice-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 12px;
  border: none;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s;
  color: var(--text-secondary);
}

.attach-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.search-btn:hover, .attach-btn:hover:not(:disabled), .voice-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.search-btn.active {
  color: var(--accent-primary);
  background: var(--accent-tertiary);
}

.voice-btn.active {
  color: var(--accent-danger);
  background: rgba(239, 68, 68, 0.1);
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
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: 12px;
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
  border-radius: 6px;
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
  border-radius: 12px;
  border: none;
  background: var(--accent-primary);
  color: white;
  cursor: pointer;
  transition: all 0.2s;
  flex-shrink: 0;
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.send-btn:not(:disabled):hover {
  transform: scale(1.05);
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.3);
}

.stop-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 12px;
  border: none;
  background: var(--accent-danger);
  color: white;
  cursor: pointer;
  transition: all 0.2s;
  flex-shrink: 0;
}

.stop-btn:hover {
  transform: scale(1.05);
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
}
</style>
