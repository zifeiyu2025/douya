/**
 * Naive UI 全局主题覆盖（themeOverrides）
 *
 * 生活类比：就像给一套精装修房子重新刷漆——Naive UI 自带一套"绿色"配色
 * （primaryColor=#18a058），但我们房子（项目）的设计稿是"GitHub 蓝"
 * （--accent-primary: #0969da）。这个 composable 就是把 Naive UI 的
 * 每一个颜色"开关"（token）都对接到我们项目的 CSS 变量值上，让所有
 * Naive UI 组件（Button/Input/Dialog/Message 等）显示项目配色而不是默认绿色。
 *
 * 设计决策：硬编码两套 overrides（light/dark），通过 isDark 切换
 *  - 优点 1：性能最佳（无 DOM 读取，无 getComputedStyle 调用）
 *  - 优点 2：不依赖 DOM，SSR 安全，可在 pinia 任意上下文使用
 *  - 缺点：需要与 styles/tokens.css 的亮色/深色令牌块手动同步
 *
 * ⚠️ 同步维护要求：修改本文件颜色值时，必须同步修改 styles/tokens.css 中
 *   亮色与深色两个令牌块的对应 CSS 变量值，反之亦然。
 *   themeTokens.test.ts 会校验 tokens.css 的令牌值，但不会校验本文件，
 *   所以改动时请手动核对两处一致。
 */
import { computed, type ComputedRef } from 'vue'
import type { GlobalThemeOverrides } from 'naive-ui'
import { useThemeStore } from '../stores/theme'

/**
 * 亮色模式 overrides
 * 与 styles/tokens.css 亮色令牌块保持同步
 */
