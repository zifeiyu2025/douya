<template>
  <!-- 书签式命令条：
       整条承担窗口拖拽（--wails-draggable: drag），交互区均为 no-drag 岛屿；
       双击空白处最大化 -->
  <header
    class="top-command-bar"
    style="--wails-draggable: drag"
    @dblclick="emit('header-double-click', $event)"
  >
    <!-- 左：品牌印记（印章 logo + 宋体字标） -->
    <div class="brand">
      <img :src="appLogo" alt="豆芽" class="brand-seal" draggable="false" />
      <div class="brand-words">
        <span class="brand-cn">豆芽</span>
        <span class="brand-en">DOUYA</span>
      </div>
    </div>

    <button
      type="button"
      class="bar-btn"
      :class="{ 'bar-btn-active': sidebarVisible }"
      style="--wails-draggable: no-drag"
      :title="sidebarVisible ? '收起会话栏' : '展开会话栏'"
      aria-label="切换会话侧栏"
      @click="emit('toggle-sidebar')"
    >
      <n-icon :size="18"><MenuOutline /></n-icon>
    </button>

    <span class="hairline-v"></span>

    <!-- 中：模型选择与服务状态（点击状态区弹出 GGUF 元数据详情卡） -->
    <div class="model-zone" style="--wails-draggable: no-drag">
      <!-- 切换中必禁；服务未运行但有模型时禁；无模型时不禁——让占位引导可见 -->
      <n-select
        :value="selectedModel"
        :options="displayModelOptions"
        size="small"
        placeholder="选择模型"
        class="model-selector"
        :disabled="isModelSwitching || (modelOptions.length > 0 && !serverStatus.running)"
        :render-label="renderModelLabel"
        @update:value="switchToModel"
      />
      <n-popover trigger="click" placement="bottom-start" :show-arrow="false" raw>
        <template #trigger>
          <button type="button" class="server-status" aria-label="查看模型详情">
            <span v-if="switchProgressStage !== 'idle'" class="state-row switching">
              <span class="loading-spinner"></span>
              <span class="state-text">
                <span class="state-model" :title="switchingModelName">
                  {{ switchingModelName }}
                </span>
                <span class="state-sep">·</span>
                <span class="state-status">{{ switchStageText }}{{ switchDuration }}</span>
              </span>
            </span>
            <span
              v-else-if="modelLoadProgress && modelLoadProgress.status === 'loading'"
              class="state-row loading-progress"
            >
              <span class="loading-spinner"></span>
              <span class="progress-col">
                <span class="state-text">
                  <span class="state-model" :title="loadProgressModelName">
                    {{ loadProgressModelName }}
                  </span>
                  <span class="state-sep">·</span>
                  <span class="state-status">加载 {{ modelLoadProgress.progress }}%</span>
                </span>
                <span class="progress-track">
                  <span
                    class="progress-fill"
                    :style="{ transform: 'scaleX(' + modelLoadProgress.progress / 100 + ')' }"
                  ></span>
                </span>
              </span>
            </span>
            <span v-else-if="modelLoadFailed" class="state-row failed">
              <span class="status-dot stopped"></span>
              <span class="state-text error-text">
                <span class="state-model" :title="errorModelName">{{ errorModelName }}</span>
                <span class="state-sep">·</span>
                <span class="state-status">加载失败</span>
              </span>
            </span>
            <span
              v-else-if="isServerLoading && switchProgressStage === 'idle' && !isFirstLoad"
              class="state-row starting"
            >
              <span class="loading-spinner"></span>
              <span class="state-text">{{ modelName || '启动中...' }}</span>
            </span>
            <span v-else class="state-row idle">
              <span class="status-dot" :class="serverStatus.running ? 'running' : 'stopped'"></span>
              <span
                class="state-text"
                :class="{ 'error-text': !serverStatus.running && serverStatus.error }"
              >
                <span class="state-model" :title="modelName">{{ modelName }}</span>
                <span class="state-sep">·</span>
                <span class="state-status">
                  {{ serverStatus.running ? '已就绪' : serverStatus.error || '未运行' }}
                </span>
              </span>
            </span>
          </button>
        </template>
        <ModelDetailCard :model="currentModelDetail" />
      </n-popover>
    </div>

    <div class="bar-spacer"></div>

    <!-- 右：命令面板入口 / 主题 / 控制台 / 窗控 -->
    <button
      type="button"
      class="palette-trigger"
      style="--wails-draggable: no-drag"
      title="命令面板 (Ctrl+K)"
      @click="emit('open-palette')"
    >
      <n-icon :size="14"><SearchOutline /></n-icon>
      <span class="palette-text">搜索或跳转</span>
      <kbd class="palette-kbd">Ctrl K</kbd>
    </button>

    <button
      type="button"
      class="bar-btn"
      style="--wails-draggable: no-drag"
      :title="isDark ? '切换晨读模式' : '切换夜读模式'"
      @click="themeStore.toggleTheme()"
    >
      <n-icon :size="16">
        <SunnyOutline v-if="isDark" />
        <MoonOutline v-else />
      </n-icon>
    </button>

    <button
      type="button"
      class="bar-btn"
      style="--wails-draggable: no-drag"
      title="服务器控制台"
      @click="emit('toggle-console')"
    >
      <n-icon :size="16"><TerminalOutline /></n-icon>
    </button>

    <span class="hairline-v"></span>

    <div class="window-controls" style="--wails-draggable: no-drag">
      <button type="button" class="win-btn" title="最小化" @click="emit('minimize')">
        <AppIcon name="minimize" :size="14" />
      </button>
      <button type="button" class="win-btn" title="最大化" @click="emit('toggle-maximize')">
        <AppIcon :name="isMaximized ? 'restore' : 'maximize'" :size="12" />
      </button>
      <button type="button" class="win-btn win-btn-close" title="关闭" @click="emit('close')">
        <AppIcon name="close" :size="14" />
      </button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed, h } from 'vue'
