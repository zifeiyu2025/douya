<template>
  <div ref="messageListRef" class="message-list" :style="backgroundStyle">
    <Transition name="switch-overlay">
      <div v-if="isSwitching" class="switch-overlay">
        <div class="switch-overlay-content">
          <div class="switch-spinner"></div>
          <div class="switch-model-name">{{ switchingToModel }}</div>
          <div class="switch-progress-msg">正在切换模型...</div>
        </div>
      </div>
    </Transition>
    <div v-if="(!messages || messages.length === 0) && !isGenerating" class="message-list-empty">
      <img :src="logoImage" alt="Logo" class="message-list-logo" />
      <div class="message-list-empty-text">开始一段新对话</div>
    </div>
    <template v-else>
      <MessageItem
        v-for="msg in messages"
        :key="msg.id"
        :message="msg"
      />
      <div v-if="isGenerating" class="message-item">
        <div class="message-avatar ai-avatar">
          <img v-if="settingsStore.config.ai_avatar" :src="settingsStore.config.ai_avatar" alt="AI" />
          <img v-else :src="defaultAiAvatar" alt="AI" class="default-avatar" />
        </div>
        <div class="message-bubble-wrapper">
          <div class="message-bubble ai-bubble">
            <ThinkBlock v-if="thinkingContent" :content="thinkingContent" :default-expanded="true" :is-thinking="isThinking" :duration="thinkingDuration" />
            <SearchStatus v-if="isSearching" :searching="true" :results="''" :query="searchQuery" />
            <SearchStatus v-else-if="searchResults" :searching="false" :results="searchResults" :default-expanded="true" />
            <div v-if="streamingContent" class="markdown-body" v-html="renderedStreaming" />
            <n-spin v-else-if="!thinkingContent && !isSearching" size="small" />
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, ref, nextTick } from 'vue'
import { NSpin, useMessage } from 'naive-ui'
import MessageItem from './MessageItem.vue'
import ThinkBlock from './ThinkBlock.vue'
import SearchStatus from './SearchStatus.vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { renderMarkdown } from '../utils/markdown'
import { formatModelName } from '../utils/model'
import logoImage from '../assets/images/logo.png'
import defaultAiAvatar from '../assets/images/ai-avatar.svg'

const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const message = useMessage()

const backgroundStyle = computed(() => {
  if (settingsStore.config.chat_background) {
    return {
      '--chat-background': `url(${settingsStore.config.chat_background})`
    } as Record<string, string>
  }
  return {}
})

const messages = computed(() => chatStore.messages)
const isGenerating = computed(() => chatStore.isGenerating)
const streamingContent = computed(() => chatStore.streamingContent)
const thinkingContent = computed(() => chatStore.thinkingContent)
const searchResults = computed(() => chatStore.searchResults)
const isSearching = computed(() => chatStore.isSearching)
const isThinking = computed(() => chatStore.isThinking)
const thinkingDuration = computed(() => chatStore.thinkingDuration)
const searchQuery = computed(() => chatStore.searchQuery)

const isSwitching = computed(() => !!settingsStore.serverStatus.switching)
const switchingToModel = computed(() => {
  if (settingsStore.serverStatus.switching_to) {
    return formatModelName(settingsStore.serverStatus.switching_to).display
  }
  return ''
})

const renderedStreaming = computed(() => renderMarkdown(streamingContent.value))

const messageListRef = ref<HTMLElement | null>(null)

const scrollToBottom = () => {
  const el = messageListRef.value
  if (el) {
    el.scrollTop = el.scrollHeight
  }
}

const isNearBottom = () => {
  const el = messageListRef.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight < 100
}

watch(() => chatStore.messages?.length, () => {
  if (isNearBottom()) {
    nextTick(scrollToBottom)
  }
})

watch(() => chatStore.streamingContent, () => {
  if (isNearBottom()) {
    nextTick(scrollToBottom)
  }
})

