<template>
  <div ref="messageListRef" class="message-list">
    <!-- 模型切换 overlay 已移至 App.vue 统一管理，避免重复 -->
    <div v-if="(!messages || messages.length === 0) && !isGenerating" class="message-list-empty">
      <div class="welcome-container">
        <!-- 背景装饰：旋转光环 + 网格纹理 -->
        <div class="welcome-aura" aria-hidden="true"></div>
        <div class="welcome-grid" aria-hidden="true"></div>

        <!-- 品牌主体：带光晕脉冲的"豆芽"二字 -->
        <div class="welcome-brand">
          <span class="welcome-dou">豆</span><span class="welcome-ya">芽</span>
        </div>

        <!-- 模型状态胶囊：左侧脉冲圆点表示就绪 -->
        <div class="welcome-model" v-if="currentModelDisplay">
          <span class="model-status-dot" aria-hidden="true"></span>
          <span class="model-name">{{ currentModelDisplay }}</span>
        </div>

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
              <!-- 等待首个 token 的指示器：三点脉冲，比 spinner 更安静、更现代 -->
              <div v-else-if="!thinkingContent && !isSearching" class="thinking-dots" aria-label="正在思考" role="status">
                <span class="thinking-dot"></span>
                <span class="thinking-dot"></span>
                <span class="thinking-dot"></span>
              </div>
              <!-- 流式光标：AI 正在生成内容时显示闪烁竖线 -->
              <span v-if="streamingContent && isGenerating" class="streaming-cursor" aria-hidden="true"></span>
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
import { computed, watch, ref, nextTick, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
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
    const errMsg = e instanceof Error ? e.message : String(e || '直接回答请求失败')
    message.error(`直接回答失败：${errMsg}`)
    console.error('停止思考失败:', e)
  } finally {
    isStoppingThinking.value = false
  }
}

// 模型切换 overlay 相关逻辑已移至 App.vue 统一管理
// 这里保留 isSwitching 等变量供其他用途（如禁用输入）

// PERF-003 + Step 3: 流式 Markdown 渲染跑在 Web Worker 中，主线程零阻塞
// - useMarkdownWorker 内部维护任务 ID 防过期、动态节流、Worker 复用
// - Worker 失败时自动降级到主线程渲染
// - 双模式：流式期间用轻量同步渲染（跳过 Worker + DOMPurify），结束后全量重渲染
const { rendered: renderedStreaming, bind: bindMarkdown, setStreamingMode: setMarkdownStreamingMode } = useMarkdownWorker()
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

// 流式生成状态切换：启用/禁用增量滚动模式 + Markdown 双模式渲染
watch(() => chatStore.isGenerating, (generating) => {
    setStreamingMode(generating)
    setMarkdownStreamingMode(generating)
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
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20px;
  /* 交错入场：子元素按顺序淡入上移，营造层次感 */
  animation: welcomeFadeIn 0.6s cubic-bezier(0.23, 1, 0.32, 1);
}

/* 背景装饰：缓慢旋转的渐变光环，呼应 AI 思考的呼吸感 */
.welcome-aura {
  position: absolute;
  top: -40px;
  left: 50%;
  width: 320px;
  height: 320px;
  transform: translateX(-50%);
  background: radial-gradient(circle, var(--accent-primary) 0%, transparent 60%);
  opacity: 0.08;
  filter: blur(40px);
  border-radius: 50%;
  animation: aura-rotate 20s linear infinite;
  pointer-events: none;
  z-index: 0;
}

@keyframes aura-rotate {
  0% { transform: translateX(-50%) rotate(0deg) scale(1); }
  50% { transform: translateX(-50%) rotate(180deg) scale(1.08); }
  100% { transform: translateX(-50%) rotate(360deg) scale(1); }
}

/* 背景装饰：微妙的点阵网格，增加科技感纹理 */
.welcome-grid {
  position: absolute;
  top: -20px;
  left: 50%;
  width: 440px;
  height: 280px;
  transform: translateX(-50%);
  background-image: radial-gradient(circle, var(--border-color) 1px, transparent 1px);
  background-size: 24px 24px;
  opacity: 0.4;
  mask-image: radial-gradient(ellipse at center, black 0%, transparent 70%);
  -webkit-mask-image: radial-gradient(ellipse at center, black 0%, transparent 70%);
  pointer-events: none;
  z-index: 0;
}

@keyframes welcomeFadeIn {
  from { opacity: 0; transform: translateY(16px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 品牌主体：置于装饰层之上，带交错入场延迟 */
.welcome-brand {
  position: relative;
  z-index: 1;
  font-size: 56px;
  font-weight: 800;
  letter-spacing: 4px;
  line-height: 1;
  user-select: none;
  animation: welcomeFadeIn 0.7s cubic-bezier(0.23, 1, 0.32, 1) both;
  animation-delay: 0.05s;
}

.welcome-dou {
  color: var(--text-primary);
}

.welcome-ya {
  color: var(--accent-primary);
  text-shadow: 0 0 24px color-mix(in srgb, var(--accent-primary) 40%, transparent);
}

.welcome-model {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 500;
  padding: 6px 14px 6px 10px;
  border-radius: 20px;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  animation: welcomeFadeIn 0.7s cubic-bezier(0.23, 1, 0.32, 1) both;
  animation-delay: 0.15s;
}

/* 模型状态点：脉冲呼吸，表示模型已就绪 */
.model-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-primary);
  box-shadow: 0 0 0 0 var(--accent-primary);
  animation: status-pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
  flex-shrink: 0;
}

@keyframes status-pulse {
  0%, 100% {
    opacity: 1;
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-primary) 50%, transparent);
  }
  50% {
    opacity: 0.6;
    box-shadow: 0 0 0 5px color-mix(in srgb, var(--accent-primary) 0%, transparent);
  }
}

