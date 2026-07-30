<!--
  HelpTip: 设置项的帮助提示气泡
  统一管理所有 settings 子组件中的"?"图标 + tooltip 模式，消除三处重复定义

  用法：
    <HelpTip content="帮助文本" />
    <template #label>字段名 <HelpTip content="该字段的说明" /></template>
-->
<template>
  <NTooltip trigger="hover" placement="top">
    <template #trigger>
      <span class="help-tip-icon" :aria-label="content || '帮助'">?</span>
    </template>
    {{ content }}
  </NTooltip>
</template>

<script setup lang="ts">
import { NTooltip } from 'naive-ui'

defineProps<{
  content?: string
}>()
</script>

<style scoped>
/* 安全实践（基于 F-1.1/F-1.2）：统一 help-tip-icon 样式，消除三处重复定义 */
.help-tip-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  font-size: 10px;
  font-weight: 600;
  color: var(--text-muted);
  background: var(--bg-tertiary);
  margin-left: 4px;
  cursor: help;
  vertical-align: middle;
  line-height: 1;
}
</style>
