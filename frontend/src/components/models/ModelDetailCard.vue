<template>
  <div v-if="model" class="model-detail-card">
    <!-- 全名区：完整文件名，允许换行 -->
    <div class="card-title" :title="model.file_name || model.name">
      {{ model.file_name || model.name }}
    </div>

    <!-- 元数据徽章行：参数量规模 + 量化类型 + 默认标记 -->
    <div v-if="model.size_label || model.quant_type || model.is_default" class="badge-row">
      <span v-if="model.size_label" class="badge badge-accent">{{ model.size_label }}</span>
      <span v-if="model.quant_type" class="badge">{{ model.quant_type }}</span>
      <span v-if="model.is_default" class="badge badge-muted">默认</span>
    </div>

    <!-- 多模态能力行：仅展示具备的能力；全部缺失则标注纯文本模型 -->
    <div class="capability-row">
      <span v-if="model.mmproj_vision" class="capability">📷 图片理解</span>
      <span v-if="model.mmproj_audio" class="capability">🎤 音频输入</span>
      <span v-if="model.mmproj_video" class="capability">🎬 视频输入</span>
      <span
        v-if="!model.mmproj_vision && !model.mmproj_audio && !model.mmproj_video"
        class="capability capability-muted"
      >
        纯文本模型
      </span>
    </div>

    <!-- 键值详情区 -->
    <div class="detail-rows">
      <div class="detail-row">
        <span class="detail-label">文件大小</span>
        <span class="detail-value">
          {{ model.file_size_bytes ? formatFileSize(model.file_size_bytes) : '未知' }}
        </span>
      </div>
      <div class="detail-row">
        <span class="detail-label">加载状态</span>
        <span class="detail-value status-value">
          <span class="status-dot" :class="statusClass" />
          {{ statusText }}
        </span>
      </div>
      <div class="detail-row detail-row-path">
        <span class="detail-label">模型路径</span>
        <span class="detail-value path-value" :title="model.model_path">
          {{ model.model_path }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
// 模型详情卡片：展示 B-5 后端提供的真实 GGUF 元数据（参数量规模/量化类型/文件大小）
// 纯展示组件：不发起请求、不发事件，由父级（AppHeader）控制弹出时机
import type { ModelOption } from '../../services/wails'
import { formatFileSize } from '../../utils/format'

const props = defineProps<{ model: ModelOption | null }>()

const statusText = computed(() => {
  if (!props.model) return ''
  if (props.model.status === 'sleeping') return '休眠中'
  if (props.model.status === 'loading') return '加载中'
  return props.model.is_loaded ? '已加载' : '未加载'
})

const statusClass = computed(() => {
  if (!props.model) return ''
  if (props.model.status === 'loading') return 'loading'
  return props.model.is_loaded ? 'loaded' : 'idle'
})
</script>

<style scoped>
/* ===== 模型详情卡 =====
 * 视觉规格遵循 styles/tokens.css 设计令牌：
 * 玻璃底 + 细边框 + GitHub 蓝强调，克制精致
 */
.model-detail-card {
  width: 300px;
  /* 实底表面：naive popover 使用了 raw 关掉了默认浮层壳，
     卡片自身必须提供不透明背景，否则内容直接叠在页面背景上不可读 */
  background: var(--surface-panel);
  border: 1px solid var(--border-color);
  border-radius: var(--border-radius-md);
  box-shadow: var(--shadow-md);
  padding: 12px 14px;
  user-select: none;
}

.card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  line-height: 1.45;
  word-break: break-all;
  margin-bottom: 10px;
}

.badge-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
}

.badge {
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
}

.badge-accent {
  color: var(--accent-primary);
  border-color: color-mix(in srgb, var(--accent-primary) 35%, transparent);
  background: color-mix(in srgb, var(--accent-primary) 8%, transparent);
}

.badge-muted {
  color: var(--text-muted);
  font-weight: 500;
}

.capability-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--border-color);
}

.capability {
  font-size: 12px;
  color: var(--text-secondary);
}

.capability-muted {
  color: var(--text-muted);
}

.detail-rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 10px;
}

.detail-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
  font-size: 12px;
  line-height: 1.5;
}

.detail-label {
  flex-shrink: 0;
  width: 52px;
  color: var(--text-muted);
}

.detail-value {
  color: var(--text-primary);
  min-width: 0;
}

.status-value {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
}

.status-dot.loaded {
  background: var(--success-color);
}

.status-dot.loading {
  background: var(--accent-warning);
  animation: pulse 1.2s ease-in-out infinite;
}

.path-value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl; /* 路径过长时保留尾部（文件名端）可见 */
  text-align: left;
}

@keyframes pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}
</style>
