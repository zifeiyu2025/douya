<template>
  <Transition name="splash" @after-leave="$emit('complete')">
    <div v-if="visible" class="splash-screen">
      <!-- HUD 扫描线背景层 -->
      <div class="splash-scanline" aria-hidden="true"></div>
      <div class="splash-content">
        <!-- Logo + 旋转弧线 -->
        <div class="splash-logo" :class="{ 'is-done': stage === 'done', 'is-failed': stage === 'failed' }">
          <svg width="72" height="72" viewBox="0 0 72 72" fill="none" xmlns="http://www.w3.org/2000/svg">
            <!-- 底层静态淡圈 -->
            <circle cx="36" cy="36" r="34" stroke="currentColor" stroke-width="2" opacity="0.12" />
            <!-- 旋转弧线（加载中） -->
            <circle v-if="stage !== 'done' && stage !== 'failed'" cx="36" cy="36" r="34" stroke="currentColor" stroke-width="2.5"
              stroke-linecap="round"
              stroke-dasharray="78 136"
              class="logo-spinner-ring" />
            <!-- 完成圆环 -->
            <circle v-else cx="36" cy="36" r="34" stroke="currentColor" stroke-width="2.5"
              stroke-linecap="round"
              class="logo-complete-ring" />
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
          <span class="status-text" :class="{ 'status-done': stage === 'done', 'status-failed': stage === 'failed' }">{{ stageText }}</span>
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
    done: '加载完成',
    failed: '加载失败',
    rolling_back: '回滚中...',
    'vram-warning': 'VRAM 不足警告，可能影响性能...',
    'spec-warning': '推测解码兼容性警告...',
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
  overflow: hidden;
}

/* ===== HUD 扫描线背景层 =====
 * 一条从上到下移动的渐变线，营造"系统扫描"氛围
 * 用 background-position 动画（扫描线效果比 transform 更合适）
 */
.splash-scanline {
  position: absolute;
  inset: 0;
  pointer-events: none;
  opacity: 0.4;
  background: linear-gradient(
    180deg,
    transparent 0%,
    transparent 45%,
    color-mix(in srgb, var(--accent-primary) 30%, transparent) 50%,
    transparent 55%,
    transparent 100%
  );
  background-size: 100% 300%;
  animation: scan-bg 3s linear infinite;
  will-change: background-position;
}

@keyframes scan-bg {
  0% { background-position: 0% 0%; }
  100% { background-position: 0% 100%; }
}

.splash-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 28px;
  position: relative;
  z-index: 1;
}

/* Logo 区域
 * 注意：不在 .splash-logo 上用 filter:drop-shadow，否则会模糊整个 SVG 包括内部 <image>
 * 发光效果改到具体的 circle 元素上（.logo-spinner-ring / .logo-complete-ring）
 */
.splash-logo {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--accent-primary);
  transition: color var(--transition-normal);
  animation: logo-enter 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.splash-logo.is-done {
  color: var(--accent-success);
}

.splash-logo.is-failed {
  color: var(--accent-danger);
}

@keyframes logo-enter {
  from {
    opacity: 0;
    transform: scale(0.8);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.logo-spinner-ring {
  transform-origin: 36px 36px;
  animation: spin 1.4s linear infinite;
  /* 仅圆环发光，不影响内部 Logo 图像 */
  filter: drop-shadow(0 0 4px currentColor);
}

/* 完成圆环动画 */
.logo-complete-ring {
  stroke-dasharray: 214;
  stroke-dashoffset: 214;
  animation: draw-circle 0.6s cubic-bezier(0.4, 0, 0.2, 1) forwards;
  /* 完成时圆环发光 */
  filter: drop-shadow(0 0 6px currentColor);
}

@keyframes draw-circle {
  to {
    stroke-dashoffset: 0;
  }
}

/* ===== 品牌标识：发光文字 ===== */
.splash-brand {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  animation: brand-enter 0.6s cubic-bezier(0.4, 0, 0.2, 1) 0.15s both;
}

@keyframes brand-enter {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.splash-title {
  font-size: 38px;
  font-weight: 700;
  color: var(--accent-primary);
  letter-spacing: 6px;
  padding-left: 6px;
  /* 文字发光效果 */
  text-shadow: 0 0 12px color-mix(in srgb, var(--accent-primary) 50%, transparent);
  animation: title-glow 2.5s ease-in-out infinite;
}

@keyframes title-glow {
  0%, 100% {
    text-shadow: 0 0 12px color-mix(in srgb, var(--accent-primary) 40%, transparent);
  }
  50% {
    text-shadow: 0 0 20px color-mix(in srgb, var(--accent-primary) 70%, transparent);
  }
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
  animation: status-enter 0.6s cubic-bezier(0.4, 0, 0.2, 1) 0.3s both;
}

@keyframes status-enter {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.status-text {
  font-size: 13px;
  color: var(--text-secondary);
  transition: color var(--transition-normal);
}

.status-done {
  color: var(--accent-success);
  font-weight: 600;
}

.status-failed {
  color: var(--accent-danger);
  font-weight: 600;
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
