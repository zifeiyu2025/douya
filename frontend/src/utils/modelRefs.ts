/**
 * 模型参考配置常量
 *
 * 从 SettingsView.vue 抽取，供设置页面显示模型推荐参数。
 * 包含各模型的官方推荐采样参数（温度、Top P、Top K 等）。
 */

export interface ModelRefConfig {
  name: string
  raw: {
    temperature: number
    top_p: number
    top_k: number
    context_size: number
    repeat_penalty: number
  }
  raw_thinking?: {
    temperature: number
    top_p: number
    top_k: number
    context_size: number
    repeat_penalty: number
  }
  params: { label: string; value: string }[]
  params_thinking?: { label: string; value: string }[]
  note?: string
}

export const MODEL_REFS: Record<string, ModelRefConfig> = {
  'qwen3.5-9b': {
    name: 'Qwen3.5-9B',
    raw: { temperature: 0.7, top_p: 0.8, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 20,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K / YaRN 扩展 1M' },
      { label: '温度', value: '0.7 (非思考/常规)' },
      { label: 'Top P', value: '0.8 (非思考)' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~9B (Dense)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K / YaRN 扩展 1M' },
      { label: '温度', value: '1.0 (思考模式/常规)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~9B (Dense)' }
    ],
    note: 'Qwen3.5-9B 官方推荐：非思考模式 temperature=0.7/top_p=0.8，思考模式 temperature=1.0/top_p=0.95。编码任务建议 temperature=0.6'
  },
  'gemma-4-e4b': {
    name: 'Gemma4-E4B',
    raw: { temperature: 1.0, top_p: 0.95, top_k: 64, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 64,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~8B (有效 4.5B)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~8B (有效 4.5B)' }
    ],
    note: 'Gemma4 E4B 端侧模型，Google 官方推荐 temperature=1.0、top_k=64，不建议开启重复惩罚'
  },
  'gemma-4-12b': {
    name: 'Gemma4-12B',
    raw: { temperature: 1.0, top_p: 0.95, top_k: 64, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 64,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~12B (Dense)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~12B (Dense)' }
    ],
    note: 'Gemma4 12B Unified 模型，Google 官方推荐 temperature=1.0、top_k=64，不建议开启重复惩罚'
  },
  'gemma-4-e2b': {
    name: 'Gemma4-E2B',
    raw: { temperature: 1.0, top_p: 0.95, top_k: 64, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 64,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~5.1B (有效 2.3B)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~5.1B (有效 2.3B)' }
    ],
    note: 'Gemma4 E2B 端侧模型，Google 官方推荐 temperature=1.0、top_k=64，不建议开启重复惩罚'
  },
  'gemma-4-26b': {
    name: 'Gemma4-26B-A4B',
    raw: { temperature: 1.0, top_p: 0.95, top_k: 64, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 64,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~25.2B (MoE, 激活 3.8B)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~25.2B (MoE, 激活 3.8B)' }
    ],
    note: 'Gemma4 26B MoE 架构，激活参数仅 3.8B，Google 官方推荐 temperature=1.0、top_k=64'
  },
  'gemma-4-31b': {
    name: 'Gemma4-31B',
    raw: { temperature: 1.0, top_p: 0.95, top_k: 64, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 64,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~30.7B (Dense)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0 (不建议开启)' },
      { label: '参数量', value: '~30.7B (Dense)' }
    ],
    note: 'Gemma4 31B Dense 架构，Google 官方推荐 temperature=1.0、top_k=64，不建议开启重复惩罚'
  },
  'qwen3.5-9b-deepseek': {
    name: 'Qwen3.5-9B-DeepSeek-V4-Flash',
    raw: { temperature: 0.7, top_p: 0.8, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 20,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 1M' },
      { label: '温度', value: '0.7 (非思考/常规)' },
      { label: 'Top P', value: '0.8 (非思考)' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~9B (Dense)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 1M' },
      { label: '温度', value: '1.0 (思考模式/常规)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~9B (Dense)' }
    ],
    note: 'DeepSeek-V4-Flash 蒸馏版，采样参数与 Qwen3.5-9B 一致'
  },
  'qwen3.5-9b-glm': {
    name: 'Qwen3.5-9B-GLM5.1-Distill-v1',
    raw: { temperature: 0.7, top_p: 0.8, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 20,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K / YaRN 扩展' },
      { label: '温度', value: '0.7 (非思考/常规)' },
      { label: 'Top P', value: '0.8 (非思考)' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~9B (Dense)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K / YaRN 扩展' },
      { label: '温度', value: '1.0 (思考模式/常规)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~9B (Dense)' }
    ],
    note: 'GLM5.1 蒸馏版，采样参数与 Qwen3.5-9B 一致'
  },
  'llama-3.1-8b': {
    name: 'Llama 3.1 8B',
    raw: { temperature: 0.6, top_p: 0.9, top_k: 40, context_size: 8192, repeat_penalty: 1.1 },
    params: [
      { label: '上下文长度', value: '8K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.1' },
      { label: '量化格式', value: 'Q4_K_M (~5GB VRAM)' },
      { label: '参数量', value: '~8B' }
    ],
    note: 'Llama 3.1 8B 原生上下文 128K，本地推荐 8K 以节省显存'
  },
  'llama-3.1-70b': {
    name: 'Llama 3.1 70B',
    raw: { temperature: 0.6, top_p: 0.9, top_k: 40, context_size: 8192, repeat_penalty: 1.1 },
    params: [
      { label: '上下文长度', value: '8K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.1' },
      { label: '量化格式', value: 'Q4_K_M (~40GB VRAM)' },
      { label: '参数量', value: '~70B' }
    ],
    note: 'Llama 3.1 70B 需要 40GB+ 显存，推荐多卡或 Q2_K 量化'
  },
  'llama-3.3-70b': {
    name: 'Llama 3.3 70B',
    raw: { temperature: 0.6, top_p: 0.9, top_k: 40, context_size: 8192, repeat_penalty: 1.1 },
    params: [
      { label: '上下文长度', value: '8K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.1' },
      { label: '量化格式', value: 'Q4_K_M (~40GB VRAM)' },
      { label: '参数量', value: '~70B' }
    ],
    note: 'Llama 3.3 70B Instruct 是 Llama 3.1 70B 的升级版，指令跟随能力更强'
  },
  'llama-4-scout': {
    name: 'Llama 4 Scout',
    raw: { temperature: 0.6, top_p: 0.9, top_k: 40, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 0.8,
      top_p: 0.95,
      top_k: 40,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 10M' },
      { label: '温度', value: '0.6 (非思考)' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~17B (MoE 16专家)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 10M' },
      { label: '温度', value: '0.8 (思考模式)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~17B (MoE 16专家)' }
    ],
    note: 'Llama 4 Scout MoE 架构，原生支持 10M 超长上下文，支持思考模式'
  },
  'llama-4-maverick': {
    name: 'Llama 4 Maverick',
    raw: { temperature: 0.6, top_p: 0.9, top_k: 40, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 0.8,
      top_p: 0.95,
      top_k: 40,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 1M' },
      { label: '温度', value: '0.6 (非思考)' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~17B (MoE 128专家)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 1M' },
      { label: '温度', value: '0.8 (思考模式)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~17B (MoE 128专家)' }
    ],
    note: 'Llama 4 Maverick MoE 架构，128专家路由，支持思考模式'
  },
  'deepseek-r1': {
    name: 'DeepSeek-R1',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 0.6,
      top_p: 0.95,
      top_k: 20,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~671B (MoE 37B激活)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~671B (MoE 37B激活)' }
    ],
    note: 'DeepSeek-R1 推理模型，思考模式为 Reasoning 类型（不可关闭思考），temperature 建议 0.5-0.7'
  },
  'deepseek-r1-distill': {
    name: 'DeepSeek-R1-Distill',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 0.6,
      top_p: 0.95,
      top_k: 20,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '1.5B/7B/8B/14B/32B/70B' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '1.5B/7B/8B/14B/32B/70B' }
    ],
    note: 'DeepSeek-R1 蒸馏版，保留推理能力，思考模式为 Reasoning 类型'
  },
  'deepseek-v3': {
    name: 'DeepSeek-V3',
    raw: { temperature: 0.6, top_p: 0.95, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 0.6,
      top_p: 0.95,
      top_k: 20,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~671B (MoE 37B激活)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~671B (MoE 37B激活)' }
    ],
    note: 'DeepSeek-V3 MoE 架构，支持思考模式（Reasoning 类型）'
  },
  'mistral-7b': {
    name: 'Mistral 7B',
    raw: { temperature: 0.7, top_p: 0.9, top_k: 40, context_size: 8192, repeat_penalty: 1.1 },
    params: [
      { label: '上下文长度', value: '8K (推荐) / 最大 32K (RoPE)' },
      { label: '温度', value: '0.7' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.1' },
      { label: '量化格式', value: 'Q4_K_M (~4.4GB VRAM)' },
      { label: '参数量', value: '~7B' }
    ],
    note: 'Mistral 7B 经典小模型，适合基础对话和文本生成'
  },
  'mistral-small': {
    name: 'Mistral Small',
    raw: { temperature: 0.7, top_p: 0.9, top_k: 40, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 0.8,
      top_p: 0.95,
      top_k: 40,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.7 (非思考)' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~24B' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K' },
      { label: '温度', value: '0.8 (思考模式)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~24B' }
    ],
    note: 'Mistral Small 3.1 支持视觉和思考模式（Template 类型），推荐 Q4_K_M 量化'
  },
  'phi-4': {
    name: 'Phi-4',
    raw: { temperature: 0.7, top_p: 0.9, top_k: 40, context_size: 16384, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '16K (推荐) / 最大 16K' },
      { label: '温度', value: '0.7' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '量化格式', value: 'Q4_K_M (~5.5GB VRAM)' },
      { label: '参数量', value: '~14B' }
    ],
    note: 'Phi-4 微软小模型，擅长推理和编程，上下文 16K'
  },
  'phi-4-reasoning': {
    name: 'Phi-4-Reasoning',
    raw: { temperature: 0.8, top_p: 0.95, top_k: 40, context_size: 16384, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 0.8,
      top_p: 0.95,
      top_k: 40,
      context_size: 16384,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '16K (推荐) / 最大 16K' },
      { label: '温度', value: '0.8' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~14B' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '16K (推荐) / 最大 16K' },
      { label: '温度', value: '0.8' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~14B' }
    ],
    note: 'Phi-4-Reasoning 推理增强版，思考模式为 Reasoning 类型'
  },
  'qwen3.6': {
    name: 'Qwen3.6-35B-A3B',
    raw: { temperature: 0.7, top_p: 0.8, top_k: 20, context_size: 32768, repeat_penalty: 1.0 },
    raw_thinking: {
      temperature: 1.0,
      top_p: 0.95,
      top_k: 20,
      context_size: 32768,
      repeat_penalty: 1.0
    },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K / YaRN 扩展' },
      { label: '温度', value: '0.7 (非思考/常规)' },
      { label: 'Top P', value: '0.8 (非思考)' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~35B (MoE, 激活 3B)' }
    ],
    params_thinking: [
      { label: '上下文长度', value: '32K (推荐) / 最大 256K / YaRN 扩展' },
      { label: '温度', value: '1.0 (思考模式/常规)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '20' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '~35B (MoE, 激活 3B)' }
    ],
    note: 'Qwen3.6 MoE 架构，35B 总参数仅激活 3B，采样参数与 Qwen3.5 一致'
  },
  'qwen2.5': {
    name: 'Qwen2.5',
    raw: { temperature: 0.7, top_p: 0.9, top_k: 40, context_size: 32768, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K (YaRN)' },
      { label: '温度', value: '0.7' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '0.5B~72B' }
    ],
    note: 'Qwen2.5 系列，0.5B 到 72B 多种规格，Coder 版本适合编程'
  },
  'qwen2.5-coder': {
    name: 'Qwen2.5-Coder',
    raw: { temperature: 0.2, top_p: 0.9, top_k: 40, context_size: 32768, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '32K (推荐) / 最大 128K (YaRN)' },
      { label: '温度', value: '0.2 (编程推荐低温度)' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '0.5B~32B' }
    ],
    note: 'Qwen2.5-Coder 编程专用，推荐低 temperature (0.1-0.3) 获得确定性输出'
  },
  'yi-1.5': {
    name: 'Yi-1.5',
    raw: { temperature: 0.6, top_p: 0.9, top_k: 40, context_size: 4096, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '4K (推荐) / 最大 16K' },
      { label: '温度', value: '0.6' },
      { label: 'Top P', value: '0.9' },
      { label: 'Top K', value: '40' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '6B/9B/34B' }
    ],
    note: 'Yi-1.5 系列，中文能力强，6B/9B 适合轻量部署'
  },
  'gemma-2': {
    name: 'Gemma 2',
    raw: { temperature: 1.0, top_p: 0.95, top_k: 64, context_size: 8192, repeat_penalty: 1.0 },
    params: [
      { label: '上下文长度', value: '8K (2B/9B) / 32K (27B)' },
      { label: '温度', value: '1.0 (Google 官方推荐)' },
      { label: 'Top P', value: '0.95' },
      { label: 'Top K', value: '64' },
      { label: '重复惩罚', value: '1.0' },
      { label: '参数量', value: '2B/9B/27B' }
    ],
    note: 'Gemma 2 Google 官方推荐 temperature=1.0，Top K=64，与 Gemma 4 参数风格一致'
  }
}
