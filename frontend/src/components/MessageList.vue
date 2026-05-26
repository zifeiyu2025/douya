<template>
  <div ref="messageListRef" class="message-list" :style="backgroundStyle">
    <Transition name="switch-overlay">
      <div v-if="isSwitching" class="switch-overlay">
        <div class="switch-overlay-content">
          <div class="switch-spinner"></div>
          <div class="switch-model-name">{{ switchingToModel }}</div>
          <div class="switch-progress-msg">
            {{ getSwitchProgressText() }}
          </div>
          <div class="switch-stage-indicator">
            <div 
              v-for="(stage, idx) in switchStages" 
              :key="idx"
              :class="['stage-item', { 
                'active': getCurrentStageIndex() >= idx,
                'completed': getCurrentStageIndex() > idx 
              }]"
            >
              <span class="stage-dot"></span>
              <span class="stage-label">{{ stage }}</span>
            </div>
          </div>
        </div>
      </div>
    </Transition>
    <div v-if="(!messages || messages.length === 0) && !isGenerating" class="message-list-empty">
      <div class="welcome-container">
        <div class="welcome-title">欢迎使用 Douya</div>
        <div class="welcome-subtitle">智能对话助手 · 本地部署 · 隐私优先</div>
        
        <div class="quick-actions">
          <div v-for="action in quickActions" :key="action.id" class="action-card" @click="handleQuickAction(action)">
            <div class="action-icon">{{ action.icon }}</div>
            <div class="action-content">
              <div class="action-title">{{ action.title }}</div>
              <div class="action-desc">{{ action.desc }}</div>
            </div>
          </div>
        </div>
        
        <div class="tips-section">
          <div class="tips-title">💡 提示</div>
          <div class="tips-grid">
            <div v-for="tip in tips" :key="tip.id" class="tip-item">{{ tip.text }}</div>
          </div>
        </div>
      </div>
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
import { computed, watch, ref, nextTick, onMounted } from 'vue'
import { NSpin, useMessage } from 'naive-ui'
import MessageItem from './MessageItem.vue'
import ThinkBlock from './ThinkBlock.vue'
import SearchStatus from './SearchStatus.vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { renderMarkdownStreaming } from '../utils/markdown'
import { formatModelName } from '../utils/model'
import { bindCodeCopyButtons } from '../utils/codeCopy'
import defaultAiAvatar from '../assets/images/appicon.png'

const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const message = useMessage()

const quickActions = [
  { id: 1, icon: '✍️', title: '写点什么', desc: '帮我写一段代码', prompt: '帮我写一段示例代码' },
  { id: 2, icon: '💡', title: '头脑风暴', desc: '创意灵感激发', prompt: '帮我做一些头脑风暴，探索新想法' },
  { id: 3, icon: '📚', title: '知识问答', desc: '探索任何话题', prompt: '我想了解一些有趣的知识' },
  { id: 4, icon: '🎨', title: '创意写作', desc: '故事、诗歌、文案', prompt: '帮我创作一些有趣的内容' },
  { id: 5, icon: '🔧', title: '问题解决', desc: '分析和解决问题', prompt: '我有一个问题需要解决' },
  { id: 6, icon: '🤖', title: '自由对话', desc: '随便聊聊什么', prompt: '你好，我们来随便聊聊吧' }
]

const tips = [
  { id: 1, text: '你可以发送图片进行视觉对话' },
  { id: 2, text: '支持语音输入和音频理解' },
  { id: 3, text: '长按消息可以复制或引用' },
  { id: 4, text: '在设置中可以自定义背景图片' }
]

function handleQuickAction(action: any) {
  chatStore.sendMessage(action.prompt, settingsStore.searchEnabled)
}

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

const isSwitching = computed(() => settingsStore.isModelSwitching)
const switchingToModel = computed(() => {
  if (settingsStore.serverStatus.switching_to) {
    return formatModelName(settingsStore.serverStatus.switching_to).display
  }
  return ''
})

const switchStages = ['准备切换', '卸载旧模型', '加载新模型', '初始化完成']

function getCurrentStageIndex(): number {
  const stage = settingsStore.switchProgress.stage
  switch (stage) {
    case 'unloading':
      return 1
    case 'loading':
      return 2
    case 'done':
      return 3
    default:
      return 0
  }
}

