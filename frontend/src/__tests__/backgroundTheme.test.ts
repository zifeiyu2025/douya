/// <reference types="node" />
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { buildBackgroundStyle } from '../utils/backgroundStyle'
import type { ThemeBackgroundParams } from '../types/chat'

/**
 * 背景「双层绘制模型」测试
 *
 * 架构说明：
 *   - ::before 图层：只负责绘制背景图 + 可选模糊，不做任何染色；
 *   - ::after 遮罩层：按 mask_alpha 强度叠色（亮色叠白 / 深色叠黑），默认 0 即完全不染色。
 * 组件底色由三层表面令牌（veil/panel/card）按需透出背景。
 *
 * 验收点：
 *   - buildBackgroundStyle 输出 7 个 CSS 变量（图/透明度/模糊/遮罩 + 三层表面 alpha）
 *   - 不存在 .has-background 类开关；App.vue 按主题选参数集；style.css 为双层模型
 *   - AppearanceSettings 无“背景图仅深色可用”限制
 */

// ESM 下通过 import.meta.url 计算 __dirname，用于读取源码做代码审查
const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// 读取源码文件路径（供下方代码审查类用例使用）
const APP_VUE_PATH = resolve(__dirname, '../App.vue')
const STYLE_CSS_PATH = resolve(__dirname, '../style.css')
const APPEARANCE_SETTINGS_VUE_PATH = resolve(
  __dirname,
  '../components/settings/AppearanceSettings.vue'
)
const SETTINGS_CORE_PATH = resolve(
  __dirname,
  '../components/settings/composables/useSettingsCore.ts'
)