import { NPopover, NSelect, NTooltip } from 'naive-ui'
import {
  MenuOutline,
  SearchOutline,
  SunnyOutline,
  MoonOutline,
  TerminalOutline
} from '@vicons/ionicons5'
import AppIcon from '../ui/AppIcon.vue'
import ModelDetailCard from '../models/ModelDetailCard.vue'
import appLogo from '../../assets/images/appicon.png'
import { useThemeStore } from '../../stores/theme'
import { useModelSelector } from '../../composables/useModelSelector'
import type { ModelOptionView } from '../../composables/useModelSelector'

// switchDuration / switchStageText 来自 App.vue 调用 useModelSwitch 后传入，
// 避免本组件重复注册 Wails 事件监听
defineProps<{
  switchDuration: string
  switchStageText: string
  isMaximized: boolean
  /** 会话侧栏当前是否展开（用于顶栏按钮高亮与文案切换） */
  sidebarVisible: boolean
}>()

const emit = defineEmits<{
  'toggle-sidebar': []
  'toggle-console': []
  'open-palette': []
  minimize: []
  'toggle-maximize': []
  close: []
  'header-double-click': [event: MouseEvent]
}>()

const themeStore = useThemeStore()
const isDark = computed(() => themeStore.isDark)

// 模型数据层单例（列表加载、状态派生、切换与删除全部内聚于此）
const {
  modelOptions,
  displayModelOptions,
  selectedModel,
  currentModelDetail,
  modelName,
  serverStatus,
  switchProgressStage,
  isModelSwitching,
  switchingModelName,
  isServerLoading,
  isFirstLoad,
  modelLoadFailed,
  modelLoadProgress,
  loadProgressModelName,
  errorModelName,
  switchToModel
} = useModelSelector()

/** 下拉项渲染：模型名 + 规模标签 + 量化类型（弱化小字）+ 多模态标记 */
function renderModelLabel(option: ModelOptionView) {
  const quantText = option.quantType || option.quantSuffix
  const childSpans = [
    h('span', option.label),
    ...(option.sizeLabel
      ? [
          h(
            'span',
            {
              style:
                'color: var(--accent-primary); font-size: 11px; margin-left: 6px; font-weight: 600;'
            },
            option.sizeLabel
          )
        ]
      : []),
    ...(quantText
      ? [
          h(
            'span',
            {
              style: 'color: var(--text-muted); font-size: 11px; margin-left: 4px;'
            },
            quantText
          )
        ]
      : [])
  ]
  const tags: string[] = []
  if (option.mmprojVision) tags.push('📷')
  if (option.mmprojAudio) tags.push('🎤')
  if (option.mmprojVideo) tags.push('🎬')
  if (tags.length > 0) {
    childSpans.push(h('span', { style: 'margin-left: 6px; font-size: 11px;' }, tags.join(' ')))
  }
  if (option.status === 'sleeping') {
    childSpans.push(h('span', { style: 'margin-left: 6px; font-size: 10px;' }, '💤'))
  } else if (option.status === 'loading') {
    childSpans.push(
      h('span', { style: 'color: var(--accent-primary); margin-left: 6px; font-size: 10px;' }, '⏳')
    )
  } else if (option.isLoaded) {
    childSpans.push(
      h('span', { style: 'color: var(--accent-primary); margin-left: 6px; font-size: 10px;' }, '●')
    )
  }
  const content = h('span', { style: 'display: inline-flex; align-items: center' }, [
    h(
      'span',
      { style: 'display: inline-flex; align-items: center; min-width: 0; overflow: hidden' },
      childSpans
    )
  ])
  return h(
    NTooltip,
    { placement: 'right', delay: 300 },
    { trigger: () => content, default: () => option.fullName || option.value }
  )
}
</script>

