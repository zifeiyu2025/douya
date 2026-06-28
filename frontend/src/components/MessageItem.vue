<template>
  <div class="message-item" :class="{ user: isUser }">
    <div class="message-avatar" :class="isUser ? 'user-avatar' : 'ai-avatar'">
      <img v-if="(isUser && settingsStore.config.user_avatar) || (!isUser && settingsStore.config.ai_avatar)" 
           :src="isUser ? settingsStore.config.user_avatar : settingsStore.config.ai_avatar" 
           :alt="isUser ? '用户' : 'AI'" />
      <img v-else :src="isUser ? defaultUserAvatar : defaultAiAvatar" 
           :alt="isUser ? '用户' : 'AI'"
           class="default-avatar" />
    </div>
    <div class="message-bubble-wrapper">
      <button
        v-if="selectionBtnVisible"
        class="selection-copy-btn"
        :style="{ left: selectionBtnX + 'px', top: selectionBtnY + 'px' }"
        @mousedown.prevent="handleCopySelection"
      >
        <AppIcon name="copy" :size="12" />
        <span>复制</span>
      </button>
      <div class="message-bubble" :class="isUser ? 'user-bubble' : 'ai-bubble'" ref="rootRef" @mouseup="handleMouseUp">
        <template v-if="isUser">
          <div v-if="parsedImages.length > 0" class="message-images">
            <img v-for="(src, idx) in parsedImages" :key="idx" :src="src" class="message-image" @click="previewImage(src)" />
          </div>
          <div v-if="nonImageAttachments.length > 0" class="message-attachments">
            <div v-for="(att, idx) in nonImageAttachments" :key="idx" class="attachment-tag" :class="'att-' + att.type">
              <AppIcon :name="attachmentIcon(att.type)" class="att-icon" :size="14" />
              <span class="att-name">{{ att.name }}</span>
            </div>
          </div>
          <div v-if="message.content" class="user-text">{{ message.content }}</div>
        </template>
        <template v-else>
          <ThinkBlock v-if="message.thinking_content" :content="message.thinking_content" :duration="message.thinking_duration" />
          <div class="markdown-body" v-html="renderedContent" />
          <SearchStatus v-if="hasSearchResults" :searching="false" :results="message.search_results" :default-expanded="false" />
        </template>
      </div>

      <div class="msg-actions" :class="{ 'user-actions': isUser, 'ai-actions': !isUser }">
        <div class="action-row">
          <span v-if="!isUser && tokensPerSecond > 0" class="token-speed">⚡ {{ tokensPerSecond }} t/s</span>
          <button class="action-btn" @click="copyContent" title="复制">
            <AppIcon name="copy" class="action-icon" :size="14" />
            <span class="action-label">复制</span>
          </button>
          <button v-if="!isUser && !chatStore.isGenerating && isLastAIMessage" class="action-btn" @click="regenerate" title="重新生成">
            <AppIcon name="regenerate" class="action-icon" :size="14" />
            <span class="action-label">重新生成</span>
          </button>
          <button v-if="isUser" class="action-btn danger" @click="deleteMsg" title="删除">
            <AppIcon name="trash" class="action-icon" :size="14" />
            <span class="action-label">删除</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useMessage, useDialog } from 'naive-ui'
import ThinkBlock from './ThinkBlock.vue'
import SearchStatus from './SearchStatus.vue'
 import { renderMarkdown, escapeHtml } from '../utils/markdown'
 import { useMermaid } from '../composables/useMermaid'
 import { setupCodeCopyDelegation } from '../utils/codeCopy'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import type { Message, AttachmentSummary } from '../services/wails'
import AppIcon from './ui/AppIcon.vue'
import defaultUserAvatar from '../assets/images/user-avatar.svg'
import defaultAiAvatar from '../assets/images/appicon.png'

const props = defineProps<{ message: Message }>()
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const messageApi = useMessage()
const dialog = useDialog()

