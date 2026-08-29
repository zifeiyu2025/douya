<!--
  AppearanceSettings: 外观设置（书房风重塑版）
  主题选择为「晨读 / 夜读」双联纸质样本卡：纯 CSS 色块实时预览两套色板，
  当前生效主题盖印章方点；聊天背景与头像的全部写入逻辑原样保留。
-->
<template>
  <!-- ===== 主题：晨读 / 夜读 双联纸质样本卡 ===== -->
  <n-form-item label="主题">
    <div class="theme-duo">
      <button
        v-for="card in THEME_CARDS"
        :key="card.mode"
        type="button"
        class="theme-card"
        :class="[card.palette, { 'is-active': effectiveTheme === card.mode }]"
        @click="handleModeChange(card.mode)"
      >
        <div class="theme-card-head">
          <span class="theme-card-name">{{ card.name }}</span>
          <!-- 印章方点：仅当前实际生效的主题显示（微倾模拟手钤印） -->
          <span class="theme-card-seal" aria-hidden="true"></span>
        </div>
        <!-- 纯 CSS 色块迷你书页：书脊 + 正文行 + 强调色印章块，零图片 -->
        <div class="theme-swatch">
          <div class="swatch-spine"></div>
          <div class="swatch-body">
            <span class="swatch-line swatch-line--title"></span>
            <span class="swatch-line swatch-line--long"></span>
            <span class="swatch-line swatch-line--short"></span>
            <div class="swatch-row">
              <span class="swatch-chip"></span>
              <span class="swatch-line swatch-row-line"></span>
            </div>
          </div>
        </div>
        <div class="theme-card-foot">
          <span class="theme-card-hex">{{ card.hex }}</span>
          <span class="theme-card-note">{{ card.note }}</span>
        </div>
      </button>
    </div>
  </n-form-item>

  <!-- 跟随系统开关：勾选后交还系统晨昏决定亮暗 -->
  <div class="theme-auto-row">
    <n-checkbox :checked="isAutoMode" @update:checked="handleAutoChange">
      跟随晨昏（依系统亮暗自动切换）
    </n-checkbox>
  </div>

  <!-- ===== 聊天背景 ===== -->
  <n-form-item label="聊天背景">
    <div class="bg-preview-wrapper">
      <!-- 无背景：hairline 虚线上传位 -->
      <div
        v-if="!formConfig.chat_background"
        class="bg-upload-placeholder"
        @click="selectBackgroundImage"
      >
        <AppIcon name="image" :size="26" />
        <span class="bg-upload-text">点击选择背景图片</span>
        <span class="bg-upload-hint">支持 JPG、PNG、WebP 格式</span>
      </div>
      <!-- 有背景：聊天气泡预览（联动当前编辑主题的参数） -->
      <div v-else class="bg-preview-area" @click="selectBackgroundImage">
        <div class="bg-preview-image" :style="previewImageStyle">
          <div class="bg-mask-layer" :style="maskLayerStyle"></div>
          <div class="bg-preview-chat">
            <div class="bg-preview-line bg-preview-line--user">你好，帮我总结一下今天的新闻</div>
            <div class="bg-preview-line bg-preview-line--ai">好的，今天的主要新闻包括...</div>
          </div>
          <div class="bg-hover-overlay">
            <span class="bg-hover-text">点击更换背景</span>
          </div>
        </div>
        <div class="bg-preview-actions">
          <span class="bg-preview-label">当前背景</span>
          <n-button size="small" type="primary" ghost @click.stop="clearBackground">
            清除背景
          </n-button>
        </div>
      </div>
    </div>
  </n-form-item>

  <!-- B5 每主题背景参数：同一张图，亮/暗主题各存一套参数。
       旧版单一「背景透明度」滑块绑定的 chat_background_opacity 已废弃
       （渲染端 App.vue 只消费 background_light/dark），故移除该死控件 -->
  <template v-if="formConfig.chat_background">
    <n-form-item label="参数作用主题">
      <n-tabs v-model:value="activeBgTheme" type="segment" size="small" class="bg-param-tabs">
        <n-tab name="light" tab="晨读" />
        <n-tab name="dark" tab="夜读" />
      </n-tabs>
    </n-form-item>
    <n-form-item label="图片不透明度">
      <n-slider
        :value="activeBgParams.opacity"
        :min="0.2"
        :max="1"
        :step="0.05"
        @update:value="v => updateBgParams('opacity', v)"
      />
      <span class="slider-value">{{ Math.round(activeBgParams.opacity * 100) }}%</span>
    </n-form-item>
    <n-form-item label="模糊半径">
      <n-slider
        :value="activeBgParams.blur"
        :min="0"
        :max="30"
        :step="1"
        @update:value="v => updateBgParams('blur', v)"
      />
      <span class="slider-value">{{ activeBgParams.blur }}px</span>
    </n-form-item>
    <n-form-item label="遮罩强度">
      <n-slider
        :value="activeBgParams.mask_alpha"
        :min="0"
        :max="1"
        :step="0.05"
        @update:value="v => updateBgParams('mask_alpha', v)"
      />
      <span class="slider-value">{{ Math.round(activeBgParams.mask_alpha * 100) }}%</span>
    </n-form-item>
  </template>

  <!-- ===== 用户头像 ===== -->
  <n-form-item label="用户头像">
    <div class="avatar-row">
      <div class="avatar-preview">
        <img :src="formConfig.user_avatar || defaultUserAvatar" class="avatar-img" alt="用户头像" />
      </div>
      <div class="avatar-btns">
        <n-upload
          :show-file-list="false"
          :custom-request="data => handleAvatarUpload(data, 'user_avatar')"
          accept="image/*"
        >
          <n-button size="small" ghost>上传</n-button>
        </n-upload>
        <n-button v-if="formConfig.user_avatar" size="small" ghost @click="clearUserAvatar">
          清除
        </n-button>
      </div>
    </div>
  </n-form-item>

  <!-- ===== AI 头像 ===== -->
  <n-form-item label="AI 头像">
    <div class="avatar-row">
      <div class="avatar-preview">
        <img :src="formConfig.ai_avatar || defaultAiAvatar" class="avatar-img" alt="AI 头像" />
      </div>
      <div class="avatar-btns">
        <n-upload
          :show-file-list="false"
          :custom-request="data => handleAvatarUpload(data, 'ai_avatar')"
          accept="image/*"
        >
          <n-button size="small" ghost>上传</n-button>
        </n-upload>
        <n-button v-if="formConfig.ai_avatar" size="small" ghost @click="clearAIAvatar">
          清除
        </n-button>
      </div>
    </div>
  </n-form-item>