function getSwitchProgressText(): string {
  const stage = settingsStore.switchProgress.stage
  const prevModel = settingsStore.previousModelBeforeSwitch
    ? formatModelName(settingsStore.previousModelBeforeSwitch).display
    : ''
  const newModel = switchingToModel.value

  switch (stage) {
    case 'unloading':
      return prevModel ? `正在卸载模型 ${prevModel}...` : '正在卸载旧模型...'
    case 'loading':
      return `正在加载模型 ${newModel}...`
    case 'waiting':
      return '等待服务器就绪...'
    case 'done':
      return '模型初始化完成！'
    case 'failed':
      return '模型加载失败'
    default:
      return '正在切换模型...'
  }
}

const renderedStreaming = computed(() => renderMarkdownStreaming(streamingContent.value))

const messageListRef = ref<HTMLElement | null>(null)

onMounted(() => {
    const el = messageListRef.value
    if (el) bindCodeCopyButtons(el)
})

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

// 监听消息变化 - 新增消息时总是滚动到底部
watch(() => chatStore.messages?.length, (newLen, oldLen) => {
  if (newLen > oldLen) {
    // 消息数量增加，说明有新消息，总是滚动到底部
    nextTick(scrollToBottom)
  } else if (isNearBottom()) {
    // 其他情况下只有在底部时才滚动
    nextTick(scrollToBottom)
  }
})

watch(() => chatStore.streamingContent, () => {
  if (isNearBottom()) {
    nextTick(() => {
      scrollToBottom()
      const el = messageListRef.value
      if (el) bindCodeCopyButtons(el)
    })
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
.message-avatar {
  width: 44px;
  height: 44px;
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

.ai-avatar {
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
  box-shadow: var(--shadow-sm);
  box-sizing: border-box;
}

.ai-bubble {
  width: 100%;
  border-top-left-radius: 4px;
  border: 1px solid var(--border-color);
}

.message-list-empty {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  overflow-y: auto;
  padding: 40px 20px;
}

.welcome-container {
  width: 100%;
  max-width: 800px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 24px;
}

.welcome-title {
  font-size: 32px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, var(--accent-primary), var(--accent-secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.welcome-subtitle {
  font-size: 15px;
  color: var(--text-secondary);
  letter-spacing: 0.3px;
}

.quick-actions {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  width: 100%;
  margin-top: 8px;
}

.action-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  background: var(--bg-primary);
  border-radius: 16px;
  border: 1px solid var(--border-color);
  cursor: pointer;
  transition: all 0.25s ease;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.action-card:hover {
  transform: translateY(-3px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
  border-color: var(--accent-primary);
}

.action-card:active {
  transform: translateY(-1px);
}

.action-icon {
  font-size: 28px;
  flex-shrink: 0;
}

.action-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.action-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}

.action-desc {
  font-size: 12px;
  color: var(--text-muted);
}

.tips-section {
  width: 100%;
  margin-top: 8px;
}

.tips-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 12px;
  text-align: center;
}

.tips-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}

.tip-item {
  font-size: 13px;
  color: var(--text-secondary);
  padding: 12px 16px;
  background: var(--bg-secondary);
  border-radius: 12px;
  text-align: center;
  border: 1px solid var(--border-color);
  transition: all 0.2s ease;
}

.tip-item:hover {
  border-color: var(--accent-primary);
  background: var(--bg-primary);
}

/* 响应式适配 */
@media (max-width: 700px) {
  .quick-actions {
    grid-template-columns: repeat(2, 1fr);
  }
  
  .tips-grid {
    grid-template-columns: 1fr;
  }
  
  .welcome-title {
    font-size: 26px;
  }
}

@media (max-width: 480px) {
  .quick-actions {
    grid-template-columns: 1fr;
  }
}

.message-list-empty-text {
  font-size: 16px;
  font-weight: 400;
  color: var(--text-secondary);
}

.switch-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: var(--bg-primary);
  opacity: 0.85;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  pointer-events: auto;
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

.switch-stage-indicator {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 24px;
}

.stage-item {
  display: flex;
  align-items: center;
  gap: 12px;
  opacity: 0.3;
  transition: opacity 0.3s ease;
}

.stage-item.active {
  opacity: 1;
}

.stage-item.completed {
  opacity: 0.7;
}

.stage-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background-color: var(--border-color);
  transition: background-color 0.3s ease;
}

.stage-item.active .stage-dot {
  background-color: var(--accent-primary);
  box-shadow: 0 0 8px var(--accent-primary);
  animation: pulse 1.5s ease-in-out infinite;
}

.stage-item.completed .stage-dot {
  background-color: var(--accent-primary);
}

.stage-label {
  font-size: 12px;
  color: var(--text-secondary);
}

.stage-item.active .stage-label {
  color: var(--text-primary);
  font-weight: 500;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
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
