<template>
  <div ref="messageListRef" class="message-list">
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
            <template v-if="thinkingAsContent">
              <div class="markdown-body" v-html="renderedThinkingAsContent" />
            </template>
            <template v-else>
              <ThinkBlock v-if="thinkingContent" :content="thinkingContent" :default-expanded="true" :is-thinking="isThinking" :duration="thinkingDuration" />
              <div v-if="canStopThinking" class="stop-thinking-wrapper">
                <n-button
                  type="warning"
                  size="small"
                  :loading="isStoppingThinking"
                  @click="handleStopThinking"
                >
                  停止思考
                </n-button>
              </div>
              <div v-if="streamingContent" class="markdown-body streaming" v-html="renderedStreaming" />
              <n-spin v-else-if="!thinkingContent && !isSearching" size="small" />
            </template>
            <SearchStatus v-if="isSearching" :searching="true" :results="''" :query="searchQuery" />
            <SearchStatus v-else-if="searchResults" :searching="false" :results="searchResults" :default-expanded="true" />
            <ContextTrimmed :data="contextTrimmed" />
          </div>
        </div>
      </div>
    </template>
    <!-- 回到底部按钮：用户向上滚动后显示 -->
    <Transition name="scroll-bottom-fade">
      <button
        v-if="!isAutoScrollEnabled && messages && messages.length > 0"
        class="scroll-to-bottom-btn"
        @click="scrollToBottom('smooth'); isAutoScrollEnabled = true"
        title="回到底部"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 5v14M19 12l-7 7-7-7" />
        </svg>
      </button>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, ref, nextTick, onMounted, onUnmounted } from 'vue'
import { NButton, NSpin, useMessage } from 'naive-ui'
import MessageItem from './MessageItem.vue'
import ThinkBlock from './ThinkBlock.vue'
import SearchStatus from './SearchStatus.vue'
import ContextTrimmed from './ContextTrimmed.vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { wails } from '../services/wails'
import { renderMarkdownStreaming } from '../utils/markdown'
import { useMarkdownWorker } from '../composables/useMarkdownWorker'
import { useScrollToBottom } from '../composables/useScrollToBottom'
import { formatModelName } from '../utils/model'
import { setupCodeCopyDelegation } from '../utils/codeCopy'
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
  chatStore.sendMessage(action.prompt, settingsStore.searchMode)
}

const messages = computed(() => chatStore.messages)
const isGenerating = computed(() => chatStore.isGenerating)
const streamingContent = computed(() => chatStore.streamingContent)
const thinkingContent = computed(() => chatStore.thinkingContent)
const searchResults = computed(() => chatStore.searchResults)
const isSearching = computed(() => chatStore.isSearching)
const isThinking = computed(() => chatStore.isThinking)
const thinkingDuration = computed(() => chatStore.thinkingDuration)
const searchQuery = computed(() => chatStore.searchQuery)
const contextTrimmed = computed(() => chatStore.contextTrimmed)

// 当思考完成且正文为空时，将思考内容作为正文展示（纯前端展示优化，不干预引擎输出）
const thinkingAsContent = computed(() => {
  return !isThinking.value && thinkingContent.value && !streamingContent.value
})

const renderedThinkingAsContent = computed(() => {
  if (!thinkingAsContent.value || !thinkingContent.value) return ''
  return renderMarkdownStreaming(thinkingContent.value)
})

// ===== 停止思考功能 =====
// isThinking 由 chat store 自动管理：
//   - 流式开始时 clearConvState 重置为 false
//   - 检测到思考内容（<think> / reasoning_content）时 handleThinking 设为 true
//   - 收到正文 token（思考结束）时 handleToken 设为 false
// 这里只补充"发送停止请求"的本地状态与按钮显隐判断
const isStoppingThinking = ref(false)

// 仅当模型支持推理（capabilities.reasoning）且当前正在思考时，才显示"停止思考"按钮
const canStopThinking = computed(() =>
  isThinking.value && settingsStore.modelCapabilities.reasoning
)

// 点击"停止思考"：调用后端 StopThinking，成功后 isThinking 会被 store 自动置 false，按钮随之隐藏
async function handleStopThinking() {
  if (isStoppingThinking.value) return
  isStoppingThinking.value = true
  try {
    await wails.stopThinking()
    // 成功后由 store 在收到后续正文 token 时将 isThinking 置 false，按钮自动隐藏
  } catch (e) {
    message.error('停止思考请求失败，请重试')
    console.error('停止思考失败:', e)
  } finally {
    isStoppingThinking.value = false
  }
}

const isSwitching = computed(() => settingsStore.isModelSwitching)
const switchingToModel = computed(() => {
  if (settingsStore.serverStatus.switching_to) {
    return formatModelName(settingsStore.serverStatus.switching_to).display
  }
  return ''
})

const switchStages = ['准备切换', '加载新模型', '初始化完成']

function getCurrentStageIndex(): number {
  const stage = settingsStore.switchProgress.stage
  switch (stage) {
    case 'preparing':
      return 0
    case 'loading':
      return 1
    case 'done':
      return 2
    default:
      return 0
  }
}