</template>

<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import { NFormItem, NSlider, NButton, NUpload, NTabs, NTab, NCheckbox } from 'naive-ui'
import AppIcon from '../ui/AppIcon.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import { useThemeStore } from '../../stores/theme'
import type { ThemeBackgroundParams } from '../../types/chat'

defineOptions({ name: 'AppearanceSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error('AppearanceSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）')
}
// 域切片：core 提供表单与保存，appearance 提供背景图 / 头像逻辑
const { core, appearance } = ctx
const { formConfig, autoSave } = core
const {
  backgroundImageUrl,
  selectBackgroundImage,
  clearBackground,
  handleAvatarUpload,
  clearUserAvatar,
  clearAIAvatar,
  defaultUserAvatar,
  defaultAiAvatar
} = appearance
const themeStore = useThemeStore()

// ===== 晨读 / 夜读 双联纸质样本卡 =====
// palette 字段对应样式中的局部色板类（palette-morning / palette-evening），
// 其变量值逐字取自 styles/tokens.css 的 :root 与 html.dark 两套色板；
// 若设计令牌更新，需同步这两组样本色（色卡的本质即固定呈现指定配色）。
const THEME_CARDS = [
  {
    mode: 'light',
    name: '晨 读',
    palette: 'palette-morning',
    hex: '#ffffff',
    note: '纯白 × 石墨 × 亮蓝印'
  },
  {
    mode: 'dark',
    name: '夜 读',
    palette: 'palette-evening',
    hex: '#1a1b1d',
    note: '石墨黑 × 雾白 × 亮蓝辉'
  }
] as const

// 此刻实际生效的主题：mode 为 auto 时由 store 的 resolvedMode 解析系统偏好
const effectiveTheme = computed<'light' | 'dark'>(() => themeStore.resolvedMode)
const isAutoMode = computed(() => themeStore.mode === 'auto')

function handleModeChange(mode: 'auto' | 'light' | 'dark') {
  themeStore.setMode(mode)
  autoSave()
}

/** 跟随系统开关：取消自动时落回"此刻实际生效"的主题，避免视觉跳变 */
function handleAutoChange(checked: boolean) {
  handleModeChange(checked ? 'auto' : themeStore.resolvedMode)
}

// ===== B5 每主题背景参数编辑 =====

// 当前编辑的主题页签（默认跟随界面当前主题，减少"改了半天发现切错页签"的困惑）
const activeBgTheme = ref<'light' | 'dark'>(themeStore.isDark ? 'dark' : 'light')

const activeBgParams = computed<ThemeBackgroundParams>(() =>
  activeBgTheme.value === 'light'
    ? formConfig.value.background_light
    : formConfig.value.background_dark
)

/**
 * 更新当前编辑主题的某个背景参数。
 * 关键点：必须整体替换 background_light / background_dark 对象（不可变更新）——
 * useSettingsCore 的脏检测与保存 diff 都基于字段引用比较，
 * 就地修改对象内部属性不会被感知，会导致参数改了却存不进配置。
 */
function updateBgParams(key: keyof ThemeBackgroundParams, value: number) {
  const field = activeBgTheme.value === 'light' ? 'background_light' : 'background_dark'
  formConfig.value = {
    ...formConfig.value,
    [field]: { ...activeBgParams.value, [key]: value }
  }
}

/** 预览图联动：应用当前编辑主题的透明度与模糊（scale 微放大防模糊边缘露白） */
const previewImageStyle = computed(() => {
  const p = activeBgParams.value
  return {
    backgroundImage: `url(${backgroundImageUrl.value})`,
    opacity: String(p.opacity),
    ...(p.blur > 0 ? { filter: `blur(${p.blur}px)`, transform: 'scale(1.08)' } : {})
  }
})

/** 遮罩预览联动：纸面叠白纱、石墨叠黑纱，强度即 mask_alpha */
const maskLayerStyle = computed(() => ({
  backgroundColor:
    activeBgTheme.value === 'light'
      ? '#ffffff' // 与 tokens.css 纸面 chat-background-mask-color 同源的纯白
      : '#000000',
  opacity: String(activeBgParams.value.mask_alpha)
}))
</script>

<style scoped>
/* ============================================================
 * 双联纸质样本卡
 * 两组局部色板变量：值逐字取自 tokens.css（:root 纸面 / html.dark 石墨），
 * 让每张卡在任何界面主题下都如实呈现自己那一套配色。
 * ============================================================ */
.palette-morning {
  --sw-bg: #ffffff; /* 纯白纸底 */
  --sw-bg2: #f8f8f9; /* 雾灰书脊 */
  --sw-text: #31353a; /* 石墨墨字 */
  --sw-accent: #2f74ff; /* TRAE 亮蓝印 */
  --sw-border: #dfe3ea; /* 石灰细线 */
}

.palette-evening {
  --sw-bg: #1a1b1d; /* 石墨黑 */
  --sw-bg2: #222427; /* 深空灰书脊 */
  --sw-text: #d1d3db; /* 雾白字 */
  --sw-accent: #387bff; /* TRAE 辉光蓝 */
  --sw-border: #303031; /* 墨边细线 */
}

.theme-duo {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  width: 100%;
}

.theme-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  text-align: left;
  font: inherit;
  color: inherit;
  background: var(--bg-secondary);
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-md);
  cursor: pointer;
  box-shadow: var(--shadow-sm); /* 至多一层低透明环境影 */
  transition:
    background-color var(--transition-fast),
    border-color var(--transition-fast);
}
.theme-card:hover {
  /* 悬浮态只做背景色阶变化 */
  background: var(--bg-hover);
}
.theme-card.is-active {
  border-color: var(--accent-primary);
}