.model-name {
  font-family: 'SF Mono', 'JetBrains Mono', 'Consolas', monospace;
  letter-spacing: 0.3px;
}

.welcome-hint {
  position: relative;
  z-index: 1;
  font-size: 15px;
  color: var(--text-secondary);
  letter-spacing: 0.2px;
  animation: welcomeFadeIn 0.7s cubic-bezier(0.23, 1, 0.32, 1) both;
  animation-delay: 0.25s;
}

.quick-actions {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: center;
  margin-top: 8px;
  animation: welcomeFadeIn 0.7s cubic-bezier(0.23, 1, 0.32, 1) both;
  animation-delay: 0.35s;
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

/* 模型切换 overlay 样式已移至 App.vue 统一管理 */

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

/* ===== AI 流式光标 =====
 * 闪烁竖线，AI 生成时跟随文字末尾
 * 用 inline-block + animation，GPU 友好
 */
.streaming-cursor {
  display: inline-block;
  width: 8px;
  height: 1.1em;
  margin-left: 2px;
  vertical-align: text-bottom;
  background: var(--accent-primary);
  border-radius: 1px;
  animation: cursor-blink 1s step-end infinite;
  will-change: opacity;
}

/* ===== 思考点指示器（替代 n-spin）=====
 * 三点错位脉冲：低饱和呼吸 + accent 光晕，比机械旋转更安静、更现代
 * - 圆点尺寸 5px，间距 6px，紧凑不抢视觉焦点
 * - 错位延迟 0/0.18/0.36s 形成横向流动感
 * - 用 transform + opacity（GPU 友好），避免布局抖动
 * - 暗色模式通过 --accent-primary 变量自动适配
 */
.thinking-dots {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 20px;
  padding: 0 2px;
}

.thinking-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--accent-primary);
  opacity: 0.35;
  transform: scale(0.8);
  animation: thinking-pulse 1.4s cubic-bezier(0.4, 0, 0.6, 1) infinite;
  will-change: transform, opacity;
}

.thinking-dot:nth-child(2) {
  animation-delay: 0.18s;
}

.thinking-dot:nth-child(3) {
  animation-delay: 0.36s;
}

@keyframes thinking-pulse {
  0%, 100% {
    opacity: 0.35;
    transform: scale(0.8);
  }
  50% {
    opacity: 1;
    transform: scale(1.15);
    box-shadow: 0 0 6px 0 var(--accent-primary);
  }
}

/* 流式 markdown 容器性能优化 */
.markdown-body.streaming {
  contain: style;
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

/* 背景图模式下增强光环亮度，弥补背景图遮挡 */
.has-background .welcome-aura {
  opacity: 0.14;
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

/* 暗色模式下光环更明显，网格点更亮 */
.dark .welcome-aura {
  opacity: 0.16;
}

.dark .welcome-grid {
  opacity: 0.5;
}

/* 尊重用户的动效偏好：关闭装饰动画，保留静态效果 */
@media (prefers-reduced-motion: reduce) {
  .welcome-aura,
  .model-status-dot {
    animation: none;
  }
  .welcome-brand,
  .welcome-model,
  .welcome-hint,
  .quick-actions {
    animation: none;
    opacity: 1;
  }
}
</style>
