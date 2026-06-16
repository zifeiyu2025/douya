<template>
  <Transition name="splash" @after-leave="$emit('complete')">
    <div v-if="visible" class="splash-screen">
      <div class="splash-content">
        <!-- 品牌标识 -->
        <div class="splash-brand">
          <div class="splash-logo">
            <img :src="appiconUrl" alt="豆芽" class="splash-logo-img" />
            <svg width="56" height="56" viewBox="0 0 56 56" fill="none" xmlns="http://www.w3.org/2000/svg"
              class="logo-progress-ring-svg">
              <circle cx="28" cy="28" r="26" stroke="currentColor" stroke-width="2.5" opacity="0.3" />
              <circle cx="28" cy="28" r="26" stroke="currentColor" stroke-width="2.5"
                stroke-dasharray="163.36" :stroke-dashoffset="163.36 - (163.36 * progress / 100)"
                class="logo-progress-ring" />
            </svg>
          </div>
          <div class="splash-title">豆芽</div>
          <div class="splash-subtitle">本地 AI 聊天助手</div>
        </div>

        <!-- 进度条 -->
        <div class="splash-progress">
          <div class="progress-track">
            <div class="progress-fill" :style="{ width: progress + '%' }" />
          </div>
          <div class="progress-status">
            <span class="status-text">{{ stageText }}</span>
            <span v-if="modelName && stage !== 'done' && stage !== 'failed'" class="status-model">{{ modelName }}</span>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import appiconUrl from '../../assets/images/appicon.png'

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
  gap: 40px;
}

/* 品牌标识 */
.splash-brand {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.splash-logo {
  position: relative;
  width: 56px;
  height: 56px;
  color: var(--accent-primary);
  animation: breathe 2.4s ease-in-out infinite;
}

.splash-logo-img {
  width: 40px;
  height: 40px;
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  border-radius: 12px;
  object-fit: contain;
}

.logo-progress-ring-svg {
  position: absolute;
  top: 0;
  left: 0;
}

.logo-progress-ring {
  transition: stroke-dashoffset 0.6s cubic-bezier(0.4, 0, 0.2, 1);
}

.splash-title {
  font-size: 42px;
  font-weight: 700;
  color: var(--accent-primary);
  letter-spacing: 4px;
  animation: breathe 2.4s ease-in-out infinite;
}

.splash-subtitle {
  font-size: 14px;
  color: var(--text-secondary);
  letter-spacing: 2px;
  margin-top: -4px;
}

@keyframes breathe {
  0%, 100% {
    opacity: 0.75;
  }
  50% {
    opacity: 1;
  }
}

/* 进度条 */
.splash-progress {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  width: 260px;
}

.progress-track {
  width: 100%;
  height: 3px;
  background: var(--border-color);
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: var(--accent-primary);
  border-radius: 2px;
  transition: width 0.6s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: 0 0 8px color-mix(in srgb, var(--accent-primary) 40%, transparent);
}

.progress-status {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
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

/* 过渡动画 */
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
