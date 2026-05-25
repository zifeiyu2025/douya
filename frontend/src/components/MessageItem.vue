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
      <div class="message-bubble" :class="isUser ? 'user-bubble' : 'ai-bubble'" ref="rootRef">
        <template v-if="isUser">
          <div v-if="parsedImages.length > 0" class="message-images">
            <img v-for="(src, idx) in parsedImages" :key="idx" :src="src" class="message-image" @click="previewImage(src)" />
          </div>
          <div v-if="nonImageAttachments.length > 0" class="message-attachments">
            <div v-for="(att, idx) in nonImageAttachments" :key="idx" class="attachment-tag" :class="'att-' + att.type">
              <svg v-if="att.type === 'audio'" class="att-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
              <svg v-else-if="att.type === 'pdf'" class="att-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
              <svg v-else class="att-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
              <span class="att-name">{{ att.name }}</span>
            </div>
          </div>
          <div v-if="message.content" class="user-text">{{ message.content }}</div>
        </template>
        <template v-else>
          <ThinkBlock v-if="message.thinking_content" :content="message.thinking_content" :duration="message.thinking_duration" />
          <SearchStatus v-if="hasSearchResults" :searching="false" :results="message.search_results" :default-expanded="false" />
          <div class="markdown-body" v-html="renderedContent" />
        </template>
      </div>

      <div class="msg-actions" :class="{ 'user-actions': isUser, 'ai-actions': !isUser }">
        <div class="action-row">
          <button class="action-btn" @click="copyContent" title="复制">
            <svg class="action-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
            </svg>
            <span class="action-label">复制</span>
          </button>
          <button v-if="!isUser && !chatStore.isGenerating && isLastAIMessage" class="action-btn" @click="regenerate" title="重新生成">
            <svg class="action-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="23 4 23 10 17 10"/>
              <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
            </svg>
            <span class="action-label">重新生成</span>
          </button>
          <button v-if="isUser" class="action-btn danger" @click="deleteMsg" title="删除">
            <svg class="action-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="3 6 5 6 21 6"/>
              <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              <line x1="10" y1="11" x2="10" y2="17"/>
              <line x1="14" y1="11" x2="14" y2="17"/>
            </svg>
            <span class="action-label">删除</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { useMessage, useDialog } from 'naive-ui'
import ThinkBlock from './ThinkBlock.vue'
import SearchStatus from './SearchStatus.vue'
import { renderMarkdown, renderMermaidInElement } from '../utils/markdown'
import { bindCodeCopyButtons } from '../utils/codeCopy'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import type { Message, AttachmentSummary } from '../services/wails'
import defaultUserAvatar from '../assets/images/user-avatar.svg'
import defaultAiAvatar from '../assets/images/appicon.png'

const props = defineProps<{ message: Message }>()
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const messageApi = useMessage()
const dialog = useDialog()

interface SearchResultItem {
  title: string
  url: string
  snippet: string
}

function linkCitations(html: string, searchResultsJson: string): string {
  let items: SearchResultItem[] = []
  try {
    const parsed = JSON.parse(searchResultsJson)
    if (Array.isArray(parsed)) items = parsed
    else if (parsed.results && Array.isArray(parsed.results)) items = parsed.results
  } catch {
    return html
  }
  if (items.length === 0) return html
  return html.replace(/\[(\d+)\]/g, (match, numStr) => {
    const idx = parseInt(numStr, 10) - 1
    if (idx >= 0 && idx < items.length && items[idx].url) {
      return `<a href="${items[idx].url}" target="_blank" rel="noopener noreferrer" class="citation-link">[${numStr}]</a>`
    }
    return match
  })
}

const isUser = computed(() => props.message.role === 'user')

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
const renderedContent = computed(() => {
  let html = renderMarkdown(props.message.content)
  if (hasSearchResults.value) {
    html = linkCitations(html, props.message.search_results)
  }
  return html
})
const isLastAIMessage = computed(() => {
  const aiMessages = chatStore.messages.filter(m => m.role === 'assistant')
  return aiMessages.length > 0 && aiMessages[aiMessages.length - 1].id === props.message.id
})

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

