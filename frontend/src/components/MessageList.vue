<template>
  <div
    ref="messageListRef"
    class="message-list"
    :class="{ 'message-list--virtual': enableVirtualScroll }"
  >
    <!-- 模型切换 overlay 已移至 App.vue 统一管理，避免重复 -->
    <div v-if="(!messages || messages.length === 0) && !isGenerating" class="message-list-empty">
      <div class="welcome-container">
        <!-- 产品 LOGO：唯一视觉锚点，静态居中显示 -->
        <div class="welcome-logo">
          <img :src="defaultAiAvatar" alt="豆芽 LOGO" />
        </div>

        <!-- 品牌主体：第二视觉锚点（中文字号略小，配合 LOGO 形成图文识别） -->
        <div class="welcome-brand">
          <span class="welcome-dou">豆</span>
          <span class="welcome-ya">芽</span>
        </div>

        <!-- 副标题：一句话说明 -->
        <div class="welcome-subtitle">本地运行的 AI 助手</div>

        <!-- 快捷操作 chips：点击即发送（沿用现有 store 机制） -->
        <div class="quick-actions">
          <button
            v-for="action in quickActions"
            :key="action.id"
            class="action-chip"
            @click="handleQuickAction(action)"
          >
            <span class="chip-text">{{ action.title }}</span>
          </button>
        </div>
      </div>
    </div>
    <template v-else>
      <!-- 任务 38：虚拟滚动分支（实验性，feature flag 控制，默认关闭） -->
      <div v-if="enableVirtualScroll" class="virtual-scroller-wrap">
        <DynamicScroller
          ref="scrollerRef"
          :items="messages"
          :min-item-size="120"
          key-field="id"
          :buffer="400"
        >
          <template #default="{ item, index, active }">
            <DynamicScrollerItem :item="item" :active="active" :data-index="index">
              <MessageItem :message="item" />
            </DynamicScrollerItem>
          </template>
        </DynamicScroller>
      </div>
      <!-- 原 v-for 渲染分支（默认，回滚兜底） -->
      <template v-else>
        <MessageItem v-for="msg in messages" :key="msg.id" :message="msg" />
      </template>
      <!-- isGenerating 占位：虚拟/非虚拟两种模式共用，作为列表末尾的兄弟元素。
           非虚拟模式下随内容滚动；虚拟模式下位于 DynamicScroller 下方常驻显示，
           生成结束自动消失，新消息进入上方列表。 -->
      <div v-if="isGenerating" class="message-item">
        <div class="message-avatar ai-avatar">
          <img
            v-if="settingsStore.config.ai_avatar"
            :src="settingsStore.config.ai_avatar"
            alt="AI"
          />
          <img v-else :src="defaultAiAvatar" alt="AI" class="default-avatar" />
        </div>
        <div class="message-bubble-wrapper">
          <div class="message-bubble ai-bubble">
            <template v-if="thinkingAsContent">
              <div class="markdown-body" v-html="renderedThinkingAsContent" />
            </template>
            <template v-else>
              <ThinkBlock
                v-if="thinkingContent"
                :content="thinkingContent"
                :default-expanded="true"
                :is-thinking="isThinking"
                :duration="thinkingDuration"
              />
              <div v-if="canStopThinking" class="stop-thinking-wrapper">
                <button
                  class="stop-thinking-btn"
                  :class="{ loading: isStoppingThinking }"
                  :disabled="isStoppingThinking"
                  @click="handleStopThinking"
                >
                  <svg
                    v-if="!isStoppingThinking"
                    width="14"
                    height="14"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  >
                    <path d="M5 12h14" />
                    <path d="M12 5l7 7-7 7" />
                  </svg>
                  <span v-else class="stop-thinking-spinner"></span>
                  直接回答
                </button>
              </div>
              <div
                v-if="streamingContent"
                ref="streamingContainerRef"
                class="markdown-body streaming"
              />
              <!-- 等待首个 token 的指示器：三点脉冲 + prompt 处理进度，比 spinner 更安静、更现代 -->
              <div
                v-else-if="!thinkingContent && !isSearching"
                class="thinking-dots"
                aria-label="正在思考"
                role="status"
              >
                <span class="thinking-dot"></span>
                <span class="thinking-dot"></span>
                <span class="thinking-dot"></span>
                <span v-if="promptPercent > 0" class="thinking-progress-text">
                  正在处理提示词 {{ promptPercent }}%
                  <span v-if="promptEta" class="thinking-progress-eta">
                    （约 {{ promptEta }}s）
                  </span>
                </span>
              </div>
              <!-- 生成速度：仅在流式生成且有速度数据时显示，低调不抢焦点 -->
              <div v-if="generationSpeed > 0" class="generation-speed">
                {{ generationSpeed.toFixed(1) }} token/s
              </div>
            </template>
            <SearchStatus
              v-if="isSearching"
              :searching="true"
              :results="''"
              :query="searchQuery"
              :error="searchError"
            />
            <SearchStatus
              v-else-if="searchError"
              :searching="false"
              :results="''"
              :error="searchError"
            />
            <SearchStatus
              v-else-if="searchResults"
              :searching="false"
              :results="searchResults"
              :default-expanded="true"
            />
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
        title="回到底部"
        @click="scrollToBottomAndEnable"
      >
        <svg width="18" height="18" viewBox="0 0 22 22" fill="none">
          <path
            d="M11 5v10M7 11l4 4 4-4"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, ref, onMounted, onUnmounted } from 'vue'
