<template>
  <div class="settings-group">
    <n-form ref="formRef" :model="formConfig" label-placement="top" class="settings-form">
      <div class="group-header">
        <span class="group-icon">🔬</span>
        <div class="group-text">
          <h3>高级</h3>
          <p>MCP、服务器配置、RAG、LoRA、实验功能</p>
        </div>
      </div>

      <n-divider style="margin: 16px 0" />

      <!-- MCP 工具插件 -->
      <div class="section-header">
        <span class="section-icon">🔧</span>
        <span class="section-title">MCP 服务器</span>
      </div>
      <MCPSettings />

      <!-- 服务器配置 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">⚙️</span>
        <span class="section-title">服务器配置</span>
      </div>

      <n-form-item>
        <template #label>
          Agent 模式
          <HelpTip
            content="一键启用 CORS 代理和所有内置工具（文件读写、shell 命令等）。实验性功能，不建议在不可信环境启用"
          />
        </template>
        <n-switch v-model:value="formConfig.agent" @update:value="handleAgentChange" />
      </n-form-item>

      <n-form-item>
        <template #label>
          MCP CORS 代理
          <HelpTip content="仅为 Web UI 的 MCP 功能启用 CORS 代理。Agent 模式已包含此项" />
        </template>
        <n-switch v-model:value="formConfig.ui_mcp_proxy" @update:value="autoSave" />
      </n-form-item>

      <n-form-item>
        <template #label>
          后端采样
          <HelpTip
            content="实验性功能，将采样逻辑移到 GPU 执行以提升性能。不兼容 Grammar 和 Reasoning Budget"
          />
        </template>
        <n-switch
          v-model:value="formConfig.backend_sampling"
          @update:value="handleBackendSamplingChange"
        />
      </n-form-item>

      <!-- 细粒度 CORS 配置 -->
      <n-form-item>
        <template #label>
          CORS 允许来源
          <HelpTip
            content="允许的浏览器跨域来源，逗号分隔（如 http://localhost:5173,*）。留空使用 llama.cpp 默认（仅 localhost）。供浏览器访问 Lua/Web 工具时自定义来源"
          />
        </template>
        <n-input
          v-model:value="formConfig.cors_origins"
          placeholder="http://localhost:5173,*"
          clearable
          @update:value="autoSave"
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          CORS 允许方法
          <HelpTip
            content="允许的 HTTP 方法，逗号分隔（如 GET,POST,PUT,DELETE）。留空使用 llama.cpp 默认"
          />
        </template>
        <n-input
          v-model:value="formConfig.cors_methods"
          placeholder="GET,POST"
          clearable
          @update:value="autoSave"
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          CORS 允许请求头
          <HelpTip
            content="允许的请求头，逗号分隔（如 Content-Type,X-Tool-Cwd）。留空使用 llama.cpp 默认"
          />
        </template>
        <n-input
          v-model:value="formConfig.cors_headers"
          placeholder="Content-Type"
          clearable
          @update:value="autoSave"
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          CORS 允许凭证
          <HelpTip
            content="是否允许跨域携带凭证（Cookie/认证头）。若与允许来源 * 同时启用，llama.cpp 会回显请求 Origin 并始终允许凭证。请仅在可信来源下开启"
          />
        </template>
        <n-switch v-model:value="formConfig.cors_credentials" @update:value="autoSave" />
      </n-form-item>

      <!-- 实验功能 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">🧪</span>
        <span class="section-title">实验功能</span>
      </div>

      <n-form-item>
        <template #label>
          Jinja2 模板
          <HelpTip
            content="使用 Jinja2 引擎处理 chat template，支持更复杂的模板语法（如循环、条件）。部分模型需要开启才能正确渲染模板"
          />
        </template>
        <n-switch
          :value="formConfig.jinja ?? true"
          @update:value="
            (v: boolean) => {
              formConfig.jinja = v
              autoSave()
            }
          "
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          自定义聊天模板
          <HelpTip
            content="指定 .jinja 模板文件路径，覆盖模型自带的聊天模板（用于自定义对话格式）。留空使用模型自带模板，文件不存在时自动忽略"
          />
        </template>
        <n-input
          v-model:value="formConfig.chat_template_file"
          placeholder="模板文件路径（如 models/my-template.jinja）"
          @blur="autoSave"
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          Prompt 缓存
          <HelpTip
            content="显式控制 prompt 缓存行为。开启后复用已计算的 KV 缓存加速重复请求，关闭则每次重新计算"
          />
        </template>
        <n-switch
          :value="formConfig.cache_prompt ?? false"
          @update:value="
            (v: boolean) => {
              formConfig.cache_prompt = v
              autoSave()
            }
          "
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          指标端点
          <HelpTip
            content="启用 /metrics 端点暴露 Prometheus 格式的服务器性能指标（请求数、token 吞吐量、缓存命中率等），便于外部监控"
          />
        </template>
        <n-switch v-model:value="formConfig.metrics" @update:value="autoSave" />
      </n-form-item>

      <n-form-item>
        <template #label>
          详细日志
          <HelpTip
            content="启用详细日志输出，包含每次请求的完整参数和响应信息。仅调试时开启，日常使用会增大日志量"
          />
        </template>
        <n-switch v-model:value="formConfig.verbose" @update:value="autoSave" />
      </n-form-item>

      <n-form-item>
        <template #label>
          内存锁定 (mlock)
          <HelpTip
            content="将模型权重锁定在物理内存，防止操作系统换页到磁盘，提升推理稳定性。内存充足时可开启"
          />
        </template>
        <n-switch
          :value="formConfig.mlock ?? false"
          @update:value="
            (v: boolean) => {
              formConfig.mlock = v
              autoSave()
            }
          "
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          统一 KV 缓存
          <HelpTip
            content="开启后 K/V 缓存共享同一内存池，减少内存碎片。llama.cpp 新特性，一般建议保持开启"
          />
        </template>
        <n-switch v-model:value="formConfig.kv_unified" @update:value="autoSave" />
      </n-form-item>

      <n-form-item>
        <template #label>
          直接 I/O
          <HelpTip
            content="绕过操作系统页面缓存直接读写磁盘，加速大模型加载。HDD 上效果明显，SSD 提升有限"
          />
        </template>
        <n-switch v-model:value="formConfig.direct_io" @update:value="autoSave" />
      </n-form-item>

      <n-form-item>
        <template #label>
          算子卸载
          <HelpTip
            content="将部分计算算子（op）从 GPU 卸载到 CPU 执行，节省显存。自动=由 llama.cpp 根据显存自动决定"
          />
        </template>
        <n-select
          :value="formConfig.op_offload === null ? 'auto' : formConfig.op_offload ? 'on' : 'off'"
          :options="opOffloadOptions"
          @update:value="
            (v: string) => {
              formConfig.op_offload = v === 'auto' ? null : v === 'on'
              autoSave()
            }
          "
        />
      </n-form-item>

      <!-- RAG 重排序配置 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">📊</span>
        <span class="section-title">RAG 重排序</span>
      </div>

      <n-form-item>
        <template #label>
          Reranker 模型
          <HelpTip
            content="RAG 检索结果重排序使用的模型路径（.gguf 文件）。留空则不启用重排序，使用向量检索的原始排序"
          />
        </template>
        <n-input
          v-model:value="formConfig.reranker_model_path"
          placeholder="Reranker 模型文件路径（如 models/bge-reranker-v2-m3.gguf）"
          @blur="autoSave"
        />
      </n-form-item>

      <n-form-item>
        <template #label>
          重排序 Top N
          <HelpTip content="重排序后保留的文档数量。值越大召回越全但耗时越长，建议 3-5" />
        </template>
        <n-input-number
          v-model:value="formConfig.rerank_top_n"
          :min="1"
          :max="20"
          :step="1"
          placeholder="5"
          style="width: 100%"
        />
      </n-form-item>

      <!-- KV 缓存持久化 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">💾</span>
        <span class="section-title">KV 缓存持久化</span>
      </div>

      <n-form-item>
        <template #label>
          启用 KV 缓存持久化
          <HelpTip
            content="开启后将对话的 KV 缓存保存到磁盘，下次加载相同上下文时可跳过预填充，加快首 token 响应速度"
          />
        </template>
        <n-switch v-model:value="formConfig.slot_save_enabled" />
      </n-form-item>

      <n-form-item v-if="formConfig.slot_save_enabled">
        <template #label>
          缓存保存路径
          <HelpTip content="KV 缓存保存到磁盘的目录路径。留空则使用默认路径（appDir/slots/）" />
        </template>
        <n-input
          v-model:value="formConfig.slot_save_path"
          placeholder="留空则使用默认路径（appDir/slots/）"
          @blur="autoSave"
        />
      </n-form-item>

      <!-- LoRA 适配器管理 -->
      <n-divider style="margin: 24px 0 16px" />
      <div class="section-header">
        <span class="section-icon">📚</span>
        <span class="section-title">LoRA 适配器</span>
      </div>

      <LoraManager v-model:lora-paths="formConfig.lora_paths" @update:lora-paths="autoSave" />
    </n-form>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { NFormItem, NSwitch, NInput, NInputNumber, NSelect } from 'naive-ui'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import MCPSettings from './MCPSettings.vue'