onMounted(() => {
    const el = rootRef.value
    if (el) {
        renderMermaidInElement(el)
        bindCodeCopyButtons(el)
    }
})

watch(renderedContent, async () => {
    await Promise.resolve()
    const el = rootRef.value
    if (el) {
        renderMermaidInElement(el)
        bindCodeCopyButtons(el)
    }
})

function copyContent() {
  navigator.clipboard.writeText(props.message.content)
  messageApi.success('已复制')
}

function previewImage(src: string) {
  const overlay = document.createElement('div')
  overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.8);z-index:9999;display:flex;align-items:center;justify-content:center;cursor:zoom-out'
  const img = document.createElement('img')
  img.src = src
  img.style.cssText = 'max-width:90%;max-height:90%;border-radius:8px;box-shadow:0 4px 20px rgba(0,0,0,0.5)'
  overlay.appendChild(img)
  overlay.addEventListener('click', () => overlay.remove())
  document.body.appendChild(overlay)
}

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
        chatStore.regenerateMessage(userMessageId, true)
      },
      onNegativeClick: () => {
        chatStore.regenerateMessage(userMessageId, false)
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
  animation: messageSlideIn 0.4s cubic-bezier(0.4, 0, 0.2, 1);
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

.message-item.user {
  flex-direction: row-reverse;
  margin-left: auto;
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
  padding: 18px 22px;
  border-radius: 20px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.06);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  box-sizing: border-box;
  position: relative;
}

.message-bubble:hover {
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.1);
}

.user-bubble {
  width: auto;
  max-width: 100%;
  min-width: 0;
  background: var(--bg-user-msg);
  color: var(--text-user-msg);
  border-bottom-right-radius: 6px;
  box-shadow: 0 8px 24px rgba(7, 193, 96, 0.15);
}

.user-bubble::after {
  content: '';
  position: absolute;
  right: -8px;
  top: 20px;
  border-width: 10px 0 10px 10px;
  border-style: solid;
  border-color: transparent transparent transparent var(--bg-user-msg);
}

.ai-bubble {
  width: auto;
  max-width: 100%;
  min-width: 0;
  background: var(--bg-ai-msg);
  color: var(--text-ai-msg);
  border-bottom-left-radius: 6px;
  border: 1px solid var(--border-color);
}

.ai-bubble::after {
  content: '';
  position: absolute;
  left: -8px;
  top: 20px;
  border-width: 10px 10px 10px 0;
  border-style: solid;
  border-color: transparent var(--bg-ai-msg) transparent transparent;
}

:global(.dark) .user-bubble {
  box-shadow: 0 8px 24px rgba(134, 230, 171, 0.2);
}

:global(.dark) .user-bubble::after {
  border-color: transparent transparent transparent var(--accent-primary);
}

.user-text {
  white-space: pre-wrap;
  line-height: 1.7;
  font-size: 15px;
  font-weight: 400;
}

.msg-actions {
  margin-top: 8px;
  min-height: 32px;
  opacity: 0;
  transition: opacity 0.25s ease, transform 0.25s ease;
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
  gap: 7px;
  padding: 7px 14px;
  border: none;
  border-radius: 10px;
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.18s ease;
  line-height: 1;
  white-space: nowrap;
}

.action-btn:hover {
  background: var(--bg-hover);
  color: var(--text-secondary);
  transform: translateY(-1px);
}

.action-btn:active {
  transform: translateY(0);
}

.action-btn.active {
  color: var(--accent-primary);
  background: rgba(7, 193, 96, 0.1);
}

.action-btn.danger:hover {
  color: var(--accent-danger);
  background: rgba(250, 81, 81, 0.12);
}

.action-icon {
  width: 17px;
  height: 17px;
  flex-shrink: 0;
}

.action-label {
  font-size: 13px;
  line-height: 1;
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
  border-radius: 12px;
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
  border-radius: 16px;
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
  border-radius: 0 14px 14px 0;
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
  border-radius: 12px;
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
  border-radius: 14px;
  margin: 14px 0;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.06);
}
</style>
