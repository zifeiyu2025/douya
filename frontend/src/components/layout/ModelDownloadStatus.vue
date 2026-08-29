<!--
  ModelDownloadStatus: 模型下载全局悬浮状态卡。
  从设置页"模型下载"面板抽出、脱离设置页单独显示——用户/审核员在任意页面
  （聊天、设置其他分区）都能看到下载进度与下载后的自动加载状态，不必守在下载面板。
  状态与事件订阅集中在 modelDownload store，本组件只做展示。
-->
<template>
  <Transition name="mds">
    <div v-if="store.visible" class="mds-card" role="status" aria-live="polite">
      <div class="mds-head">
        <span class="mds-title">模型下载</span>
        <button class="mds-close" aria-label="关闭下载状态" @click="store.dismiss()">×</button>
      </div>

      <!-- 进行中：进度条列表 -->
      <div v-if="store.hasActive" class="mds-body">
        <div v-for="p in store.activeItems" :key="p.file_path" class="mds-item">
          <div class="mds-item__head">
            <span class="mds-item__file" :title="p.file_path">{{ p.file_path }}</span>
            <span class="mds-item__pct">{{ Math.round(p.percent) }}%</span>
          </div>
          <n-progress
            type="line"
            :percentage="p.status === 'waiting' ? 0 : Math.round(p.percent)"
            indicator-placement="inside"
            status="default"
            :height="6"
          />
          <div class="mds-item__meta">
            <span>{{ downloadStatusText(p.status) }}</span>
            <span v-if="p.status === 'downloading' && p.total_bytes > 0">
              {{ formatSize(p.downloaded) }} / {{ formatSize(p.total_bytes) }}
            </span>
          </div>
          <div v-if="p.status === 'failed' && p.error" class="mds-item__error">{{ p.error }}</div>
        </div>
      </div>

      <!-- 完成/失败结果 -->
      <div v-else-if="store.lastComplete" class="mds-body">
        <template v-if="store.lastComplete.success">
          <div
            class="mds-status"
            :class="store.activateFailed ? 'mds-status--err' : 'mds-status--ok'"
          >
            {{ doneText }}
          </div>
          <n-spin
            v-if="
              store.lastComplete.activate === 'auto' && !store.modelReady && !store.activateFailed
            "
            size="small"
          />
          <div v-if="showRestartFallback" class="mds-actions">
            <button class="mds-btn" :disabled="store.restarting" @click="store.restartApp()">
              {{ store.restarting ? '正在重启…' : '重启应用' }}
            </button>
          </div>
        </template>
        <template v-else>
          <div class="mds-status mds-status--err">下载中断：{{ failureText }}</div>
          <div class="mds-hint">已下载的部分已保留断点，重试将从断点继续，无需从头下载</div>
          <div class="mds-actions">
            <button class="mds-btn" :disabled="store.retrying" @click="store.retry()">
              {{ store.retrying ? '正在重试…' : '重试下载' }}
            </button>
          </div>
        </template>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed, onUnmounted, watch } from 'vue'
import { NProgress, NSpin } from 'naive-ui'
import { useModelDownloadStore } from '../../stores/modelDownload'
import { downloadStatusText, formatSize } from '../../utils/model'
import { logError } from '../../utils/logger'

defineOptions({ name: 'ModelDownloadStatus' })

const store = useModelDownloadStore()

// 完成态主文案：auto=自动加载中/已就绪，listed=已入列表，restart/失败兜底=需重启
const doneText = computed(() => {
  const result = store.lastComplete
  if (!result) return ''
  if (result.activate === 'auto') {
    if (store.activateFailed) {
      return `模型自动加载失败：${result.error || '模型可能不兼容，可尝试重启应用或换一个量化版本'}`
    }
    if (store.modelReady) return '模型已就绪，返回对话即可开始使用 🎉'
    return '下载完成，正在自动加载，无需重启…'
  }
  if (result.activate === 'listed') return '下载完成，已加入顶部模型列表，可随时切换使用'
  return '下载完成，重启应用后即可加载使用'
})

// 重启兜底按钮：非 auto 模式（后端明确要求重启）或自动加载失败时显示
const showRestartFallback = computed(() => {
  const result = store.lastComplete
  if (!result?.success) return false
  return result.activate !== 'auto' || store.activateFailed
})

// 失败文案：优先完成事件携带的错误，其次各进度项里的错误详情
const failureText = computed(() => {
  const result = store.lastComplete
  if (result?.error) return result.error
  const failed = Object.values(store.progressMap).find(p => p.error)
  return failed?.error || '未知错误'
})

// 自动加载成功后 8 秒自动收起悬浮卡（成功无需打扰，用户没看到也无妨）
let autoHideTimer: ReturnType<typeof setTimeout> | null = null
watch(
  () => store.lastComplete?.success && store.lastComplete.activate === 'auto' && store.modelReady,
  ready => {
    if (autoHideTimer) {
      clearTimeout(autoHideTimer)
      autoHideTimer = null
    }
    if (ready) {
      autoHideTimer = setTimeout(() => {
        try {
          store.dismiss()
        } catch (e) {
          logError('自动收起下载状态卡失败', e)
        }
      }, 8000)
    }
  }
)
onUnmounted(() => {
  if (autoHideTimer) clearTimeout(autoHideTimer)
})
</script>

<style scoped>
/* 悬浮于右下角：低于切换遮罩/退出遮罩/启动屏（z-index 1000+），不挡模态流程 */
.mds-card {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 900;
  width: 320px;
  max-width: calc(100vw - 40px);
  padding: 12px 14px;
  background: var(--surface-panel);
  border: 1px solid var(--border-light);
  border-radius: 10px;
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.18);
}

.mds-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.mds-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
  letter-spacing: 0.04em;
}

.mds-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  color: var(--text-muted);
  background: transparent;
  border: none;
  border-radius: var(--border-radius-sm);
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
}

.mds-close:hover {
  color: var(--text-primary);
  background: var(--border-light);
}

.mds-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.mds-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.mds-item__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.mds-item__file {
  overflow: hidden;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mds-item__pct {
  flex-shrink: 0;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--text-muted);
}

.mds-item__meta {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-muted);
}

.mds-item__error,
.mds-status--err {
  color: var(--error-color);
}

.mds-status {
  font-size: 13px;
  font-weight: 600;
  word-break: break-all;
}

.mds-status--ok {
  color: var(--success-color);
}

.mds-status--err {
  font-weight: 500;
}

.mds-hint {
  font-size: 11px;
  color: var(--text-muted);
}

.mds-actions {
  display: flex;
  justify-content: flex-end;
}

.mds-btn {
  padding: 5px 14px;
  font-size: 12px;
  font-weight: 600;
  color: var(--accent-primary);
  background: transparent;
  border: 1px solid var(--accent-primary);
  border-radius: 6px;
  cursor: pointer;
  transition:
    color 0.2s,
    background 0.2s;
}

.mds-btn:hover:not(:disabled) {
  color: #fff;
  background: var(--accent-primary);
}

.mds-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* 进出场：右下角轻微浮入 */
.mds-enter-active,
.mds-leave-active {
  transition:
    opacity 0.2s ease,
    transform 0.2s ease;
}

.mds-enter-from,
.mds-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
