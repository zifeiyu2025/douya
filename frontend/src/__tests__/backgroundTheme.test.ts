import { describe, it, expect, beforeEach, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { buildBackgroundStyle } from '../utils/backgroundStyle'

/**
 * Task 21: 背景图双主题支持测试
 *
 * 生活类比：背景图就像手机壁纸——以前只有"深色模式"才能用壁纸（亮色被禁用），
 * Task 4 把这个限制拆掉了，现在两种模式都能用同一张壁纸。
 * 这个测试文件就是来"验收"这件事的：
 *   1. 验证 buildBackgroundStyle 这个"包装工人"在两种主题下都肯干活（21.1）
 *   2. 验证 App.vue 的 .has-background 开关不再看主题脸色（21.2）
 *   3. 验证 BasicSettings 设置页不再弹出"仅深色模式"的禁用提示（21.3）
 *
 * 说明：buildBackgroundStyle 是从 App.vue mainAreaStyle 抽取的纯函数，
 * 它不接收 isDark 参数——这本身就是"双主题都支持"的体现。
 * 因此"亮色模式"与"深色模式"的用例本质是验证：函数对同一输入返回相同结果，
 * 不再存在 Task 4 之前那种"亮色就返回 {}"的硬限制。
 */

// ESM 下通过 import.meta.url 计算 __dirname，用于读取源码做代码审查
const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

// 读取源码文件路径（用于 SubTask 21.2 / 21.3 的代码审查）
const APP_VUE_PATH = resolve(__dirname, '../App.vue')
const BASIC_SETTINGS_VUE_PATH = resolve(__dirname, '../components/settings/BasicSettings.vue')

describe('Task 21: 背景图双主题支持', () => {
  beforeEach(() => {
    // 当前测试文件不依赖 pinia / localStorage / matchMedia，
    // 但保留 beforeEach 用于未来扩展时的状态清理隔离
    vi.clearAllMocks()
  })

  // ===== SubTask 21.1: 验证 mainAreaStyle 亮色模式注入变量 =====
  describe('SubTask 21.1: buildBackgroundStyle 注入 CSS 变量', () => {
    it('亮色模式 + chat_background 存在 → 返回 --chat-background 与 --chat-background-opacity', () => {
      // buildBackgroundStyle 不接收 isDark，亮色下行为与深色一致
      const style = buildBackgroundStyle('/path/to/bg.png', 0.8)
      expect(style).toHaveProperty('--chat-background')
      expect(style).toHaveProperty('--chat-background-opacity')
      expect(style['--chat-background-opacity']).toBe('0.8')
    })

    it('深色模式 + chat_background 存在 → 同样返回变量（验证不再有 isDark 限制）', () => {
      // 同一输入在"深色模式"下结果与亮色完全一致
      // 这正是 Task 4 移除 isDark 硬限制后的预期行为
      const lightStyle = buildBackgroundStyle('/path/to/bg.png', 0.8)
      const darkStyle = buildBackgroundStyle('/path/to/bg.png', 0.8)
      expect(darkStyle).toEqual(lightStyle)
      expect(darkStyle).toHaveProperty('--chat-background')
      expect(darkStyle).toHaveProperty('--chat-background-opacity')
    })

    it('chat_background 为空字符串 → 返回空对象', () => {
      expect(buildBackgroundStyle('', 0.9)).toEqual({})
    })

    it('chat_background 为空字符串且未传 opacity → 返回空对象', () => {
      expect(buildBackgroundStyle('')).toEqual({})
    })

    it('data: URL → 直接使用，不经过 /local-file/ 编码', () => {
      const dataUrl = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=='
      const style = buildBackgroundStyle(dataUrl, 0.9)
      // 应直接包裹成 url(data:...)，不出现 /local-file/ 前缀
      expect(style['--chat-background']).toBe(`url(${dataUrl})`)
      expect(style['--chat-background']).not.toContain('/local-file/')
    })

    it('文件路径 → 转为 /local-file/<encodeURIComponent(path)> URL', () => {
      const filePath = 'C:\\Users\\douya\\bg.png'
      const style = buildBackgroundStyle(filePath, 0.9)
      const expected = 'url(/local-file/' + encodeURIComponent(filePath) + ')'
      expect(style['--chat-background']).toBe(expected)
    })

    it('文件路径含特殊字符 → encodeURIComponent 正确编码', () => {
      const filePath = 'D:/我的 图片/bg test.png'
      const style = buildBackgroundStyle(filePath, 0.9)
      // 验证编码后的路径出现在结果中
      const encoded = encodeURIComponent(filePath)
      expect(style['--chat-background']).toBe(`url(/local-file/${encoded})`)
    })

    it('opacity 默认值为 0.9', () => {
      const style = buildBackgroundStyle('/path/to/bg.png')
      expect(style['--chat-background-opacity']).toBe('0.9')
    })

    it('opacity 显式传 0.5 → 使用 0.5', () => {
      const style = buildBackgroundStyle('/path/to/bg.png', 0.5)
      expect(style['--chat-background-opacity']).toBe('0.5')
    })

    it('opacity 显式传 0 → 保留 0（不被默认值覆盖）', () => {
      // 0 是合法的 falsy 值，使用 ?? 而非 || 才能保留 0
      const style = buildBackgroundStyle('/path/to/bg.png', 0)
      expect(style['--chat-background-opacity']).toBe('0')
    })

    it('返回值类型为 Record<string, string>', () => {
      const style = buildBackgroundStyle('/path/to/bg.png', 0.9)
      expect(typeof style).toBe('object')
      // 所有 value 都应为 string 类型
      for (const key of Object.keys(style)) {
        expect(typeof style[key]).toBe('string')
      }
    })
  })

  // ===== SubTask 21.2: 验证亮色模式下 .has-background 类应用 =====
  describe('SubTask 21.2: .has-background 类不再依赖 isDark', () => {
    it('buildBackgroundStyle 在亮色模式下返回非空对象（背景图变量已注入）', () => {
      // 模拟亮色模式：函数无 isDark 参数，直接验证 chat_background 存在时返回非空
      const style = buildBackgroundStyle('/path/to/bg.png', 0.9)
      expect(Object.keys(style).length).toBeGreaterThan(0)
      expect(style['--chat-background']).toBeTruthy()
    })

    it('App.vue 的 .has-background class 条件不再依赖 isDark（代码审查）', () => {
      // 读取 App.vue 源码，验证 has-background 绑定条件不含 isDark
      const content = readFileSync(APP_VUE_PATH, 'utf-8')
      // 定位 has-background 的 class 绑定片段
      expect(content).toContain("'has-background'")
      // Task 4 之前为 isDark && !!chat_background，现在应仅为 !!chat_background
      // 断言不再出现 isDark 与 has-background 在同一绑定条件中
      const hasBackgroundLine = content.match(/'has-background':\s*[^,}]*/)?.[0] ?? ''
      expect(hasBackgroundLine).toBeTruthy()
      expect(hasBackgroundLine).not.toContain('isDark')
      // 应直接基于 chat_background 判断
      expect(hasBackgroundLine).toContain('chat_background')
    })

    it('App.vue mainAreaStyle 调用 buildBackgroundStyle，不再内联 isDark 判断（代码审查）', () => {
      const content = readFileSync(APP_VUE_PATH, 'utf-8')
      // mainAreaStyle 应调用抽取出的纯函数
      expect(content).toContain('buildBackgroundStyle')
      // mainAreaStyle 内不应再出现 isDark.value 的硬限制
      const mainAreaStyleBlock =
        content.match(/const mainAreaStyle = computed\(\(\) => \{[\s\S]*?\}\)/)?.[0] ?? ''
      expect(mainAreaStyleBlock).toBeTruthy()
      expect(mainAreaStyleBlock).not.toContain('isDark.value')
    })
  })

  // ===== SubTask 21.3: 验证 BasicSettings 亮色模式显示上传区 =====
  describe('SubTask 21.3: BasicSettings 亮色模式显示上传区', () => {
    it('BasicSettings.vue 不包含"背景图仅支持深色模式"禁用提示', () => {
      const content = readFileSync(BASIC_SETTINGS_VUE_PATH, 'utf-8')
      expect(content).not.toContain('背景图仅支持深色模式')
      expect(content).not.toContain('仅支持深色模式')
    })

    it('BasicSettings.vue 背景图上传区不再被 v-if="!isDark" 禁用', () => {
      const content = readFileSync(BASIC_SETTINGS_VUE_PATH, 'utf-8')
      // 不应存在用 !isDark 禁用背景图上传区的条件
      expect(content).not.toContain('v-if="!isDark"')
    })

    it('BasicSettings.vue 聊天背景 form-item 始终显示（无 isDark 条件）', () => {
      const content = readFileSync(BASIC_SETTINGS_VUE_PATH, 'utf-8')
      // 定位"聊天背景"form-item 块
      const bgBlock =
        content.match(/<n-form-item label="聊天背景">[\s\S]*?<\/n-form-item>/)?.[0] ?? ''
      expect(bgBlock).toBeTruthy()
      // 背景图区块不应被 isDark 条件包裹
      expect(bgBlock).not.toContain('isDark')
    })

    it('BasicSettings.vue 上传占位区在无背景图时显示（v-if 仅依赖 chat_background）', () => {
      const content = readFileSync(BASIC_SETTINGS_VUE_PATH, 'utf-8')
      // upload-placeholder 的 v-if 应仅依赖 formConfig.chat_background
      // 正则允许 <div 与 class 之间有其他属性和换行（prettier 格式化后属性分散多行）
      const placeholderLine =
        content.match(/<div\b[^>]*class="upload-placeholder"[^>]*>/)?.[0] ?? ''
      expect(placeholderLine).toBeTruthy()
      expect(placeholderLine).toContain('v-if="!formConfig.chat_background"')
      expect(placeholderLine).not.toContain('isDark')
    })
  })
})
