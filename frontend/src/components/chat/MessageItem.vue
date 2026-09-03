<template>
  <div class="message-item" :class="{ user: isUser }">
    <div class="message-avatar" :class="isUser ? 'user-avatar' : 'ai-avatar'">
      <img
        v-if="
          (isUser && settingsStore.config.user_avatar) ||
          (!isUser && settingsStore.config.ai_avatar)
        "
        :src="isUser ? settingsStore.config.user_avatar : settingsStore.config.ai_avatar"
        :alt="isUser ? '用户' : 'AI'"
      />
      <img
        v-else
        :src="isUser ? defaultUserAvatar : defaultAiAvatar"
        :alt="isUser ? '用户' : 'AI'"
        class="default-avatar"
      />
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
      <!-- 三件套分解：气泡为纯展示子组件，mouseup 经属性透传落到其根元素；
           气泡 DOM 通过组件实例的 $el 访问（选区定位 / 富文本复制依赖它） -->
      <MessageBubble
        ref="bubbleComp"
        :message="message"
        :editing="isEditing"
        @mouseup="handleMouseUp"
        @save="saveEdit"
        @cancel="cancelEdit"
        @preview="previewImage"
      />
      <MessageActions
        :is-user="isUser"
        :tokens-per-second="tokensPerSecond"
        :show-tts="showTts"
        :tts-active="ttsActive"
        :tts-disabled="ttsDisabled"
        :tts-backend="ttsBackend"
        :can-regenerate="canRegenerate"
        :can-edit="canEdit"
        @copy="copyContent"
        @speak="toggleSpeak"
        @regenerate="regenerate"
        @report="reportProblem"
        @delete="deleteMsg"
        @edit="startEdit"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue'
import { useMessage } from 'naive-ui'
import MessageBubble from './MessageBubble.vue'
import MessageActions from './MessageActions.vue'
import { setupCodeCopyDelegation } from '../../utils/codeCopy'
import { useChatStore } from '../../stores/chat'
import { useSettingsStore } from '../../stores/settings'
import { useTTS } from '../../composables/useTTS'
import { useReportProblem } from '../../composables/useReportProblem'
import { wails } from '../../services/wails'
import type { Message } from '../../services/wails'
import AppIcon from '../ui/AppIcon.vue'
import defaultUserAvatar from '../../assets/images/user-avatar.svg'
import defaultAiAvatar from '../../assets/images/appicon.png'

const props = defineProps<{ message: Message }>()
const chatStore = useChatStore()
const settingsStore = useSettingsStore()
const messageApi = useMessage()
// TTS 播音员调度台（全局单例，所有消息共用）
const tts = useTTS()
// 举报弹窗调度台（全局单例，ChatView 挂载的 ReportDialog 渲染）
const report = useReportProblem()

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

// —— 动作条可视状态（分解后由控制器统一计算，动作条保持纯展示）——
const showTts = computed(
  () => !isUser.value && tts.isSupported.value && settingsStore.config.tts_enabled
)
const ttsActive = computed(() => tts.isSpeaking(props.message.id))
const ttsDisabled = computed(() => chatStore.isGenerating && isLastAIMessage.value)
const ttsBackend = computed(() => tts.currentBackend.value ?? null)
const canRegenerate = computed(
  () => !isUser.value && !chatStore.isAnyGenerating && isLastAIMessage.value
)
// 编辑入口：仅用户消息且当前无任何会话在生成时可用
const canEdit = computed(() => isUser.value && !chatStore.isAnyGenerating)

// —— 编辑态编排：保存落库 → 本地同步 → 截断重生成 ——
const isEditing = ref(false)

function startEdit() {
  if (!chatStore.isAnyGenerating) {
    isEditing.value = true
  }
}

function cancelEdit() {
  isEditing.value = false
}

async function saveEdit(newContent: string) {
  const trimmed = newContent.trim()
  // 空内容不允许保存（与后端 EditMessage 校验语义一致）；未变更则静默退出
  if (!trimmed || trimmed === props.message.content) {
    isEditing.value = false
    return
  }
  try {
    await wails.editMessage(props.message.id, trimmed)
    // 本地同步内容，保证截断重生成期间 UI 立即反映新文本
    const target = chatStore.messages.find(m => m.id === props.message.id)
    if (target) target.content = trimmed
    isEditing.value = false
    // 决策记录「编辑后截断重生成」：复用 regenerateMessage 的既有编排
    // （其内部会截断该用户消息之后的回复并重新发起生成，失败自动回滚）
    await chatStore.regenerateMessage(props.message.id, settingsStore.searchMode)
  } catch {
    messageApi.error('保存失败')
  }
}

// 任意会话开始生成时强制退出编辑态，避免编辑中的旧文本与新回复错位
watch(
  () => chatStore.isAnyGenerating,
  generating => {
    if (generating && isEditing.value) {
      cancelEdit()
    }
  }
)

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

// 气泡根元素引用：分解后气泡位于子组件内部，经实例的 $el 获取
const bubbleComp = ref<{ $el: HTMLElement } | null>(null)
let cleanupCodeCopyDelegation: (() => void) | null = null

onMounted(() => {
  const el = bubbleComp.value?.$el
  if (el) {
    cleanupCodeCopyDelegation = setupCodeCopyDelegation(el)
  }
  document.addEventListener('mousedown', handleDocumentMouseDown)
  // scroll 监听器改为按需注册（见下方 watch(selectionBtnVisible)），
  // 避免每个 MessageItem 实例都常驻全局 scroll 监听导致长会话滚动卡顿
})

