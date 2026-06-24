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
        <div class="welcome-brand">
          <span class="welcome-dou">豆</span><span class="welcome-ya">芽</span>
        </div>
        <div class="welcome-model" v-if="currentModelDisplay">{{ currentModelDisplay }}</div>
        <div class="welcome-hint">输入消息开始对话，或选择一个话题</div>
        <div class="quick-actions">
          <button v-for="action in quickActions" :key="action.id" class="action-chip" @click="handleQuickAction(action)">
            <span class="chip-icon">{{ action.icon }}</span>
            <span class="chip-text">{{ action.title }}</span>
          </button>
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
                <button
                  class="stop-thinking-btn"
                  :class="{ loading: isStoppingThinking }"
                  @click="handleStopThinking"
                  :disabled="isStoppingThinking"
                >
                  <svg v-if="!isStoppingThinking" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M5 12h14" />
                    <path d="M12 5l7 7-7 7" />
                  </svg>
                  <span class="stop-thinking-spinner" v-else></span>
                  直接回答
                </button>
              </div>
              <div v-if="streamingContent" class="markdown-body streaming" v-html="renderedStreaming" />
              <n-spin v-else-if="!thinkingContent && !isSearching" size="small" />
              <!-- 生成速度：仅在流式生成且有速度数据时显示，低调不抢焦点 -->
              <div v-if="generationSpeed > 0" class="generation-speed">
                {{ generationSpeed.toFixed(1) }} token/s
              </div>
            </template>
            <SearchStatus v-if="isSearching" :searching="true" :results="''" :query="searchQuery" />
            <SearchStatus v-else-if="searchResults" :searching="false" :results="searchResults" :default-expanded="true" />
            <ContextTrimmed :data="contextTrimmed" />
          </div>
        </div>
      </div>
    </template>
    <!-- 回到底部按钮 -->
    <Transition name="scroll-bottom-fade">
      <button
        v-if="!isAutoScrollEnabled && messages && messages.length > 0"
        class="scroll-to-bottom-btn"
        @click="scrollToBottom('smooth'); isAutoScrollEnabled = true"
        title="回到底部"
      >
        <svg width="22" height="22" viewBox="0 0 22 22" fill="none">
          <path d="M11 5v10M7 11l4 4 4-4" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </button>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, ref, nextTick, onMounted, onUnmounted } from 'vue'
import { NSpin, useMessage } from 'naive-ui'
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
  { id: 1, icon: '✍️', title: '帮我写代码', prompt: '帮我写一段示例代码' },
  { id: 2, icon: '💡', title: '头脑风暴', prompt: '帮我做一些头脑风暴，探索新想法' },
  { id: 3, icon: '📚', title: '知识问答', prompt: '我想了解一些有趣的知识' },
  { id: 4, icon: '🔧', title: '解决问题', prompt: '我有一个问题需要分析和解决' },
]

const currentModelDisplay = computed(() => {
  const model = settingsStore.currentModel
  if (!model) return ''
  return formatModelName(model).display
})

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
const generationSpeed = computed(() => chatStore.generationSpeed)

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
    message.error('直接回答请求失败，请重试')
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
    setStreamingMode,
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

// 流式生成状态切换：启用/禁用增量滚动模式
watch(() => chatStore.isGenerating, (generating) => {
    setStreamingMode(generating)
})

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

/* 流式内容容器：仅用 contain: style 隔离样式重算
 * 不用 contain: layout（高度增长时强制重排导致跳跃）
 * 不用 content-visibility: auto（流式场景下切换渲染状态导致高度突变）
 */
.markdown-body.streaming {
  contain: style;
}

/* 生成速度指示器：小字号、次要颜色，不抢视觉焦点 */
.generation-speed {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 4px;
}

/* 停止思考按钮容器：紧贴思考块下方，左对齐 */
.stop-thinking-wrapper {
  margin-top: 8px;
  display: flex;
  justify-content: flex-start;
}

/* "直接回答"按钮：与 ThinkBlock 风格统一，pill 形状，accent-warning 色调 */
.stop-thinking-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 14px;
  border: 1px solid var(--accent-warning);
  border-radius: 20px;
  background: transparent;
  color: var(--accent-warning);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  line-height: 1;
  white-space: nowrap;
}

