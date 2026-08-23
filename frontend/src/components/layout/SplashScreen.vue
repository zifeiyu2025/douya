<template>
  <Transition name="splash" @after-leave="$emit('complete')">
    <div v-if="visible" class="splash-screen" style="--wails-draggable: drag">
      <!-- 背景氛围层：单一径向渐变，营造空间感（非装饰覆盖层，避免堆叠问题） -->
      <div class="splash-ambient" aria-hidden="true"></div>

      <div class="splash-content">
        <!-- LOGO 区域：appicon.png 作为唯一视觉锚点 -->
        <div
          class="splash-logo-wrap"
          :class="{ 'is-done': stage === 'done', 'is-failed': stage === 'failed' }"
        >
          <!-- 外层发光环：加载中旋转弧线，完成时画圆 -->
          <svg
            class="logo-ring"
            width="112"
            height="112"
            viewBox="0 0 112 112"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            aria-hidden="true"
          >
            <!-- 底层静态淡圈（轨道） -->
            <circle cx="56" cy="56" r="52" stroke="currentColor" stroke-width="1" opacity="0.1" />
            <!-- 旋转弧线（加载中） -->
            <circle
              v-if="stage !== 'done' && stage !== 'failed'"
              cx="56"
              cy="56"
              r="52"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-dasharray="80 246"
              class="logo-spinner"
            />
            <!-- 完成圆环（画圆动画） -->
            <circle
              v-else
              cx="56"
              cy="56"
              r="52"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              class="logo-complete"
            />
          </svg>

          <!-- LOGO 图像：appicon.png -->
          <img class="logo-image" :src="appLogo" alt="豆芽" draggable="false" />
        </div>

        <!-- 品牌标识：品牌名最响亮 -->
        <div class="splash-brand">
          <h1 class="splash-title">豆芽</h1>
          <p class="splash-subtitle">本地 AI 聊天助手</p>
        </div>

        <!-- 状态指示器：底部安静展示 -->
        <div class="splash-status">
          <span class="status-dot" :class="'dot-' + stage" aria-hidden="true"></span>
          <span
            class="status-text"
            :class="{ 'status-done': stage === 'done', 'status-failed': stage === 'failed' }"
          >
            {{ stageText }}
          </span>
          <!-- downloading 阶段 stageText 已包含 label，不重复显示 status-model -->
          <span
            v-if="modelName && stage !== 'done' && stage !== 'failed' && stage !== 'downloading'"
            class="status-model"
          >
            {{ modelName }}
          </span>
        </div>

        <!-- 下载进度条：仅在 downloading 阶段显示 -->
        <div v-if="stage === 'downloading'" class="splash-download">
          <div class="download-bar">
            <div
              class="download-bar-fill"
              :style="{ transform: 'scaleX(' + progress / 100 + ')' }"
            ></div>
          </div>
          <span class="download-percent">{{ progress }}%</span>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import appLogo from '../../assets/images/appicon.png'

const props = withDefaults(
  defineProps<{
    visible?: boolean
    stage?: string
    modelName?: string
    progress?: number
  }>(),
  {
    visible: false,
    stage: 'idle',
    modelName: '',
    progress: 0
  }
)

defineEmits<{
  complete: []
}>()

const stageText = computed(() => {
  // downloading 阶段：根据 label 动态显示精准文本
  // label 可能是"推理后端"、"cudart 依赖包"、"解压安装中"、"重启中"
  if (props.stage === 'downloading') {
    const label = props.modelName || ''
    if (label === '解压安装中') return '正在解压安装...'
    if (label === '重启中') return '正在重启应用...'
    if (label === '下载完成') return '下载完成'
    if (label) return `正在下载${label}...`
    return '正在下载...'
  }

  const map: Record<string, string> = {
    idle: '初始化中',
    preparing: '准备启动引擎',
    loading: '加载模型中',
    waiting: '初始化模型',
    detecting: '检测模型能力',
    downloading: '正在下载...',
    done: '加载完成',
    failed: '加载失败',
    rolling_back: '回滚中',
    'vram-warning': 'VRAM 不足警告',
    'spec-warning': '推测解码兼容性警告'
  }
  return map[props.stage] || '加载中'
})
</script>

<style scoped>
/* ===== 启动屏容器 =====
 * 支持窗口拖动（--wails-draggable:drag 在 template 内联）
 * 单一背景层 + 单一氛围层，避免复杂堆叠
 */
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

/* 背景氛围层：径向渐变营造空间感
 * 避免使用多层覆盖，单一层即可
 */
.splash-ambient {
  position: absolute;
  inset: 0;
  background: radial-gradient(circle at 50% 45%, rgba(9, 105, 218, 0.06) 0%, transparent 60%);
  pointer-events: none;
}

/* 暗色主题下的氛围层调整 */
:global(body.theme-dark) .splash-ambient {
  background: radial-gradient(circle at 50% 45%, rgba(68, 147, 248, 0.08) 0%, transparent 60%);
}

/* ===== 内容区域 ===== */
.splash-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 32px;
  position: relative;
  z-index: 1;
  padding: 0 24px;
}

/* ===== LOGO 区域 =====
 * LOGO 图像作为唯一视觉锚点，外层发光环指示状态
 * 不使用 filter:drop-shadow 在整个容器上，避免模糊图像
 */
.splash-logo-wrap {
  position: relative;
  width: 112px;
  height: 112px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--accent-primary);
  transition: color 0.3s ease;
  animation: logo-enter 0.8s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.splash-logo-wrap.is-done {
  color: var(--accent-success);
}

.splash-logo-wrap.is-failed {
  color: var(--accent-danger);
}

