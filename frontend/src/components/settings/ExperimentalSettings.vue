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
    <n-switch :value="formConfig.jinja ?? false" @update:value="(v: boolean) => { formConfig.jinja = v; autoSave() }" />
  </n-form-item>
  <n-form-item>
    <template #label>Prompt 缓存 <HelpTip content="显式控制 prompt 缓存行为。开启后复用已计算的 KV 缓存加速重复请求，关闭则每次重新计算" /></template>
    <n-switch :value="formConfig.cache_prompt ?? false" @update:value="(v: boolean) => { formConfig.cache_prompt = v; autoSave() }" />
  </n-form-item>
  <n-form-item>
    <template #label>指标端点 <HelpTip content="启用 /metrics 端点暴露 Prometheus 格式的服务器性能指标（请求数、token 吞吐量、缓存命中率等），便于外部监控" /></template>
    <n-switch v-model:value="formConfig.metrics" @update:value="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>详细日志 <HelpTip content="启用详细日志输出，包含每次请求的完整参数和响应信息。仅调试时开启，日常使用会增大日志量" /></template>
    <n-switch v-model:value="formConfig.verbose" @update:value="autoSave" />
  </n-form-item>

  <!-- 任务 38：虚拟滚动（实验性，纯前端开关，不进入后端 Config）
       默认关闭，开启后长会话仅渲染可见区域消息，降低内存与滚动开销。
       若出现滚动跳动或 Markdown 重渲染异常，关闭即可立即恢复原 v-for 渲染。 -->
  <n-form-item>
    <template #label>虚拟滚动 <HelpTip content="实验性功能。开启后长会话（100+ 条消息）仅渲染可见区域内的消息，降低内存占用与滚动开销；可能存在滚动跳动或消息重新进入视口时短暂重渲染。关闭即可恢复默认渲染" /></template>
    <n-switch v-model:value="enableVirtualScroll" />
  </n-form-item>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { NFormItem, NSwitch } from 'naive-ui'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
// F-1.1/F-1.2：抽取为公共组件，消除三处重复定义
import HelpTip from '../ui/HelpTip.vue'
// 任务 38：虚拟滚动 feature flag（纯前端，localStorage 持久化）
import { useVirtualScroll } from '../../composables/useVirtualScroll'

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)!

const {
  formConfig, autoSave,
  handleAgentChange, handleBackendSamplingChange,
} = ctx

// 虚拟滚动开关：与 MessageList 共享同一单例响应式状态
const { enableVirtualScroll } = useVirtualScroll()
</script>

<style scoped>
/* F-1.2：.help-tip-icon 样式已抽取到 ui/HelpTip.vue，此处不再重复定义 */
</style>
