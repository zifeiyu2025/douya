/// <reference types="node" />
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { LIGHT_PALETTE, DARK_PALETTE, type ThemePalette } from '../composables/useThemeOverrides'

/**
 * 豆芽主题令牌验证测试
 *
 * 验证各 CSS 变量均符合设计语言规范（纸面纯白 × 石墨深空，与 TRAE 配色同源）。
 *
 * 本测试通过读取 styles/tokens.css 文件（设计令牌单一事实来源），
 * 解析亮色与深色两个令牌块中的 CSS 变量，验证其值符合
 * 设计系统规范；同时对全部样式文件做"禁用色"扫描。
 */

// 读取令牌文件与全局样式文件内容
const tokensCssPath = resolve(__dirname, '../styles/tokens.css')
const tokensContent = readFileSync(tokensCssPath, 'utf-8')
const styleContent = readFileSync(resolve(__dirname, '../style.css'), 'utf-8')

/**
 * 从指定选择器块中提取 CSS 变量
 * @param content CSS 文件内容
 * @param selector 选择器名称（如 ':root' 或 'html.dark'）
 * @returns 变量名到值的映射
 */
function extractCssVars(content: string, selector: string): Record<string, string> {
  // 匹配选择器块：selector { ... }
  // 注意 :root 需要转义冒号
  const escaped = selector.replace(/:/g, '\\:')
  const blockRegex = new RegExp(`${escaped}\\s*\\{([\\s\\S]*?)\\}`, 'g')
  const match = blockRegex.exec(content)
  if (!match) return {}

  const blockContent = match[1]
  const vars: Record<string, string> = {}
  // 匹配 --var-name: value;
  const varRegex = /(--[\w-]+)\s*:\s*([^;]+);/g
  let varMatch: RegExpExecArray | null
  while ((varMatch = varRegex.exec(blockContent)) !== null) {
    vars[varMatch[1]] = varMatch[2].trim()
  }
  return vars
}

const lightVars = extractCssVars(tokensContent, ':root')
const darkVars = extractCssVars(tokensContent, 'html.dark')

describe('豆芽主题令牌 - 纸面（亮色模式，与 TRAE Light 同源）', () => {
  it('主色应为 TRAE 亮蓝 #2f74ff', () => {
    expect(lightVars['--accent-primary']).toBe('#2f74ff')
  })

  it('次主色应为亮蓝提亮 #5589ff', () => {
    expect(lightVars['--accent-secondary']).toBe('#5589ff')
  })

  it('主色浅背景应为亮蓝浅纸 #e4edff', () => {
    expect(lightVars['--accent-tertiary']).toBe('#e4edff')
  })

  it('成功色应为翠青 #40b08b', () => {
    expect(lightVars['--accent-success']).toBe('#40b08b')
  })

  it('警告色应为琥珀橙 #e28a00', () => {
    expect(lightVars['--accent-warning']).toBe('#e28a00')
  })

  it('危险色应为警示红 #e8463a', () => {
    expect(lightVars['--accent-danger']).toBe('#e8463a')
  })

  it('主背景应为纯白基底 #ffffff（中性白，对背景图友好）', () => {
    expect(lightVars['--bg-primary']).toBe('#ffffff')
  })

  it('次背景应为雾灰次面 #f8f8f9', () => {
    expect(lightVars['--bg-secondary']).toBe('#f8f8f9')
  })

  it('用户手记块接入三层表面体系（阅读层 panel，基色令牌保持便签冷灰 #eff2f6）', () => {
    expect(lightVars['--bg-user-msg-base']).toBe('#eff2f6')
    const { alphaVar } = parseColorMix(lightVars['--bg-user-msg'])
    expect(alphaVar).toBe('--surface-panel-alpha')
    expect(lightVars['--bg-user-msg']).toContain('var(--bg-user-msg-base)')
  })

  it('用户手记文字应为石墨 #31353a（与主文字一致，不做彩色）', () => {
    expect(lightVars['--text-user-msg']).toBe('#31353a')
  })

  it('边框色应为石灰细线 #dfe3ea', () => {
    expect(lightVars['--border-color']).toBe('#dfe3ea')
  })

  it('链接色应为 TRAE 亮链 #0066bf', () => {
    expect(lightVars['--link-light']).toBe('#0066bf')
  })

  it('主文字色应为石墨 #31353a', () => {
    expect(lightVars['--text-primary']).toBe('#31353a')
  })

  it('代码块背景接入三层表面体系（浮起层 card，基色令牌保持代码灰纸 #f4f5f7）', () => {
    expect(lightVars['--bg-code-base']).toBe('#f4f5f7')
    const { alphaVar } = parseColorMix(lightVars['--bg-code'])
    expect(alphaVar).toBe('--surface-card-alpha')
    expect(lightVars['--bg-code']).toContain('var(--bg-code-base)')
  })
})