@keyframes logo-enter {
  from {
    opacity: 0;
    transform: scale(0.85);
    filter: blur(8px);
  }
  to {
    opacity: 1;
    transform: scale(1);
    filter: blur(0);
  }
}

/* 发光环 SVG：绝对定位，环绕 LOGO */
.logo-ring {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

/* 旋转弧线：加载中 */
.logo-spinner {
  transform-origin: 56px 56px;
  animation: spin 1.6s linear infinite;
  filter: drop-shadow(0 0 4px currentColor);
}

/* @keyframes spin 已在 style.css 全局定义，此处不再重复 */

/* 完成圆环：画圆动画 */
.logo-complete {
  stroke-dasharray: 327;
  stroke-dashoffset: 327;
  animation: draw-circle 0.7s cubic-bezier(0.4, 0, 0.2, 1) forwards;
  filter: drop-shadow(0 0 6px currentColor);
}

@keyframes draw-circle {
  to {
    stroke-dashoffset: 0;
  }
}

/* LOGO 图像：appicon.png
 * 圆角处理，尺寸略小于发光环，让环清晰可见
 */
.logo-image {
  width: 72px;
  height: 72px;
  border-radius: 16px;
  object-fit: cover;
  /* 微妙的阴影增强立体感 */
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  user-select: none;
  -webkit-user-drag: none;
}

/* 暗色主题下 LOGO 阴影调整 */
:global(body.theme-dark) .logo-image {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
}

/* ===== 品牌标识 =====
 * 品牌名是最响亮的文字，字重重、字号大
 * 副标题安静地跟随
 */
.splash-brand {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  animation: brand-enter 0.7s cubic-bezier(0.4, 0, 0.2, 1) 0.2s both;
}

@keyframes brand-enter {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.splash-title {
  margin: 0;
  font-size: 36px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 8px;
  padding-left: 8px;
  line-height: 1;
  /* 用 rgba 替代 color-mix，兼容旧版 WebView2 */
  text-shadow: 0 0 16px rgba(9, 105, 218, 0.25);
  animation: title-glow 3s ease-in-out infinite;
}

/* 暗色主题下标题阴影 */
:global(body.theme-dark) .splash-title {
  text-shadow: 0 0 16px rgba(68, 147, 248, 0.35);
}

@keyframes title-glow {
  0%,
  100% {
    text-shadow: 0 0 16px rgba(9, 105, 218, 0.25);
  }
  50% {
    text-shadow: 0 0 24px rgba(9, 105, 218, 0.4);
  }
}

:global(body.theme-dark) .splash-title {
  animation: title-glow-dark 3s ease-in-out infinite;
}

@keyframes title-glow-dark {
  0%,
  100% {
    text-shadow: 0 0 16px rgba(68, 147, 248, 0.35);
  }
  50% {
    text-shadow: 0 0 24px rgba(68, 147, 248, 0.5);
  }
}

.splash-subtitle {
  margin: 0;
  font-size: 13px;
  font-weight: 400;
  color: var(--text-secondary);
  letter-spacing: 4px;
  padding-left: 4px;
}

/* ===== 状态指示器 =====
 * 底部安静展示：圆点 + 状态文字 + 模型名
 * 圆点颜色跟随阶段变化
 */
.splash-status {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  min-height: 40px;
  animation: status-enter 0.6s cubic-bezier(0.4, 0, 0.2, 1) 0.4s both;
}

@keyframes status-enter {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

/* 状态圆点：加载中脉动，完成静止，失败闪烁 */
.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-primary);
  animation: dot-pulse 1.8s ease-in-out infinite;
}

.dot-done {
  background: var(--accent-success);
  animation: none;
}

.dot-failed {
  background: var(--accent-danger);
  animation: dot-blink 1s ease-in-out infinite;
}

@keyframes dot-pulse {
  0%,
  100% {
    opacity: 0.4;
    transform: scale(0.85);
  }
  50% {
    opacity: 1;
    transform: scale(1.15);
  }
}

@keyframes dot-blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.3;
  }
}

.status-text {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
  transition: color 0.3s ease;
  letter-spacing: 0.5px;
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
  color: var(--text-tertiary, var(--text-secondary));
  max-width: 280px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  opacity: 0.7;
}

/* ===== 下载进度条 =====
 * 仅在 downloading 阶段显示，安静地展示百分比进度
 */
.splash-download {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  width: 240px;
  animation: status-enter 0.4s cubic-bezier(0.4, 0, 0.2, 1) both;
}

.download-bar {
  width: 100%;
  height: 4px;
  background: var(--bg-tertiary, rgba(0, 0, 0, 0.08));
  border-radius: 2px;
  overflow: hidden;
}

:global(body.theme-dark) .download-bar {
  background: rgba(255, 255, 255, 0.1);
}

.download-bar-fill {
  width: 100%;
  height: 100%;
  background: var(--accent-primary);
  border-radius: 2px;
  transform-origin: left;
  transition: transform 0.3s ease;
  box-shadow: 0 0 6px var(--accent-primary);
}

.download-percent {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  letter-spacing: 0.5px;
}

/* ===== 进出场过渡 ===== */
.splash-enter-active {
  transition: opacity 0.4s ease;
}

.splash-leave-active {
  transition:
    opacity 0.6s ease,
    transform 0.6s cubic-bezier(0.4, 0, 0.2, 1);
}

.splash-enter-from {
  opacity: 0;
}

.splash-leave-to {
  opacity: 0;
  transform: translateY(-12px);
}

/* ===== 尊重减少动画偏好 ===== */
@media (prefers-reduced-motion: reduce) {
  .logo-spinner,
  .splash-title,
  .status-dot {
    animation: none;
  }

  .splash-logo-wrap,
  .splash-brand,
  .splash-status {
    animation: none;
  }
}
</style>