<style scoped>
/* ===== 命令条外壳：纸面横幅，底部发丝线 ===== */
.top-command-bar {
  position: relative;
  z-index: var(--z-command-bar);
  display: flex;
  align-items: center;
  gap: 8px;
  height: var(--header-height);
  padding: 0 10px;
  background: var(--surface-panel);
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
  user-select: none;
}

/* ---- 品牌印记 ---- */
.brand {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 0 4px 0 2px;
  min-width: 0;
}

.brand-seal {
  width: 24px;
  height: 24px;
  border-radius: var(--border-radius-xs);
  object-fit: cover;
  -webkit-user-drag: none;
  flex-shrink: 0;
}

.brand-words {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1;
}

.brand-cn {
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 2px;
  color: var(--text-primary);
}

.brand-en {
  font-size: 8px;
  letter-spacing: 3px;
  color: var(--text-muted);
}

/* ---- 通用小方钮 ---- */
.bar-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  flex-shrink: 0;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
  appearance: none;
  -webkit-appearance: none;
}

.bar-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

/* 展开态高亮：提示"会话栏正在显示" */
.bar-btn-active {
  background: var(--bg-active);
  color: var(--accent-primary);
}

.hairline-v {
  width: 1px;
  height: 18px;
  background: var(--border-light);
  flex-shrink: 0;
}

.bar-spacer {
  flex: 1;
  height: 1px;
  min-width: 8px;
}

/* ---- 模型区 ---- */
.model-zone {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.model-selector {
  min-width: 130px;
  max-width: 240px;
  flex-shrink: 0;
}

/* 状态区按钮化：透明底 + hover 反馈 + 宽度约束防文字跳动 */
.server-status {
  display: flex;
  align-items: center;
  min-width: 150px;
  max-width: 260px;
  padding: 5px 10px;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  line-height: 1.3;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  cursor: pointer;
  transition: background var(--transition-fast);
  appearance: none;
  -webkit-appearance: none;
}

.server-status:hover {
  background: var(--bg-hover);
}

.state-row {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}

/* 状态文本：flex 双段，型号名段可省略、状态词段固定不缩，
   实现"头尾完整、中间省略"——避免窄窗硬切裁断可读性变差 */
.state-text {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  overflow: hidden;
}

/* 型号名段：可收缩 + 尾部省略（保留开头与分隔符前主体） */
.state-model {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 分隔符与状态词段：固定不缩，保证状态语义始终完整可见 */
.state-sep,
.state-status {
  flex-shrink: 0;
  white-space: nowrap;
}

.state-row.switching {
  color: var(--accent-warning);
  animation: state-pulse 1.5s ease-in-out infinite;
}

.state-row.loading-progress {
  color: var(--accent-primary);
}

.state-row.failed .error-text {
  color: var(--accent-danger);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-dot.running {
  background: var(--accent-success);
}

.status-dot.stopped {
  background: var(--text-muted);
}

.loading-spinner {
  width: 13px;
  height: 13px;
  border: 2px solid var(--border-color);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  flex-shrink: 0;
}

.progress-col {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.progress-track {
  width: 110px;
  height: 3px;
  background: var(--bg-secondary);
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  display: block;
  width: 100%;
  height: 100%;
  background: var(--accent-primary);
  border-radius: 2px;
  transform-origin: left;
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes state-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.55;
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* ---- 命令面板入口：书签形小票签 ---- */
.palette-trigger {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 28px;
  padding: 0 10px;
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-family: inherit;
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
  flex-shrink: 0;
  transition:
    background var(--transition-fast),
    border-color var(--transition-fast),
    color var(--transition-fast);
  appearance: none;
  -webkit-appearance: none;
}

.palette-trigger:hover {
  background: var(--bg-hover);
  border-color: var(--border-color);
  color: var(--text-secondary);
}

.palette-kbd {
  font-family: var(--font-mono);
  font-size: 10px;
  line-height: 1;
  padding: 3px 5px;
  border: 1px solid var(--border-light);
  border-bottom-width: 2px;
  border-radius: 3px;
  background: var(--bg-secondary);
  color: var(--text-muted);
}

/* ---- 窗口控制 ---- */
.window-controls {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

.win-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: none;
  border-radius: var(--border-radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
  flex-shrink: 0;
}

.win-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.win-btn-close:hover {
  background: #b0432e;
  color: #ffffff;
}
</style>
