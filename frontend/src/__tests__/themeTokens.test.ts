/// <reference types="node" />
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

/**
 * GitHub 主题令牌验证测试
 *
 * 生活类比：就像装修房子前先量好尺寸清单，确保每块瓷砖（CSS 变量）
 * 都符合设计师（GitHub Primer）的规格要求，避免装错颜色。
 *
 * 本测试通过读取 styles/tokens.css 文件（设计令牌单一事实来源），
 * 解析亮色与深色两个令牌块中的 CSS 变量，验证其值符合
 * GitHub Primer 设计系统规范；同时对全部样式文件做"禁用色"扫描。
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

describe('GitHub 主题令牌 - 亮色模式', () => {
  it('主色调应为 GitHub 蓝 #0969da', () => {
    expect(lightVars['--accent-primary']).toBe('#0969da')
  })

  it('次主色应为 #218bff', () => {
    expect(lightVars['--accent-secondary']).toBe('#218bff')
  })

  it('主色浅色背景应为 #ddf4ff', () => {
    expect(lightVars['--accent-tertiary']).toBe('#ddf4ff')
  })

  it('成功色应为 GitHub 绿 #1f883d', () => {
    expect(lightVars['--accent-success']).toBe('#1f883d')
  })

  it('警告色应为 #d4a72c', () => {
    expect(lightVars['--accent-warning']).toBe('#d4a72c')
  })

  it('危险色应为 #cf222e', () => {
    expect(lightVars['--accent-danger']).toBe('#cf222e')
  })

  it('主背景应为柔和米白 #fbfbfc', () => {
    expect(lightVars['--bg-primary']).toBe('#fbfbfc')
  })

  it('次背景应为柔和米白 #f3f4f7', () => {
    expect(lightVars['--bg-secondary']).toBe('#f3f4f7')
  })

  it('用户气泡背景应为柔和蓝灰 #e3edf7', () => {
    expect(lightVars['--bg-user-msg']).toBe('#e3edf7')
  })

  it('用户气泡文字应为 GitHub 蓝 #0969da', () => {
    expect(lightVars['--text-user-msg']).toBe('#0969da')
  })

  it('边框色应为 GitHub default #d0d7de', () => {
    expect(lightVars['--border-color']).toBe('#d0d7de')
  })

  it('链接色应为 GitHub 蓝 #0969da', () => {
    expect(lightVars['--link-light']).toBe('#0969da')
  })

  it('主文字色应为 GitHub default #1f2328', () => {
    expect(lightVars['--text-primary']).toBe('#1f2328')
  })

  it('代码块背景应为柔和米白 #e8eaef', () => {
    expect(lightVars['--bg-code']).toBe('#e8eaef')
  })
})

describe('GitHub 主题令牌 - 深色模式', () => {
  it('主色调应为 GitHub dark_high_contrast 蓝 #4493f8', () => {
    expect(darkVars['--accent-primary']).toBe('#4493f8')
  })

  it('次主色应为 #5b9eff', () => {
    expect(darkVars['--accent-secondary']).toBe('#5b9eff')
  })

  it('主色深色背景应为 #0c2d6b', () => {
    expect(darkVars['--accent-tertiary']).toBe('#0c2d6b')
  })

  it('成功色应为 GitHub 深色绿 #3fb950', () => {
    expect(darkVars['--accent-success']).toBe('#3fb950')
  })

  it('危险色应为 #f85149', () => {
    expect(darkVars['--accent-danger']).toBe('#f85149')
  })

  it('主背景应为纯黑 #000000（交互区域纯黑）', () => {
    expect(darkVars['--bg-primary']).toBe('#000000')
  })

  it('次背景应为 GitHub 深色 subtle #0d1117', () => {
    expect(darkVars['--bg-secondary']).toBe('#0d1117')
  })

  it('三背景应为 GitHub 深色 muted #161b22', () => {
    expect(darkVars['--bg-tertiary']).toBe('#161b22')
  })

  it('用户气泡背景应为深色 GitHub 蓝 #0c2d6b', () => {
    expect(darkVars['--bg-user-msg']).toBe('#0c2d6b')
  })

  it('用户气泡文字应为深色 GitHub 蓝 #4493f8', () => {
    expect(darkVars['--text-user-msg']).toBe('#4493f8')
  })

  it('边框色应为 GitHub 深色 default #30363d', () => {
    expect(darkVars['--border-color']).toBe('#30363d')
  })

  it('链接色应为 GitHub dark_high_contrast 蓝 #4493f8', () => {
    expect(darkVars['--link-dark']).toBe('#4493f8')
  })

  it('主文字色应为 GitHub dark_high_contrast default #f0f6fc（高对比度白）', () => {
    expect(darkVars['--text-primary']).toBe('#f0f6fc')
  })

  it('代码块背景应为 #0d1117', () => {
    expect(darkVars['--bg-code']).toBe('#0d1117')
  })
})

describe('移除微信绿配色（扫描令牌文件与全局样式）', () => {
  it('不应再出现微信绿 #07c160', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#07c160')
    expect(styleContent.toLowerCase()).not.toContain('#07c160')
  })

  it('不应再出现微信用户气泡绿 #95ec69', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#95ec69')
    expect(styleContent.toLowerCase()).not.toContain('#95ec69')
  })

  it('不应再出现深色微信绿 #86e6ab', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#86e6ab')
    expect(styleContent.toLowerCase()).not.toContain('#86e6ab')
  })

  it('不应再出现深色用户气泡绿 #4a9f44', () => {
    expect(tokensContent.toLowerCase()).not.toContain('#4a9f44')
    expect(styleContent.toLowerCase()).not.toContain('#4a9f44')
  })
})

describe('深色模式背景层次正确（primary 最深，tertiary 最浅）', () => {
  it('深色模式 bg-primary 应比 bg-secondary 深', () => {
    // #0d1117 (13,17,23) 应比 #161b22 (22,27,34) 深
    const primary = parseHexColor(darkVars['--bg-primary'])
    const secondary = parseHexColor(darkVars['--bg-secondary'])
    const primaryLum = luminance(primary)
    const secondaryLum = luminance(secondary)
    expect(primaryLum).toBeLessThan(secondaryLum)
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
      '--msg-max-width',
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