describe('背景双层绘制模型', () => {
  beforeEach(() => {
    // 当前测试文件不依赖 pinia / localStorage / matchMedia，
    // 但保留 beforeEach 用于未来扩展时的状态清理隔离
    vi.clearAllMocks()
  })

  // ===== buildBackgroundStyle 注入 7 个 CSS 变量 =====
  describe('buildBackgroundStyle 注入 CSS 变量', () => {
    it('完整参数 → 返回图 + opacity/blur/mask_alpha + 三层表面 alpha + 七个固化令牌覆盖共 14 个变量', () => {
      const params: ThemeBackgroundParams = { opacity: 0.8, blur: 6, mask_alpha: 0.25 }
      const style = buildBackgroundStyle('/path/to/bg.png', params)
      expect(style).toEqual({
        '--chat-background': 'url(/local-file/%2Fpath%2Fto%2Fbg.png)',
        '--chat-background-opacity': '0.8',
        '--chat-background-blur': '6px',
        '--chat-background-mask-alpha': '0.25',
        '--surface-veil-alpha': '0.45',
        '--surface-panel-alpha': '0.7',
        '--surface-card-alpha': '0.82',
        // 固化令牌覆盖：var() 替换发生在声明处（:root），仅覆盖 alpha 无法
        // 让 --surface-veil 等重新求值，必须在挂载点直接覆盖令牌本身
        '--surface-veil': 'color-mix(in srgb, var(--bg-primary) 45%, transparent)',
        '--surface-panel': 'color-mix(in srgb, var(--bg-primary) 70%, transparent)',
        '--surface-card': 'color-mix(in srgb, var(--bg-primary) 82%, transparent)',
        '--bg-user-msg': 'color-mix(in srgb, var(--bg-user-msg-base) 70%, transparent)',
        '--bg-ai-msg': 'color-mix(in srgb, var(--bg-ai-msg-base) 70%, transparent)',
        '--bg-input': 'color-mix(in srgb, var(--bg-input-base) 70%, transparent)',
        '--bg-code': 'color-mix(in srgb, var(--bg-code-base) 82%, transparent)'
      })
    })

    it('chat_background 为空字符串 → 返回空对象（无背景时不产生任何变量）', () => {
      expect(buildBackgroundStyle('', { opacity: 0.9, blur: 0, mask_alpha: 0 })).toEqual({})
      expect(buildBackgroundStyle('')).toEqual({})
    })

    it('data: URL → 直接使用，不经过 /local-file/ 编码', () => {
      const dataUrl = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=='
      const style = buildBackgroundStyle(dataUrl, null)
      expect(style['--chat-background']).toBe(`url(${dataUrl})`)
      expect(style['--chat-background']).not.toContain('/local-file/')
    })

    it('文件路径含特殊字符 → encodeURIComponent 正确编码', () => {
      const filePath = 'D:/我的 图片/bg test.png'
      const style = buildBackgroundStyle(filePath, null)
      const encoded = encodeURIComponent(filePath)
      expect(style['--chat-background']).toBe(`url(/local-file/${encoded})`)
    })

    it('params 缺省 → opacity 默认 0.85、blur 默认 0px、mask_alpha 默认 0（不染色）', () => {
      const style = buildBackgroundStyle('/path/to/bg.png')
      expect(style['--chat-background-opacity']).toBe('0.85')
      expect(style['--chat-background-blur']).toBe('0px')
      expect(style['--chat-background-mask-alpha']).toBe('0')
    })

    it('opacity 显式传 0 → 保留 0（?? 语义不被默认值覆盖）', () => {
      const style = buildBackgroundStyle('/path/to/bg.png', {
        opacity: 0,
        blur: 0,
        mask_alpha: 0
      })
      expect(style['--chat-background-opacity']).toBe('0')
    })

    it('参数越界 → 钳位到合法区间（opacity/mask_alpha ∈ [0,1]，blur ≥ 0）', () => {
      const style = buildBackgroundStyle('/p.png', { opacity: 1.5, blur: -3, mask_alpha: 2 })
      expect(style['--chat-background-opacity']).toBe('1')
      expect(style['--chat-background-mask-alpha']).toBe('1')
      expect(style['--chat-background-blur']).toBe('0px')
    })

    it('blur 允许大于 1px 的任意非负值', () => {
      const style = buildBackgroundStyle('/p.png', { opacity: 1, blur: 12, mask_alpha: 0 })
      expect(style['--chat-background-blur']).toBe('12px')
    })

    it('非法数值（NaN）→ 回退默认值', () => {
      const style = buildBackgroundStyle('/p.png', {
        opacity: Number.NaN,
        blur: Number.NaN,
        mask_alpha: Number.NaN
      })
      expect(style['--chat-background-opacity']).toBe('0.85')
      expect(style['--chat-background-blur']).toBe('0px')
      expect(style['--chat-background-mask-alpha']).toBe('0')
    })
  })

  // ===== .has-background 补丁体系已彻底移除 =====
  describe('双层绘制模型架构（代码审查）', () => {
    it('App.vue 不再包含 has-background 类开关', () => {
      const content = readFileSync(APP_VUE_PATH, 'utf-8')
      expect(content).not.toContain('has-background')
    })

    it('App.vue 模板 class 绑定仅剩 dark 开关，外观完全由 CSS 变量驱动', () => {
      const content = readFileSync(APP_VUE_PATH, 'utf-8')
      expect(content).toContain(':class="{ dark: isDark }"')
    })

    it('App.vue mainAreaStyle 按 isDark 选择 background_light / background_dark 参数集', () => {
      const content = readFileSync(APP_VUE_PATH, 'utf-8')
      const block =
        content.match(/const mainAreaStyle = computed\(\(\) => \{[\s\S]*?\}\)/)?.[0] ?? ''
      expect(block).toBeTruthy()
      expect(block).toContain('background_light')
      expect(block).toContain('background_dark')
      expect(block).toContain('buildBackgroundStyle')
    })

    it('style.css 采用双层伪元素：::before 纯图层（画图+模糊+出血）、::after 遮罩层（可调染色）', () => {
      const content = readFileSync(STYLE_CSS_PATH, 'utf-8')
      // 图层块：只消费变量画图与模糊，不含任何渐变染色（根治亮色白雾）
      const beforeBlock = content.match(/\.app-layout::before\s*\{[\s\S]*?\}/)?.[0] ?? ''
      expect(beforeBlock).toBeTruthy()
      expect(beforeBlock).toContain('--chat-background')
      expect(beforeBlock).toContain('--chat-background-blur')
      expect(beforeBlock).toContain('--chat-background-opacity')
      expect(beforeBlock).toContain('-48px') // 四周出血，防模糊边缘发虚
      expect(beforeBlock).not.toContain('gradient')
      // 遮罩块：强度由 mask_alpha 驱动，默认 0 即完全不染色
      const afterBlock = content.match(/\.app-layout::after\s*\{[\s\S]*?\}/)?.[0] ?? ''
      expect(afterBlock).toBeTruthy()
      expect(afterBlock).toContain('--chat-background-mask-alpha')
      // 深色模式遮罩颜色切换为黑（html.dark 前缀规则存在）
      expect(content).toMatch(/html\.dark \.app-layout::after/)
    })

    it('style.css 全文不再出现 has-background 选择器', () => {
      const content = readFileSync(STYLE_CSS_PATH, 'utf-8')
      expect(content).not.toContain('has-background')
    })
  })

  // ===== 验证 AppearanceSettings 无“仅深色可用”限制 =====
  describe('AppearanceSettings 无主题限制', () => {
    it('AppearanceSettings.vue 不包含“背景图仅支持深色模式”禁用提示', () => {
      const content = readFileSync(APPEARANCE_SETTINGS_VUE_PATH, 'utf-8')
      expect(content).not.toContain('背景图仅支持深色模式')
      expect(content).not.toContain('仅支持深色模式')
    })

    it('AppearanceSettings.vue 背景图上传区不再被 v-if="!isDark" 禁用', () => {
      const content = readFileSync(APPEARANCE_SETTINGS_VUE_PATH, 'utf-8')
      // 不应存在用 !isDark 禁用背景图上传区的条件
      expect(content).not.toContain('v-if="!isDark"')
    })

    it('AppearanceSettings.vue 聊天背景 form-item 始终显示（无 isDark 条件）', () => {
      const content = readFileSync(APPEARANCE_SETTINGS_VUE_PATH, 'utf-8')
      // 定位“聊天背景”form-item 块
      const bgBlock =
        content.match(/<n-form-item label="聊天背景">[\s\S]*?<\/n-form-item>/)?.[0] ?? ''
      expect(bgBlock).toBeTruthy()
      // 背景图区块不应被 isDark 条件包裹
      expect(bgBlock).not.toContain('isDark')
    })

    it('AppearanceSettings.vue 上传占位区在无背景图时显示（v-if 仅依赖 chat_background）', () => {
      const content = readFileSync(APPEARANCE_SETTINGS_VUE_PATH, 'utf-8')
      // bg-upload-placeholder 的 v-if 应仅依赖 formConfig.chat_background
      // 正则允许 <div 和 class 之间有其他属性和换行（prettier 格式化后属性分散多行）
      const placeholderLine =
        content.match(/<div\b[^>]*class="bg-upload-placeholder"[^>]*>/)?.[0] ?? ''
      expect(placeholderLine).toBeTruthy()
      expect(placeholderLine).toContain('v-if="!formConfig.chat_background"')
      expect(placeholderLine).not.toContain('isDark')
    })
  })

  // ===== 每主题背景参数 UI 与保存链路 =====
  describe('每主题背景参数 UI 与保存链路', () => {
    it('useSettingsCore 保存白名单包含 background_light / background_dark（否则参数永远存不进配置）', () => {
      const content = readFileSync(SETTINGS_CORE_PATH, 'utf-8')
      // ALL_CONFIG_KEYS 是自动保存的 diff 字段白名单，缺了这两个字段，
      // 嵌套对象参数改了也不会触发脏检测与合并保存
      expect(content).toContain("'background_light'")
      expect(content).toContain("'background_dark'")
    })

    it('AppearanceSettings 已移除绑定废弃字段 chat_background_opacity 的死滑块', () => {
      const content = readFileSync(APPEARANCE_SETTINGS_VUE_PATH, 'utf-8')
      // 渲染端 App.vue 只消费 background_light/dark，旧字段滑块无任何视觉效果
      expect(content).not.toContain('v-model:value="formConfig.chat_background_opacity"')
    })

    it('AppearanceSettings 提供亮/暗主题页签与 opacity/blur/mask 三参数入口', () => {
      const content = readFileSync(APPEARANCE_SETTINGS_VUE_PATH, 'utf-8')
      expect(content).toContain('activeBgTheme')
      expect(content).toContain("updateBgParams('opacity'")
      expect(content).toContain("updateBgParams('blur'")
      expect(content).toContain("updateBgParams('mask_alpha'")
    })

    it('参数更新必须走不可变整体替换（引用比较脏检测才能感知嵌套对象变化）', () => {
      const content = readFileSync(APPEARANCE_SETTINGS_VUE_PATH, 'utf-8')
      const block = content.match(/function updateBgParams\([\s\S]*?\n\}/)?.[0] ?? ''
      expect(block).toBeTruthy()
      // 整体替换 formConfig 与展开旧参数对象，保证 background_light/dark 引用变化
      expect(block).toContain('...formConfig.value')
      expect(block).toContain('...activeBgParams.value')
    })
  })
})
