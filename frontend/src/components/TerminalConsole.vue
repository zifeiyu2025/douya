<template>
  <div class="terminal-wrapper">
    <div ref="terminalContainer" class="terminal-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import '@xterm/xterm/css/xterm.css'
import { wails } from '../services/wails'
import { useThemeStore } from '../stores/theme'

const terminalContainer = ref<HTMLElement>()
let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let paused = false
let resizeTimer: ReturnType<typeof setTimeout> | null = null
let resizeObserver: ResizeObserver | null = null
// F-1.10：保存 subscribeTerminalData 返回的 unsubscribe 函数，替代原 offTerminalData 调用
let unsubscribeTerminalData: (() => void) | null = null

const themeStore = useThemeStore()

// 从 CSS 变量读取终端配色，确保与全局主题对齐（背景跟随 --bg-secondary，文字跟随 --text-primary）
function readCssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function buildTerminalTheme() {
  return {
    background: readCssVar('--bg-secondary'),
    foreground: readCssVar('--text-primary')
  }
}

// 主题切换时同步终端配色，避免亮/暗模式下背景与文字对比度失效
watch(
  () => themeStore.isDark,
  () => {
    if (terminal) {
      terminal.options.theme = buildTerminalTheme()
    }
  }
)

// base64 字符串解码为 Uint8Array（Wails 传递 []byte 时自动编码为 base64）
function base64ToUint8Array(base64: string): Uint8Array {
  const binaryString = atob(base64)
  const bytes = new Uint8Array(binaryString.length)
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i)
  }
  return bytes
}

// 初始化 xterm.js 终端
async function initTerminal() {
  if (!terminalContainer.value) return

  terminal = new Terminal({
    fontSize: 12,
    fontFamily: 'Consolas, Monaco, "Courier New", monospace',
    scrollback: 5000, // 保留 5000 行历史
    convertEol: false, // 保留原始 \r\n（llama-server 的进度条用 \r 刷新）
    cursorBlink: false,
    disableStdin: true, // 禁止用户输入（llama-server 不需要 stdin）
    allowProposedApi: true,
    // 终端配色跟随全局主题变量（背景 --bg-secondary，文字 --text-primary）
    theme: buildTerminalTheme()
  })

  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)

  // 尝试加载 WebGL 渲染器（GPU 加速），不支持时回退到默认 DOM 渲染
  try {
    const webglAddon = new WebglAddon()
    terminal.loadAddon(webglAddon)
  } catch {
    console.warn('[terminal] WebGL addon not available, using DOM renderer')
  }

  terminal.open(terminalContainer.value)
  fitAddon.fit()

  // 拉取历史日志并写入终端
  try {
    const history = await wails.getTerminalHistory()
    if (history && terminal) {
      terminal.write(history)
    }
  } catch (err) {
    console.warn('[terminal] Failed to load history:', err)
  }

  // 监听后端终端数据事件
  unsubscribeTerminalData = wails.subscribeTerminalData((data: string) => {
    if (paused || !terminal) return
    const bytes = base64ToUint8Array(data)
    terminal.write(bytes)
  })

  // 监听容器尺寸变化，自动调整终端尺寸
  resizeObserver = new ResizeObserver(() => {
    if (resizeTimer) clearTimeout(resizeTimer)
    resizeTimer = setTimeout(() => {
      if (fitAddon && terminal) {
        try {
          fitAddon.fit()
          // 通知后端调整 ConPTY 尺寸
          const cols = terminal.cols
          const rows = terminal.rows
          wails.resizeTerminal(cols, rows).catch(() => {})
        } catch {
          // 忽略 fit 失败（容器可能未可见）
        }
      }
    }, 100)
  })
  resizeObserver.observe(terminalContainer.value)
}

// 暴露给父组件的方法
defineExpose({
  clear: () => {
    terminal?.clear()
  },
  copy: async (): Promise<string> => {
    if (!terminal) return ''
    // 优先获取选中文本，否则获取整个缓冲区
    const selection = terminal.getSelection()
    if (selection) return selection
    // 获取整个缓冲区内容
    const buffer = terminal.buffer.active
    let content = ''
    for (let i = 0; i < buffer.length; i++) {
      const line = buffer.getLine(i)
      if (line) {
        content += line.translateToString(true) + '\n'
      }
    }
    return content.trim()
  },
  setPaused: (p: boolean) => {
    paused = p
  },
  isPaused: () => paused,
  fit: () => {
    if (fitAddon) {
      try {
        fitAddon.fit()
      } catch {
        // 忽略
      }
    }
  }
})

onMounted(async () => {
  await nextTick()
  await initTerminal()
})

onUnmounted(() => {
  // F-1.10：调用 unsubscribe 函数取消订阅，替代原 wails.offTerminalData()
  if (unsubscribeTerminalData) {
    unsubscribeTerminalData()
    unsubscribeTerminalData = null
  }
  if (resizeTimer) clearTimeout(resizeTimer)
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (terminal) {
    terminal.dispose()
    terminal = null
  }
})
</script>

<style scoped>
.terminal-wrapper {
  height: 100%;
  width: 100%;
  overflow: hidden;
  background: var(--bg-secondary);
}
.terminal-container {
  height: 100%;
  width: 100%;
  padding: 4px;
}
.terminal-container :deep(.xterm) {
  height: 100%;
}
.terminal-container :deep(.xterm-viewport) {
  background-color: var(--bg-secondary) !important;
}
.terminal-container :deep(.xterm-screen) {
  background-color: var(--bg-secondary) !important;
}
</style>