const ATTACHMENT_ICON_MAP: Record<string, 'audio' | 'video' | 'pdf' | 'file' | 'image'> = {
    audio: 'audio',
    video: 'video',
    pdf: 'pdf',
    text: 'file',
    image: 'image',
}

function attachmentIcon(type: string): 'audio' | 'video' | 'pdf' | 'file' | 'image' {
    return ATTACHMENT_ICON_MAP[type] || 'file'
}

const isUser = computed(() => props.message.role === 'user')

const isLastAIMessage = computed(() => {
    return chatStore.lastAIMessageId === props.message.id
})

const tokensPerSecond = computed(() => {
    // 流式中：使用实时速度数据
    if (chatStore.isGenerating && isLastAIMessage.value && chatStore.tokensPerSecond > 0) {
        return Math.round(chatStore.tokensPerSecond * 10) / 10
    }
    // 流式后：使用消息中保存的速度数据
    const tps = props.message.tokens_per_second
    if (!tps || tps <= 0) return 0
    return Math.round(tps * 10) / 10
})

const hasSearchResults = computed(() => {
    if (!props.message.search_results) return false
    if (props.message.search_results === '[]') return false
    return props.message.search_results.length > 0
})

const parsedImages = computed(() => {
  if (!props.message.images) return []
  try {
    const arr = JSON.parse(props.message.images)
    if (Array.isArray(arr)) return arr
  } catch {}
  return []
})

const nonImageAttachments = computed<AttachmentSummary[]>(() => {
  if (!props.message.attachments || !Array.isArray(props.message.attachments)) return []
  return props.message.attachments.filter(a => a.type !== 'image')
})

// remark 是异步的，使用 ref + watch 模式
const renderedContent = ref('')

watch(() => props.message.content, async (newContent) => {
  if (!newContent) {
    renderedContent.value = ''
    return
  }
  try {
    let html = await renderMarkdown(newContent)
    renderedContent.value = html
  } catch (_) {
    // 渲染失败时转义后作为纯文本显示，避免直接赋值原始未消毒内容到 v-html（XSS 防护）
    renderedContent.value = escapeHtml(newContent)
  }
}, { immediate: true })

const findPreviousUserMessage = () => {
  const index = chatStore.messages.findIndex(m => m.id === props.message.id)
  if (index > 0) {
    for (let i = index - 1; i >= 0; i--) {
      if (chatStore.messages[i].role === 'user') {
        return chatStore.messages[i].id
      }
    }
  }
  return null
}

const rootRef = ref<HTMLElement>()
let cleanupCodeCopyDelegation: (() => void) | null = null

// 使用 IntersectionObserver 懒加载 mermaid（仅进入视口才下载 2.84MB chunk）
const { refreshObservation } = useMermaid(rootRef)

onMounted(() => {
    const el = rootRef.value
    if (el) {
        cleanupCodeCopyDelegation = setupCodeCopyDelegation(el)
    }
    document.addEventListener('mousedown', handleDocumentMouseDown)
    document.addEventListener('scroll', hideSelectionBtn, true)
})

watch(renderedContent, async () => {
    await Promise.resolve()
    const el = rootRef.value
    if (el) {
        // 内容更新后重新观察新出现的 .mermaid 元素
        refreshObservation()
    }
})

async function copyContent() {
  try {
    const markdownEl = rootRef.value?.querySelector('.markdown-body') as HTMLElement | null
    if (markdownEl) {
      // 克隆 DOM，移除代码头部（语言标签和复制按钮），只保留纯正文
      const clone = markdownEl.cloneNode(true) as HTMLElement
      clone.querySelectorAll('.code-header').forEach(el => el.remove())
      const htmlBlob = new Blob([clone.innerHTML], { type: 'text/html' })
      const textBlob = new Blob([clone.innerText], { type: 'text/plain' })
      await navigator.clipboard.write([
        new ClipboardItem({
          'text/html': htmlBlob,
          'text/plain': textBlob,
        })
      ])
    } else {
      await navigator.clipboard.writeText(props.message.content)
    }
    messageApi.success('已复制')
  } catch {
    try {
      await navigator.clipboard.writeText(props.message.content)
      messageApi.success('已复制')
    } catch {
      messageApi.error('复制失败')
    }
  }
}