.theme-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.theme-card-name {
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: 4px;
}

/* 印章方点：默认隐没，激活时以微倾角度浮现（手钤印意象） */
.theme-card-seal {
  width: 9px;
  height: 9px;
  background: var(--seal-color);
  border-radius: 1px;
  opacity: 0;
  transform: scale(0.5) rotate(-8deg);
  transition:
    opacity var(--transition-fast),
    transform var(--transition-fast);
}
.theme-card.is-active .theme-card-seal {
  opacity: 1;
  transform: scale(1) rotate(-8deg);
}

/* ---- 纯 CSS 色块迷你书页 ---- */
.theme-swatch {
  display: flex;
  height: 92px;
  background: var(--sw-bg);
  border: 1px solid var(--sw-border);
  border-radius: var(--border-radius-sm);
  overflow: hidden;
}

.swatch-spine {
  width: 22%;
  background: var(--sw-bg2);
  border-right: 1px solid var(--sw-border);
}

.swatch-body {
  flex: 1;
  min-width: 0;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 7px;
}

/* 墨迹行：用 text 色调低浓度模拟正文笔画 */
.swatch-line {
  display: block;
  height: 5px;
  border-radius: var(--border-radius-xs);
  background: color-mix(in srgb, var(--sw-text) 26%, transparent);
}
.swatch-line--title {
  width: 58%;
  height: 8px;
  background: color-mix(in srgb, var(--sw-text) 72%, transparent);
}
.swatch-line--long {
  width: 88%;
}
.swatch-line--short {
  width: 64%;
}

