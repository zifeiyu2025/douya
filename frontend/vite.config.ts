import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
import {fileURLToPath, URL} from 'node:url'

export default defineConfig({
  plugins: [vue()],
  base: './',
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  build: {
    rollupOptions: {
      output: {
        banner: '/*!\n * 豆芽 - AI 聊天助手\n * Copyright © 2025 zifeiyu. All rights reserved.\n */'
      }
    }
  }
})