import LoraManager from '../LoraManager.vue'
import HelpTip from '../ui/HelpTip.vue'

defineOptions({ name: 'AdvancedExperimentalSettings' })

const ctx = inject<SettingsContext>(SETTINGS_CONTEXT_KEY)
if (!ctx) {
  throw new Error(
    'AdvancedExperimentalSettings 必须在 SettingsView 内使用（缺少 settingsContext provide）'
  )
}
const { formConfig, autoSave } = ctx

/** Agent 模式切换处理 */
function handleAgentChange() {
  if (formConfig.value.agent) {
    formConfig.value.ui_mcp_proxy = false
  }
  autoSave()
}

/** 后端采样切换处理 */
function handleBackendSamplingChange() {
  if (formConfig.value.backend_sampling) {
    formConfig.value.reasoning_budget = -1
  }
  autoSave()
}

/** 算子卸载选项（null=自动，对应 llama.cpp 默认） */
const opOffloadOptions = [
  { label: '自动（llama.cpp 决定）', value: 'auto' },
  { label: '开启', value: 'on' },
  { label: '关闭', value: 'off' }
]
</script>

<style scoped>
.settings-group {
  margin-bottom: 32px;
}

.group-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 0;
}

.group-icon {
  font-size: 28px;
}

.group-text h3 {
  font-size: 18px;
  font-weight: 600;
  margin: 0;
  color: var(--n-text-color);
}

.group-text p {
  font-size: 13px;
  color: var(--n-text-color-3);
  margin: 2px 0 0;
}

.settings-form {
  max-width: 720px;
  margin: 0 auto;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.section-icon {
  font-size: 18px;
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--n-text-color);
}
</style>