watch(() => chatStore.thinkingContent, () => {
  if (isNearBottom()) {
    nextTick(scrollToBottom)
  }
})

watch(() => chatStore.lastError, (err) => {
    if (err) {
        message.error(err)
    }
})
</script>

<style scoped>
.message-list-logo {
  width: 140px;
  height: 140px;
  object-fit: contain;
  margin-bottom: 20px;
}

.message-avatar {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
  font-weight: 600;
  flex-shrink: 0;
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}

.message-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.message-avatar .default-avatar {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.ai-avatar {
  background: linear-gradient(135deg, #788c5d 0%, #6a7b52 100%);
  color: white;
}

:global(.dark) .ai-avatar {
  background: linear-gradient(135deg, #788c5d 0%, #6a7b52 100%);
  color: white;
}

.message-item {
  display: flex;
  gap: 12px;
  max-width: var(--msg-max-width);
  width: 100%;
  margin: 0 auto;
}

.message-bubble-wrapper {
  flex: 1;
  min-width: 0;
  max-width: 100%;
}

.message-bubble {
  padding: 16px 20px;
  border-radius: 16px;
  word-break: break-word;
  position: relative;
  box-shadow: var(--shadow-sm);
  box-sizing: border-box;
}

.ai-bubble {
  width: 100%;
  background: var(--bg-ai-msg);
  color: var(--text-ai-msg);
  border-top-left-radius: 4px;
  border: 1px solid var(--border-color);
}

.message-bubble :deep(.markdown-body) p {
  margin-bottom: 12px;
  line-height: 1.65;
}

.message-bubble :deep(.markdown-body) p:last-child {
  margin-bottom: 0;
}

.message-bubble :deep(.markdown-body) pre {
  background: var(--bg-code);
  border-radius: 12px;
  padding: 16px 18px;
  overflow-x: auto;
  margin: 14px 0;
}

.message-bubble :deep(.markdown-body) code {
  font-family: 'SF Mono', 'Fira Code', 'Consolas', 'Monaco', monospace;
  font-size: 14.5px;
}

.message-bubble :deep(.markdown-body) :not(pre) > code {
  background: rgba(0, 0, 0, 0.08);
  padding: 4px 10px;
  border-radius: 8px;
  font-size: 0.92em;
  color: var(--text-primary);
}

.message-bubble.ai-bubble a {
  color: var(--link-light);
  text-decoration: none;
  font-weight: 400;
  transition: color 0.2s;
}

.message-bubble.ai-bubble a:hover {
  color: var(--link-hover-light);
  text-decoration: underline;
}

:global(.dark) .message-bubble.ai-bubble a {
  color: var(--link-dark);
}

:global(.dark) .message-bubble.ai-bubble a:hover {
  color: var(--link-hover-dark);
}

.message-bubble :deep(.citation-link) {
  color: var(--link-light);
  text-decoration: none;
  font-weight: 500;
  font-size: 0.88em;
  cursor: pointer;
  transition: color 0.2s;
}

.message-bubble :deep(.citation-link:hover) {
  color: var(--link-hover-light);
  text-decoration: underline;
}

:global(.dark) .message-bubble :deep(.citation-link) {
  color: var(--link-dark);
}

:global(.dark) .message-bubble :deep(.citation-link:hover) {
  color: var(--link-hover-dark);
}

.message-list-empty {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  gap: 16px;
}

.message-list-empty-text {
  font-size: 16px;
  font-weight: 400;
  color: var(--text-secondary);
}

.switch-overlay {
  position: absolute;
  inset: 0;
  background: var(--bg-primary);
  opacity: 0.85;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}

.switch-overlay-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}

.switch-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid var(--border-color);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

.switch-model-name {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.switch-progress-msg {
  font-size: 13px;
  color: var(--text-secondary);
}

.switch-overlay-enter-active,
.switch-overlay-leave-active {
  transition: opacity 0.3s ease;
}

.switch-overlay-enter-from,
.switch-overlay-leave-to {
  opacity: 0;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
