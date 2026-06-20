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

const errorPatterns: ErrorPattern[] = [
  {
    pattern: /out of memory|OOM|CUDA out of memory|VRAM|gpu.*memory/i,
    guidance: {
      category: 'VRAM不足',
      title: 'GPU 显存不足',
      description: '模型加载需要的显存超过了 GPU 可用显存。',
      suggestions: [
        '在设置中降低 GPU 层数（n-gpu-layers）',
        '使用更小量化的模型版本（如 Q4_0 代替 Q8_0）',
        '关闭其他占用 GPU 的程序',
        '减小上下文窗口大小（context_size）',
      ],
    },
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
        '暂时不使用多模态功能（已自动降级为纯文本）',
      ],
    },
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
        '确认文件没有被杀毒软件隔离',
      ],
    },
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
        '查看任务管理器中 llama-server 进程是否正常运行',
      ],
    },
  },
  {
    pattern: /context.*size|context.*overflow|exceed.*context|prompt.*too.*long/i,
    guidance: {
      category: '上下文溢出',
      title: '上下文长度超限',
      description: '对话内容或附件超过了模型的上下文窗口大小。',
      suggestions: [
        '在设置中增大上下文窗口大小（context_size）',
        '减少对话历史或开启上下文移位',
        '缩短上传的文件内容',
        '减少附件数量',
      ],
    },
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
        '查看日志获取详细崩溃信息',
      ],
    },
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
        '更新 llama-server 到支持该格式的版本',
      ],
    },
  },
]

/**
 * 根据错误信息匹配错误分类和修复指引
 */
export function classifyError(errorMsg: string): ErrorGuidance | null {
  if (!errorMsg) return null
  for (const { pattern, guidance } of errorPatterns) {
    if (pattern.test(errorMsg)) {
      return guidance
    }
  }
  return null
}