const selectionBtnVisible = ref(false)
const selectionBtnX = ref(0)
const selectionBtnY = ref(0)

function handleMouseUp() {
  if (isUser.value) {
    selectionBtnVisible.value = false
    return
  }
  if (chatStore.isGenerating) {
    selectionBtnVisible.value = false
    return
  }
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0) {
    selectionBtnVisible.value = false
    return
  }
  const text = sel.toString().trim()
  if (!text) {
    selectionBtnVisible.value = false
    return
  }
  const range = sel.getRangeAt(0)
  const bubbleEl = rootRef.value
  if (!bubbleEl || !bubbleEl.contains(range.commonAncestorContainer)) {
    selectionBtnVisible.value = false
    return
  }
  const rect = range.getBoundingClientRect()
  const btnWidth = 70
  const btnHeight = 32
  let x = rect.left + rect.width / 2 - btnWidth / 2
  let y = rect.top - btnHeight - 6
  if (x < 4) x = 4
  if (x + btnWidth > window.innerWidth - 4) x = window.innerWidth - btnWidth - 4
  if (y < 4) y = rect.bottom + 6
  selectionBtnX.value = x
  selectionBtnY.value = y
  selectionBtnVisible.value = true
}

async function handleCopySelection() {
  const sel = window.getSelection()
  if (!sel) return
  const text = sel.toString()
  if (!text) {
    selectionBtnVisible.value = false
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    messageApi.success('已复制')
  } catch {
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
      messageApi.success('已复制')
    } catch {
      messageApi.error('复制失败')
    }
  }
  selectionBtnVisible.value = false
  sel.removeAllRanges()
}

function hideSelectionBtn() {
  selectionBtnVisible.value = false
}

function handleDocumentMouseDown(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.selection-copy-btn')) {
    selectionBtnVisible.value = false
  }
}

// 组件级变量：跟踪当前打开的图片预览清理函数（用于组件卸载时清理，避免内存泄漏）
let activePreviewCleanup: (() => void) | null = null

function previewImage(src: string) {
  const overlay = document.createElement('div')
  overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.8);z-index:9999;display:flex;align-items:center;justify-content:center;cursor:zoom-out'
  const img = document.createElement('img')
  img.src = src
  img.style.cssText = 'max-width:90%;max-height:90%;border-radius:8px;box-shadow:0 4px 20px rgba(0,0,0,0.5)'
  overlay.appendChild(img)
  const close = () => {
    overlay.remove()
    document.body.style.overflow = ''
    document.removeEventListener('keydown', onKey)
    activePreviewCleanup = null
  }
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') close()
  }
  overlay.addEventListener('click', close)
  document.addEventListener('keydown', onKey)
  document.body.style.overflow = 'hidden'
  document.body.appendChild(overlay)
  // 记录清理函数，供 onUnmounted 调用
  activePreviewCleanup = () => {
    document.removeEventListener('keydown', onKey)
    overlay.remove()
    document.body.style.overflow = ''
  }
}

// 组件卸载时清理可能残留的图片预览 overlay 和 keydown 监听器，避免内存泄漏
onUnmounted(() => {
  if (cleanupCodeCopyDelegation) {
    cleanupCodeCopyDelegation()
    cleanupCodeCopyDelegation = null
  }
  if (activePreviewCleanup) {
    activePreviewCleanup()
    activePreviewCleanup = null
  }
  document.removeEventListener('mousedown', handleDocumentMouseDown)
  document.removeEventListener('scroll', hideSelectionBtn, true)
})

async function deleteMsg() {
  await chatStore.deleteMessage(props.message.id)
}

