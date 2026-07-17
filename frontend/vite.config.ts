import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [
    vue(),
    Components({
      resolvers: [NaiveUiResolver()],
      dts: 'src/components.d.ts',
      dirs: ['src/components']
    })
  ],
  base: './',
  // L-6：显式关闭 Vue 生产环境 DevTools，防止组件树和 Pinia 状态泄漏
  // Vue 3 默认在生产构建中关闭，但显式设置避免未来插件默认行为变更导致意外启用
  define: {
    __VUE_PROD_DEVTOOLS__: false
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  build: {
    // Wails 本地 WebView 加载，无网络传输；heic-to 库本身 ~3MB 且已懒加载，无法再拆
    chunkSizeWarningLimit: 3000,
    cssCodeSplit: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        // Vite 8 (Rolldown) 第三方库显式分块：让浏览器并行下载,且按需加载
        // Vite 8 推荐 codeSplitting.groups（manualChunks/advancedChunks 均已弃用）
        // [\\/] 兼容 Windows 反斜杠路径分隔符；groups 按顺序匹配第一个命中的
        codeSplitting: {
          groups: [
            // Markdown 解析（marked）
            { name: 'lib-markdown', test: /node_modules[\\/]marked/ },
            // 代码高亮（highlight.js）
            { name: 'lib-highlight', test: /node_modules[\\/]highlight\.js/ },
            // Naive UI 组件库及其依赖
            {
              name: 'lib-naive-ui',
              test: /node_modules[\\/](naive-ui|@css-render|@juggle|date-fns|evtd)/
            },
            // HTML 消毒库
            { name: 'lib-sanitize', test: /node_modules[\\/]dompurify/ },
            // HEIC 图片解码库（~2.9MB，仅上传 HEIC 时懒加载）
            { name: 'lib-heic', test: /node_modules[\\/]heic-to/ },
            // Vue 生态核心（@vue/*、pinia、vue-router、vue 本体）
            {
              name: 'lib-vue',
              test: /node_modules[\\/](@vue[\\/]|pinia|vue-router|vue@|vue[\\/])/
            },
            // 图标库
            { name: 'lib-icons', test: /node_modules[\\/](@vicons[\\/]|@iconify)/ },
            // 终端模拟器（xterm + addons，仅在终端控制台使用）
            { name: 'lib-xterm', test: /node_modules[\\/]@xterm/ },
            // 兜底：其余 node_modules 统一归入 vendor
            { name: 'lib-vendor', test: /node_modules[\\/]/ }
          ]
        },
        banner: '/*!\n * 豆芽 - AI 聊天助手\n * Copyright © 2025 zifeiyu. All rights reserved.\n */'
      }
    }
  },
  optimizeDeps: {
    include: ['vue', 'pinia', 'vue-router', 'naive-ui', 'dompurify', 'marked', 'highlight.js']
  },
  test: {
    environment: 'happy-dom',
    globals: true,
    include: ['src/**/*.{test,spec}.ts']
  }
})
