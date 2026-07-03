<template>
  <!-- 搜索 API KEY -->
  <n-form-item label="Ollama API Key">
    <div class="api-key-row">
      <n-input
        v-model:value="newOllamaApiKey"
        type="password"
        show-password-on="click"
        :placeholder="searchKeys.ollama_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'"
        :loading="savingSearchKeys"
        :disabled="savingSearchKeys"
        @blur="saveSearchKeys"
        @keyup.enter="saveSearchKeys"
        class="api-key-input"
      />
      <n-tag v-if="searchKeys.ollama_api_key_set" type="success" size="small">已设置</n-tag>
    </div>
    <template #feedback>
      <span class="api-key-hint">
        获取地址：<a href="https://ollama.com/settings/keys" class="external-link" @click.prevent="openExternal('https://ollama.com/settings/keys')">https://ollama.com/settings/keys</a>
      </span>
    </template>
  </n-form-item>

  <n-form-item label="Tavily API Key">
    <div class="api-key-row">
      <n-input
        v-model:value="newTavilyApiKey"
        type="password"
        show-password-on="click"
        :placeholder="searchKeys.tavily_api_key_set ? '已设置，输入新值覆盖' : '输入 API Key 保存'"
        :loading="savingSearchKeys"
        :disabled="savingSearchKeys"
        @blur="saveSearchKeys"
        @keyup.enter="saveSearchKeys"
        class="api-key-input"
      />
      <n-tag v-if="searchKeys.tavily_api_key_set" type="success" size="small">已设置</n-tag>
    </div>
    <template #feedback>
      <span class="api-key-hint">
        获取地址：<a href="https://app.tavily.com/" class="external-link" @click.prevent="openExternal('https://app.tavily.com/')">https://app.tavily.com/</a>
        （免费额度 1000 次/月）
      </span>
    </template>
  </n-form-item>

  <!-- 服务 API KEY -->
  <n-form-item>
    <template #label>本机服务地址 <HelpTip content="llama-server 的 HTTP 服务地址。默认 http://127.0.0.1:8080，修改后需重启应用生效" /></template>
    <n-input
      v-model:value="formConfig.api_base"
      placeholder="http://127.0.0.1:8080"
      @blur="autoSave"
    />
  </n-form-item>
  <n-form-item label="暴露服务器地址" label-width="140" label-placement="left">
    <n-switch v-model:value="formConfig.expose_server" @update:value="onExposeServerToggle" />
    <span class="setting-hint">开启后局域网设备可通过本机 IP 访问（需重启服务生效）。开启后必须配置 API Key，否则服务将拒绝启动</span>
  </n-form-item>
  <n-form-item label-width="140" label-placement="left">
    <template #label>启用 API Key 验证 <HelpTip content="开启后所有 API 请求需携带 API Key，防止未授权访问。暴露到局域网时强烈建议开启" /></template>
    <n-switch v-model:value="formConfig.server_api_key_enabled" @update:value="onServerAPIKeyToggle" />
  </n-form-item>
  <n-form-item v-if="formConfig.server_api_key_enabled" label="API Key" label-width="80">
    <div class="api-key-row">
      <n-input
        v-model:value="serverApiKey"
        type="password"
        show-password-on="click"
        :placeholder="hasServerApiKey ? '已设置，留空保持不变' : '设置后 API 请求需携带此密钥'"
        :loading="savingServerApiKey"
        :disabled="savingServerApiKey"
        @blur="saveServerApiKey"
        @keyup.enter="saveServerApiKey"
        class="api-key-input"
      />
      <n-tag v-if="hasServerApiKey" type="success" size="small">已设置</n-tag>
    </div>
  </n-form-item>

  <!-- 模型量化类型（只读显示） -->
  <n-form-item v-if="modelFtype">
    <template #label>量化类型 <HelpTip content="当前模型的量化格式，从 GGUF 元数据解析。影响模型质量与显存占用的平衡" /></template>
    <n-tag size="small" type="info">{{ modelFtype }}</n-tag>
  </n-form-item>

  <!-- 生成参数扩展 -->
  <n-divider style="margin: 8px 0" />
  <n-form-item>
    <template #label>内存映射 (mmap) <HelpTip content="将模型文件映射到内存而非全部加载。开启可加快启动速度、减少内存占用，关闭则全部预加载到内存" /></template>
    <n-switch v-model:value="formConfig.mmap" />
  </n-form-item>
  <n-form-item>
    <template #label>KV 缓存 K 类型 <HelpTip content="Key 缓存的量化精度。K 决定注意力查找方向，建议用高精度（q8_0）。选「自动」由系统根据显存自动选择" /></template>
    <n-select v-model:value="formConfig.cache_type_k" :options="cacheTypeKOptions" placeholder="自动（q8_0）" clearable />
  </n-form-item>
  <n-form-item>
    <template #label>KV 缓存 V 类型 <HelpTip content="Value 缓存的量化精度。V 是实际内容，可以更激进压缩。选「自动」由系统智能选择" /></template>
    <n-select v-model:value="formConfig.cache_type_v" :options="cacheTypeVOptions" placeholder="自动（q4_0）" clearable />
  </n-form-item>
  <n-form-item>
    <template #label>KV 缓存卸载 <HelpTip content="开启后允许将 KV 缓存卸载到 CPU 内存，节省显存但会降低速度。显存不足时可开启" /></template>
    <n-switch v-model:value="formConfig.kv_offload" />
  </n-form-item>
  <n-form-item>
    <template #label>上下文移位 <HelpTip content="当对话超出上下文长度时，自动移除最早的内容腾出空间，而非直接报错。开启可支持更长的连续对话" /></template>
    <n-switch v-model:value="formConfig.context_shift" />
  </n-form-item>
  <n-form-item>
    <template #label>检查点最小步长 <HelpTip content="上下文检查点之间的最小 token 步数。0=使用默认值，设置后每隔指定步数保存一次检查点，便于回溯和 KV 缓存复用" /></template>
    <n-input-number v-model:value="formConfig.checkpoint_min_step" :min="0" :max="1000" :step="1" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>GPU 设备 <HelpTip content="指定使用的 GPU 设备。留空自动选择，多卡场景用逗号分隔（如 0,1）" /></template>
    <n-input v-model:value="formConfig.device" placeholder="留空自动选择，多卡如 0,1" />
  </n-form-item>
  <n-form-item>
    <template #label>并发槽位数 <HelpTip content="同时处理的请求数。0=自动（通常为 1），增大可支持多用户并发但会按比例增加显存占用" /></template>
    <n-input-number v-model:value="formConfig.parallel" :min="0" placeholder="0 = 自动" style="width: 100%" />
  </n-form-item>
  <n-form-item>
    <template #label>分组注意力 N <HelpTip content="将上下文分成 N 组进行注意力计算，用于超长文本生成。0=禁用，需同时设置 W 才生效" /></template>
    <n-input-number v-model:value="formConfig.grp_attn_n" :min="0" :max="128" :step="1" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>分组注意力 W <HelpTip content="分组注意力的窗口宽度（token 数）。0=禁用，需同时设置 N 才生效" /></template>
    <n-input-number v-model:value="formConfig.grp_attn_w" :min="0" :max="131072" :step="512" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-text v-if="(formConfig.grp_attn_n > 0 && formConfig.grp_attn_w === 0) || (formConfig.grp_attn_w > 0 && formConfig.grp_attn_n === 0)" depth="3" style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px;">
    分组注意力需要同时设置 N 和 W 才能生效
  </n-text>
  <n-form-item>
    <template #label>缓存复用块大小 <HelpTip content="KV 缓存复用的块大小。0=禁用，设置后启用 KV shifting 复用缓存块，可减少重复计算" /></template>
    <n-input-number v-model:value="formConfig.cache_reuse" :min="0" :step="1" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>空闲休眠(秒) <HelpTip content="服务器空闲指定秒数后自动休眠以节省资源。-1=禁用休眠，0=立即休眠" /></template>
    <n-input-number v-model:value="formConfig.sleep_idle_seconds" :min="-1" :step="1" placeholder="-1" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>预填充助手消息 <HelpTip content="开启后，最后一条助手消息会被预填充到 KV 缓存，加速后续推理。关闭则不预填充" /></template>
    <n-switch v-model:value="formConfig.prefill_assistant" />
  </n-form-item>

  <!-- 推测解码 -->
  <n-form-item>
    <template #label>推测类型 <HelpTip content="加速推理的推测解码技术。draft-mtp 需要模型内置 MTP 头（如 Qwen3.6-UD），draft-eagle3 需要 Eagle3 草稿模型，ngram 类型对所有模型可用。自动模式下检测到 MTP 头会自动启用 draft-mtp，否则启用 ngram-mod" /></template>
    <n-select v-model:value="formConfig.spec_type" :options="specTypeOptions" placeholder="自动检测" clearable :disabled="formConfig.spec_default" />
  </n-form-item>
  <n-text v-if="!settingsStore.modelCapabilities.has_mtp" depth="3" style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px;">
    当前模型不支持 MTP，draft-mtp 选项已隐藏
  </n-text>
  <template v-if="formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-dflash' || formConfig.spec_type === 'draft-simple'">
    <n-text v-if="!formConfig.spec_draft_model" type="warning" style="font-size: 12px; display: block; margin-bottom: 8px;">
      未配置 Draft 模型路径，推测解码将无法工作
    </n-text>
    <n-form-item>
      <template #label>Draft 模型路径 <HelpTip content="Eagle3/DFlash 或 Draft 草稿模型的 .gguf 文件路径。draft-eagle3/draft-dflash/draft-simple 模式需要" /></template>
      <n-input v-model:value="formConfig.spec_draft_model" placeholder="Eagle3/DFlash/Draft 草稿模型文件路径" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>Draft GPU 层数 <HelpTip content="草稿模型加载到 GPU 的层数。0=全部用 CPU，100=全部用 GPU。建议与主模型一致以保证加速效果" /></template>
      <n-input-number v-model:value="formConfig.spec_draft_ngl" :min="0" :max="100" :step="1" placeholder="0" style="width: 100%" />
    </n-form-item>
    <n-form-item>
      <template #label>Draft 设备 <HelpTip content="草稿模型使用的 GPU 设备。留空自动选择，多卡场景可指定如 cuda:0" /></template>
      <n-input v-model:value="formConfig.spec_draft_device" placeholder="留空自动选择，如 cuda:0" @blur="autoSave" />
    </n-form-item>
  </template>
  <n-form-item v-if="formConfig.spec_type === 'draft-mtp' || formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-dflash' || formConfig.spec_type === 'draft-simple'">
    <template #label>最大 Draft Token 数 <HelpTip content="每次推测最多生成的 token 数。值越大潜在加速越快但准确率下降，建议 3-4。DFlash 可达 block_size-1（通常 15）" /></template>
    <n-input-number v-model:value="formConfig.spec_draft_n_max" :min="1" :max="15" :step="1" placeholder="3" @blur="autoSave" />
  </n-form-item>
  <n-form-item v-if="formConfig.spec_type === 'draft-mtp' || formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-dflash' || formConfig.spec_type === 'draft-simple'">
    <template #label>最小 Draft Token 数 <HelpTip content="每次推测最少生成的 token 数。0=不限制，设置后即使准确率低也会生成此数量的 token" /></template>
    <n-input-number v-model:value="formConfig.spec_draft_n_min" :min="0" :max="15" :step="1" placeholder="0" @blur="autoSave" />
  </n-form-item>
  <template v-if="formConfig.spec_type === 'ngram-mod'">
    <n-form-item>
      <template #label>ngram-mod 最小 token 数 <HelpTip content="ngram-mod 推测的最小 n-gram 长度。值越小匹配越宽松，建议 48" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_mod_n_min" :min="1" :max="256" :step="1" placeholder="48" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>ngram-mod 最大 token 数 <HelpTip content="ngram-mod 推测的最大 n-gram 长度。值越大覆盖范围越广，建议 64" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_mod_n_max" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>ngram-mod 查找长度 <HelpTip content="ngram-mod 触发推测所需的最小匹配数。值越大越保守，建议 24" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_mod_n_match" :min="1" :max="256" :step="1" placeholder="24" @blur="autoSave" />
    </n-form-item>
  </template>
  <template v-if="formConfig.spec_type === 'ngram-simple'">
    <n-form-item>
      <template #label>Size-N <HelpTip content="ngram-simple 的 n-gram 长度。控制匹配窗口大小，建议 64" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_simple_size_n" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>Size-M <HelpTip content="ngram-simple 的 m-gram 长度。控制预测窗口大小，建议 64" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_simple_size_m" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>Min-Hits <HelpTip content="ngram-simple 触发推测所需的最小命中次数。值越大越保守，建议 1" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_simple_min_hits" :min="1" :max="256" :step="1" placeholder="1" @blur="autoSave" />
    </n-form-item>
  </template>
  <template v-if="formConfig.spec_type === 'ngram-map-k'">
    <n-form-item>
      <template #label>Size-N <HelpTip content="ngram-map-k 的 n-gram 长度。控制匹配窗口大小，建议 64" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_map_k_size_n" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>Size-M <HelpTip content="ngram-map-k 的 m-gram 长度。控制预测窗口大小，建议 64" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_map_k_size_m" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>Min-Hits <HelpTip content="ngram-map-k 触发推测所需的最小命中次数。值越大越保守，建议 1" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_map_k_min_hits" :min="1" :max="256" :step="1" placeholder="1" @blur="autoSave" />
    </n-form-item>
  </template>
  <template v-if="formConfig.spec_type === 'ngram-map-k4v'">
    <n-form-item>
      <template #label>Size-N <HelpTip content="ngram-map-k4v 的 n-gram 长度。控制匹配窗口大小，建议 64" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_map_k4v_size_n" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>Size-M <HelpTip content="ngram-map-k4v 的 m-gram 长度。控制预测窗口大小，建议 64" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_map_k4v_size_m" :min="1" :max="256" :step="1" placeholder="64" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>Min-Hits <HelpTip content="ngram-map-k4v 触发推测所需的最小命中次数。值越大越保守，建议 1" /></template>
      <n-input-number v-model:value="formConfig.spec_ngram_map_k4v_min_hits" :min="1" :max="256" :step="1" placeholder="1" @blur="autoSave" />
    </n-form-item>
  </template>
  <template v-if="formConfig.spec_type === 'ngram-cache'">
    <n-form-item>
      <template #label>静态缓存路径 <HelpTip content="预构建的静态 lookup 缓存文件路径。可显著加速推测解码，通常由训练工具生成" /></template>
      <n-input v-model:value="formConfig.lookup_cache_static" placeholder="lookup-cache-static 文件路径" @blur="autoSave" />
    </n-form-item>
    <n-form-item>
      <template #label>动态缓存路径 <HelpTip content="运行时生成的动态 lookup 缓存文件路径。会在推理过程中自动更新，加速后续请求" /></template>
      <n-input v-model:value="formConfig.lookup_cache_dynamic" placeholder="lookup-cache-dynamic 文件路径" @blur="autoSave" />
    </n-form-item>
  </template>
  <n-form-item v-if="formConfig.spec_type === 'draft-mtp' || formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-dflash' || formConfig.spec_type === 'draft-simple'">
    <template #label>Draft 线程数 <HelpTip content="Draft 模型使用的线程数。0=使用主模型线程数" /></template>
    <n-input-number v-model:value="formConfig.spec_draft_threads" :min="0" :max="256" :step="1" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item v-if="formConfig.spec_type === 'draft-mtp' || formConfig.spec_type === 'draft-eagle3' || formConfig.spec_type === 'draft-dflash' || formConfig.spec_type === 'draft-simple'">
    <template #label>Draft 批处理线程数 <HelpTip content="Draft 模型批处理时使用的线程数。0=使用主模型线程数" /></template>
    <n-input-number v-model:value="formConfig.spec_draft_threads_batch" :min="0" :max="256" :step="1" placeholder="0" style="width: 100%" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>默认推测配置 <HelpTip content="使用 llama.cpp 的默认推测解码配置，自动选择合适的参数。启用后其他推测参数将被忽略" /></template>
    <n-switch v-model:value="formConfig.spec_default" @update:value="autoSave" />
  </n-form-item>
  <n-text v-if="formConfig.spec_default" depth="3" style="font-size: 12px; margin-top: -12px; display: block; margin-bottom: 8px;">
    默认推测配置已启用，其他推测参数将被忽略
  </n-text>

  <!-- RAG 重排序配置 -->
  <n-form-item>
    <template #label>Reranker 模型 <HelpTip content="RAG 检索结果重排序使用的模型路径（.gguf 文件）。留空则不启用重排序，使用向量检索的原始排序" /></template>
    <n-input v-model:value="formConfig.reranker_model_path" placeholder="Reranker 模型文件路径（如 models/bge-reranker-v2-m3.gguf）" @blur="autoSave" />
  </n-form-item>
  <n-form-item>
    <template #label>重排序 Top N <HelpTip content="重排序后保留的文档数量。值越大召回越全但耗时越长，建议 3-5" /></template>
    <n-input-number v-model:value="formConfig.rerank_top_n" :min="1" :max="20" :step="1" placeholder="5" style="width: 100%" />
  </n-form-item>

  <!-- KV 缓存持久化 -->
  <n-form-item>
    <template #label>启用 KV 缓存持久化 <HelpTip content="开启后将对话的 KV 缓存保存到磁盘，下次加载相同上下文时可跳过预填充，加快首 token 响应速度" /></template>
    <n-switch v-model:value="formConfig.slot_save_enabled" />
  </n-form-item>
  <n-form-item v-if="formConfig.slot_save_enabled">
    <template #label>缓存保存路径 <HelpTip content="KV 缓存保存到磁盘的目录路径。留空则使用默认路径（appDir/slots/）" /></template>
    <n-input v-model:value="formConfig.slot_save_path" placeholder="留空则使用默认路径（appDir/slots/）" @blur="autoSave" />
  </n-form-item>

  <!-- LoRA 适配器管理 -->
  <LoraManager v-model:loraPaths="formConfig.lora_paths" @update:loraPaths="autoSave" />