function regenerate() {
  const userMessageId = findPreviousUserMessage()
  if (userMessageId) {
    dialog.create({
      title: '重新生成',
      content: '是否启用联网搜索？',
      positiveText: '联网搜索',
      negativeText: '直接生成',
      onPositiveClick: () => {
        chatStore.regenerateMessage(userMessageId, 'on')
      },
      onNegativeClick: () => {
        chatStore.regenerateMessage(userMessageId, 'off')
      },
    })
  }
}

</script>

<style scoped>
.message-item {
  display: flex;
  gap: 14px;
  margin-bottom: 26px;
  position: relative;
  width: 100%;
  max-width: var(--msg-max-width);
  align-items: flex-start;
}

@keyframes messageSlideIn {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* messageSlideIn 动画只应用于用户消息，避免 AI 流式消息动画干扰 */
.message-item.user {
  flex-direction: row-reverse;
  margin-left: auto;
  animation: messageSlideIn 0.4s cubic-bezier(0.23, 1, 0.32, 1);
}

.message-item:not(.user) {
  margin-right: auto;
}

.message-avatar {
  width: 42px;
  height: 42px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
  font-weight: 600;
  flex-shrink: 0;
  overflow: hidden;
  background: transparent;
}

.message-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 50%;
  display: block;
}

.user-avatar {
}

.ai-avatar {
}

