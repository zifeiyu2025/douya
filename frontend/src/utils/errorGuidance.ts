/**
 * 错误分类和修复指引工具
 * 将后端返回的错误信息映射为用户友好的提示和修复建议
 */

export interface ErrorGuidance {
  category: string
  title: string
  description: string
  suggestions: string[]
}

interface ErrorPattern {
  pattern: RegExp
  guidance: ErrorGuidance
}

/**
 * F-2.5：共享 ErrorGuidance 常量
 * errCodeGuidanceMap（错误码精确匹配）和 errorPatterns（字符串模式匹配）
 * 原各自维护一份相同的 guidance 对象，抽取为独立常量确保两处一致。
 *
 * 生活类比：像同一份产品说明书——不管顾客是通过条形码（错误码）
 * 还是通过外观描述（字符串匹配）查找商品，拿到的说明书都是同一份。
 */
const GUIDANCE_DLL_MISSING: ErrorGuidance = {
  category: 'DLL缺失',
  title: '运行时 DLL 文件缺失',
  description: 'llama-server 引擎依赖的 DLL 文件缺失，无法正常启动。',
  suggestions: [
    '检查 runtime/ 目录是否包含所有必要的 DLL 文件',
    '核心 DLL 包括：llama.dll、ggml.dll、ggml-base.dll、ggml-cpu.dll 等',
    '如果使用 NVIDIA GPU，还需 CUDA 运行时 DLL：cudart64_13.dll、cublas64_13.dll、cublasLt64_13.dll',
    '重新下载或解压完整的 runtime 压缩包'
  ]
}

const GUIDANCE_ENGINE_MISSING: ErrorGuidance = {
  category: '引擎缺失',
  title: '引擎程序文件缺失',
  description: 'llama-server.exe 引擎程序文件不存在，无法启动推理服务。',
  suggestions: [
    '检查 runtime/ 目录下是否存在 llama-server.exe',
    '在设置中检查 llama_server_path 配置路径是否正确',
    '重新下载或解压完整的 runtime 压缩包'
  ]
}

const GUIDANCE_MODEL_MISSING: ErrorGuidance = {
  category: '模型缺失',
  title: '未找到模型文件',
  description: 'models/ 目录下没有找到任何 GGUF 模型文件。',
  suggestions: [
    '将 GGUF 模型文件放入 models/ 目录',
    '确认模型文件扩展名为 .gguf',
    '从 Hugging Face 或 ModelScope 下载模型文件'
  ]
}

const GUIDANCE_CTX_OVERFLOW: ErrorGuidance = {
  category: '上下文溢出',
  title: '上下文长度超限',
  description: '对话内容或附件超过了模型的上下文窗口大小。',
  suggestions: [
    '在设置中增大上下文窗口大小（context_size）',
    '减少对话历史或开启上下文移位',
    '缩短上传的文件内容',
    '减少附件数量'
  ]
}

const GUIDANCE_OOM: ErrorGuidance = {
  category: '显存/内存不足',
  title: '显存或内存不足',
  description: '模型加载需要的显存或内存超过了系统可用资源。',
  suggestions: [
    '在设置中降低 GPU 层数（n-gpu-layers）',
    '使用更小量化的模型版本（如 Q4_0 代替 Q8_0）',
    '关闭其他占用 GPU/内存的程序',
    '减小上下文窗口大小（context_size）'
  ]
}

const GUIDANCE_PERMANENT_FAILURE: ErrorGuidance = {
  category: '永久失败',
  title: '服务器反复崩溃，已停止自动重启',
  description: 'llama-server 连续多次启动失败，系统已停止自动重试以避免资源浪费。',
  suggestions: [
    '检查模型文件是否完整、量化类型是否受支持',
    '降低 GPU 层数、上下文大小等参数后重试',
    '查看日志获取每次崩溃的具体原因',
    '重启豆芽应用以重置失败计数'
  ]
}

const GUIDANCE_TIMEOUT: ErrorGuidance = {
  category: '请求超时',
  title: '请求超时',
  description: '请求在规定时间内未收到响应，可能是网络问题或服务繁忙。',
  suggestions: [
    '检查网络连接是否正常',
    '稍后重试',
    '查看任务管理器中 llama-server 进程是否正常运行'
  ]
}

/**
 * 统一错误码 → 前端指引的映射表
 * 与后端 internal/chat/errorcodes.go 中的常量保持一致。
 * 后端 enhanceErrorWithHint 会在提示信息前加 "[ERR_CODE]" 前缀，
 * 前端优先通过该前缀精确匹配，避免字符串匹配的不一致问题。
 *
 * 生活类比：像快递单号查询，凭单号（错误码）能直接定位到对应的处理流程，
 * 不需要再根据包裹外观（错误文本）猜测分类。
 */
const errCodeGuidanceMap: Record<string, ErrorGuidance> = {
  // 上下文长度超限
  ERR_CTX_OVERFLOW: GUIDANCE_CTX_OVERFLOW,
  // 运行时 DLL 文件缺失
  ERR_DLL_MISSING: GUIDANCE_DLL_MISSING,
  // 引擎程序文件缺失
  ERR_ENGINE_MISSING: GUIDANCE_ENGINE_MISSING,
  // 模型文件未找到
  ERR_MODEL_MISSING: GUIDANCE_MODEL_MISSING,
  // 显存/内存不足
  ERR_OOM: GUIDANCE_OOM,
  // 服务反复崩溃，已停止自动重启
  ERR_PERMANENT_FAILURE: GUIDANCE_PERMANENT_FAILURE,
  // 请求超时
  ERR_TIMEOUT: GUIDANCE_TIMEOUT
}