const lightOverrides: GlobalThemeOverrides = {
  common: {
    // ===== 主色调（与 --accent-* 同步）=====
    primaryColor: '#0969da', // --accent-primary
    primaryColorHover: '#218bff', // --accent-secondary
    primaryColorPressed: '#0550ae', // 比 primary 深（Naive UI 按下态）
    primaryColorSuppl: '#0969da', // 与 primaryColor 一致（补充态）
    infoColor: '#0969da', // 复用 primary（项目无独立 info 色）
    infoColorHover: '#218bff',
    infoColorPressed: '#0550ae',
    infoColorSuppl: '#0969da',
    successColor: '#1f883d', // --accent-success
    successColorHover: '#1a7f37',
    successColorPressed: '#1a7f37',
    successColorSuppl: '#1f883d',
    warningColor: '#d4a72c', // --accent-warning
    warningColorHover: '#bf8700',
    warningColorPressed: '#9a6700',
    warningColorSuppl: '#d4a72c',
    errorColor: '#cf222e', // --accent-danger
    errorColorHover: '#a40e26',
    errorColorPressed: '#82071e',
    errorColorSuppl: '#cf222e',

    // ===== 文字色系（与 --text-* 同步）=====
    textColorBase: '#1f2328', // --text-primary
    textColor1: '#1f2328', // --text-primary（主文字）
    textColor2: '#656d76', // --text-secondary（次文字）
    textColor3: '#848d97', // --text-muted（静默文字）
    placeholderColor: '#848d97', // --text-muted（占位符）
    placeholderColorDisabled: '#afb8c1',
    iconColor: '#656d76',
    iconColorHover: '#1f2328',
    iconColorPressed: '#1f2328',
    iconColorDisabled: '#afb8c1',

    // ===== 边框 / 分隔线 =====
    borderColor: '#d0d7de', // --border-color
    dividerColor: '#eaeef2', // --border-light

    // ===== 背景色系（与 --bg-* 同步）=====
    bodyColor: '#fbfbfc', // --bg-primary
    cardColor: '#f3f4f7', // --bg-secondary
    modalColor: '#fbfbfc', // 模态用主背景
    popoverColor: '#fbfbfc', // 弹出层用主背景
    tableColor: '#fbfbfc',
    inputColor: '#fbfbfc', // --bg-input
    inputColorDisabled: '#f3f4f7',
    actionColor: '#f3f4f7',
    hoverColor: '#eceef2', // --bg-hover
    tableColorHover: '#eceef2',
    tableColorStriped: '#f3f4f7',

    // ===== 圆角（与 style.css --border-radius-* 对齐）=====
    borderRadius: '8px', // --border-radius-sm
    borderRadiusSmall: '4px', // --border-radius-xs

    // ===== 关闭按钮（Naive UI common 用 closeIconColor 系列）=====
    closeIconColor: '#848d97', // 关闭图标颜色（--text-muted）
    closeIconColorHover: '#1f2328', // hover 时图标颜色（--text-primary）
    closeIconColorPressed: '#1f2328', // pressed 时图标颜色
    closeColorHover: '#eceef2', // hover 时背景色（--bg-hover）
    closeColorPressed: '#e0e3e9' // pressed 时背景色（--bg-active）
  },

  Button: {
    // 主色按钮文字始终白色（保证对比度）
    textColorPrimary: '#ffffff',
    textColorHover: '#ffffff',
    textColorPressed: '#ffffff',
    textColorFocus: '#ffffff'
  },

  Input: {
    color: '#fbfbfc', // --bg-input
    colorFocus: '#fbfbfc',
    border: '1px solid #d0d7de', // --border-color
    borderHover: '1px solid #0969da', // 聚焦 hover 用 primary
    borderFocus: '1px solid #0969da',
    textColor: '#1f2328', // --text-primary
    placeholderColor: '#848d97', // --text-muted
    caretColor: '#0969da' // --accent-primary
  },

  Select: {
    // Select 内部使用 InternalSelection（不是 Input），peers 同步配色
    peers: {
      InternalSelection: {
        color: '#fbfbfc',
        colorActive: '#fbfbfc',
        border: '1px solid #d0d7de',
        borderHover: '1px solid #0969da',
        borderActive: '1px solid #0969da',
        borderFocus: '1px solid #0969da',
        textColor: '#1f2328',
        placeholderColor: '#848d97',
        caretColor: '#0969da',
        arrowColor: '#656d76'
      }
    }
  },

  Card: {
    color: '#f3f4f7', // --bg-secondary
    colorModal: '#fbfbfc',
    colorPopover: '#fbfbfc',
    textColor: '#1f2328', // --text-primary
    borderColor: '#d0d7de' // --border-color
  },

  Dialog: {
    color: '#fbfbfc', // --bg-primary
    textColor: '#1f2328', // --text-primary
    borderColor: '#d0d7de' // --border-color
  },

  Message: {
    // 注意：颜色这里仅作为兜底；style.css 的 .n-message 规则仍会覆盖
    // 但移除 !important 后，Naive UI 内联样式优先级更高，所以这里必须正确
    color: '#fbfbfc', // --bg-primary
    textColor: '#1f2328', // --text-primary
    borderRadius: '10px'
  },

  Drawer: {
    color: '#fbfbfc', // --bg-primary
    headerBorderBottom: '1px solid #d0d7de',
    footerBorderTop: '1px solid #d0d7de'
  },

  Slider: {
    fillColor: '#0969da', // --accent-primary
    fillColorHover: '#218bff', // --accent-secondary
    handleColor: '#0969da' // --accent-primary
  },

  Collapse: {
    titleTextColor: '#1f2328', // --text-primary
    titleFontWeight: '500',
    arrowColor: '#656d76', // --text-secondary
    dividerColor: '#eaeef2' // --border-light
  },

  Form: {
    labelTextColor: '#1f2328', // --text-primary
    labelFontWeight: '500',
    feedbackTextColorError: '#cf222e', // --accent-danger
    feedbackTextColorWarning: '#d4a72c' // --accent-warning
  }
}

/**
 * 深色模式 overrides
 * 与 styles/tokens.css 深色令牌块保持同步
 */