function getSwitchProgressText(): string {
  const stage = settingsStore.switchProgress.stage
  const newModel = switchingToModel.value

  switch (stage) {
    case 'preparing':
      return '准备切换模型...'
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

// PERF-003 + Step 3: 流式 Markdown 渲染跑在 Web Worker 中，主线程零阻塞
// - useMarkdownWorker 内部维护任务 ID 防过期、动态节流、Worker 复用
// - Worker 失败时自动降级到主线程渲染
const { rendered: renderedStreaming, bind: bindMarkdown } = useMarkdownWorker()
bindMarkdown(() => streamingContent.value)

// 滚动控制：流式期间即时滚动 + 用户滚动检测 + 回到底部按钮
const {
    containerRef: messageListRef,
    isAutoScrollEnabled,
    isNearBottom,
    scrollToBottom,
    watchContentChange,
    watchMessagesLength,
    startObserver,
} = useScrollToBottom()

onMounted(() => {
    const el = messageListRef.value
    if (el) {
        // 事件委托：只在容器绑定一次，动态新增按钮自动响应
        setupCodeCopyDelegation(el)
    }
    startObserver()
})

// 新消息时滚动到底部
watchMessagesLength(() => chatStore.messages?.length || 0)

// 流式内容变化时平滑滚动跟随
watchContentChange(() => chatStore.streamingContent)
// 思考内容变化时平滑滚动跟随
watchContentChange(() => chatStore.thinkingContent)

// done 事件更新消息后重新滚动
watch(() => chatStore.messages, () => {
  if (isNearBottom()) {
    nextTick(scrollToBottom)
  }
}, { deep: false })

watch(() => chatStore.lastError, (err) => {
    if (err) {
        message.destroyAll()
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
  border-radius: var(--border-radius-xl) var(--border-radius-xl) var(--border-radius-xl) var(--border-radius-sm);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  box-sizing: border-box;
  line-height: 1.65;
}

.ai-bubble {
  width: 100%;
  border: 1px solid var(--border-color);
}

/* 流式内容容器：用 contain 限制重排范围，不用 content-visibility 避免高度突变 */
.markdown-body.streaming {
  contain: layout style;
}

/* 停止思考按钮容器：紧贴思考块下方，左对齐 */
.stop-thinking-wrapper {
  margin-top: 8px;
  display: flex;
  justify-content: flex-start;
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
  border-radius: var(--border-radius-lg);
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
  border-radius: var(--border-radius-md);
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

.scroll-to-bottom-btn {
  position: sticky;
  bottom: 20px;
  align-self: center;
  width: 38px;
  height: 38px;
  border-radius: 50%;
  border: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-primary) 88%, transparent);
  backdrop-filter: blur(12px) saturate(180%);
  -webkit-backdrop-filter: blur(12px) saturate(180%);
  color: var(--text-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-md);
  z-index: 10;
  transition: all var(--transition-fast);
}

.scroll-to-bottom-btn:hover {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
  color: #ffffff;
  transform: scale(1.08);
  box-shadow: var(--shadow-lg);
}

.scroll-to-bottom-btn:active {
  transform: scale(0.95);
}

.scroll-bottom-fade-enter-active,
.scroll-bottom-fade-leave-active {
  transition: opacity 0.25s var(--transition-normal), transform 0.25s var(--transition-normal);
}

.scroll-bottom-fade-enter-from,
.scroll-bottom-fade-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.9);
}
</style>

<style>
/* ===== 欢迎界面：背景图模式毛玻璃 ===== */

.has-background .action-card {
  background: color-mix(in srgb, var(--bg-primary) 76%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
}

.has-background .action-card:hover {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
  border-color: var(--accent-primary);
}

.has-background .tip-item {
  background: color-mix(in srgb, var(--bg-secondary) 74%, transparent);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.has-background .tip-item:hover {
  background: color-mix(in srgb, var(--bg-primary) 78%, transparent);
}

.has-background .welcome-subtitle {
  color: var(--text-secondary);
}

/* 暗色模式 + 背景图：更透的玻璃效果 */
.dark .has-background .action-card {
  background: color-mix(in srgb, var(--bg-primary) 70%, transparent);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2), 0 0 0 1px rgba(255, 255, 255, 0.03);
}

.dark .has-background .action-card:hover {
  box-shadow: 0 4px 18px rgba(0, 0, 0, 0.4), 0 0 0 1px var(--accent-primary);
}

.dark .has-background .tip-item {
  background: color-mix(in srgb, var(--bg-secondary) 68%, transparent);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}

.dark .has-background .tip-item:hover {
  background: color-mix(in srgb, var(--bg-primary) 72%, transparent);
}

/* ===== 欢迎界面：暗色模式基础适配 ===== */

.dark .action-card {
  background: var(--bg-tertiary);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.04);
  border-color: color-mix(in srgb, var(--border-color) 80%, transparent);
}

.dark .action-card:hover {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(255, 255, 255, 0.06);
  border-color: var(--accent-primary);
  transform: translateY(-2px);
}

.dark .action-card:active {
  transform: translateY(-1px);
}

.dark .tip-item {
  background: var(--bg-tertiary);
  border-color: color-mix(in srgb, var(--border-color) 80%, transparent);
}

.dark .tip-item:hover {
  background: var(--bg-active);
  border-color: var(--accent-primary);
}

.dark .welcome-subtitle {
  color: var(--text-secondary);
}
</style>
