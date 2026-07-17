// ESLint v9 Flat Config
// 文档: https://eslint.org/docs/latest/use/configure/configuration-files
//
// 设计原则:
// - 保守优先: 使用官方 recommended 规则集，避免过度自定义
// - Vue 3 + TypeScript: 启用 typescript-eslint + eslint-plugin-vue
// - 与 Prettier 共存: 通过 @vue/eslint-config-prettier 禁用冲突规则
// - 生成代码豁免: wailsjs/ 由 Wails 自动生成，不参与 lint
import js from '@eslint/js'
import vue from 'eslint-plugin-vue'
import vueParser from 'vue-eslint-parser'
import tseslint from 'typescript-eslint'
import prettierConfig from '@vue/eslint-config-prettier'
import globals from 'globals'

export default [
  // 全局忽略
  {
    ignores: [
      'dist/**',
      'wailsjs/**', // Wails 自动生成的绑定代码
      'node_modules/**',
      'src/components.d.ts', // unplugin-vue-components 自动生成
      'src/vite-env.d.ts' // Vite 自动生成
    ]
  },

  // JS 基础规则
  js.configs.recommended,

  // TypeScript 规则
  ...tseslint.configs.recommended,

  // Vue 规则（Vue 3 flat config）
  ...vue.configs['flat/recommended'],

  // Vue + TypeScript 文件解析配置
  {
    files: ['**/*.vue', '**/*.ts', '**/*.tsx'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: tseslint.parser,
        sourceType: 'module',
        ecmaVersion: 'latest',
        extraFileExtensions: ['.vue']
      },
      globals: {
        ...globals.browser, // 浏览器全局变量（document/window/navigator 等）
        ...globals.es2021, // ES2021 全局变量（Promise/Map/Set 等）
        // Web Speech API 类型（不在标准 browser globals 中）
        SpeechRecognitionResultList: 'readonly',
        SpeechRecognitionErrorEvent: 'readonly',
        SpeechRecognition: 'readonly'
      }
    }
  },

  // 测试文件添加 vitest 全局变量
  {
    files: ['**/*.test.ts', '**/*.spec.ts', '**/__tests__/**'],
    languageOptions: {
      globals: {
        ...globals.vitest
      }
    }
  },

  // 项目自定义规则
  {
    rules: {
      // Vue 规则放宽
      'vue/multi-word-component-names': 'off', // 项目有单文件视图（如 ChatView），允许单词命名
      'vue/no-v-html': 'off', // 项目使用 dompurify 净化后渲染，已处理 XSS

      // TypeScript 规则放宽
      '@typescript-eslint/no-explicit-any': 'off', // 渐进式迁移，允许显式 any（Wails 绑定大量使用 any）
      '@typescript-eslint/no-unused-vars': [
        'warn',
        {
          argsIgnorePattern: '^_', // _ 前缀参数允许未使用
          varsIgnorePattern: '^_', // _ 前缀变量允许未使用
          caughtErrorsIgnorePattern: '^_' // _ 前缀 catch 错误允许未使用
        }
      ],

      // 通用规则
      'no-console': 'off', // 项目使用自定义 logger，console 允许
      'no-debugger': 'warn', // 提醒移除 debugger
      'prefer-const': 'warn' // 建议使用 const
    }
  },

  // 测试文件放宽
  {
    files: ['**/*.test.ts', '**/*.spec.ts', '**/__tests__/**'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      'vue/no-v-html': 'off'
    }
  },

  // Prettier 兼容（禁用与 Prettier 冲突的规则，放在最后）
  prettierConfig
]