.message-bubble-wrapper {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.message-item.user .message-bubble-wrapper {
  max-width: 75%;
  align-items: flex-end;
}

.message-item:not(.user) .message-bubble-wrapper {
  max-width: 100%;
  align-items: flex-start;
}

.message-bubble {
  padding: 12px 18px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  transition: box-shadow 0.2s ease, transform 0.2s ease;
  box-sizing: border-box;
  position: relative;
  line-height: 1.65;
}

.message-bubble:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.user-bubble {
  width: auto;
  max-width: 100%;
  min-width: 0;
  background: var(--bg-user-msg);
  color: var(--text-user-msg);
  border-radius: var(--border-radius-xl) var(--border-radius-xl) var(--border-radius-sm) var(--border-radius-xl);
}

.ai-bubble {
  width: auto;
  max-width: 100%;
  min-width: 0;
  background: var(--bg-ai-msg);
  color: var(--text-ai-msg);
  border-radius: var(--border-radius-xl) var(--border-radius-xl) var(--border-radius-xl) var(--border-radius-sm);
  border: 1px solid var(--border-color);
}

:global(.dark) .user-bubble {
  background: var(--bg-user-msg);
}

.user-text {
  white-space: pre-wrap;
  line-height: 1.7;
  font-size: 15px;
  font-weight: 400;
}

.msg-actions {
  margin-top: 5px;
  min-height: 30px;
  opacity: 0;
  transition: opacity 0.22s ease, transform 0.22s ease;
  transform: translateY(-4px);
}

.message-item:hover .msg-actions {
  opacity: 1;
  transform: translateY(0);
}

.user-actions {
  display: flex;
  justify-content: flex-end;
}

.ai-actions {
  display: flex;
  justify-content: flex-start;
}

.action-row {
  display: flex;
  gap: 6px;
  align-items: center;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  line-height: 1;
  white-space: nowrap;
}

.action-btn:hover {
  background: var(--bg-hover);
  color: var(--text-secondary);
}

.action-btn.danger:hover {
  color: var(--accent-danger);
  background: rgba(250, 81, 81, 0.1);
}

.action-icon {
  width: 15px;
  height: 15px;
  flex-shrink: 0;
}

.action-label {
  font-size: 12.5px;
  line-height: 1;
}

.token-speed {
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1;
  padding: 6px 8px;
  white-space: nowrap;
  user-select: none;
}

:deep(.citation-link) {
  color: var(--link-light);
  text-decoration: none;
  font-weight: 500;
  font-size: 0.88em;
  cursor: pointer;
  transition: color 0.2s;
}

:deep(.citation-link:hover) {
  color: var(--link-hover-light);
  text-decoration: underline;
}

:global(.dark) :deep(.citation-link) {
  color: var(--link-dark);
}

:global(.dark) :deep(.citation-link:hover) {
  color: var(--link-hover-dark);
}

.message-images {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.message-attachments {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.attachment-tag {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: var(--border-radius-md);
  background: rgba(0, 0, 0, 0.05);
  font-size: 13px;
  font-weight: 500;
  line-height: 1;
  max-width: 240px;
  overflow: hidden;
  transition: all 0.2s ease;
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.attachment-tag:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

:global(.dark) .attachment-tag {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.1);
}

.attachment-tag.att-audio {
  background: rgba(52, 152, 219, 0.1);
  border-color: rgba(52, 152, 219, 0.2);
}

:global(.dark) .attachment-tag.att-audio {
  background: rgba(52, 152, 219, 0.15);
  border-color: rgba(52, 152, 219, 0.25);
}

.attachment-tag.att-video {
  background: rgba(139, 92, 246, 0.1);
  border-color: rgba(139, 92, 246, 0.3);
}

:global(.dark) .attachment-tag.att-video {
  background: rgba(139, 92, 246, 0.15);
  border-color: rgba(139, 92, 246, 0.25);
}

.attachment-tag.att-pdf {
  background: rgba(231, 76, 60, 0.08);
  border-color: rgba(231, 76, 60, 0.18);
}

:global(.dark) .attachment-tag.att-pdf {
  background: rgba(231, 76, 60, 0.12);
  border-color: rgba(231, 76, 60, 0.22);
}

.att-icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.att-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.message-image {
  max-width: 260px;
  max-height: 260px;
  border-radius: var(--border-radius-lg);
  cursor: zoom-in;
  object-fit: cover;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  border: 1px solid var(--border-color);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.message-image:hover {
  transform: scale(1.03);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
}

.message-bubble :deep(.markdown-body) blockquote {
  border-left: 4px solid var(--accent-primary);
  padding-left: 18px;
  margin: 16px 0;
  color: var(--text-secondary);
  background: rgba(7, 193, 96, 0.05);
  padding-top: 12px;
  padding-bottom: 12px;
  padding-right: 16px;
  border-radius: 0 var(--border-radius-md) var(--border-radius-md) 0;
}

:global(.dark) .message-bubble :deep(.markdown-body) blockquote {
  border-left-color: var(--accent-secondary);
  color: var(--text-secondary);
  background: rgba(134, 230, 171, 0.08);
}

.message-bubble :deep(.markdown-body) table {
  border-collapse: collapse;
  width: 100%;
  margin: 16px 0;
  font-size: 14.5px;
  border-radius: var(--border-radius-md);
  overflow: hidden;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.message-bubble :deep(.markdown-body) th,
.message-bubble :deep(.markdown-body) td {
  border: 1px solid var(--border-color);
  padding: 14px 18px;
  text-align: left;
}

.message-bubble :deep(.markdown-body) th {
  background: var(--bg-hover);
  font-weight: 600;
}

.message-bubble :deep(.markdown-body) ul,
.message-bubble :deep(.markdown-body) ol {
  padding-left: 28px;
  margin: 14px 0;
}

.message-bubble :deep(.markdown-body) li {
  margin: 10px 0;
  line-height: 1.7;
}

.message-bubble :deep(.markdown-body) img {
  max-width: 100%;
  border-radius: var(--border-radius-md);
  margin: 14px 0;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.06);
}

.selection-copy-btn {
  position: fixed;
  z-index: 1000;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-sm);
  box-shadow: var(--shadow-md);
  color: var(--text-primary);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.15s;
  animation: selectionBtnIn 0.12s ease;
}

@keyframes selectionBtnIn {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}

.selection-copy-btn:hover {
  background: var(--bg-hover);
  border-color: var(--accent-primary);
  color: var(--accent-primary);
}
</style>

<style>
.has-background .ai-bubble {
  background: color-mix(in srgb, var(--bg-ai-msg) 88%, transparent);
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
}

.has-background .user-bubble {
  background: color-mix(in srgb, var(--bg-user-msg) 88%, transparent);
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
}
</style>