</template>

<script setup lang="ts">
import { inject, defineComponent, h, ref, onMounted, watch } from 'vue'
import {
  NButton, NFormItem, NInput, NDivider,
  NSwitch, NInputNumber, NSelect, NTooltip, NTag,
} from 'naive-ui'
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime'
import LoraManager from '../LoraManager.vue'
import { SETTINGS_CONTEXT_KEY, type SettingsContext } from './settingsContext'
import { wails } from '../../services/wails'

/**
 * 安全实践（基于 VUE-XSS-004 #4）：外部链接统一走 BrowserOpenURL 用系统默认浏览器打开
 * 防止 Wails WebView 内部导航，与 MessageList.vue 的 handleLinkClick 保持一致
 */
function openExternal(url: string) {
  BrowserOpenURL(url)
}

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
  newOllamaApiKey, newTavilyApiKey, searchKeys, saveSearchKeys, savingSearchKeys,
  serverApiKey, hasServerApiKey, saveServerApiKey, savingServerApiKey,
  onServerAPIKeyToggle, onExposeServerToggle,
  cacheTypeKOptions, cacheTypeVOptions, specTypeOptions,
  settingsStore,
} = ctx

// 模型量化类型（从 GGUF 元数据解析）
const modelFtype = ref('')

async function loadModelFtype() {
  try {
    const smartParams = await wails.getSmartParams()
    modelFtype.value = smartParams.model.ftype || ''
  } catch {
    modelFtype.value = ''
  }
}

onMounted(loadModelFtype)
// 模型切换时重新加载量化类型
watch(() => settingsStore.currentModel, loadModelFtype)
</script>

<style scoped>
.api-key-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.api-key-input {
  flex: 1;
}

.api-key-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
}

/* 外部链接样式：与 naive-ui 默认链接外观一致 */
.external-link {
  color: var(--n-color-primary, #18a058);
  text-decoration: none;
  cursor: pointer;
}

.external-link:hover {
  text-decoration: underline;
}

/* "已设置"标签淡入动画：保存成功后的微妙视觉确认 */
.api-key-row :deep(.n-tag) {
  animation: tagFadeIn 0.3s ease;
}

@keyframes tagFadeIn {
  from { opacity: 0; transform: scale(0.85); }
  to { opacity: 1; transform: scale(1); }
}

.setting-hint {
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-left: 12px;
}

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
