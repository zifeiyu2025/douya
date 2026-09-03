<template>
  <div class="msg-actions" :class="{ 'user-actions': isUser, 'ai-actions': !isUser }">
    <div class="action-row">
      <span v-if="!isUser && tokensPerSecond > 0" class="token-speed">
        {{ tokensPerSecond }} t/s
      </span>
      <button class="action-btn" title="复制" @click="$emit('copy')">
        <AppIcon name="copy" class="action-icon" :size="14" />
        <span class="action-label">复制</span>
      </button>
      <!-- TTS 朗读按钮：仅 AI 消息显示，需启用 TTS 且流式生成中禁用 -->
      <button
        v-if="showTts"
        class="action-btn"
        :class="{ active: ttsActive }"
        :title="ttsActive ? '停止朗读' : '朗读'"
        :disabled="ttsDisabled"
        @click="$emit('speak')"
      >
        <AppIcon :name="ttsActive ? 'stop' : 'volume'" class="action-icon" :size="14" />
        <span class="action-label">
          {{ ttsActive ? '停止' : '朗读' }}
        </span>
      </button>
      <!-- 朗读后端徽标：仅当前正在朗读的消息显示，标明走的是在线/本地 -->
      <span
        v-if="ttsActive && ttsBackend"
        class="tts-backend-badge"
        :class="ttsBackend"
        :title="
          ttsBackend === 'online' ? '正在使用微软在线神经语音' : '正在使用本地 Web Speech 语音'
        "
      >
        {{ ttsBackend === 'online' ? '在线' : '本地' }}
      </span>
      <button v-if="canRegenerate" class="action-btn" title="重新生成" @click="$emit('regenerate')">
        <AppIcon name="regenerate" class="action-icon" :size="14" />
        <span class="action-label">重新生成</span>
      </button>
      <!-- 报告问题按钮：仅 AI 消息显示，用于举报 AI 生成的不当内容（商店政策 11.16 合规） -->
      <button v-if="!isUser" class="action-btn" title="报告问题" @click="$emit('report')">
        <AppIcon name="flag" class="action-icon" :size="14" />
        <span class="action-label">报告问题</span>
      </button>
      <!-- 编辑按钮：仅用户消息显示，保存后截断重生成 -->
      <button v-if="canEdit" class="action-btn" title="编辑" @click="$emit('edit')">
        <AppIcon name="edit" class="action-icon" :size="14" />
        <span class="action-label">编辑</span>
      </button>
      <button v-if="isUser" class="action-btn danger" title="删除" @click="$emit('delete')">
        <AppIcon name="trash" class="action-icon" :size="14" />
        <span class="action-label">删除</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
// 纯展示的动作条：所有业务判断由 MessageItem 计算后以 props 传入，
// 本组件只负责渲染并把用户意图以事件形式上报（intent emitter 模式）
defineProps<{
  isUser: boolean
  tokensPerSecond: number
  showTts: boolean
  ttsActive: boolean
  ttsDisabled: boolean
  ttsBackend: string | null
  canRegenerate: boolean
  canEdit: boolean
}>()

defineEmits<{
  copy: []
  speak: []
  regenerate: []
  report: []
  delete: []
  edit: []
}>()
</script>

<style scoped>
/* 以下样式自原 MessageItem 的动作区段逐字迁移（模板归属本组件） */
.msg-actions {
  margin-top: 5px;
  min-height: 30px;
  opacity: 0;
  transition:
    opacity 0.22s ease,
    transform 0.22s ease;
  transform: translateY(-4px);
}

.user-actions {
  display: flex;
  justify-content: flex-end;
}

.ai-actions {
  display: flex;
  justify-content: flex-start;
}

.action-row {
  display: flex;
  gap: 6px;
  align-items: center;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: none;
  border-radius: var(--border-radius-sm);
  /* 统一风格：透明底色 + 字体颜色（与删除按钮一致） */
  background: transparent;
  color: var(--text-muted);
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
  transition:
    color 0.15s ease,
    background-color 0.15s ease;
  line-height: 1;
  white-space: nowrap;
}

/* hover 时所有按钮统一用柔和半透明背景（不用实色 bg-hover，保持透明感） */
.action-btn:hover {
  background: color-mix(in srgb, var(--text-primary) 8%, transparent);
  color: var(--text-primary);
}

/* 删除按钮保留语义色：hover 时变红，提示危险操作 */
.action-btn.danger:hover {
  color: var(--accent-danger);
  background: var(--accent-r-soft);
}

/* 朗读按钮激活态：正在朗读时高亮显示（主色调背景） */
.action-btn.active {
  color: var(--accent-primary);
  background: var(--accent-tertiary);
}

/* TTS 后端徽标：朗读中显示，区分在线（苔绿）/本地（灰），圆角走阶梯 */
.tts-backend-badge {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  margin-left: 4px;
  font-size: 11px;
  line-height: 16px;
  border-radius: var(--border-radius-sm);
  font-weight: 500;
  white-space: nowrap;
  vertical-align: middle;
}

/* 移除 hex fallback；语义别名 --success-color 在 tokens.css 中指向未定义变量而失效，
 * 故直接使用真实令牌 --accent-success（竹青） */
.tts-backend-badge.online {
  color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 12%, transparent);
}

.tts-backend-badge.local {
  color: var(--text-secondary);
  background: color-mix(in srgb, var(--text-secondary) 12%, transparent);
}

.action-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.action-icon {
  width: 15px;
  height: 15px;
  flex-shrink: 0;
}

.action-label {
  font-size: 12.5px;
  line-height: 1;
}

.token-speed {
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1;
  padding: 6px 8px;
  white-space: nowrap;
  user-select: none;
}
</style>
