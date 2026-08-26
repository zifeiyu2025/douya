import type { ThemeBackgroundParams } from '../types/chat'

/**
 * 有背景图时三层表面的通透度（数值越小越透，范围 0~1）
 *
 * 与 tokens.css 三层表面令牌体系配套：
 * - veil：结构层（侧边栏/顶栏/输入区容器），最透 —— 让背景图充分参与空间层次
 * - panel：阅读层（消息气泡/输入框），中等实度 —— 保证正文阅读可读性
 * - card：浮起层（代码块/卡片/弹层），最实 —— 保证代码与浮动内容的对比度
 *
 * 无背景图时 buildBackgroundStyle 返回 {}，这些变量不会被注入，
 * tokens.css 中默认值 1 生效，界面自动退化为普通实色主题。
 */
const SURFACE_ALPHAS = {
  // C3 调优：0.6/0.8 的白色叠层在亮色下呈"雾面白"（用户实测反馈），
  // 降到 0.45/0.7/0.82 后玻璃感真实可感：
  // - veil 0.45：侧栏/顶栏/输入区若隐若现，背景图充分参与空间层次
  // - panel 0.7：气泡/输入框明显通透且正文可读（配合每主题默认遮罩
  //   亮 0.15/暗 0.4 兜底文字对比度）
  // - card 0.82：代码块/弹层保持足够实度
  veil: '0.45',
  panel: '0.7',
  card: '0.82'
} as const

/** 透明度类参数钳位到 0~1（opacity / mask_alpha） */
function clampUnit(value: number | undefined, fallback: number): number {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return fallback
  }
  return Math.min(1, Math.max(0, value))
}

/** 模糊半径只要求非负（允许 > 1px），异常值回退 0 */
function clampBlur(value: number | undefined): number {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return 0
  }
  return Math.max(0, value)
}

/** 把 0~1 的 alpha 字符串转成 color-mix 百分比（'0.45' → '45%'） */
function toPercent(alpha: string): string {
  return `${parseFloat(alpha) * 100}%`
}

/**
 * 构建背景相关的 CSS 变量注入样式
 *
 * @param chat_background 背景图原始值（文件路径或 data:URL），为空表示未设置背景
 * @param params 当前主题的背景参数（亮色取 background_light，深色取 background_dark）
 *   - opacity:   背景图不透明度（越低越融入底色）
 *   - blur:      背景图模糊半径 px（配合 ::before 图层四周出血防边缘发虚）
 *   - mask_alpha: 遮罩强度（::after 层，默认 0 = 不额外染色，图片保持原味）
 * @returns CSS 变量样式对象；chat_background 为空时返回 {}
 */
export function buildBackgroundStyle(
  chat_background: string,
  params?: ThemeBackgroundParams | null
): Record<string, string> {
  if (!chat_background) {
    return {}
  }

  // data:URL 可直接使用；本地文件路径走后端 /local-file/ 代理
  const bgUrl = chat_background.startsWith('data:')
    ? chat_background
    : '/local-file/' + encodeURIComponent(chat_background)

  // 关键机制：CSS 自定义属性的 var() 替换发生在「声明处」——
  // --surface-veil 等复合令牌在 :root 求值时 alpha 已固化为 1，
  // 仅覆盖后代上的 --surface-*-alpha 无法让其重新求值（浏览器 readback 实测证实）。
  // 因此必须在挂载点（.app-layout）直接覆盖令牌本身；覆盖值引用
  // var(--bg-primary) / var(--bg-*-base)，在挂载点上求值，随亮暗主题
  // 自动切换基色；无背景图时不注入，自动退化为 :root 实色。
  const veilMix = `color-mix(in srgb, var(--bg-primary) ${toPercent(SURFACE_ALPHAS.veil)}, transparent)`
  const panelMix = `color-mix(in srgb, var(--bg-primary) ${toPercent(SURFACE_ALPHAS.panel)}, transparent)`
  const cardMix = `color-mix(in srgb, var(--bg-primary) ${toPercent(SURFACE_ALPHAS.card)}, transparent)`

  return {
    // 背景图层（::before）：图片本体 + 用户可控的透明度/模糊
    '--chat-background': `url(${bgUrl})`,
    '--chat-background-opacity': String(clampUnit(params?.opacity, 0.85)),
    '--chat-background-blur': `${clampBlur(params?.blur)}px`,
    // 遮罩层（::after）：默认 0 强度，不再强制给图片叠暗角/白雾
    '--chat-background-mask-alpha': String(clampUnit(params?.mask_alpha, 0)),
    // 三层表面通透度 alpha（语义记录，供直接引用处读取）
    '--surface-veil-alpha': SURFACE_ALPHAS.veil,
    '--surface-panel-alpha': SURFACE_ALPHAS.panel,
    '--surface-card-alpha': SURFACE_ALPHAS.card,
    // 三层复合表面令牌覆盖（真正让侧栏/顶栏/气泡/输入区变通透的生效机制）
    '--surface-veil': veilMix,
    '--surface-panel': panelMix,
    '--surface-card': cardMix,
    // 中间层令牌覆盖（消息气泡/输入框/代码块，基色引用 tokens.css 的 base 变量）
    '--bg-user-msg': `color-mix(in srgb, var(--bg-user-msg-base) ${toPercent(SURFACE_ALPHAS.panel)}, transparent)`,
    '--bg-ai-msg': `color-mix(in srgb, var(--bg-ai-msg-base) ${toPercent(SURFACE_ALPHAS.panel)}, transparent)`,
    '--bg-input': `color-mix(in srgb, var(--bg-input-base) ${toPercent(SURFACE_ALPHAS.panel)}, transparent)`,
    '--bg-code': `color-mix(in srgb, var(--bg-code-base) ${toPercent(SURFACE_ALPHAS.card)}, transparent)`
  }
}
