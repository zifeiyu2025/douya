/**
 * 上下文菜单 composable（右键菜单：剪切/复制/粘贴/全选）
 *
 * 生活类比：就像在文字上右键弹出的小工具箱——提供常用的文本操作快捷入口。
 * 菜单位置会自动检测边界，不会被屏幕边缘裁切。
 *
 * 从 ChatInput.vue 抽取（基于架构优化：ChatInput.vue 1789 行→拆分独立职责）：
 * - 菜单显隐和位置状态
 * - 剪切/复制/粘贴/全选操作
 * - 边界检测逻辑
 */
import { ref, nextTick, type Ref } from 'vue'
import type { MessageApi } from 'naive-ui'

/**
 * @param textareaRef textarea 元素引用
 * @param inputText 输入框文本（双向：粘贴时写入此 ref）
 * @param handlePaste 粘贴处理回调（处理文件类粘贴，由调用方提供）
 * @param message naive-ui 消息 API（用于错误提示）
 */
export function useContextMenu(
  textareaRef: Ref<HTMLTextAreaElement | null>,
  inputText: Ref<string>,
  handlePaste: (e: ClipboardEvent) => void,
  message: MessageApi
) {
  const contextMenuVisible = ref(false)
  const contextMenuX = ref(0)
  const contextMenuY = ref(0)
  const canCut = ref(false)
  const canCopy = ref(false)

  function handleContextMenu(e: MouseEvent) {
    e.preventDefault()
    const ta = textareaRef.value
    if (!ta) return
    // 检查是否有选中文本
    const start = ta.selectionStart
    const end = ta.selectionEnd
    canCut.value = start !== end
    canCopy.value = start !== end
    // 边界检测：确保菜单不超出视窗
    const menuWidth = 160
    const menuHeight = 180
    let x = e.clientX
    let y = e.clientY
    if (x + menuWidth > window.innerWidth) x = window.innerWidth - menuWidth - 4
    if (y + menuHeight > window.innerHeight) y = window.innerHeight - menuHeight - 4
    contextMenuX.value = x
    contextMenuY.value = y
    contextMenuVisible.value = true
  }

  function closeContextMenu() {
    contextMenuVisible.value = false
  }

  function ctxCut() {
    if (!canCut.value) return
    document.execCommand('cut')
    closeContextMenu()
  }

  function ctxCopy() {
    if (!canCopy.value) return
    document.execCommand('copy')
    closeContextMenu()
  }

  async function ctxPaste() {
    closeContextMenu()
    try {
      // 优先尝试读取剪贴板文件
      const clipboardItems = await navigator.clipboard.read()
      for (const item of clipboardItems) {
        for (const type of item.types) {
          if (
            type.startsWith('image/') ||
            type.startsWith('audio/') ||
            type.startsWith('video/') ||
            type === 'application/pdf'
          ) {
            const blob = await item.getType(type)
            const file = new File([blob], `pasted-${Date.now()}.${type.split('/')[1]}`, { type })
            // 模拟 paste 事件处理
            const fakeEvent = {
              clipboardData: { items: [{ kind: 'file', type, getAsFile: () => file }] },
              preventDefault: () => {}
            } as unknown as ClipboardEvent
            handlePaste(fakeEvent)
            return
          }
        }
      }
      // 没有文件，读取文本
      const text = await navigator.clipboard.readText()
      if (text) {
        const ta = textareaRef.value
        if (ta) {
          const start = ta.selectionStart
          const end = ta.selectionEnd
          inputText.value = inputText.value.slice(0, start) + text + inputText.value.slice(end)
          nextTick(() => {
            ta.selectionStart = ta.selectionEnd = start + text.length
            ta.focus()
          })
        }
      }
    } catch {
      message.warning('粘贴失败，请使用 Ctrl+V')
    }
  }

  function ctxSelectAll() {
    const ta = textareaRef.value
    if (ta) {
      ta.select()
      ta.focus()
    }
    closeContextMenu()
  }

  return {
    contextMenuVisible,
    contextMenuX,
    contextMenuY,
    canCut,
    canCopy,
    handleContextMenu,
    closeContextMenu,
    ctxCut,
    ctxCopy,
    ctxPaste,
    ctxSelectAll
  }
}