.stop-thinking-btn:hover:not(:disabled) {
  background: var(--accent-warning);
  color: var(--bg-primary);
  box-shadow: 0 2px 8px rgba(255, 195, 0, 0.25);
}

.stop-thinking-btn:active:not(:disabled) {
  transform: scale(0.96);
}

.stop-thinking-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.stop-thinking-btn svg {
  flex-shrink: 0;
}

/* 加载旋转动画 */
.stop-thinking-spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid transparent;
  border-top-color: currentColor;
  border-radius: 50%;
  animation: stopThinkingSpin 0.6s linear infinite;
}

@keyframes stopThinkingSpin {
  to { transform: rotate(360deg); }
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
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20px;
  animation: welcomeFadeIn 0.6s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes welcomeFadeIn {
  from { opacity: 0; transform: translateY(16px); }
  to { opacity: 1; transform: translateY(0); }
}

.welcome-brand {
  font-size: 56px;
  font-weight: 800;
  letter-spacing: 4px;
  line-height: 1;
  user-select: none;
}

.welcome-dou {
  color: var(--text-primary);
}

.welcome-ya {
  color: var(--accent-primary);
}

.welcome-model {
  font-size: 14px;
  color: var(--text-secondary);
  font-weight: 500;
  padding: 4px 16px;
  border-radius: 20px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
}

.welcome-hint {
  font-size: 15px;
  color: var(--text-secondary);
  letter-spacing: 0.2px;
}

.quick-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: center;
  margin-top: 8px;
}

.action-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: 24px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
  transition: all 0.2s ease;
  font-family: inherit;
  line-height: 1;
}

.action-chip:hover {
  border-color: var(--accent-primary);
  background: var(--accent-tertiary);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.06);
}

.action-chip:active {
  transform: translateY(0);
}

.chip-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.chip-text {
  white-space: nowrap;
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

/* 回到底部按钮：正圆包裹箭头 */
.scroll-to-bottom-btn {
  position: sticky;
  bottom: 24px;
  align-self: center;
  width: 48px;
  height: 48px;
  border-radius: 50%;
  border: 1px solid var(--border-color);
  background: var(--bg-primary);
  color: var(--text-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.12);
  z-index: 10;
  transition: all 0.2s ease;
}

.scroll-to-bottom-btn:hover {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
  color: #ffffff;
  transform: translateY(-2px);
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.18);
}

.scroll-to-bottom-btn:active {
  transform: translateY(0) scale(0.95);
}

.scroll-bottom-fade-enter-active {
  transition: opacity 0.25s ease, transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.scroll-bottom-fade-leave-active {
  transition: opacity 0.15s ease;
}

.scroll-bottom-fade-enter-from,
.scroll-bottom-fade-leave-to {
  opacity: 0;
  transform: translateY(12px);
}
</style>

<style>
/* ===== 欢迎界面：背景图模式毛玻璃 ===== */

.has-background .action-chip {
  background: color-mix(in srgb, var(--bg-primary) 76%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.has-background .action-chip:hover {
  background: color-mix(in srgb, var(--accent-tertiary) 80%, transparent);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.has-background .welcome-model {
  background: color-mix(in srgb, var(--bg-secondary) 74%, transparent);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

/* 暗色模式 + 背景图 */
.dark .has-background .action-chip {
  background: color-mix(in srgb, var(--bg-primary) 70%, transparent);
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
}

.dark .has-background .action-chip:hover {
  background: color-mix(in srgb, var(--accent-tertiary) 72%, transparent);
  box-shadow: 0 4px 18px rgba(0, 0, 0, 0.4), 0 0 0 1px var(--accent-primary);
}

/* ===== 欢迎界面：暗色模式基础适配 ===== */

.dark .action-chip {
  background: var(--bg-tertiary);
  border-color: color-mix(in srgb, var(--border-color) 80%, transparent);
}

.dark .action-chip:hover {
  border-color: var(--accent-primary);
  background: var(--accent-tertiary);
}

.dark .welcome-model {
  background: var(--bg-tertiary);
  border-color: color-mix(in srgb, var(--border-color) 80%, transparent);
}
</style>
