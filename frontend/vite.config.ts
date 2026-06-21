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
            dirs: ['src/components'],
        }),
    ],
    base: './',
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url)),
        },
    },
    build: {
        chunkSizeWarningLimit: 1500,
        cssCodeSplit: true,
        sourcemap: false,
        rollupOptions: {
            output: {
                // Rolldown 代码分割优化：按模块级别拆分，减少单个 chunk 体积
                codeSplitting: 'advanced',
                // 第三方库显式分块：让浏览器并行下载,且按需加载
                manualChunks(id) {
                    if (!id.includes('node_modules')) return undefined
                    if (id.includes('mermaid') || id.includes('cytoscape') || id.includes('d3')) {
                        return 'lib-mermaid'
                    }
                    if (id.includes('katex')) {
                        return 'lib-katex'
                    }
                    if (id.includes('lowlight') || id.includes('highlight.js')) {
                        return 'lib-highlight'
                    }
                    if (id.includes('naive-ui') || id.includes('@css-render') || id.includes('@juggle') || id.includes('date-fns') || id.includes('evtd')) {
                        return 'lib-naive-ui'
                    }
                    if (id.includes('rehype') || id.includes('remark') || id.includes('unist-util') || id.includes('mdast') || id.includes('hast') || id.includes('micromark') || id.includes('bail') || id.includes('is-plain-obj') || id.includes('trough') || id.includes('vfile') || id.includes('zwitch')) {
                        return 'lib-markdown'
                    }
                    if (id.includes('dompurify')) {
                        return 'lib-sanitize'
                    }
                    if (id.includes('@vue/') || id.includes('pinia') || id.includes('vue-router') || id.includes('vue@')) {
                        return 'lib-vue'
                    }
                    if (id.includes('@vicons/') || id.includes('@iconify')) {
                        return 'lib-icons'
                    }
                    return 'lib-vendor'
                },
                chunkLoadingGlobal: 'douyaChunk',
                banner: '/*!\n * 豆芽 - AI 聊天助手\n * Copyright © 2025 zifeiyu. All rights reserved.\n */',
            },
        },
    },
    optimizeDeps: {
        include: [
            'vue',
            'pinia',
            'vue-router',
            'naive-ui',
            'dompurify',
        ],
    },
    worker: {
        format: 'es',
    },
    test: {
        environment: 'happy-dom',
        globals: true,
        include: ['src/**/*.{test,spec}.ts'],
    },
})
