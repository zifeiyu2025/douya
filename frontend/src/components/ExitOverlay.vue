<template>
  <Transition name="exit-overlay">
    <div v-if="show" class="switch-overlay switch-overlay--exit">
      <!-- 退出动效：渐变收缩消散装饰 -->
      <svg
        class="exit-deco"
        width="360"
        height="360"
        viewBox="0 0 360 360"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <!-- 三层同心圆，从外到内逐渐消失 -->
        <circle
          cx="180"
          cy="180"
          r="160"
          stroke="currentColor"
          stroke-width="1"
          opacity="0.08"
          class="exit-ring-outer"
        />
        <circle
          cx="180"
          cy="180"
          r="120"
          stroke="currentColor"
          stroke-width="1.5"
          opacity="0.15"
          class="exit-ring-mid"
        />
        <circle
          cx="180"
          cy="180"
          r="80"
          stroke="currentColor"
          stroke-width="2"
          opacity="0.25"
          class="exit-ring-inner"
        />
      </svg>
      <div class="switch-overlay-content">
        <!-- 退出动效：仅 LOGO（简洁） -->
        <div class="exit-logo-wrapper">
          <img :src="appLogo" alt="豆芽" class="exit-logo-img" />
        </div>
        <div class="switch-model-name">正在退出豆芽</div>
        <div class="switch-progress-msg">{{ exitProgress }}</div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import appLogo from '../assets/images/appicon.png'

// 退出确认 overlay：纯展示组件
// - show：是否显示（对应 App.vue 中 useAppLifecycle 的 showExitOverlay）
// - exitProgress：退出进度文案（对应 exitProgress）
defineProps<{
  show: boolean
  exitProgress: string
}>()
</script>

<style scoped>
/* 基础 overlay 样式（与 ModelSwitchOverlay 共用，scoped 各自定义一份） */
.switch-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  /* 实色背景：主题对齐 */
  background: var(--bg-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  pointer-events: auto;
  /* 径向渐变营造氛围 */
  background-image: radial-gradient(
    circle at 50% 50%,
    color-mix(in srgb, var(--accent-primary) 4%, transparent) 0%,
    transparent 70%
  );
}

.switch-overlay-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  position: relative;
  z-index: 1;
}

.switch-model-name {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
  text-align: center;
  max-width: 320px;
  word-break: break-word;
}

.switch-progress-msg {
  font-size: 13px;
  color: var(--text-secondary);
}

/* ===== 退出动效（exit-overlay）独特设计 =====
 * 与切换动效区分：装饰层向中心收缩 + 图标 + 文字渐变色
 */
.switch-overlay--exit {
  /* 退出语义色用 --accent-danger 派生，移除硬编码 rgba */
  background-image: radial-gradient(
    circle at 50% 50%,
    color-mix(in srgb, var(--accent-danger) 4%, transparent) 0%,
    transparent 70%
  );
}

.exit-deco {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: var(--accent-primary);
  pointer-events: none;
  z-index: 0;
  /* 整体缓慢收缩 */
  animation: exit-deco-shrink 1.2s ease-out forwards;
}

@keyframes exit-deco-shrink {
  from {
    transform: translate(-50%, -50%) scale(1.4);
    opacity: 0;
  }
  30% {
    opacity: 1;
  }
  to {
    transform: translate(-50%, -50%) scale(0.6);
    opacity: 0.3;
  }
}

/* 三层圆环从外到内依次消失 */
.exit-ring-outer {
  animation: exit-ring-fade 0.8s ease-out 0.2s forwards;
}
.exit-ring-mid {
  animation: exit-ring-fade 0.8s ease-out 0.4s forwards;
}
.exit-ring-inner {
  animation: exit-ring-fade 0.8s ease-out 0.6s forwards;
}

@keyframes exit-ring-fade {
  to {
    opacity: 0;
  }
}

.exit-center-dot {
  animation: exit-dot-pulse 1s ease-in-out infinite;
}

@keyframes exit-dot-pulse {
  0%,
  100% {
    opacity: 0.4;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.5);
  }
}

/* ===== 退出动效：仅 LOGO（简洁） =====
 * LOGO 居中显示，缓慢变淡传递离开感
 */
.exit-logo-wrapper {
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: exit-logo-enter 0.6s cubic-bezier(0.34, 1.56, 0.64, 1);
}

@keyframes exit-logo-enter {
  from {
    opacity: 0;
    transform: scale(0.8);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.exit-logo-img {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  object-fit: cover;
  /* 退出时 LOGO 缓慢变淡 */
  animation: exit-logo-fade 1.2s ease-out 0.3s forwards;
}

@keyframes exit-logo-fade {
  from {
    opacity: 1;
    transform: scale(1);
  }
  to {
    opacity: 0.6;
    transform: scale(0.92);
  }
}

/* ===== 退出 overlay 过渡：入场从右滑入 + 出场向下消散 ===== */
.exit-overlay-enter-active {
  transition:
    opacity 0.4s ease,
    transform 0.4s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.exit-overlay-leave-active {
  transition:
    opacity 0.6s ease,
    transform 0.6s cubic-bezier(0.4, 0, 0.2, 1),
    filter 0.6s ease;
}

.exit-overlay-enter-from {
  opacity: 0;
  transform: translateY(-12px);
}
.exit-overlay-leave-to {
  opacity: 0;
  transform: translateY(20px) scale(0.98);
  filter: blur(6px);
}

/* 尊重用户的减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .exit-deco,
  .exit-ring-outer,
  .exit-ring-mid,
  .exit-ring-inner,
  .exit-center-dot,
  .exit-logo-wrapper,
  .exit-logo-img {
    animation: none;
  }
}
</style>
