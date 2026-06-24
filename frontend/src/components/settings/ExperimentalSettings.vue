<template>
  <!-- Agent 与 MCP -->
  <n-form-item>
    <template #label>Agent 模式 <HelpTip content="一键启用 CORS 代理和所有内置工具（文件读写、shell 命令等）。实验性功能，不建议在不可信环境启用" /></template>
    <n-switch v-model:value="formConfig.agent" :disabled="formConfig.ui_mcp_proxy" @update:value="handleAgentChange" />
  </n-form-item>
  <n-form-item>
    <template #label>MCP CORS 代理 <HelpTip content="仅为 Web UI 的 MCP 功能启用 CORS 代理。Agent 模式已包含此项" /></template>
    <n-switch v-model:value="formConfig.ui_mcp_proxy" :disabled="formConfig.agent" @update:value="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>后端采样 <HelpTip content="实验性功能，将采样逻辑移到 GPU 执行以提升性能。不兼容 Grammar 和 Reasoning Budget" /></template>
    <n-switch v-model:value="formConfig.backend_sampling" @update:value="handleBackendSamplingChange" />
  </n-form-item>
  <n-form-item>
    <template #label>Jinja2 模板 <HelpTip content="使用 Jinja2 引擎处理 chat template，支持更复杂的模板语法（如循环、条件）。部分模型需要开启才能正确渲染模板" /></template>
    <n-switch v-model:value="formConfig.jinja" @update:value="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>Prompt 缓存 <HelpTip content="显式控制 prompt 缓存行为。开启后复用已计算的 KV 缓存加速重复请求，关闭则每次重新计算" /></template>
    <n-switch v-model:value="formConfig.cache_prompt" @update:value="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>指标端点 <HelpTip content="启用 /metrics 端点暴露 Prometheus 格式的服务器性能指标（请求数、token 吞吐量、缓存命中率等），便于外部监控" /></template>
    <n-switch v-model:value="formConfig.metrics" @update:value="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>详细日志 <HelpTip content="启用详细日志输出，包含每次请求的完整参数和响应信息。仅调试时开启，日常使用会增大日志量" /></template>
    <n-switch v-model:value="formConfig.verbose" @update:value="autoSave" />
  </n-form-item>
</template>

<script setup lang="ts">
import { inject, defineComponent, h } from 'vue'
import { NFormItem, NSwitch, NTooltip } from 'naive-ui'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'

const HelpTip = defineComponent({
  props: { content: String },
  setup(props) {
    return () => h(NTooltip, { trigger: 'hover' }, {
      trigger: () => h('span', { class: 'help-tip-icon' }, '?'),
      default: () => props.content
    })
  }
})

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)!

const {
  formConfig, autoSave,
  handleAgentChange, handleBackendSamplingChange,
} = ctx
</script>

<style scoped>
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
  background: var(--bg-tertiary, rgba(0,0,0,0.06));
  margin-left: 4px;
  cursor: help;
  vertical-align: middle;
  line-height: 1;
}
</style>
