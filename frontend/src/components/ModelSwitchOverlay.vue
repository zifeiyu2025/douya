<template>
  <Transition name="switch-overlay">
    <div v-if="show" class="switch-overlay switch-overlay--model">
      <!-- SVG 装饰层：同心圆 + 双层旋转弧线 -->
      <svg
        class="switch-deco"
        width="360"
        height="360"
        viewBox="0 0 360 360"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <circle cx="180" cy="180" r="160" stroke="currentColor" stroke-width="1" opacity="0.06" />
        <circle cx="180" cy="180" r="120" stroke="currentColor" stroke-width="1" opacity="0.08" />
        <circle
          cx="180"
          cy="180"
          r="160"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-dasharray="50 320"
          class="switch-deco-outer"
          opacity="0.35"
        />
        <circle
          cx="180"
          cy="180"
          r="120"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-dasharray="35 240"
          class="switch-deco-mid"
          opacity="0.45"
        />
      </svg>
      <div class="switch-overlay-content">
        <!-- 圆形进度环 + 中心 LOGO 图片 -->
        <div class="switch-ring-wrapper">
          <svg class="switch-ring-svg" width="80" height="80" viewBox="0 0 80 80">
            <circle
              cx="40"
              cy="40"
              r="36"
              stroke="var(--border-color)"
              stroke-width="2"
              fill="none"
              opacity="0.3"
            />
            <circle
              cx="40"
              cy="40"
              r="36"
              stroke="var(--accent-primary)"
              stroke-width="2.5"
              stroke-linecap="round"
              fill="none"
              stroke-dasharray="85 150"
              class="switch-ring-arc"
            />
          </svg>
          <div class="switch-ring-center">
            <img :src="appLogo" alt="豆芽" class="switch-ring-logo" />
          </div>
        </div>
        <div class="switch-model-name">{{ overlayModelName }}</div>
        <div class="switch-progress-msg">{{ switchStageText }}</div>
        <!-- 阶段指示器：3 阶段进度 -->
        <div class="switch-stage-indicator">
          <div
            v-for="(stage, idx) in switchStages"
            :key="stage"
            :class="[
              'stage-item',
              {
                active: getSwitchStageIndex() >= idx,
                completed: getSwitchStageIndex() > idx
              }
            ]"
          >
            <span class="stage-dot"></span>
            <span class="stage-label">{{ stage }}</span>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import appLogo from '../assets/images/appicon.png'

// 模型切换 overlay：纯展示组件
// 所有数据由 App.vue 调用 useModelSwitch 后通过 props 传入，
// 避免在本组件重复调用 useModelSwitch 导致事件监听重复注册。
defineProps<{
  show: boolean
  overlayModelName: string
  switchStageText: string
  switchStages: readonly string[]
  getSwitchStageIndex: () => number
}>()
</script>

<style scoped>
/* 基础 overlay 样式（与 ExitOverlay 共用，scoped 各自定义一份） */
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

/* ===== SVG 装饰层（切换动效）===== */
.switch-deco {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  color: var(--accent-primary);
  pointer-events: none;
  z-index: 0;
}

.switch-deco-outer {
  transform-origin: 180px 180px;
  animation: spin 30s linear infinite;
  will-change: transform;
}

.switch-deco-mid {
  transform-origin: 180px 180px;
  animation: spin-reverse 20s linear infinite;
  will-change: transform;
}

@keyframes spin-reverse {
  to {
    transform: rotate(-360deg);
  }
}

/* 旋转关键帧：外层弧、中层弧、头像旁进度环弧线共用。
   固定在组件内定义，不依赖其他组件的 scoped 样式，确保动画生效。 */
@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* ===== 圆形进度环（与 MessageList 一致） ===== */
.switch-ring-wrapper {
  position: relative;
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 使用 color-mix 跟随主色调，filter 支持 color-mix */
  filter: drop-shadow(0 0 8px color-mix(in srgb, var(--accent-primary) 40%, transparent));
}

.switch-ring-svg {
  display: block;
}

.switch-ring-arc {
  transform-origin: 40px 40px;
  animation: spin 1.4s linear infinite;
}

.switch-ring-center {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 圆环中心 LOGO 图片（替代原 pulse 点） */
.switch-ring-logo {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  object-fit: cover;
  /* 轻微呼吸缩放，提示正在加载 */
  animation: logo-breath 1.8s ease-in-out infinite;
}

@keyframes logo-breath {
  0%,
  100% {
    transform: scale(1);
    opacity: 0.85;
  }
  50% {
    transform: scale(1.08);
    opacity: 1;
  }
}

/* ===== 阶段指示器（3 阶段）=====
 * 实用性：让用户知道当前进度到了哪一步
 */
.switch-stage-indicator {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-top: 8px;
}

.stage-item {
  display: flex;
  align-items: center;
  gap: 6px;
  opacity: 0.4;
  transition: opacity 0.3s ease;
}

.stage-item.active {
  opacity: 1;
}

.stage-item.completed {
  opacity: 0.8;
}

.stage-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--border-color);
  transition:
    background 0.3s ease,
    box-shadow 0.3s ease;
}

.stage-item.active .stage-dot {
  background: var(--accent-primary);
  box-shadow: 0 0 8px color-mix(in srgb, var(--accent-primary) 60%, transparent);
  animation: stage-dot-pulse 1.5s ease-in-out infinite;
}

.stage-item.completed .stage-dot {
  background: var(--accent-primary);
}

@keyframes stage-dot-pulse {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.3);
  }
}

.stage-label {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.stage-item.active .stage-label {
  color: var(--accent-primary);
  font-weight: 500;
}

/* ===== 切换 overlay 过渡：入场缩放 + 出场模糊 ===== */
.switch-overlay-enter-active {
  transition:
    opacity 0.3s ease,
    transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.switch-overlay-leave-active {
  transition:
    opacity 0.4s ease,
    transform 0.4s cubic-bezier(0.4, 0, 0.2, 1),
    filter 0.4s ease;
}

.switch-overlay-enter-from {
  opacity: 0;
  transform: scale(1.08);
}
.switch-overlay-leave-to {
  opacity: 0;
  transform: scale(0.96);
  filter: blur(4px);
}

/* 尊重用户的减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .switch-deco-outer,
  .switch-deco-mid,
  .switch-ring-arc,
  .switch-ring-logo,
  .stage-item.active .stage-dot {
    animation: none;
  }
}
</style>
