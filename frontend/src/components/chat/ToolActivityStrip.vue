<!--
  ToolActivityStrip: 工具执行时间线（Agent / 搜索工具的瞬态活动记录）。
  业界 agent 应用的标配 UI（对齐 Claude Code / Cursor 的 tool use block）：
  每个工具调用一行，实时翻转状态（运行中 → 成功/失败/被拒绝/待审批）。
  条目为生成期瞬态，随消息完成由 store 清空。
-->
<template>
  <div v-if="activities.length > 0" class="tool-activity-strip" role="status" aria-live="polite">
    <div class="tas-head">
      <span class="tas-title">工具执行</span>
      <span class="tas-count">{{ activities.length }}</span>
    </div>
    <div class="tas-list">
      <div v-for="a in activities" :key="a.toolCallId" class="tas-item" :class="'tas-' + a.status">
        <span class="tas-status-dot" :title="statusText(a.status)">
          <svg
            v-if="a.status === 'ok'"
            width="11"
            height="11"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="3"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M20 6L9 17l-5-5" />
          </svg>
          <svg
            v-else-if="a.status === 'failed'"
            width="11"
            height="11"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="3"
            stroke-linecap="round"
          >
            <path d="M18 6L6 18M6 6l12 12" />
          </svg>
          <svg
            v-else-if="a.status === 'denied'"
            width="11"
            height="11"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
          >
            <circle cx="12" cy="12" r="9" />
            <path d="M5.5 5.5l13 13" />
          </svg>
          <svg
            v-else-if="a.status === 'pending_approval'"
            width="11"
            height="11"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M12 9v4m0 4h.01" />
            <circle cx="12" cy="12" r="9" />
          </svg>
          <span v-else class="tas-spinner"></span>
        </span>
        <span class="tas-name">{{ toolLabel(a.tool) }}</span>
        <span class="tas-args" :title="a.argsPreview">{{ a.argsPreview }}</span>
        <span v-if="a.status === 'pending_approval'" class="tas-flag tas-flag--warn">待审批</span>
        <span v-if="a.durationMs != null" class="tas-duration">
          {{ formatDuration(a.durationMs) }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ToolActivity } from '../../types/chat'

defineOptions({ name: 'ToolActivityStrip' })

defineProps<{
  activities: ToolActivity[]
}>()

// 已知工具的中文标签（llama.cpp 内置 agent 工具 + 自实现搜索）；
// MCP 工具名无映射时回退显示原始名
const TOOL_LABELS: Record<string, string> = {
  read_file: '读取文件',
  write_file: '写入文件',
  edit_file: '编辑文件',
  exec_shell_command: '执行命令',
  grep_search: '内容搜索',
  file_glob_search: '文件搜索',
  get_info: '运行时信息',
  search: '联网搜索'
}

function toolLabel(name: string): string {
  return TOOL_LABELS[name] || name
}

function statusText(status: ToolActivity['status']): string {
  switch (status) {
    case 'running':
      return '执行中'
    case 'pending_approval':
      return '等待审批'
    case 'ok':
      return '已完成'
    case 'failed':
      return '失败'
    case 'denied':
      return '已拒绝'
  }
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}
</script>

<style scoped>
.tool-activity-strip {
  margin: 8px 0;
  padding: 8px 12px;
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-md);
  font-size: 12px;
}

.tas-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  font-weight: 600;
  color: var(--text-muted);
  font-size: 11px;
  letter-spacing: 0.04em;
}

.tas-count {
  padding: 0 6px;
  background: var(--border-light);
  border-radius: var(--border-radius-md);
  font-variant-numeric: tabular-nums;
}

.tas-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.tas-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  line-height: 1.6;
}

.tas-status-dot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  color: var(--text-muted);
}

.tas-ok .tas-status-dot {
  color: var(--success-color);
}

.tas-failed .tas-status-dot,
.tas-denied .tas-status-dot {
  color: var(--error-color);
}

.tas-pending_approval .tas-status-dot {
  color: var(--accent-primary);
}

.tas-spinner {
  width: 9px;
  height: 9px;
  border: 1.5px solid var(--border-light);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: tas-spin 0.8s linear infinite;
}

@keyframes tas-spin {
  to {
    transform: rotate(360deg);
  }
}

.tas-name {
  flex-shrink: 0;
  font-weight: 600;
  color: var(--text-primary);
}

.tas-args {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--text-muted);
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
  font-size: 11px;
}

.tas-flag {
  flex-shrink: 0;
  padding: 0 6px;
  border-radius: var(--border-radius-md);
  font-size: 10px;
}

.tas-flag--warn {
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
}

.tas-duration {
  flex-shrink: 0;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}
</style>