import { useMessage } from 'naive-ui'
import MessageItem from './MessageItem.vue'
import ThinkBlock from './ThinkBlock.vue'
import SearchStatus from './SearchStatus.vue'
import ContextTrimmed from './ContextTrimmed.vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { wails } from '../services/wails'
import { renderMarkdown, escapeHtml } from '../utils/markdown'
import { useMorphRender } from '../composables/useMorphRender'
import { useScrollToBottom } from '../composables/useScrollToBottom'
// 任务 38：虚拟滚动 feature flag（默认关闭，纯前端 localStorage 开关）
import { useVirtualScroll } from '../composables/useVirtualScroll'
import { usePromptProgress } from '../composables/usePromptProgress'
import { setupCodeCopyDelegation } from '../utils/codeCopy'
import { logError } from '../utils/logger'
import { isSafeUrl } from '../utils/lightSanitize'
import { classifyError } from '../utils/errorGuidance'
import { discreteDialog } from '../utils/discrete'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import defaultAiAvatar from '../assets/images/appicon.png'
// 任务 38：虚拟滚动组件（局部导入便于 vue-tsc 类型解析；插件已在 main.ts 全局注册）
import { DynamicScroller, DynamicScrollerItem } from 'vue-virtual-scroller'

const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const message = useMessage()

// L-20：定义 QuickAction 接口替代 any，提供编译期类型保护
interface QuickAction {
  id: number
  icon: string
  title: string
  prompt: string
}

const quickActions: QuickAction[] = [
  { id: 1, icon: '', title: '如何使用豆芽', prompt: '如何使用豆芽？请介绍一下主要功能' },
  { id: 2, icon: '', title: '写一段代码', prompt: '帮我写一段示例代码' },
  { id: 3, icon: '', title: '翻译一段文字', prompt: '帮我翻译一段中文为英文' },
  { id: 4, icon: '', title: '头脑风暴', prompt: '帮我做一些头脑风暴，探索新想法' }
]

