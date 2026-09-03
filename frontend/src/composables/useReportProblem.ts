/**
 * useReportProblem.ts — AI 内容举报（微软商店政策 11.16 合规）
 *
 * 模块级单例状态：MessageItem 点击"报告问题"→ openReport(content)，
 * ChatView 挂载的 <ReportDialog /> 读取同一份状态渲染举报弹窗。
 * 与 useTTS 的"模块级单例"模式一致，避免每出现一条消息就挂一个弹窗组件。
 */
import { ref } from 'vue'

const visible = ref(false)
const content = ref('')

export function useReportProblem() {
  /** 打开举报弹窗并带入被举报的 AI 内容 */
  function openReport(text: string) {
    content.value = text || ''
    visible.value = true
  }

  /** 关闭举报弹窗（不重置已填内容，方便下次复用） */
  function closeReport() {
    visible.value = false
  }

  return { visible, content, openReport, closeReport }
}
