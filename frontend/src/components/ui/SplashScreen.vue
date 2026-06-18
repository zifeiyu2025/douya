<template>
  <Transition name="splash" @after-leave="$emit('complete')">
    <div v-if="visible" class="splash-screen">
      <div class="splash-content">
        <!-- Logo + 无限旋转弧线 -->
        <div class="splash-logo">
          <svg width="72" height="72" viewBox="0 0 72 72" fill="none" xmlns="http://www.w3.org/2000/svg">
            <!-- 底层静态淡圈 -->
            <circle cx="36" cy="36" r="34" stroke="currentColor" stroke-width="2" opacity="0.12" />
            <!-- 旋转弧线 -->
            <circle cx="36" cy="36" r="34" stroke="currentColor" stroke-width="2.5"
              stroke-linecap="round"
              stroke-dasharray="78 136"
              class="logo-spinner-ring" />
            <!-- Logo 图标 -->
            <image x="10" y="10" width="52" height="52" :href="appLogo" />
          </svg>
        </div>

        <!-- 品牌标识 -->
        <div class="splash-brand">
          <div class="splash-title">豆芽</div>
          <div class="splash-subtitle">本地 AI 聊天助手</div>
        </div>

        <!-- 状态文字 -->
        <div class="splash-status">
          <span class="status-text">{{ stageText }}</span>
          <span v-if="modelName && stage !== 'done' && stage !== 'failed'" class="status-model">{{ modelName }}</span>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import appLogo from '../../assets/images/appicon.png'

const props = withDefaults(defineProps<{
  visible: boolean
  stage: string
  modelName: string
  progress: number
}>(), {
  visible: false,
  stage: 'idle',
  modelName: '',
  progress: 0,
})

defineEmits<{
  complete: []
}>()

const stageText = computed(() => {
  const map: Record<string, string> = {
    idle: '初始化中...',
    preparing: '准备启动引擎...',
    loading: '加载模型中...',
    waiting: '初始化模型...',
    detecting: '检测模型能力...',
    done: '就绪',
    failed: '加载失败',
    rolling_back: '回滚中...',
  }
  return map[props.stage] || '加载中...'
})
</script>

<style scoped>
.splash-screen {
  position: fixed;
  inset: 0;
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-primary);
}

.splash-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 28px;
}

/* Logo 旋转弧线 */
.splash-logo {
  color: var(--accent-primary);
}

.logo-spinner-ring {
  transform-origin: 36px 36px;
  animation: spin 1.4s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* 品牌标识 */
.splash-brand {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.splash-title {
  font-size: 38px;
  font-weight: 700;
  color: var(--accent-primary);
  letter-spacing: 6px;
  /* 标题左侧补偿字距，视觉居中 */
  padding-left: 6px;
}

.splash-subtitle {
  font-size: 13px;
  color: var(--text-secondary);
  letter-spacing: 3px;
  padding-left: 3px;
}

/* 状态文字 */
.splash-status {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  min-height: 34px;
}

.status-text {
  font-size: 13px;
  color: var(--text-secondary);
}

.status-model {
  font-size: 12px;
  color: var(--text-muted);
  max-width: 240px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 进出场过渡 */
.splash-enter-active {
  transition: opacity 0.4s ease;
}

.splash-leave-active {
  transition: opacity 0.6s ease, transform 0.6s cubic-bezier(0.4, 0, 0.2, 1);
}

.splash-enter-from {
  opacity: 0;
}

.splash-leave-to {
  opacity: 0;
  transform: translateY(-16px);
}
</style>