/**
 * 切换朗读状态：未朗读→开始朗读，正在朗读→停止
 * 如果用户在消息气泡内选中了文字，只朗读选中部分；否则朗读整条消息。
 */
function toggleSpeak() {
  // 正在朗读本条 → 停止
  if (tts.isSpeaking(props.message.id)) {
    tts.stop()
    return
  }
  // 获取用户选中的文字（如果有）
  const selection = window.getSelection()
  const selectedText = selection?.toString().trim()
  const textToSpeak = selectedText && selectedText.length > 0 ? selectedText : props.message.content
  if (textToSpeak) {
    tts.speak(textToSpeak, props.message.id)
  }
}

async function copyContent() {
  try {
    const markdownEl = bubbleComp.value?.$el?.querySelector('.markdown-body') as HTMLElement | null
    if (markdownEl) {
      // 克隆 DOM，移除代码头部（语言标签和复制按钮），只保留纯正文
      const clone = markdownEl.cloneNode(true) as HTMLElement
      clone.querySelectorAll('.code-header').forEach(el => el.remove())
      const htmlBlob = new Blob([clone.innerHTML], { type: 'text/html' })
      const textBlob = new Blob([clone.innerText], { type: 'text/plain' })
      await navigator.clipboard.write([
        new ClipboardItem({
          'text/html': htmlBlob,
          'text/plain': textBlob
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
  const bubbleEl = bubbleComp.value?.$el
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

// scroll 监听器按需注册：仅在选择按钮可见时注册，隐藏时移除
// 避免每个 MessageItem 实例常驻全局 scroll 监听导致长会话滚动卡顿
watch(selectionBtnVisible, visible => {
  if (visible) {
    document.addEventListener('scroll', hideSelectionBtn, true)
  } else {
    document.removeEventListener('scroll', hideSelectionBtn, true)
  }
})

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
  overlay.className = 'image-preview-overlay'
  const img = document.createElement('img')
  img.src = src
  img.className = 'image-preview-img'
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

// 报告问题：打开全局举报弹窗，带上本条 AI 内容（商店政策 11.16 合规）
function reportProblem() {
  report.openReport(props.message.content)
}

function regenerate() {
  const userMessageId = findPreviousUserMessage()
  if (userMessageId) {
    chatStore.regenerateMessage(userMessageId, settingsStore.searchMode)
  }
}
</script>

<style scoped>
.message-item {
  display: flex;
  gap: 14px;
  /* 行距节奏：22px 在信息密度与呼吸感之间取平衡（原 26px 偏松散） */
  margin-bottom: 22px;
  position: relative;
  width: 100%;
  /* 行宽撑满聊天区：AI 头像贴左缘、用户气泡贴右缘；
   * 水平留白统一由 MessageList.vue 滚动容器 24px 内边距提供，
   * 与底部输入舱左右边缘依然精确对齐；气泡宽度由内部 bubble-wrapper 控制 */
  align-items: flex-start;
  /* 渐进式性能优化：contain: content 等同于 contain: layout style paint，
     让浏览器跳过离屏 MessageItem 的布局计算和绘制工作，长会话下减少渲染开销。
     注意：不用 content-visibility: auto（项目记忆中该属性在流式场景导致高度跳转）。
     完整虚拟滚动（DynamicScroller）需单独迭代，因与 useScrollToBottom 的
     MutationObserver + RAF 滚动控制深度耦合。 */
  contain: content;
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
  justify-content: flex-start;
  animation: messageSlideIn 0.4s cubic-bezier(0.23, 1, 0.32, 1);
}

.message-item:not(.user) {
  justify-content: flex-start;
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

/* AI 头像品牌微光环：科幻感的点睛细节，与欢迎页 LOGO 色环呼应；
 * 用户头像保持素净，形成"机器有光、人无光"的微妙身份差异 */
.message-item:not(.user) .message-avatar img {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-primary) 18%, transparent);
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
  /* 微信式：AI 气泡贴左侧头像、限宽留白——不再通栏拉满，
   * 与用户侧 75% 上限形成左右镜像的对话感 */
  max-width: 72%;
  align-items: flex-start;
}

/* 动作条 hover 显隐：基础样式在 MessageActions 内，
   此处以 :deep 穿透表达跨组件的祖先 hover 关系 */
.message-item:hover :deep(.msg-actions) {
  opacity: 1;
  transform: translateY(0);
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
  /* 晨读：基于链接本色（苔绿）加深一档，替代失效的 --link-hover-light */
  color: color-mix(in srgb, var(--link-light) 78%, black);
  text-decoration: underline;
}

:global(.dark) :deep(.citation-link) {
  color: var(--link-dark);
}

:global(.dark) :deep(.citation-link:hover) {
  /* 夜读：基于链接本色（浅苔绿）提亮一档，替代失效的 --link-hover-dark */
  color: color-mix(in srgb, var(--link-dark) 75%, white);
}

.selection-copy-btn {
  position: fixed;
  z-index: 1000;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  /* 实色浮按钮：双主题下可读 */
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
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.selection-copy-btn:hover {
  background: var(--bg-hover);
  border-color: var(--accent-primary);
  color: var(--accent-primary);
}
</style>

<style>
/* 图片预览遮罩（双主题一致的深色遮罩，确保浅色图片可读） */
.image-preview-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.8);
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: zoom-out;
}

.image-preview-img {
  max-width: 90%;
  max-height: 90%;
  border-radius: var(--border-radius-md);
  box-shadow: var(--shadow-lg);
}
</style>