.swatch-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 3px;
}

/* 强调色印章块 */
.swatch-chip {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  border-radius: 3px;
  background: var(--sw-accent);
}
.swatch-row-line {
  width: 46%;
  height: 4px;
}

.theme-card-foot {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.theme-card-hex {
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-muted);
}

.theme-card-note {
  font-size: 11px;
  color: var(--text-muted);
  letter-spacing: 0.5px;
}

/* 跟随系统开关行：紧贴样本卡下方 */
.theme-auto-row {
  margin-top: -6px;
  margin-bottom: 16px;
  padding-left: 2px;
}

/* ===== 背景预览 ===== */
.bg-preview-wrapper {
  width: 100%;
}

.bg-upload-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 36px 16px;
  border: 1px dashed var(--border-color); /* hairline 虚线，比粗虚线轻盈 */
  border-radius: var(--border-radius-md);
  background: var(--bg-secondary);
  cursor: pointer;
  color: var(--text-muted);
  transition:
    border-color var(--transition-fast),
    background-color var(--transition-fast);
}
.bg-upload-placeholder:hover {
  border-color: var(--accent-primary);
  background: var(--bg-hover);
}
.bg-upload-text {
  font-size: 14px;
  color: var(--text-secondary);
  font-weight: 500;
}
.bg-upload-hint {
  font-size: 12px;
  color: var(--text-muted);
}

/* 有背景时的预览区 */
.bg-preview-area {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.bg-preview-image {
  position: relative;
  height: 180px;
  border-radius: var(--border-radius-md);
  background-size: cover;
  background-position: center;
  overflow: hidden;
  cursor: pointer;
  border: 1px solid var(--border-color);
  box-shadow: var(--shadow-sm);
}
.bg-preview-chat {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 12px;
  padding: 20px 24px;
}

/* 预览文字行：贴近聊天视图的真实质感——用户便签块、AI 直排纸上 */
.bg-preview-line {
  font-size: 12px;
  line-height: 1.6;
}
.bg-preview-line--user {
  align-self: flex-end;
  max-width: 75%;
  padding: 8px 14px;
  border-radius: var(--border-radius-md);
  border-bottom-right-radius: 3px;
  background: var(--bg-user-msg-base);
  color: var(--text-user-msg);
}
.bg-preview-line--ai {
  align-self: flex-start;
  max-width: 85%;
  padding: 0 4px;
  color: var(--text-ai-msg);
}

/* 悬浮更换提示：纸色纱罩替代黑幕，贴合书房气质 */
.bg-hover-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--bg-primary) 62%, transparent);
  opacity: 0;
  transition: opacity 0.25s;
}
.bg-preview-image:hover .bg-hover-overlay {
  opacity: 1;
}
.bg-hover-text {
  font-family: var(--font-display);
  color: var(--text-primary);
  font-size: 14px;
  letter-spacing: 3px;
}

.bg-preview-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 4px;
}
.bg-preview-label {
  font-size: 12px;
  color: var(--text-muted);
}

/* B5 参数主题页签占满表单项宽度 */
.bg-param-tabs {
  width: 100%;
}

/* ===== 头像 ===== */
.avatar-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.avatar-preview {
  width: 56px;
  height: 56px;
  overflow: hidden;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color); /* hairline 细框 */
  border-radius: 50%;
  flex-shrink: 0;
}
.avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.avatar-btns {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* ===== 通用 ===== */
/* 数值读数用等宽字体，跳动时不抖动版式 */
.slider-value {
  min-width: 48px;
  text-align: right;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
}

/* B5 遮罩预览层：模拟 ::after 遮罩层的白纱/黑纱强度 */
.bg-mask-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
  transition: opacity 0.2s;
}
</style>
