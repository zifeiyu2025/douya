// 工具审批请求队列：Agent 模式硬门禁的前端侧状态。
// 聊天 store 收到 tool_approval_request 事件后 push 到这里，
// 全局审批弹窗（ToolApprovalModal）消费队列展示，用户决定后经 Wails 回传后端。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { wails } from '../services/wails'
import { logError } from '../utils/logger'

/** 一条待审批的工具调用请求 */
export interface ToolApprovalRequest {
  convId: string
  toolCallId: string
  tool: string
  displayName: string
  risk: 'safe' | 'write' | 'unknown' | 'all'
  arguments: string
}

export const useToolApprovalStore = defineStore('toolApproval', () => {
  // 请求队列：多个工具同时待审批时逐个弹出（后端门禁按序等待，天然串行）
  const queue = ref<ToolApprovalRequest[]>([])
  // 当前弹窗展示的请求（队首）
  const current = computed(() => queue.value[0] ?? null)
  const resolving = ref(false)
  // 提交失败（如请求已过期）时展示的错误
  const resolveError = ref('')

  function push(req: ToolApprovalRequest) {
    // 同一 tool_call_id 去重（事件重放保护）
    if (queue.value.some(q => q.toolCallId === req.toolCallId)) return
    queue.value.push(req)
    resolveError.value = ''
  }

  /** 回传用户决定并弹出下一条 */
  async function resolve(approved: boolean, remember: boolean) {
    const req = current.value
    if (!req || resolving.value) return
    resolving.value = true
    resolveError.value = ''
    try {
      await wails.approveToolCall(req.toolCallId, approved, remember)
      queue.value.shift()
    } catch (e) {
      // 请求已超时/取消：移除并提示，避免弹窗卡死
      resolveError.value = String(e || '该审批请求已过期')
      logError('工具审批回传失败', e)
      queue.value.shift()
    } finally {
      resolving.value = false
    }
  }

  /** 忽略当前请求（仅关闭弹窗；后端 120s 超时后会自动拒绝） */
  function dismiss() {
    queue.value.shift()
  }

  return { queue, current, resolving, resolveError, push, resolve, dismiss }
})
