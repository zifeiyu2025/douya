<!--
  PerformanceModeSelector: 三档性能模式选择器
  生活类比：像汽车的 ECO/COMFORT/SPORT 驾驶模式按钮，一键切换参数组合

  三档模式：
  - compatible（兼容）：保守配置，排查问题或首次运行未知模型
  - balanced（平衡）：日常使用，平衡性能与稳定性（默认）
  - performance（性能）：榨干硬件性能，适合确认硬件支持后追求极致速度
-->
<template>
  <n-form-item>
    <template #label>
      性能模式
      <HelpTip
        content="一键切换参数组合（GPU层数/Flash Attention/上下文/推测解码）。兼容=保守排查，平衡=日常使用，性能=榨干速度。切换后下次启动生效"
      />
    </template>
    <div class="mode-selector" role="radiogroup" aria-label="性能模式选择">
      <div
        v-for="mode in modes"
        :key="mode.value"
        class="mode-card"
        :class="{
          active: currentMode === mode.value,
          [mode.cardClass]: true
        }"
        role="radio"
        tabindex="0"
        :aria-checked="currentMode === mode.value"
        :aria-label="`${mode.label}模式：${mode.desc}`"
        @click="selectMode(mode.value)"
        @keydown.enter.prevent="selectMode(mode.value)"
        @keydown.space.prevent="selectMode(mode.value)"
      >
        <div class="mode-card-header">
          <n-icon size="22" class="mode-icon">
            <component :is="mode.icon" />
          </n-icon>
          <span class="mode-title">{{ mode.label }}</span>
          <n-icon v-if="currentMode === mode.value" size="16" class="mode-check">
            <CheckmarkCircle />
          </n-icon>
        </div>
        <div class="mode-desc">{{ mode.desc }}</div>
        <div class="mode-params">
          <span v-for="(param, i) in mode.params" :key="i" class="param-tag">
            {{ param }}
          </span>
        </div>
      </div>
    </div>
  </n-form-item>
</template>

<script setup lang="ts">
import { inject, ref } from 'vue'
import { NFormItem, NIcon, useMessage } from 'naive-ui'
import { LeafOutline, SpeedometerOutline, RocketOutline, CheckmarkCircle } from '@vicons/ionicons5'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import HelpTip from '../ui/HelpTip.vue'

defineOptions({ name: 'PerformanceModeSelector' })

// 空值保护：如果组件被误用在 SettingsView 之外，给出明确错误而非 undefined 崩溃
const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error(
    'PerformanceModeSelector 必须在 SettingsView 内使用（缺少 settingsContext provide）'
  )
}
// performance_mode 已随 smart-params 一起移除（改为 llama.cpp 原生 auto 加载）。
// 本组件保留为只读展示，不再读写任何配置字段。
const message = useMessage()

// 性能模式定义
const modes = [
  {
    value: 'compatible',
    label: '兼容',
    cardClass: 'mode-compatible',
    icon: LeafOutline,
    desc: '保守配置，适合排查问题或首次运行未知模型',
    params: ['GPU 自决', 'Flash Off', 'Ctx ≤ 4K', '推测解码 Off']
  },
  {
    value: 'balanced',
    label: '平衡',
    cardClass: 'mode-balanced',
    icon: SpeedometerOutline,
    desc: '日常使用，平衡性能与稳定性（推荐）',
    params: ['全层卸载', 'Flash On', '智能上下文', '推测解码自动']
  },
  {
    value: 'performance',
    label: '性能',
    cardClass: 'mode-performance',
    icon: RocketOutline,
    desc: '榨干硬件性能，适合确认硬件支持后追求极致速度',
    params: ['全层卸载', 'Flash On', 'Ctx 拉满', '推测解码强制']
  }
] as const

// 当前选中的模式（本地状态，不再持久化到配置）
const selectedMode = ref('balanced')
const currentMode = selectedMode

/** 切换性能模式（仅更新本地展示，不再写入配置） */
function selectMode(mode: string) {
  if (selectedMode.value === mode) return
  selectedMode.value = mode
  const labelMap: Record<string, string> = {
    compatible: '兼容',
    balanced: '平衡',
    performance: '性能'
  }
  message.success(
    `已选择「${labelMap[mode]}」模式（注：性能模式已移除，参数由 llama.cpp 自动决定）`,
    { duration: 4000 }
  )
}
</script>

<style scoped>
/* 三卡片横向布局，窄屏自动换行 */
.mode-selector {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  width: 100%;
}

/* 单张卡片：使用项目主题变量（非 Naive UI --n-* 变量，那些只在 Naive UI 组件内生效） */
.mode-card {
  position: relative;
  padding: 12px 14px;
  border: 1.5px solid var(--border-color);
  border-radius: 8px;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background 0.2s ease,
    transform 0.15s ease;
  background: var(--bg-secondary);
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 96px;
}

.mode-card:hover {
  transform: translateY(-1px);
  border-color: var(--accent-secondary);
}

/* 键盘聚焦样式：与 hover 区分，用 outline 保证可见性 */
.mode-card:focus-visible {
  outline: 2px solid var(--accent-primary);
  outline-offset: 2px;
}

/* 选中态：边框高亮 + 背景色微染（用 color-mix 适配亮暗主题） */
.mode-card.active {
  border-width: 2px;
  padding: 11px 13px; /* 抵消边框加粗 */
}

.mode-card.active.mode-compatible {
  border-color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 12%, transparent);
}

.mode-card.active.mode-balanced {
  border-color: var(--accent-success);
  background: color-mix(in srgb, var(--accent-success) 12%, transparent);
}

.mode-card.active.mode-performance {
  border-color: var(--accent-warning);
  background: color-mix(in srgb, var(--accent-warning) 14%, transparent);
}

/* 卡片头部：图标 + 标题 + 选中勾 */
.mode-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mode-icon {
  flex-shrink: 0;
}

.mode-compatible .mode-icon {
  color: var(--accent-primary);
}
.mode-balanced .mode-icon {
  color: var(--accent-success);
}
.mode-performance .mode-icon {
  color: var(--accent-warning);
}

.mode-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  flex: 1;
}

.mode-check {
  color: var(--accent-success);
  flex-shrink: 0;
}

.mode-compatible.active .mode-check {
  color: var(--accent-primary);
}
.mode-performance.active .mode-check {
  color: var(--accent-warning);
}

/* 描述文字 */
.mode-desc {
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.4;
  min-height: 30px;
}

/* 参数标签 */
.mode-params {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 2px;
}

.param-tag {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  background: var(--surface-subtle);
  color: var(--text-muted);
  white-space: nowrap;
}

/* 中等宽度：两列，避免三卡挤在一起 */
@media (max-width: 900px) {
  .mode-selector {
    grid-template-columns: repeat(2, 1fr);
  }
}

/* 窄屏：单列堆叠 */
@media (max-width: 600px) {
  .mode-selector {
    grid-template-columns: 1fr;
  }
}
</style>