const darkOverrides: GlobalThemeOverrides = {
  common: {
    // ===== 主色调（GitHub dark_high_contrast 蓝）=====
    primaryColor: '#4493f8', // --accent-primary
    primaryColorHover: '#5b9eff', // --accent-secondary
    primaryColorPressed: '#1f6feb', // 比 primary 深
    primaryColorSuppl: '#4493f8',
    infoColor: '#4493f8',
    infoColorHover: '#5b9eff',
    infoColorPressed: '#1f6feb',
    infoColorSuppl: '#4493f8',
    successColor: '#3fb950', // --accent-success
    successColorHover: '#46c144',
    successColorPressed: '#2da44e',
    successColorSuppl: '#3fb950',
    warningColor: '#d29922', // --accent-warning
    warningColorHover: '#e3b341',
    warningColorPressed: '#bb8009',
    warningColorSuppl: '#d29922',
    errorColor: '#f85149', // --accent-danger
    errorColorHover: '#ff6b6b',
    errorColorPressed: '#da3633',
    errorColorSuppl: '#f85149',

    // ===== 文字色系（GitHub dark_high_contrast 高对比度白）=====
    textColorBase: '#f0f6fc', // --text-primary
    textColor1: '#f0f6fc', // --text-primary
    textColor2: '#c9d1d9', // --text-secondary
    textColor3: '#8b949e', // --text-muted
    placeholderColor: '#8b949e', // --text-muted
    placeholderColorDisabled: '#6e7681',
    iconColor: '#c9d1d9',
    iconColorHover: '#f0f6fc',
    iconColorPressed: '#f0f6fc',
    iconColorDisabled: '#6e7681',

    // ===== 边框 / 分隔线 =====
    borderColor: '#30363d', // --border-color
    dividerColor: '#161b22', // --border-light

    // ===== 背景色系（纯黑 + GitHub dark 层次）=====
    bodyColor: '#000000', // --bg-primary
    cardColor: '#0d1117', // --bg-secondary
    modalColor: '#000000',
    popoverColor: '#0d1117',
    tableColor: '#0d1117',
    inputColor: '#000000', // --bg-input
    inputColorDisabled: '#161b22',
    actionColor: '#161b22',
    hoverColor: '#161b22', // --bg-hover
    tableColorHover: '#161b22',
    tableColorStriped: '#0d1117',

    // ===== 圆角 =====
    borderRadius: '8px',
    borderRadiusSmall: '4px',

    // ===== 关闭按钮 =====
    closeIconColor: '#8b949e',
    closeIconColorHover: '#f0f6fc',
    closeIconColorPressed: '#f0f6fc',
    closeColorHover: '#161b22',
    closeColorPressed: '#21262d'
  },

  Button: {
    textColorPrimary: '#ffffff',
    textColorHover: '#ffffff',
    textColorPressed: '#ffffff',
    textColorFocus: '#ffffff'
  },

  Input: {
    color: '#000000', // --bg-input
    colorFocus: '#000000',
    border: '1px solid #30363d', // --border-color
    borderHover: '1px solid #4493f8', // primary
    borderFocus: '1px solid #4493f8',
    textColor: '#f0f6fc', // --text-primary
    placeholderColor: '#8b949e', // --text-muted
    caretColor: '#4493f8' // --accent-primary
  },

  Select: {
    peers: {
      InternalSelection: {
        color: '#000000',
        colorActive: '#000000',
        border: '1px solid #30363d',
        borderHover: '1px solid #4493f8',
        borderActive: '1px solid #4493f8',
        borderFocus: '1px solid #4493f8',
        textColor: '#f0f6fc',
        placeholderColor: '#8b949e',
        caretColor: '#4493f8',
        arrowColor: '#c9d1d9'
      }
    }
  },

  Card: {
    color: '#0d1117', // --bg-secondary
    colorModal: '#000000',
    colorPopover: '#0d1117',
    textColor: '#f0f6fc', // --text-primary
    borderColor: '#30363d' // --border-color
  },

  Dialog: {
    color: '#000000', // --bg-primary
    textColor: '#f0f6fc', // --text-primary
    borderColor: '#30363d' // --border-color
  },

  Message: {
    color: '#0d1117', // --bg-secondary（深色用 secondary 提升层次）
    textColor: '#f0f6fc', // --text-primary
    borderRadius: '10px'
  },

  Drawer: {
    color: '#000000', // --bg-primary
    headerBorderBottom: '1px solid #30363d',
    footerBorderTop: '1px solid #30363d'
  },

  Slider: {
    fillColor: '#4493f8', // --accent-primary
    fillColorHover: '#5b9eff', // --accent-secondary
    handleColor: '#4493f8' // --accent-primary
  },

  Collapse: {
    titleTextColor: '#f0f6fc', // --text-primary
    titleFontWeight: '500',
    arrowColor: '#c9d1d9', // --text-secondary
    dividerColor: '#161b22' // --border-light
  },

  Form: {
    labelTextColor: '#f0f6fc', // --text-primary
    labelFontWeight: '500',
    feedbackTextColorError: '#f85149', // --accent-danger
    feedbackTextColorWarning: '#d29922' // --accent-warning
  }
}

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