function handleQuickAction(action: QuickAction) {
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
const searchError = computed(() => chatStore.searchError)
const contextTrimmed = computed(() => chatStore.contextTrimmed)
const generationSpeed = computed(() => chatStore.generationSpeed)
// Prompt 处理进度：搜索完成后到首 token 之间，向用户展示"正在处理提示词 X%"，
// 避免用户误以为卡死。生活类比：像电梯里的楼层显示屏，看到数字在动心里就不慌。
const promptProgress = computed(() => chatStore.promptProgress)
// 安全实践（基于 F-1.3+F-3.11）：promptPercent/promptEta 抽取到 usePromptProgress composable，
// 与 TokenCounter.vue 共享同一计算逻辑，避免一处改漏导致两处显示不一致
const { percent: promptPercent, eta: promptEta } = usePromptProgress(() => promptProgress.value)

// 当思考完成且正文为空时，将思考内容作为正文展示（纯前端展示优化，不干预引擎输出）
const thinkingAsContent = computed(() => {
  return !isThinking.value && thinkingContent.value && !streamingContent.value
})

// 渲染思考内容为 HTML（仅在思考结束且无正文时触发，此时思考已完成，直接全量渲染）。
// renderMarkdown 是 async 函数返回 Promise<string>，不能用 computed（v-html 会渲染成 [object Promise]），
// 改用 ref + watch 异步模式，与 MessageItem.vue 的 renderedContent 写法一致。
// M20 修复：使用 onCleanup 取消前一次渲染，避免快速变化时旧 Promise 覆盖新结果
// 生活类比：厨师接到新订单时取消上一份未完成的菜，避免上错菜
const renderedThinkingAsContent = ref('')
watch(
  [thinkingAsContent, thinkingContent],
  async (_newVal, _oldVal, onCleanup) => {
    if (!thinkingAsContent.value || !thinkingContent.value) {
      renderedThinkingAsContent.value = ''
      return
    }
    let cancelled = false
    onCleanup(() => {
      cancelled = true
    })
    try {
      const html = await renderMarkdown(thinkingContent.value)
      // 校验：渲染期间若 thinkingContent 已变化，丢弃本次结果
      if (!cancelled) {
        renderedThinkingAsContent.value = html
      }
    } catch (_) {
      // 渲染失败时转义后作为纯文本显示，避免直接赋值原始未消毒内容到 v-html（XSS 防护）
      if (!cancelled) {
        renderedThinkingAsContent.value = escapeHtml(thinkingContent.value)
      }
    }
  },
  { immediate: true }
)

// ===== 停止思考功能 =====
// isThinking 由 chat store 自动管理：
//   - 流式开始时 clearConvState 重置为 false
//   - 检测到思考内容（<think> / reasoning_content）时 handleThinking 设为 true
//   - 收到正文 token（思考结束）时 handleToken 设为 false
// 这里只补充"发送停止请求"的本地状态与按钮显隐判断
const isStoppingThinking = ref(false)

// 仅当模型支持推理（capabilities.reasoning）且当前正在思考时，才显示"停止思考"按钮
const canStopThinking = computed(
  () => isThinking.value && settingsStore.modelCapabilities.reasoning
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
    logError('停止思考失败:', e)
  } finally {
    isStoppingThinking.value = false
  }
}

// 回到底部并重新启用自动滚动（抽取为方法避免模板内多语句与 prettier semi:false 冲突）
function scrollToBottomAndEnable() {
  scrollToBottom('smooth')
  isAutoScrollEnabled.value = true
}

// 模型切换 overlay 相关逻辑已移至 App.vue 统一管理
// 这里保留 isSwitching 等变量供其他用途（如禁用输入）

// 流式渲染（对标 llama.cpp webui：stable/unstable 分块缓存 + 实时格式化）：
// - 流式中：marked.lexer 分块，stable blocks 缓存，只重新渲染最后一个 unstable block
// - 流式结束：finalizeRender() 用 renderMarkdown（marked + DOMPurify）全量渲染确保完整
// - RAF 合帧：同一帧内多次 token 只渲染一次（60fps）
const { containerRef: streamingContainerRef, bind: bindMarkdown, finalizeRender } = useMorphRender()
bindMarkdown(() => streamingContent.value)

// 滚动控制：统一每帧绝对滚动 + 用户滚动检测 + 回到底部按钮
const {
  containerRef,
  isAutoScrollEnabled,
  scrollToBottom,
  watchContentChange,
  watchMessagesLength,
  resetState,
  startObserver
} = useScrollToBottom()

// 任务 38：虚拟滚动 feature flag（默认关闭）
const { enableVirtualScroll } = useVirtualScroll()

// 外层 .message-list 容器 ref（非虚拟模式下的滚动容器；虚拟模式下作为稳定外壳）
// 注意：此处不再等同于 useScrollToBottom 的 containerRef，containerRef 由下方 watcher 按开关切换
const messageListRef = ref<HTMLElement | null>(null)
// DynamicScroller 组件实例 ref（虚拟模式下用于取其内部滚动元素 $el）
const scrollerRef = ref<InstanceType<typeof DynamicScroller> | null>(null)

// 根据虚拟滚动开关切换 useScrollToBottom 监听的滚动容器：
// - 关闭（默认）：外层 .message-list 作为滚动容器（保持原行为）
// - 开启：DynamicScroller 根元素（.vue-recycle-scroller，$el）作为滚动容器
// useScrollToBottom 内部 watch(containerRef, { flush: 'sync' }) 会自动重绑 scroll 监听器
watch(
  [() => enableVirtualScroll.value, messageListRef, scrollerRef],
  () => {
    if (enableVirtualScroll.value && scrollerRef.value) {
      // DynamicScroller 的 $el 即其内部 RecycleScroller 的滚动根节点
      // 注：DynamicScroller 的 $el 类型未在 vue-virtual-scroller 类型声明中导出，
      // 此处用 as any 绕过，见安全审查 #39。后续可扩展 shims-vue-virtual-scroller.d.ts。
      const el = (scrollerRef.value as any)?.$el as HTMLElement | undefined
      containerRef.value = el ?? messageListRef.value
    } else {
      containerRef.value = messageListRef.value
    }
  },
  { flush: 'sync', immediate: true }
)