describe('豆芽主题令牌 - 石墨（暗色模式，与 TRAE Dark 同源）', () => {
  it('主色应为 TRAE 辉光蓝 #387bff', () => {
    expect(darkVars['--accent-primary']).toBe('#387bff')
  })

  it('次主色应为辉光蓝提亮 #5b92ff', () => {
    expect(darkVars['--accent-secondary']).toBe('#5b92ff')
  })

  it('主色深背景应为深蓝底 #223052', () => {
    expect(darkVars['--accent-tertiary']).toBe('#223052')
  })

  it('成功色应为夜航翠 #00a56e', () => {
    expect(darkVars['--accent-success']).toBe('#00a56e')
  })

  it('警告色应为暗琥珀 #dc8730', () => {
    expect(darkVars['--accent-warning']).toBe('#dc8730')
  })

  it('危险色应为警示红 #f65a5a', () => {
    expect(darkVars['--accent-danger']).toBe('#f65a5a')
  })

  it('主背景应为石墨黑 #1a1b1d（非死黑，保留层次呼吸感）', () => {
    expect(darkVars['--bg-primary']).toBe('#1a1b1d')
  })

  it('次背景应为石墨次表面 #222427', () => {
    expect(darkVars['--bg-secondary']).toBe('#222427')
  })

  it('三背景应为浮起灰面 #2a2d31', () => {
    expect(darkVars['--bg-tertiary']).toBe('#2a2d31')
  })

  it('用户手记块接入三层表面体系（阅读层 panel，基色令牌保持便签深灰 #2a2d31）', () => {
    expect(darkVars['--bg-user-msg-base']).toBe('#2a2d31')
    const { alphaVar } = parseColorMix(darkVars['--bg-user-msg'])
    expect(alphaVar).toBe('--surface-panel-alpha')
    expect(darkVars['--bg-user-msg']).toContain('var(--bg-user-msg-base)')
  })

  it('用户手记文字应为雾白 #d1d3db（文字不彩色）', () => {
    expect(darkVars['--text-user-msg']).toBe('#d1d3db')
  })

  it('边框色应为石墨细线 #303031', () => {
    expect(darkVars['--border-color']).toBe('#303031')
  })

  it('链接色应为辉光蓝 #387bff', () => {
    expect(darkVars['--link-dark']).toBe('#387bff')
  })

  it('主文字色应为雾白 #d1d3db', () => {
    expect(darkVars['--text-primary']).toBe('#d1d3db')
  })

  it('代码块背景接入三层表面体系（浮起层 card，基色令牌保持代码墨底 #17191a）', () => {
    expect(darkVars['--bg-code-base']).toBe('#17191a')
    const { alphaVar } = parseColorMix(darkVars['--bg-code'])
    expect(alphaVar).toBe('--surface-card-alpha')
    expect(darkVars['--bg-code']).toContain('var(--bg-code-base)')
  })
})

describe('历史配色清扫（扫描令牌文件与全局样式）', () => {
  it('不应再出现微信绿 #07c160', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#07c160')
    expect(styleContent.toLowerCase()).not.toContain('#07c160')
  })

  it('不应再出现微信用户气泡绿 #95ec69', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#95ec69')
    expect(styleContent.toLowerCase()).not.toContain('#95ec69')
  })

  it('不应再出现 v4 深空苗圃主色 #3ddc97', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#3ddc97')
    expect(styleContent.toLowerCase()).not.toContain('#3ddc97')
  })

  it('不应再出现 v4 冷白实验室基底 #f5f8fa', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#f5f8fa')
    expect(styleContent.toLowerCase()).not.toContain('#f5f8fa')
  })
})

