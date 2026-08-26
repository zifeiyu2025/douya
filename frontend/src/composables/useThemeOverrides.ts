/**
 * Naive UI 全局主题覆盖（themeOverrides）— 调色板派生版
 *
 * 架构说明：
 *   - LIGHT_PALETTE / DARK_PALETTE：两套扁平调色板，唯一的手写颜色源；
 *     导出供 themeTokens.test.ts 与 tokens.css 自动比对，漂移即测试红。
 *   - buildOverrides(palette)：从调色板派生完整 GlobalThemeOverrides，
 *     common / Input / Select(peers) / Card / Dialog ... 全部由此生成。
 *
 * 表面色接入「三层表面体系」（见 styles/tokens.css 头注释）：
 *   - 凡是「表面」类 key（inputColor/cardColor/modalColor/popoverColor 等）
 *     直接传 CSS 变量引用 var(--surface-panel) / var(--surface-card)：
 *       · 未设背景图：alpha=1 → color-mix 退化为实色，观感不变；
 *       · 设了背景图：App.vue 注入 alpha 数值 → Naive UI 组件随全局一起通透，
 *         无需逐组件补丁（这就是 VSCode 背景插件式体验的实现路径）。
 *   - 「会被 Naive UI 内部做透明度/混色运算」的颜色（primary/error 等
 *     状态色、hoverColor）必须保持实色 hex —— 喂 var() 会让其内部
 *     color 解析器拿到无法计算的字符串，产生无效 CSS。
 *
 * ⚠️ 同步维护要求：LIGHT_PALETTE / DARK_PALETTE 的每个值都必须与
 *   styles/tokens.css 对应令牌块的 CSS 变量一致（含注释里的对照名）。
 *   themeTokens.test.ts 同时校验两边，改动任一处都会被测试抓住。
 */
import { computed, type ComputedRef } from 'vue'
import type { GlobalThemeOverrides } from 'naive-ui'
import { useThemeStore } from '../stores/theme'

/** 单主题调色板：全部手写颜色的唯一声明处（与 tokens.css 令牌一一对应） */
export interface ThemePalette {
  // ===== 主色系（--accent-primary / -secondary；pressed 为手工加深档）=====
  primary: string
  primaryHover: string
  primaryPressed: string
  // ===== 状态色（--accent-success/warning/danger 及其 hover/pressed 档）=====
  success: string
  successHover: string
  successPressed: string
  warning: string
  warningHover: string
  warningPressed: string
  error: string
  errorHover: string
  errorPressed: string
  // ===== 文字色系（--text-primary/secondary/muted + 禁用档）=====
  textPrimary: string
  textSecondary: string
  textMuted: string
  textDisabled: string
  // ===== 背景色系（实色兜底档：--bg-primary/secondary/hover/active）=====
  bgBase: string
  bgSubtle: string
  bgHover: string
  bgActive: string
  /** 禁用输入框/操作条底色（亮同 bgSubtle / 暗同 bgHover，保持中性微差） */
  bgMuted: string
  // ===== 边框（--border-color / --border-light）=====
  border: string
  borderLight: string
}

/** 亮色调色板（与 tokens.css :root 块同步 —— 纸面：纯白底 × 石墨墨 × TRAE 亮蓝） */
export const LIGHT_PALETTE: ThemePalette = {
  primary: '#2f74ff',
  primaryHover: '#5589ff',
  primaryPressed: '#245fd9',
  success: '#40b08b',
  successHover: '#58bf9e',
  successPressed: '#358f70',
  warning: '#e28a00',
  warningHover: '#ef9b1a',
  warningPressed: '#c27400',
  error: '#e8463a',
  errorHover: '#ee6055',
  errorPressed: '#cc362c',
  textPrimary: '#31353a',
  textSecondary: '#5b6066',
  textMuted: '#8a9096',
  textDisabled: '#aab0b8',
  bgBase: '#ffffff',
  bgSubtle: '#f8f8f9',
  bgHover: '#eaedf1',
  bgActive: '#e2e6ec',
  bgMuted: '#f0f2f5',
  border: '#dfe3ea',
  borderLight: '#ebeef3'
}