// 安全实践（#17）：拦截 Markdown 正文中的链接点击，走系统默认浏览器，防止 webview 内部导航
const handleLinkClick = (e: MouseEvent) => {
  const target = e.target as HTMLElement
  const anchor = target.closest('a[target="_blank"]') as HTMLAnchorElement | null
  if (anchor && anchor.href) {
    e.preventDefault()
    // 校验协议安全后再打开
    if (isSafeUrl(anchor.href)) {
      BrowserOpenURL(anchor.href)
    }
  }
}

// P6 修复：保存 setupCodeCopyDelegation 的 cleanup 函数，避免潜在的事件监听器泄漏
let cleanupCodeCopyDelegation: (() => void) | null = null

onMounted(() => {
  const el = messageListRef.value
  if (el) {
    // 事件委托：只在容器绑定一次，动态新增按钮自动响应
    // 虚拟模式下 DynamicScroller 的子项也在 .message-list 内，事件仍可冒泡到此
    cleanupCodeCopyDelegation = setupCodeCopyDelegation(el)
    // 安全实践（#17）：委托 markdown-body 中的链接点击，走系统默认浏览器
    el.addEventListener('click', handleLinkClick)
  }
  startObserver()
})

onUnmounted(() => {
  const el = messageListRef.value
  if (el) {
    el.removeEventListener('click', handleLinkClick)
  }
  // P6 修复：清理代码复制的事件委托监听器
  if (cleanupCodeCopyDelegation) {
    cleanupCodeCopyDelegation()
    cleanupCodeCopyDelegation = null
  }
})

// 新消息时滚动到底部
// 传入 getLastRole 区分用户发消息（强制滚动）和 AI 回复完成（尊重用户查看历史）
watchMessagesLength(
  () => chatStore.messages?.length || 0,
  () => chatStore.messages?.[chatStore.messages.length - 1]?.role || ''
)

// 流式内容变化时平滑滚动跟随
// useMorphRender 用 innerHTML 更新 DOM（分块渲染），由 watchContentChange 响应式触发 scheduleScroll
watchContentChange(() => streamingContent.value)
// 思考内容变化时平滑滚动跟随
watchContentChange(() => chatStore.thinkingContent)

// 流式生成结束：触发 Markdown 全量重渲染（Worker + DOMPurify）
// 豆芽始终流式，仅在生成结束时调用 finalizeRender
watch(
  () => chatStore.isGenerating,
  generating => {
    if (!generating) {
      finalizeRender()
    }
  }
)

// 切换会话时重置滚动状态，避免误显示"回到底部"按钮
watch(
  () => chatStore.currentConversationId,
  () => {
    resetState()
  }
)

// 剥离 "[ERR_CODE]" 技术前缀，避免向用户暴露内部错误码
function stripErrCodePrefix(err: string): string {
  return err.replace(/^\[[A-Z_]+\]\s*/, '')
}

// 生成错误展示：用户可手动解决的错误（上下文溢出/OOM/文件缺失等）弹结构化指引
// 对话框（标题+描述+编号建议），其余错误保持 toast 展示原始信息
watch(
  () => chatStore.lastError,
  err => {
    if (!err) return
    const guidance = classifyError(err)
    if (guidance) {
      const suggestions = guidance.suggestions.map((s, i) => `${i + 1}. ${s}`).join('\n')
      discreteDialog.error({
        title: guidance.title,
        content: `${guidance.description}\n\n修复建议：\n${suggestions}\n\n错误详情：${stripErrCodePrefix(err)}`,
        positiveText: '知道了',
        style: { whiteSpace: 'pre-wrap' }
      })
    } else {
      message.destroyAll()
      message.error(stripErrCodePrefix(err))
    }
  }
)
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
  gap: 14px;
  width: 100%;
  margin: 0 auto;
  align-items: flex-start;
}

