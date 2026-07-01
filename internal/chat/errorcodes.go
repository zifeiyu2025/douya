// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

// 统一错误码常量
// 后端 enhanceErrorWithHint 会将这些错误码以 "[ERR_CODE] 提示信息" 的格式
// 作为前缀加在返回的提示信息前；前端 classifyError 优先匹配该前缀进行分类，
// 从而避免前后端各自通过字符串匹配导致的分类不一致问题。
//
// 生活类比：像快递单号，无论包裹经过多少中转站，单号始终唯一标识这个包裹。
// 错误码就是错误的"单号"，前后端都认这个唯一标识。
const (
	// ErrCodeContextOverflow 上下文长度超限
	ErrCodeContextOverflow = "ERR_CTX_OVERFLOW"
	// ErrCodeDLLMissing 运行时 DLL 文件缺失
	ErrCodeDLLMissing = "ERR_DLL_MISSING"
	// ErrCodeEngineMissing 引擎程序文件缺失（如 llama-server.exe 不存在）
	ErrCodeEngineMissing = "ERR_ENGINE_MISSING"
	// ErrCodeModelMissing 模型文件未找到
	ErrCodeModelMissing = "ERR_MODEL_MISSING"
	// ErrCodeOOM 显存/内存不足
	ErrCodeOOM = "ERR_OOM"
	// ErrCodePermanentFailure 服务反复崩溃，已停止自动重启
	ErrCodePermanentFailure = "ERR_PERMANENT_FAILURE"
	// ErrCodeTimeout 请求超时
	ErrCodeTimeout = "ERR_TIMEOUT"
)

// formatErrCode 在提示信息前加上 "[ERR_CODE]" 前缀
// 例如：formatErrCode(ErrCodeContextOverflow, "上下文长度超限...") 返回 "[ERR_CTX_OVERFLOW] 上下文长度超限..."
func formatErrCode(code string, msg string) string {
	return "[" + code + "] " + msg
}