/** 深色调色板（与 tokens.css html.dark 块同步 —— 石墨：深空灰黑 × 雾白 × TRAE 亮蓝） */
export const DARK_PALETTE: ThemePalette = {
  primary: '#387bff',
  primaryHover: '#5b92ff',
  primaryPressed: '#2f68d8',
  success: '#00a56e',
  successHover: '#1cb87f',
  successPressed: '#008457',
  warning: '#dc8730',
  warningHover: '#e69a4d',
  warningPressed: '#b06c22',
  error: '#f65a5a',
  errorHover: '#f77676',
  errorPressed: '#cf4444',
  textPrimary: '#d1d3db',
  textSecondary: '#9599a6',
  textMuted: '#666b75',
  textDisabled: '#565b64',
  bgBase: '#1a1b1d',
  bgSubtle: '#222427',
  bgHover: '#2b2e33',
  bgActive: '#33373e',
  bgMuted: '#222427',
  border: '#303031',
  borderLight: '#3a3d43'
}

/**
 * 从调色板派生完整主题覆盖。
 *
 * 表面色分层约定：
 *   var(--surface-panel) → 阅读表面（输入框、内容卡片、表格）
 *   var(--surface-card)  → 浮起表面（弹窗、下拉、消息提示、抽屉）
 *   var(--bg-base) 不直接使用 —— bodyColor 用 bgBase 实色：
 *   应用根底色由 .app-layout 的背景系统接管，避免双重绘制。
 */