/* AI、用户消息都撑满宽度；气泡宽度由内部 bubble-wrapper 控制 */
.message-item.user {
  max-width: none;
}

.message-bubble-wrapper {
  flex: 1;
  min-width: 0;
  max-width: 100%;
  align-items: flex-start;
}

.message-bubble {
  padding: 14px 20px;
  /* 与 MessageItem.vue .ai-bubble 一致：左上角小圆角（贴近左侧 AI 头像）
   * border-radius 顺序：左上、右上、右下、左下 */
  border-radius: 4px var(--border-radius-lg) var(--border-radius-lg) var(--border-radius-lg);
  box-shadow: none;
  box-sizing: border-box;
  line-height: 1.65;
}

.ai-bubble {
  /* Q10: 自适应宽度：与 MessageItem.vue 的 .ai-bubble 有意不同
   * MessageItem.vue 的 .ai-bubble 是 width:100%（消息已落库，撑满气泡）
   * 此处是 width:auto（流式期间气泡随内容增长由窄变宽，视觉更自然）
   * 不要强行合并，避免流式气泡变成 100% 宽度出现空白天窗 */
  width: auto;
  max-width: 100%;
  min-width: 0;
  background: var(--bg-ai-msg);
  color: var(--text-ai-msg);
  border: none;
}

/* 流式内容容器：仅用 contain: style 隔离样式重算
 * 不用 contain: layout（高度增长时强制重排导致跳跃）
 * 不用 content-visibility: auto（流式场景下切换渲染状态导致高度突变）
 *
 * 实时格式化方案：流式中用 innerHTML 渲染 markdown HTML（stable/unstable 分块缓存）
 * 不需要 pre-wrap（marked 已将换行转为 <br> 或块级元素）
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

/* "直接回答"按钮：与 ThinkBlock 风格统一，pill 形状，绿色思考色调 */
.stop-thinking-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 14px;
  border: 1px solid var(--accent-think);
  border-radius: 20px;
  background: transparent;
  color: var(--accent-think);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--transition-fast);
  line-height: 1;
  white-space: nowrap;
}

.stop-thinking-btn:hover:not(:disabled) {
  background: var(--accent-think);
  color: var(--bg-primary);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--accent-think-glow) 35%, transparent);
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
  to {
    transform: rotate(360deg);
  }
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
  gap: 24px;
}

/* 产品 LOGO：唯一视觉锚点，静态居中
 * 120px 大尺寸圆形，背景透明，仅保留品牌色环 + 柔和外阴影
 * 双主题自适应，背景图模式下 LOGO 直接穿透显示 */
.welcome-logo {
  width: 120px;
  height: 120px;
  border-radius: 50%;
  overflow: hidden;
  background: transparent;
  box-shadow:
    0 0 0 2px color-mix(in srgb, var(--accent-primary) 18%, transparent),
    0 12px 32px rgba(0, 0, 0, 0.1);
}

.welcome-logo img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  user-select: none;
  -webkit-user-drag: none;
}

/* 品牌主体：第二视觉锚点（中文字号略小，配合 LOGO 形成图文识别） */
.welcome-brand {
  position: relative;
  z-index: 1;
  font-size: 42px;
  font-weight: 700;
  letter-spacing: 4px;
  line-height: 1;
  user-select: none;
  padding-left: 4px;
}

.welcome-dou {
  color: var(--text-primary);
}

.welcome-ya {
  color: var(--accent-primary);
}

/* 副标题：一句话说明，次要文字色 */
.welcome-subtitle {
  position: relative;
  z-index: 1;
  font-size: 17px;
  color: var(--text-secondary);
  letter-spacing: 0.2px;
  margin-top: -10px;
}

