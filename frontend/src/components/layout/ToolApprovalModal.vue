<!--
  ToolApprovalModal: Agent 工具执行审批弹窗（硬门禁的 UI 端）。
  业界成熟方案（Claude Code / Cursor permission prompt）：写操作与未验证的
  MCP 工具在执行前必须经用户确认；拒绝结果回传模型改道作答，而非中断对话。
  后端 120s 超时自动拒绝，弹窗不会永久阻塞循环。
-->
<template>
  <Teleport to="body">
    <Transition name="tam">
      <div v-if="store.current" class="tam-mask" role="dialog" aria-modal="true" aria-label="工具执行审批">
        <div class="tam-card">
          <div class="tam-head">
            <span class="tam-title">Agent 请求执行工具</span>
            <span class="tam-risk" :class="'tam-risk--' + store.current.risk">{{ riskText }}</span>
          </div>

          <div class="tam-tool">
            <span class="tam-tool-name">{{ store.current.displayName }}</span>
            <span class="tam-tool-id">{{ store.current.tool }}</span>
          </div>

          <pre class="tam-args">{{ prettyArgs }}</pre>

          <div v-if="store.resolveError" class="tam-error">{{ store.resolveError }}</div>

          <div class="tam-hint">
            只读操作通常安全；请检查参数中是否包含你不认可的路径或命令。
          </div>

          <div class="tam-actions">
            <button class="tam-btn" :disabled="store.resolving" @click="store.dismiss()">
              关闭
            </button>
            <button
              class="tam-btn tam-btn--danger"
              :disabled="store.resolving"
              @click="store.resolve(false, false)"
            >
              拒绝
            </button>
            <button
              class="tam-btn tam-btn--ghost"
              :disabled="store.resolving"
              @click="store.resolve(true, false)"
            >
              仅本次允许
            </button>
            <button
              class="tam-btn tam-btn--primary"
              :disabled="store.resolving"
              @click="store.resolve(true, true)"
            >
              本会话都允许
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useToolApprovalStore } from '../../stores/toolApproval'

defineOptions({ name: 'ToolApprovalModal' })

const store = useToolApprovalStore()

const riskText = computed(() => {
  switch (store.current?.risk) {
    case 'write':
      return '写操作'
    case 'unknown':
      return '未验证工具'
    case 'all':
      return '全部确认模式'
    default:
      return '需确认'
  }
})

// 参数格式化预览：JSON 展开 + 兜底原文
const prettyArgs = computed(() => {
  const raw = store.current?.arguments || ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
})
</script>

<style scoped>
.tam-mask {
  position: fixed;
  inset: 0;
  z-index: 950; /* 高于悬浮下载卡(900)，低于切换/退出遮罩(1000) */
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.45);
}

.tam-card {
  width: 480px;
  max-width: calc(100vw - 48px);
  padding: 18px 20px;
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  border-radius: 12px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.28);
}

.tam-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.tam-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-primary);
}

.tam-risk {
  padding: 2px 10px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 600;
}

.tam-risk--write {
  color: #fff;
  background: var(--error-color);
}

.tam-risk--unknown {
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 14%, transparent);
}

.tam-risk--all {
  color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
}

.tam-tool {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 10px;
}

.tam-tool-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.tam-tool-id {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
}

.tam-args {
  max-height: 180px;
  margin: 0 0 10px;
  padding: 10px 12px;
  overflow: auto;
  background: color-mix(in srgb, var(--border-light) 30%, transparent);
  border: 1px solid var(--border-light);
  border-radius: 8px;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-all;
}

.tam-error {
  margin-bottom: 8px;
  color: var(--error-color);
  font-size: 12px;
}

.tam-hint {
  margin-bottom: 14px;
  color: var(--text-muted);
  font-size: 12px;
}

.tam-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.tam-btn {
  padding: 7px 14px;
  border: 1px solid var(--border-light);
  border-radius: 8px;
  background: transparent;
  color: var(--text-primary);
  font-size: 13px;
  cursor: pointer;
  transition:
    background 0.15s,
    color 0.15s,
    border-color 0.15s;
}

.tam-btn:hover:not(:disabled) {
  border-color: var(--text-muted);
}

.tam-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.tam-btn--danger:hover:not(:disabled) {
  color: var(--error-color);
  border-color: var(--error-color);
}

.tam-btn--ghost:hover:not(:disabled) {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
}

.tam-btn--primary {
  color: #fff;
  background: var(--accent-primary);
  border-color: var(--accent-primary);
}

.tam-btn--primary:hover:not(:disabled) {
  opacity: 0.88;
}

/* 进出场 */
.tam-enter-active,
.tam-leave-active {
  transition: opacity 0.18s ease;
}

.tam-enter-active .tam-card,
.tam-leave-active .tam-card {
  transition: transform 0.18s ease;
}

.tam-enter-from,
.tam-leave-to {
  opacity: 0;
}

.tam-enter-from .tam-card,
.tam-leave-to .tam-card {
  transform: scale(0.97);
}
</style>