// 匹配 "[ERR_CODE] 提示信息" 前缀的错误码
const errCodePrefixPattern = /^\[([A-Z_]+)\]\s*/

const errorPatterns: ErrorPattern[] = [
  {
    pattern:
      /DLL.*缺失|核心DLL|CUDA运行时DLL|dll not found|\.dll.*not found|specified module could not be found/i,
    guidance: GUIDANCE_DLL_MISSING
  },
  {
    pattern:
      /引擎程序.*不存在|引擎程序文件不存在|llama-server.*not found|the system cannot find the file/i,
    guidance: GUIDANCE_ENGINE_MISSING
  },
  {
    pattern: /模型文件.*未找到|未找到任何.*\.gguf|no models found/i,
    guidance: GUIDANCE_MODEL_MISSING
  },
  {
    pattern:
      /显存不足|out of memory|OOM|CUDA out of memory|CUDA error|VRAM|gpu.*memory|failed to allocate cuda/i,
    guidance: {
      category: 'VRAM不足',
      title: 'GPU 显存不足',
      description: '模型加载需要的显存超过了 GPU 可用显存。',
      suggestions: [
        '在设置中降低 GPU 层数（n-gpu-layers）',
        '使用更小量化的模型版本（如 Q4_0 代替 Q8_0）',
        '关闭其他占用 GPU 的程序',
        '减小上下文窗口大小（context_size）'
      ]
    }
  },
  {
    pattern:
      /内存不足|bad allocation|cannot allocate memory|mmap failed|std::bad_alloc|memory allocation failed/i,
    guidance: {
      category: '内存不足',
      title: '系统内存不足',
      description: '系统物理内存不足以加载模型，可能是内存不足或交换空间不够。',
      suggestions: [
        '关闭其他占用内存的程序',
        '增加系统虚拟内存（交换空间）',
        '使用更小量化的模型版本',
        '在设置中启用 --mmap（内存映射加载）'
      ]
    }
  },
  {
    pattern: /mmproj|projector|clip\.has_vision|vision_encoder/i,
    guidance: {
      category: '多模态',
      title: '多模态投影模型不兼容',
      description: 'mmproj 文件与当前 llama-server 版本或模型不兼容。',
      suggestions: [
        '更新 llama-server 到最新版本',
        '确认 mmproj 文件与模型版本匹配',
        '暂时不使用多模态功能（已自动降级为纯文本）'
      ]
    }
  },
  {
    pattern: /model.*not found|file.*not found|no such file|cannot find model/i,
    guidance: {
      category: '文件缺失',
      title: '模型文件未找到',
      description: '指定的模型文件路径不存在或无法访问。',
      suggestions: [
        '检查 models/ 目录下是否有对应的 GGUF 文件',
        '在设置中重新选择模型路径',
        '确认文件没有被杀毒软件隔离'
      ]
    }
  },
  {
    pattern: /connection refused|connect.*failed|ECONNREFUSED|timeout|timed out/i,
    guidance: {
      category: '连接错误',
      title: '服务连接失败',
      description: '无法连接到本地推理服务，可能是服务未启动或端口被占用。',
      suggestions: [
        '重启豆芽应用',
        '检查端口 8080 是否被其他程序占用',
        '查看任务管理器中 llama-server 进程是否正常运行'
      ]
    }
  },
  {
    pattern: /context.*size|context.*overflow|exceed.*context|prompt.*too.*long/i,
    guidance: GUIDANCE_CTX_OVERFLOW
  },
  {
    pattern: /crash|segfault|signal|killed|abort/i,
    guidance: {
      category: '进程崩溃',
      title: '推理服务崩溃',
      description: 'llama-server 进程异常退出，可能是模型不兼容或硬件问题。',
      suggestions: [
        '更新 llama-server 和 CUDA 驱动到最新版本',
        '尝试使用不同量化版本的模型',
        '检查 GPU 驱动是否正常',
        '查看日志获取详细崩溃信息'
      ]
    }
  },
  {
    pattern: /永久失败|permanent.*failure/i,
    guidance: GUIDANCE_PERMANENT_FAILURE
  },
  {
    pattern: /invalid.*model|corrupt|bad.*magic|unknown.*format/i,
    guidance: {
      category: '模型损坏',
      title: '模型文件损坏或格式不支持',
      description: 'GGUF 文件可能已损坏或格式不被当前版本支持。',
      suggestions: [
        '重新下载模型文件',
        '确认文件是有效的 GGUF 格式',
        '更新 llama-server 到支持该格式的版本'
      ]
    }
  }
]

/**
 * 根据错误信息匹配错误分类和修复指引
 *
 * 匹配优先级：
 * 1. 优先匹配 "[ERR_CODE]" 前缀（与后端 errorcodes.go 统一错误码精确对应）
 * 2. 若无前缀，回退到原有的字符串匹配逻辑（保持向后兼容）
 */
export function classifyError(errorMsg: string): ErrorGuidance | null {
  if (!errorMsg) return null

  // 1. 优先匹配统一错误码前缀 [ERR_CODE]
  const match = errCodePrefixPattern.exec(errorMsg)
  if (match) {
    const code = match[1]
    const guidance = errCodeGuidanceMap[code]
    if (guidance) {
      return guidance
    }
    // 未知错误码：继续走回退逻辑，避免漏分类
  }

  // 2. 回退到原有字符串匹配逻辑（向后兼容）
  for (const { pattern, guidance } of errorPatterns) {
    if (pattern.test(errorMsg)) {
      return guidance
    }
  }
  return null
}