function buildOverrides(p: ThemePalette): GlobalThemeOverrides {
  return {
    common: {
      // ===== 主色调（info 复用 primary，项目无独立 info 色）=====
      primaryColor: p.primary,
      primaryColorHover: p.primaryHover,
      primaryColorPressed: p.primaryPressed,
      primaryColorSuppl: p.primary,
      infoColor: p.primary,
      infoColorHover: p.primaryHover,
      infoColorPressed: p.primaryPressed,
      infoColorSuppl: p.primary,
      successColor: p.success,
      successColorHover: p.successHover,
      successColorPressed: p.successPressed,
      successColorSuppl: p.success,
      warningColor: p.warning,
      warningColorHover: p.warningHover,
      warningColorPressed: p.warningPressed,
      warningColorSuppl: p.warning,
      errorColor: p.error,
      errorColorHover: p.errorHover,
      errorColorPressed: p.errorPressed,
      errorColorSuppl: p.error,

      // ===== 文字色系 =====
      textColorBase: p.textPrimary,
      textColor1: p.textPrimary,
      textColor2: p.textSecondary,
      textColor3: p.textMuted,
      placeholderColor: p.textMuted,
      placeholderColorDisabled: p.textDisabled,
      iconColor: p.textSecondary,
      iconColorHover: p.textPrimary,
      iconColorPressed: p.textPrimary,
      iconColorDisabled: p.textDisabled,

      // ===== 边框 / 分隔线 =====
      borderColor: p.border,
      dividerColor: p.borderLight,

      // ===== 背景色系（表面接三层令牌，反馈色保持实色）=====
      bodyColor: p.bgBase,
      // 内容卡片 = 阅读表面（设置页分组卡、知识库卡片等）
      cardColor: 'var(--surface-panel)',
      // 弹窗/弹出层/表格容器 = 浮起表面
      modalColor: 'var(--surface-card)',
      popoverColor: 'var(--surface-card)',
      tableColor: 'var(--surface-panel)',
      // 输入框 = 阅读表面（有背景时与气泡同步通透）
      inputColor: 'var(--surface-panel)',
      inputColorDisabled: p.bgMuted,
      actionColor: p.bgMuted,
      // hover 反馈必须清晰且会被 Naive UI 内部混色运算，保持实色
      hoverColor: p.bgHover,
      tableColorHover: p.bgHover,
      tableColorStriped: p.bgSubtle,

      // ===== 圆角（与 tokens.css --border-radius-sm/xs 对齐）=====
      borderRadius: '10px',
      borderRadiusSmall: '6px',

      // ===== 关闭按钮 =====
      closeIconColor: p.textMuted,
      closeIconColorHover: p.textPrimary,
      closeIconColorPressed: p.textPrimary,
      closeColorHover: p.bgHover,
      closeColorPressed: p.bgActive
    },

    Button: {
      // 主色按钮文字始终白色（保证任意主题下的对比度）
      textColorPrimary: '#ffffff',
      textColorHover: '#ffffff',
      textColorPressed: '#ffffff',
      textColorFocus: '#ffffff'
    },

    Input: {
      // 阅读表面：跟随全局通透
      color: 'var(--surface-panel)',
      colorFocus: 'var(--surface-panel)',
      border: `1px solid ${p.border}`,
      borderHover: `1px solid ${p.primary}`,
      borderFocus: `1px solid ${p.primary}`,
      textColor: p.textPrimary,
      placeholderColor: p.textMuted,
      caretColor: p.primary
    },

    Select: {
      // Select 内部使用 InternalSelection（不是 Input），peers 同步配色
      peers: {
        InternalSelection: {
          color: 'var(--surface-panel)',
          colorActive: 'var(--surface-panel)',
          border: `1px solid ${p.border}`,
          borderHover: `1px solid ${p.primary}`,
          borderActive: `1px solid ${p.primary}`,
          borderFocus: `1px solid ${p.primary}`,
          textColor: p.textPrimary,
          placeholderColor: p.textMuted,
          caretColor: p.primary,
          arrowColor: p.textSecondary
        }
      }
    },

    Card: {
      color: 'var(--surface-panel)',
      colorModal: 'var(--surface-card)',
      colorPopover: 'var(--surface-card)',
      textColor: p.textPrimary,
      borderColor: p.border
    },

    Dialog: {
      color: 'var(--surface-card)',
      textColor: p.textPrimary,
      borderColor: p.border
    },

    Message: {
      // 浮动提示 = 浮起表面（注意：颜色这里仅作为兜底，
      // style.css 的 .n-message 书房风规则在其上叠加）
      color: 'var(--surface-card)',
      textColor: p.textPrimary,
      borderRadius: '4px'
    },

    Drawer: {
      color: 'var(--surface-card)',
      headerBorderBottom: `1px solid ${p.border}`,
      footerBorderTop: `1px solid ${p.border}`
    },

    Slider: {
      fillColor: p.primary,
      fillColorHover: p.primaryHover,
      handleColor: p.primary
    },

    Collapse: {
      titleTextColor: p.textPrimary,
      titleFontWeight: '500',
      arrowColor: p.textSecondary,
      dividerColor: p.borderLight
    },

    Form: {
      labelTextColor: p.textPrimary,
      labelFontWeight: '500',
      feedbackTextColorError: p.error,
      feedbackTextColorWarning: p.warning
    }
  }
}

/** 亮色 overrides（模块级构建一次，切主题零开销） */
export const lightOverrides: GlobalThemeOverrides = buildOverrides(LIGHT_PALETTE)

/** 深色 overrides（模块级构建一次，切主题零开销） */
export const darkOverrides: GlobalThemeOverrides = buildOverrides(DARK_PALETTE)

/**
 * 主题覆盖 composable
 *
 * 用法：
 * ```ts
 * const themeOverrides = useThemeOverrides()
 * // 传给 <n-config-provider :theme-overrides="themeOverrides">
 * ```
 *
 * 返回值是 ComputedRef<GlobalThemeOverrides>，会随 isDark 自动切换。
 * n-config-provider 接受 ref，会自动 unwrap，所以模板里直接传 ref 即可。
 */
export function useThemeOverrides(): ComputedRef<GlobalThemeOverrides> {
  const themeStore = useThemeStore()
  return computed(() => (themeStore.isDark ? darkOverrides : lightOverrides))
}
