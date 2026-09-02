<template>
  <!-- 启动致命错误卡：后端无法继续启动时全屏展示原因，用户点击「退出」后应用关闭。
       因为这是致命错误，不允许点遮罩/按 Esc 关闭——只能点「退出」。 -->
  <n-modal
    :show="show"
    :mask-closable="false"
    :close-on-esc="false"
    preset="card"
    :title="payload?.title || '启动失败'"
    style="width: 560px; max-width: 92vw"
    :bordered="false"
    class="startup-error-modal"
  >
    <!-- 简述区：一句话说明严重程度 -->
    <div v-if="payload?.brief" class="error-brief">
      <n-icon size="18" color="var(--accent-danger)">
        <svg viewBox="0 0 24 24" fill="currentColor">
          <path
            d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"
          />
        </svg>
      </n-icon>
      <span>{{ payload.brief }}</span>
    </div>

    <!-- 详情区：多行原因 + 建议，支持换行 -->
    <div class="error-detail">
      {{ payload?.detail || '未知错误' }}
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button type="error" @click="onExit">退出</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { NButton, NIcon, NModal, NSpace } from 'naive-ui'

export interface StartupErrorCardPayload {
  title: string
  brief: string
  detail: string
}

defineProps<{
  show: boolean
  payload: StartupErrorCardPayload | null
}>()

const emit = defineEmits<{
  (e: 'exit'): void
}>()

function onExit() {
  emit('exit')
}
</script>

<style scoped>
.error-brief {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 13px;
  color: var(--n-text-color-2, #ccc);
  margin-bottom: 12px;
  /* 让图标跟首行文字对齐 */
  padding-top: 2px;
}

.error-detail {
  background: var(--n-color-target, rgba(232, 128, 128, 0.06));
  border: 1px solid var(--n-divider-color, rgba(255, 255, 255, 0.08));
  border-radius: var(--border-radius-md);
  padding: 12px 14px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--n-text-color, #fff);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 42vh;
  overflow-y: auto;
}
</style>
