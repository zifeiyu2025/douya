/**
 * 构建聊天背景图的 CSS 变量样式对象
 *
 * 生活类比：这个函数就像一个"背景图包装工人"——
 * 你给它一张图片（路径或 data:URL）和透明度，它就帮你打包成
 * CSS 能直接认识的盒子（带 --chat-background 和 --chat-background-opacity 标签）。
 * 如果你不给图片，它就还你一个空盒子。
 *
 * 抽取自 App.vue 的 mainAreaStyle 计算属性（Task 21），
 * 目的是让背景图双主题逻辑可被单元测试覆盖。
 *
 * 注：此函数不再依赖 isDark，亮色与深色模式均会注入变量，
 * 由调用方（App.vue）根据 chat_background 是否存在决定是否应用 .has-background 类。
 *
 * @param chat_background 背景图原始值，可以是文件路径或 data:URL
 * @param opacity 背景透明度（0-1），默认 0.9
 * @returns CSS 变量样式对象；chat_background 为空时返回 {}
 */
export function buildBackgroundStyle(
  chat_background: string,
  opacity?: number,
): Record<string, string> {
  // 双主题都支持背景图：仅判断 chat_background 是否存在
  if (!chat_background) {
    return {}
  }
  const actualOpacity = opacity ?? 0.9
  let bgUrl: string
  if (chat_background.startsWith('data:')) {
    // data:URL（如 base64）直接使用，避免二次编码
    bgUrl = chat_background
  } else {
    // 文件路径通过 /local-file/ 前端路由代理读取本地文件
    bgUrl = '/local-file/' + encodeURIComponent(chat_background)
  }
  return {
    '--chat-background': `url(${bgUrl})`,
    '--chat-background-opacity': String(actualOpacity),
  }
}