.quick-actions {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: center;
  margin-top: 4px;
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

.chip-text {
  white-space: nowrap;
}

.message-list-empty-text {
  font-size: 16px;
  font-weight: 400;
  color: var(--text-secondary);
}

/* 模型切换 overlay 样式已移至 App.vue 统一管理 */

/* 回到底部按钮：正圆包裹箭头（36x36，密度优化） */
.scroll-to-bottom-btn {
  position: sticky;
  bottom: 20px;
  align-self: center;
  width: 36px;
  height: 36px;
  /* 防止 flex 容器压缩高度导致椭圆：flex-shrink 默认 1 会在空间不足时压缩主轴尺寸 */
  flex-shrink: 0;
  /* 正圆包裹箭头：固定 50% 而非 --border-radius-md 变量（变量是圆角方形） */
  border-radius: 50%;
  border: 1px solid var(--border-color);
  background: var(--bg-primary);
  color: var(--text-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 半透明：不抢视觉焦点，hover 时恢复不透明 */
  opacity: 0.85;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
  z-index: 10;
  /* 只过渡颜色和阴影，不过渡 transform，避免 transform 分量时机不同导致椭圆 */
  transition:
    background 0.2s ease,
    border-color 0.2s ease,
    color 0.2s ease,
    opacity 0.2s ease,
    box-shadow 0.2s ease;
}

.scroll-to-bottom-btn:hover {
  opacity: 1;
  background: var(--accent-primary);
  border-color: var(--accent-primary);
  color: #ffffff;
  transform: translateY(-2px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
}

.scroll-to-bottom-btn:active {
  transform: translateY(0) scale(0.95);
}

.scroll-bottom-fade-enter-active {
  transition:
    opacity 0.25s ease,
    transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.scroll-bottom-fade-leave-active {
  transition: opacity 0.15s ease;
}

.scroll-bottom-fade-enter-from,
.scroll-bottom-fade-leave-to {
  opacity: 0;
  transform: translateY(12px);
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

/* Prompt 处理进度文本：搜索完成后到首 token 之间显示，让用户知道正在处理而非卡死 */
.thinking-progress-text {
  margin-left: 6px;
  font-size: 12px;
  color: var(--text-secondary, #888);
  white-space: nowrap;
  user-select: none;
}

.thinking-progress-eta {
  opacity: 0.7;
  margin-left: 2px;
}

@keyframes thinking-pulse {
  0%,
  100% {
    opacity: 0.35;
    transform: scale(0.8);
  }
  50% {
    opacity: 1;
    transform: scale(1.15);
    box-shadow: 0 0 6px 0 var(--accent-primary);
  }
}

/* ===== 任务 38：虚拟滚动布局 =====
 * 虚拟模式下，.message-list 不再作为滚动容器（DynamicScroller 内部自带
 * overflow:auto 的 .vue-recycle-scroller 作为滚动根）。此处覆盖全局
 * .message-list 的 overflow，让外层仅作弹性外壳，由内部 scroller 滚动。
 * 生活类比：原来整个房间都是跑道，现在把跑道收进一台跑步机，房间只负责把
 * 跑步机固定住并留出空间。
 */
.message-list--virtual {
  /* 关闭外层滚动，避免与 DynamicScroller 内部滚动产生双重滚动条 */
  overflow: hidden;
  /* 作为 scroll-to-bottom 按钮绝对定位的参照系 */
  position: relative;
}

/* DynamicScroller 外壳：填充剩余高度，让内部 scroller 拿到确定高度 */
.virtual-scroller-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
}

/* DynamicScroller 根元素（.vue-recycle-scroller）填满外壳并作为滚动容器 */
.message-list--virtual .virtual-scroller-wrap :deep(.vue-recycle-scroller) {
  flex: 1;
  min-height: 0;
  height: 100%;
  /* 复用全局滚动条样式（webkit 细滚动条由全局 ::-webkit-scrollbar 提供） */
}

/* 虚拟模式下回到底部按钮改用绝对定位：
 * 原始 position:sticky 依赖滚动容器，而虚拟模式下滚动发生在 DynamicScroller
 * 内部，sticky 相对 .message-list（overflow:hidden）无法生效。 */
.message-list--virtual .scroll-to-bottom-btn {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
}

/* 虚拟模式 hover/active：保留 translateX(-50%) 居中，叠加垂直位移和缩放 */
.message-list--virtual .scroll-to-bottom-btn:hover {
  transform: translateX(-50%) translateY(-2px);
}

.message-list--virtual .scroll-to-bottom-btn:active {
  transform: translateX(-50%) scale(0.95);
}
</style>

<style>
/* ===== 欢迎界面：暗色模式基础适配 ===== */

.dark .action-chip {
  background: var(--bg-tertiary);
  border-color: color-mix(in srgb, var(--border-color) 80%, transparent);
}

.dark .action-chip:hover {
  border-color: var(--accent-primary);
  background: var(--accent-tertiary);
}

/* 背景图模式：流式气泡半透明，与 MessageItem.vue 的气泡层 80% 一致 */
.has-background .message-bubble.ai-bubble {
  background: color-mix(in srgb, var(--bg-ai-msg) 80%, transparent);
}
</style>
