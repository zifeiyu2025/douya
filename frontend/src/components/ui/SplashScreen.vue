<template>
  <Transition name="splash" @after-leave="$emit('complete')">
    <div v-if="visible" class="splash-screen" style="--wails-draggable:drag">
      <!-- SVG 装饰层：网格 + 多层弧线，全部在 SVG 内部，不遮挡文字 -->
      <svg class="splash-deco" width="400" height="400" viewBox="0 0 400 400" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <!-- 同心圆装饰 -->
        <circle cx="200" cy="200" r="180" stroke="currentColor" stroke-width="1" opacity="0.05" />
        <circle cx="200" cy="200" r="140" stroke="currentColor" stroke-width="1" opacity="0.08" />
        <circle cx="200" cy="200" r="100" stroke="currentColor" stroke-width="1" opacity="0.1" />

        <!-- 外层旋转弧线（慢速） -->
        <circle cx="200" cy="200" r="180" stroke="currentColor" stroke-width="1.5"
          stroke-linecap="round"
          stroke-dasharray="60 360"
          class="deco-ring-outer" opacity="0.4" />

        <!-- 中层旋转弧线（中速，反向） -->
        <circle cx="200" cy="200" r="140" stroke="currentColor" stroke-width="2"
          stroke-linecap="round"
          stroke-dasharray="40 280"
          class="deco-ring-mid" opacity="0.5" />

        <!-- 网格点缀（四个角的十字标记） -->
        <g opacity="0.15" stroke="currentColor" stroke-width="1">
          <path d="M40 40 L40 55 M40 40 L55 40" />
          <path d="M360 40 L360 55 M360 40 L345 40" />
          <path d="M40 360 L40 345 M40 360 L55 360" />
          <path d="M360 360 L360 345 M360 360 L345 360" />
        </g>
      </svg>

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
/* 整个启动屏支持窗口拖动（--wails-draggable:drag 在 template 内联） */
.splash-screen {
  position: fixed;
  inset: 0;
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-primary);
  overflow: hidden;
  /* 微妙的径向渐变背景，营造氛围（不影响文字可见性） */
  background-image: radial-gradient(circle at 50% 50%, color-mix(in srgb, var(--accent-primary) 4%, transparent) 0%, transparent 70%);
}

/* ===== SVG 装饰层 =====
 * 全部在 SVG 内部，不用 absolute 覆盖层，避免 WebView2 堆叠 bug
 * currentColor 跟随 .splash-screen 的 color（继承 accent-primary）
 */
.splash-deco {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: var(--accent-primary);
  pointer-events: none;
  /* 装饰层在内容之下（DOM 顺序 + 默认堆叠） */
  z-index: 0;
}

/* 外层弧线：60s 慢速旋转 */
.deco-ring-outer {
  transform-origin: 200px 200px;
  animation: spin 60s linear infinite;
  will-change: transform;
}

/* 中层弧线：40s 反向旋转 */
.deco-ring-mid {
  transform-origin: 200px 200px;
  animation: spin-reverse 40s linear infinite;
  will-change: transform;
}

@keyframes spin-reverse {
  to { transform: rotate(-360deg); }
}

.splash-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 24px;
  position: relative;
  z-index: 1;
}

/* Logo 区域
 * 不在 .splash-logo 上用 filter:drop-shadow，否则会模糊整个 SVG 包括内部 <image>
 * 发光效果改到具体的 circle 元素上
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
  filter: drop-shadow(0 0 6px currentColor);
}

@keyframes draw-circle {
  to {
    stroke-dashoffset: 0;
  }
}

/* ===== 品牌标识 ===== */
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
  /* 阴影色从 --accent-primary 派生，自动跟随主题切换（亮/暗一套规则） */
  text-shadow: 0 0 12px color-mix(in srgb, var(--accent-primary) 40%, transparent);
  animation: title-glow 2.5s ease-in-out infinite;
}

@keyframes title-glow {
  0%, 100% {
    text-shadow: 0 0 12px color-mix(in srgb, var(--accent-primary) 40%, transparent);
  }
  50% {
    text-shadow: 0 0 20px color-mix(in srgb, var(--accent-primary) 60%, transparent);
  }
}

.splash-subtitle {
  font-size: 13px;
  color: var(--text-secondary);
  letter-spacing: 3px;
  padding-left: 3px;
}

/* ===== 进度条已移除（用户要求）===== */

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
  color: var(--text-primary);
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

/* 尊重用户的减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .deco-ring-outer,
  .deco-ring-mid,
  .logo-spinner-ring,
  .splash-title {
    animation: none;
  }
}
</style>