describe('深色模式背景层次正确（primary 最深，tertiary 最浅）', () => {
  it('深色模式 bg-primary 应比 bg-secondary 深', () => {
    // #1a1b1d 应比 #222427 深
    const primary = parseHexColor(darkVars['--bg-primary'])
    const secondary = parseHexColor(darkVars['--bg-secondary'])
    expect(luminance(primary)).toBeLessThan(luminance(secondary))
  })

  it('深色模式 bg-secondary 应比 bg-tertiary 深', () => {
    const secondary = parseHexColor(darkVars['--bg-secondary'])
    const tertiary = parseHexColor(darkVars['--bg-tertiary'])
    expect(luminance(secondary)).toBeLessThan(luminance(tertiary))
  })
})

describe('令牌文件完整性（防回归守卫）', () => {
  it('style.css 中不应再定义令牌块（已迁移至 tokens.css）', () => {
    expect(styleContent).not.toMatch(/^:root\s*\{/m)
    expect(styleContent).not.toMatch(/^html\.dark\s*\{/m)
  })

  it('tokens.css 应定义全部核心布局尺寸令牌', () => {
    for (const key of [
      '--sidebar-width',
      '--header-height',
      '--border-radius-sm',
      '--border-radius-lg',
      '--transition-fast'
    ]) {
      expect(lightVars[key]).toBeTruthy()
    }
  })

  it('深色令牌应是亮色令牌的子集（防止单侧拼错变量名；布局/圆角/动效等主题无关令牌仅亮色定义）', () => {
    const normalizeKey = (k: string) => k.replace(/-(light|dark)$/, '')
    const lightKeys = new Set(Object.keys(lightVars).map(normalizeKey))
    const darkKeys = Object.keys(darkVars).map(k => normalizeKey(k))
    const orphan = darkKeys.filter(k => !lightKeys.has(k))
    expect(orphan).toEqual([])
  })
})

/**
 * 解析十六进制颜色为 RGB
 */
function parseHexColor(hex: string): { r: number; g: number; b: number } {
  const cleaned = hex.replace('#', '')
  return {
    r: parseInt(cleaned.substring(0, 2), 16),
    g: parseInt(cleaned.substring(2, 4), 16),
    b: parseInt(cleaned.substring(4, 6), 16)
  }
}

/**
 * 计算相对亮度（用于比较颜色深浅）
 * 数值越小颜色越深
 */
function luminance({ r, g, b }: { r: number; g: number; b: number }): number {
  return (0.299 * r + 0.587 * g + 0.114 * b) / 255
}

/**
 * 从 color-mix(...) 复合令牌值中提取基色与所引用的 alpha 变量名
 *
 * 例：'color-mix(in srgb, #17191a calc(var(--surface-card-alpha) * 100%), transparent)'
 * → { base: '#17191a', alphaVar: '--surface-card-alpha' }
 */
function parseColorMix(value: string): { base: string; alphaVar: string } {
  const base = value.match(/#[0-9a-fA-F]{6}/)?.[0] ?? ''
  // 复合令牌中可能同时引用 var(--bg-primary) 与 var(--surface-x-alpha)，
  // alpha 驱动变量位于 calc() 内，这里专门匹配它
  const alphaVar = value.match(/calc\(var\((--[\w-]+)\)/)?.[1] ?? ''
  return { base, alphaVar }
}

describe('三层表面体系令牌（背景架构核心）', () => {
  // 三层语义：veil 结构层最透 / panel 阅读层保读 / card 浮起层最实
  const layerAlphaPairs: Array<[string, string]> = [
    ['--surface-veil', '--surface-veil-alpha'],
    ['--surface-panel', '--surface-panel-alpha'],
    ['--surface-card', '--surface-card-alpha']
  ]

  it.each(['--surface-veil-alpha', '--surface-panel-alpha', '--surface-card-alpha'])(
    '亮色与深色都定义 %s 且默认为 1（无背景图时全实色兜底）',
    key => {
      expect(lightVars[key]).toBe('1')
      expect(darkVars[key]).toBe('1')
    }
  )

  it('三层复合令牌必须引用各自对应的 alpha 变量（亮/暗一致）', () => {
    for (const [token, alphaVar] of layerAlphaPairs) {
      expect(parseColorMix(lightVars[token]).alphaVar).toBe(alphaVar)
      expect(parseColorMix(darkVars[token]).alphaVar).toBe(alphaVar)
    }
  })

  it('复合令牌基色为 var(--bg-primary)（使用处求值，随主题自动换基色）', () => {
    for (const [token] of layerAlphaPairs) {
      expect(lightVars[token]).toContain('var(--bg-primary)')
      expect(darkVars[token]).toContain('var(--bg-primary)')
    }
  })

  it('背景图效果参数兜底：亮色块定义默认值，深色经 CSS 继承共享', () => {
    expect(lightVars['--chat-background']).toBe('none')
    expect(lightVars['--chat-background-blur']).toBe('0px')
    expect(lightVars['--chat-background-mask']).toBe('0')
    // 深色块只覆写遮罩色，其余背景图参数依赖 html.dark 对 :root 的原生继承
    expect(darkVars['--chat-background-mask-color']).toBe('#101113')
  })

  it('全部表面类变量接入对应层（抽屉 veil / 手记输入 panel / 代码块 card）', () => {
    const expectations: Array<[Record<string, string>, string, string]> = [
      // [变量表, 变量名, 期望引用的 alpha]
      [lightVars, '--bg-sidebar', '--surface-veil-alpha'],
      [lightVars, '--bg-ai-msg', '--surface-panel-alpha'],
      [lightVars, '--bg-input', '--surface-panel-alpha'],
      [darkVars, '--bg-sidebar', '--surface-veil-alpha'],
      [darkVars, '--bg-ai-msg', '--surface-panel-alpha'],
      [darkVars, '--bg-input', '--surface-panel-alpha']
    ]
    for (const [vars, key, expectedAlpha] of expectations) {
      expect(parseColorMix(vars[key]).alphaVar).toBe(expectedAlpha)
    }
  })

  it('hover/active 反馈色保持实色 hex（Naive UI 内部混色运算依赖实色）', () => {
    for (const key of ['--bg-hover', '--bg-active']) {
      expect(lightVars[key]).toMatch(/^#[0-9a-fA-F]{6}$/)
      expect(darkVars[key]).toMatch(/^#[0-9a-fA-F]{6}$/)
    }
  })
})

describe('PALETTE ↔ tokens.css 自动比对契约', () => {
  /**
   * useThemeOverrides.ts 的调色板键 ↔ tokens.css 的 CSS 变量名映射。
   * 两边任何一边改动而另一边没同步，本组测试立刻变红 —— 把
   * 「手动同步维护要求」变成机器强制执行。
   *
   * 注：textDisabled / bgMuted 在两主题中对应的 CSS 变量不一致
   * （仅作为 Naive UI 独立取值），不参与本契约比对。
   */
  const paletteToCssVar: Array<[keyof ThemePalette, string]> = [
    ['primary', '--accent-primary'],
    ['primaryHover', '--accent-secondary'],
    ['success', '--accent-success'],
    ['warning', '--accent-warning'],
    ['error', '--accent-danger'],
    ['textPrimary', '--text-primary'],
    ['textSecondary', '--text-secondary'],
    ['textMuted', '--text-muted'],
    ['bgBase', '--bg-primary'],
    ['bgSubtle', '--bg-secondary'],
    ['bgHover', '--bg-hover'],
    ['bgActive', '--bg-active'],
    ['border', '--border-color'],
    ['borderLight', '--border-light']
  ]

  it('LIGHT_PALETTE 与 :root 令牌一一对应', () => {
    for (const [paletteKey, cssVar] of paletteToCssVar) {
      expect(LIGHT_PALETTE[paletteKey].toLowerCase()).toBe(lightVars[cssVar]?.toLowerCase())
    }
  })

  it('DARK_PALETTE 与 html.dark 令牌一一对应', () => {
    for (const [paletteKey, cssVar] of paletteToCssVar) {
      expect(DARK_PALETTE[paletteKey].toLowerCase()).toBe(darkVars[cssVar]?.toLowerCase())
    }
  })
})
