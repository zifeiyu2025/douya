// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"os"
	"path/filepath"
	"testing"
)

// 本文件补充 server_args.go 中未覆盖函数的测试。
// 每个测试检查一个具名风险，聚焦参数生成的关键分支和边界条件。
//
// 风险地图：
//   - baseArgs: host 绑定安全（暴露 vs 仅本机）
//   - appendModelLoadArgs: 崩溃降级、cache-type 校验
//   - appendRuntimeArgs: ctx-size 崩溃降级
//   - appendReasoningArgs: 后端采样与 reasoning-budget 互斥
//   - appendKVCacheArgs: context-shift + keep 保护 system prompt
//   - appendValidatedCacheType: 无效 cache-type 跳过
//   - appendSpeculativeArgs: SpecDefault 互斥逻辑
//   - appendNewFeatureArgs: MCP 配置文件存在性检查
//   - appendCPUMoeArgs: op-offload 开关方向
//   - appendLoraArgs: LoRA 路径解析

// ===== baseArgs 测试 =====

// TestBaseArgs_HostBindLocal 验证 ExposeServer=false 时绑定 127.0.0.1（仅本机）
// 风险：暴露到 0.0.0.0 会让局域网其他设备能访问 llama-server，造成安全风险。
func TestBaseArgs_HostBindLocal(t *testing.T) {
	s := newTestServer()
	s.config.ExposeServer = false

	args := s.baseArgs()
	if got := argValue(args, "--host"); got != "127.0.0.1" {
		t.Errorf("期望 --host=127.0.0.1（仅本机），实际: %q", got)
	}
}

// TestBaseArgs_HostBindExposed 验证 ExposeServer=true 时绑定 0.0.0.0（局域网可访问）
func TestBaseArgs_HostBindExposed(t *testing.T) {
	s := newTestServer()
	s.config.ExposeServer = true

	args := s.baseArgs()
	if got := argValue(args, "--host"); got != "0.0.0.0" {
		t.Errorf("期望 --host=0.0.0.0（暴露），实际: %q", got)
	}
}

// TestBaseArgs_NoWebUI 验证 EnableWebUI=false 时传递 --no-webui
// 风险：未禁用 webui 会让用户通过浏览器访问 llama-server 原生界面，绕过豆芽前端。
func TestBaseArgs_NoWebUI(t *testing.T) {
	s := newTestServer()
	s.config.EnableWebUI = false

	args := s.baseArgs()
	if !containsArg(args, "--no-webui") {
		t.Errorf("期望包含 --no-webui，实际 args: %v", args)
	}
}

// TestBaseArgs_EnableWebUI 验证 EnableWebUI=true 时不传递 --no-webui
func TestBaseArgs_EnableWebUI(t *testing.T) {
	s := newTestServer()
	s.config.EnableWebUI = true

	args := s.baseArgs()
	if containsArg(args, "--no-webui") {
		t.Errorf("期望不包含 --no-webui，实际 args: %v", args)
	}
}

// ===== appendModelLoadArgs 测试 =====

// TestAppendModelLoadArgs_CrashDegradeLevel2 验证崩溃降级级别 2 时 gpu-layers 被强制为 auto
// 风险：连续崩溃后未降级 gpu-layers，会继续用相同配置崩溃，无法恢复。
func TestAppendModelLoadArgs_CrashDegradeLevel2(t *testing.T) {
	s := newTestServer()
	s.config.GPULayers = "99"
	s.crashDegradeLevel.Store(2) // 模拟崩溃降级级别 2

	args := s.appendModelLoadArgs(nil)
	if got := argValue(args, "--gpu-layers"); got != "auto" {
		t.Errorf("期望降级级别 2 时 --gpu-layers=auto，实际: %q", got)
	}
}

// TestAppendModelLoadArgs_NoDegrade 验证无降级时 gpu-layers 保持原值
func TestAppendModelLoadArgs_NoDegrade(t *testing.T) {
	s := newTestServer()
	s.config.GPULayers = "99"

	args := s.appendModelLoadArgs(nil)
	if got := argValue(args, "--gpu-layers"); got != "99" {
		t.Errorf("期望无降级时 --gpu-layers=99，实际: %q", got)
	}
}

// TestAppendModelLoadArgs_InvalidCacheType 验证无效 cache-type 被跳过
// 风险：传递已废弃的 cache-type（如 q4_k）会导致 llama-server 启动失败。
func TestAppendModelLoadArgs_InvalidCacheType(t *testing.T) {
	s := newTestServer()
	s.config.CacheTypeK = "q4_k" // 已废弃的类型
	s.config.CacheTypeV = "f16"  // 合法类型

	args := s.appendModelLoadArgs(nil)
	if containsArg(args, "--cache-type-k") {
		t.Errorf("期望无效 cache-type-k 被跳过，实际 args: %v", args)
	}
	if got := argValue(args, "--cache-type-v"); got != "f16" {
		t.Errorf("期望合法 --cache-type-v=f16，实际: %q", got)
	}
}

// TestAppendModelLoadArgs_ModelsPreset 验证 ModelsPreset 非空时传递 --no-models-autoload
// 风险：未禁用自动加载会导致 llama-server 自动加载模型，与豆芽显式加载逻辑冲突。
func TestAppendModelLoadArgs_ModelsPreset(t *testing.T) {
	s := newTestServer()
	s.config.ModelsPreset = "my-preset"

	args := s.appendModelLoadArgs(nil)
	if !containsArg(args, "--no-models-autoload") {
		t.Errorf("期望包含 --no-models-autoload，实际 args: %v", args)
	}
	if got := argValue(args, "--models-preset"); got != "my-preset" {
		t.Errorf("期望 --models-preset=my-preset，实际: %q", got)
	}
}

// ===== appendRuntimeArgs 测试 =====

// TestAppendRuntimeArgs_CrashDegradeLevel1 验证崩溃降级级别 1 时 ctx-size 被减半
// 风险：连续崩溃后未减小 ctx-size，会继续因 OOM 崩溃。
func TestAppendRuntimeArgs_CrashDegradeLevel1(t *testing.T) {
	s := newTestServer()
	s.config.ContextSize = 8192
	s.crashDegradeLevel.Store(1)

	args := s.appendRuntimeArgs(nil)
	if got := argValue(args, "-c"); got != "4096" {
		t.Errorf("期望降级级别 1 时 -c=4096（8192/2），实际: %q", got)
	}
}

// TestAppendRuntimeArgs_CrashDegradeMinFloor 验证 ctx-size 降级不低于 2048
func TestAppendRuntimeArgs_CrashDegradeMinFloor(t *testing.T) {
	s := newTestServer()
	s.config.ContextSize = 3000 // 减半后 1500 < 2048，应被抬高到 2048
	s.crashDegradeLevel.Store(1)

	args := s.appendRuntimeArgs(nil)
	if got := argValue(args, "-c"); got != "2048" {
		t.Errorf("期望降级后不低于 2048，实际: %q", got)
	}
}

// TestAppendRuntimeArgs_NoDegrade 验证无降级时 ctx-size 保持原值
func TestAppendRuntimeArgs_NoDegrade(t *testing.T) {
	s := newTestServer()
	s.config.ContextSize = 8192

	args := s.appendRuntimeArgs(nil)
	if got := argValue(args, "-c"); got != "8192" {
		t.Errorf("期望无降级时 -c=8192，实际: %q", got)
	}
}

// TestAppendRuntimeArgs_Mlock 验证 Mlock=true 时通过 --load-mode mlock 生效
func TestAppendRuntimeArgs_Mlock(t *testing.T) {
	s := newTestServer()
	s.config.Mlock = true

	args := s.appendRuntimeArgs(nil)
	if got := argValue(args, "--load-mode"); got != "mlock" {
		t.Errorf("期望 --load-mode mlock，实际 %q，args: %v", got, args)
	}
	if containsArg(args, "--mlock") {
		t.Errorf("废弃参数 --mlock 不应再传递（已迁移到 --load-mode），实际 args: %v", args)
	}
}

// TestAppendRuntimeArgs_MmprojOffload 验证 MmprojOffload=true 时传递 --mmproj-offload
func TestAppendRuntimeArgs_MmprojOffload(t *testing.T) {
	s := newTestServer()
	s.config.MmprojOffload = true

	args := s.appendRuntimeArgs(nil)
	if !containsArg(args, "--mmproj-offload") {
		t.Errorf("期望包含 --mmproj-offload，实际 args: %v", args)
	}
}

// ===== appendReasoningArgs 测试 =====

// TestAppendReasoningArgs_BackendSamplingMutex 验证后端采样启用时跳过 reasoning-budget
// 风险：后端采样与 reasoning-budget 互斥，同时传递会导致 llama-server 行为异常。
func TestAppendReasoningArgs_BackendSamplingMutex(t *testing.T) {
	s := newTestServer()
	s.config.BackendSampling = true
	s.config.ReasoningBudget = 1000

	args := s.appendReasoningArgs(nil)
	if containsArg(args, "--reasoning-budget") {
		t.Errorf("期望后端采样启用时跳过 --reasoning-budget，实际 args: %v", args)
	}
}

// TestAppendReasoningArgs_BudgetPassed 验证非后端采样时传递 reasoning-budget
func TestAppendReasoningArgs_BudgetPassed(t *testing.T) {
	s := newTestServer()
	s.config.BackendSampling = false
	s.config.ReasoningBudget = 1000

	args := s.appendReasoningArgs(nil)
	if got := argValue(args, "--reasoning-budget"); got != "1000" {
		t.Errorf("期望 --reasoning-budget=1000，实际: %q", got)
	}
}

// TestAppendReasoningArgs_PreserveTrue 验证 ReasoningPreserve=true 传递 --reasoning-preserve
func TestAppendReasoningArgs_PreserveTrue(t *testing.T) {
	s := newTestServer()
	preserveTrue := true
	s.config.ReasoningPreserve = &preserveTrue

	args := s.appendReasoningArgs(nil)
	if !containsArg(args, "--reasoning-preserve") {
		t.Errorf("期望包含 --reasoning-preserve，实际 args: %v", args)
	}
}

// TestAppendReasoningArgs_PreserveFalse 验证 ReasoningPreserve=false 传递 --no-reasoning-preserve
func TestAppendReasoningArgs_PreserveFalse(t *testing.T) {
	s := newTestServer()
	falseVal := false
	s.config.ReasoningPreserve = &falseVal

	args := s.appendReasoningArgs(nil)
	if !containsArg(args, "--no-reasoning-preserve") {
		t.Errorf("期望包含 --no-reasoning-preserve，实际 args: %v", args)
	}
}

// TestAppendReasoningArgs_PreserveNil 验证 ReasoningPreserve=nil 不传递相关参数
func TestAppendReasoningArgs_PreserveNil(t *testing.T) {
	s := newTestServer()
	s.config.ReasoningPreserve = nil

	args := s.appendReasoningArgs(nil)
	if containsArg(args, "--reasoning-preserve") || containsArg(args, "--no-reasoning-preserve") {
		t.Errorf("期望 nil 时不传递 preserve 参数，实际 args: %v", args)
	}
}

// ===== appendKVCacheArgs 测试 =====

// TestAppendKVCacheArgs_ContextShiftWithKeep 验证 context-shift 启用且 KeepSize>0 时传递 --keep
// 风险：启用滑窗但未保护 system prompt，会导致豆芽身份/规则被从上下文前面丢弃。
func TestAppendKVCacheArgs_ContextShiftWithKeep(t *testing.T) {
	s := newTestServer()
	s.config.ContextShift = true
	s.config.KeepSize = 512

	args := s.appendKVCacheArgs(nil)
	if got := argValue(args, "--keep"); got != "512" {
		t.Errorf("期望 --keep=512 保护 system prompt，实际: %q", got)
	}
}

// TestAppendKVCacheArgs_ContextShiftNoKeep 验证 context-shift 启用但 KeepSize=0 时不传递 --keep
func TestAppendKVCacheArgs_ContextShiftNoKeep(t *testing.T) {
	s := newTestServer()
	s.config.ContextShift = true
	s.config.KeepSize = 0

	args := s.appendKVCacheArgs(nil)
	if containsArg(args, "--keep") {
		t.Errorf("期望 KeepSize=0 时不传递 --keep，实际 args: %v", args)
	}
}

// TestAppendKVCacheArgs_NoContextShift 验证 context-shift 关闭时不传递 --keep
func TestAppendKVCacheArgs_NoContextShift(t *testing.T) {
	s := newTestServer()
	s.config.ContextShift = false
	s.config.KeepSize = 512

	args := s.appendKVCacheArgs(nil)
	if containsArg(args, "--keep") {
		t.Errorf("期望 context-shift 关闭时不传递 --keep，实际 args: %v", args)
	}
}

// TestAppendRuntimeArgs_NoMmap 验证 Mmap=false 时传 --load-mode none（迁移自 --no-mmap）
// 风险：Mmap 关闭时未传递加载模式，会导致模型权重被映射到内存，浪费 RAM。
func TestAppendRuntimeArgs_NoMmap(t *testing.T) {
	s := newTestServer()
	s.config.Mmap = false

	args := s.appendRuntimeArgs(nil)
	if got := argValue(args, "--load-mode"); got != "none" {
		t.Errorf("期望 --load-mode none，实际 %q，args: %v", got, args)
	}
	if containsArg(args, "--no-mmap") {
		t.Errorf("废弃参数 --no-mmap 不应再传递（已迁移到 --load-mode），实际 args: %v", args)
	}
}

// TestAppendKVArgs_MmapEnabled 验证 Mmap=true（模式 mmap 为默认）时不在 KVCache 传 --no-mmap
func TestAppendKVCacheArgs_MmapEnabled(t *testing.T) {
	s := newTestServer()
	s.config.Mmap = true

	args := s.appendKVCacheArgs(nil)
	if containsArg(args, "--no-mmap") {
		t.Errorf("期望 Mmap=true 时不包含 --no-mmap，实际 args: %v", args)
	}
}

// TestAppendKVCacheArgs_NoKVOffload 验证 KVOffload=false 时传递 --no-kv-offload
// 风险：KVOffload 关闭时未传递 --no-kv-offload，会导致 KV 缓存被卸载到 GPU，与用户预期不符。
func TestAppendKVCacheArgs_NoKVOffload(t *testing.T) {
	s := newTestServer()
	s.config.KVOffload = false

	args := s.appendKVCacheArgs(nil)
	if !containsArg(args, "--no-kv-offload") {
		t.Errorf("期望包含 --no-kv-offload，实际 args: %v", args)
	}
}

// TestAppendKVCacheArgs_KVOffloadEnabled 验证 KVOffload=true 时不传递 --no-kv-offload
func TestAppendKVCacheArgs_KVOffloadEnabled(t *testing.T) {
	s := newTestServer()
	s.config.KVOffload = true

	args := s.appendKVCacheArgs(nil)
	if containsArg(args, "--no-kv-offload") {
		t.Errorf("期望 KVOffload=true 时不包含 --no-kv-offload，实际 args: %v", args)
	}
}

// ===== appendValidatedCacheType 测试 =====

// TestAppendValidatedCacheType_Empty 验证空值不传递参数
func TestAppendValidatedCacheType_Empty(t *testing.T) {
	s := newTestServer()
	args := s.appendValidatedCacheType(nil, "--cache-type-k", "")
	if len(args) != 0 {
		t.Errorf("期望空值不传递参数，实际 args: %v", args)
	}
}

// TestAppendValidatedCacheType_Valid 验证合法类型正确传递
func TestAppendValidatedCacheType_Valid(t *testing.T) {
	s := newTestServer()
	args := s.appendValidatedCacheType(nil, "--cache-type-k", "f16")
	if got := argValue(args, "--cache-type-k"); got != "f16" {
		t.Errorf("期望 --cache-type-k=f16，实际: %q", got)
	}
}

// TestAppendValidatedCacheType_Invalid 验证无效类型被跳过
// 风险：传递已废弃的类型会导致 llama-server 启动失败。
func TestAppendValidatedCacheType_Invalid(t *testing.T) {
	s := newTestServer()
	args := s.appendValidatedCacheType(nil, "--cache-type-k", "q4_k")
	if containsArg(args, "--cache-type-k") {
		t.Errorf("期望无效类型被跳过，实际 args: %v", args)
	}
}

// ===== appendSpeculativeArgs 测试 =====

// TestAppendSpeculativeArgs_SpecDefaultSkipsAll 验证 SpecDefault=true 时跳过所有推测参数
// 风险：SpecDefault 与其他推测参数互斥，同时传递会导致配置冲突。
func TestAppendSpeculativeArgs_SpecDefaultSkipsAll(t *testing.T) {
	s := newTestServer()
	s.config.SpecDefault = true
	s.config.SpecType = "ngram-mod" // 设置了具体类型，但应被跳过

	args := s.appendSpeculativeArgs(nil)
	if containsArg(args, "--spec-type") {
		t.Errorf("期望 SpecDefault=true 时跳过 --spec-type，实际 args: %v", args)
	}
}

// TestAppendSpeculativeArgs_SpecTypePassed 验证 SpecDefault=false 时传递 spec-type
func TestAppendSpeculativeArgs_SpecTypePassed(t *testing.T) {
	s := newTestServer()
	s.config.SpecDefault = false
	s.config.SpecType = "ngram-mod"

	args := s.appendSpeculativeArgs(nil)
	if got := argValue(args, "--spec-type"); got != "ngram-mod" {
		t.Errorf("期望 --spec-type=ngram-mod，实际: %q", got)
	}
}

// TestAppendSpeculativeArgs_MtpDisabled 验证 mtpFallbackDisabled=true 时跳过 spec-type
// 风险：MTP 不支持时仍传递 spec-type，会导致 llama-server 启动失败。
func TestAppendSpeculativeArgs_MtpDisabled(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-mod"
	s.mtpFallbackDisabled.Store(true)

	args := s.appendSpeculativeArgs(nil)
	if containsArg(args, "--spec-type") {
		t.Errorf("期望 mtpFallbackDisabled=true 时跳过 --spec-type，实际 args: %v", args)
	}
}

// ===== appendNewFeatureArgs 测试 =====

// TestAppendNewFeatureArgs_MCPConfigExist 验证 MCP 配置文件存在时传递 --mcp-servers-config
func TestAppendNewFeatureArgs_MCPConfigExist(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	// 创建 mcp_servers.json 文件
	mcpPath := filepath.Join(tmpDir, "mcp_servers.json")
	if err := os.WriteFile(mcpPath, []byte(`{"servers":{}}`), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	args := s.appendNewFeatureArgs(nil)
	if got := argValue(args, "--mcp-servers-config"); got != mcpPath {
		t.Errorf("期望 --mcp-servers-config=%q，实际: %q", mcpPath, got)
	}
}

// TestAppendNewFeatureArgs_MCPConfigNotExist 验证 MCP 配置文件不存在时跳过参数
// 风险：指向不存在的文件会导致 llama-server 启动失败。
func TestAppendNewFeatureArgs_MCPConfigNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	// 不创建 mcp_servers.json 文件

	args := s.appendNewFeatureArgs(nil)
	if containsArg(args, "--mcp-servers-config") {
		t.Errorf("期望文件不存在时跳过 --mcp-servers-config，实际 args: %v", args)
	}
}

// TestAppendNewFeatureArgs_AgentMode 验证 Agent=true 时传递 --agent
func TestAppendNewFeatureArgs_AgentMode(t *testing.T) {
	s := newTestServer()
	s.config.Agent = true

	args := s.appendNewFeatureArgs(nil)
	if !containsArg(args, "--agent") {
		t.Errorf("期望包含 --agent，实际 args: %v", args)
	}
}

// TestAppendNewFeatureArgs_UIMcpProxy 验证 Agent=false 且 UIMcpProxy=true 时传递 --ui-mcp-proxy
func TestAppendNewFeatureArgs_UIMcpProxy(t *testing.T) {
	s := newTestServer()
	s.config.Agent = false
	s.config.UIMcpProxy = true

	args := s.appendNewFeatureArgs(nil)
	if !containsArg(args, "--ui-mcp-proxy") {
		t.Errorf("期望包含 --ui-mcp-proxy，实际 args: %v", args)
	}
}

// TestAppendNewFeatureArgs_AgentOverridesUIMcpProxy 验证 Agent 和 UIMcpProxy 同时启用时优先 Agent
func TestAppendNewFeatureArgs_AgentOverridesUIMcpProxy(t *testing.T) {
	s := newTestServer()
	s.config.Agent = true
	s.config.UIMcpProxy = true

	args := s.appendNewFeatureArgs(nil)
	if !containsArg(args, "--agent") {
		t.Errorf("期望包含 --agent（优先），实际 args: %v", args)
	}
	if containsArg(args, "--ui-mcp-proxy") {
		t.Errorf("期望不包含 --ui-mcp-proxy（被 Agent 覆盖），实际 args: %v", args)
	}
}

// TestAppendNewFeatureArgs_CORS 验证细粒度 CORS 配置参数透传（仅显式配置时传递）
func TestAppendNewFeatureArgs_CORS(t *testing.T) {
	s := newTestServer()
	s.config.CorsOrigins = "http://localhost:5173,http://192.168.1.5:3000"
	s.config.CorsMethods = "GET,POST"
	s.config.CorsHeaders = "Content-Type,X-Tool-Cwd"
	s.config.CorsCredentials = true

	args := s.appendNewFeatureArgs(nil)
	if got := argValue(args, "--cors-origins"); got != "http://localhost:5173,http://192.168.1.5:3000" {
		t.Errorf("期望 --cors-origins=%q，实际 %q", "http://localhost:5173,http://192.168.1.5:3000", got)
	}
	if got := argValue(args, "--cors-methods"); got != "GET,POST" {
		t.Errorf("期望 --cors-methods=GET,POST，实际 %q", got)
	}
	if got := argValue(args, "--cors-headers"); got != "Content-Type,X-Tool-Cwd" {
		t.Errorf("期望 --cors-headers=%q，实际 %q", "Content-Type,X-Tool-Cwd", got)
	}
	if !containsArg(args, "--cors-credentials") {
		t.Errorf("期望包含 --cors-credentials，实际 args: %v", args)
	}
}

// TestAppendNewFeatureArgs_CORS_Empty 验证 CORS 为空时不传递任何 --cors-* 参数（用 llama.cpp 默认）
func TestAppendNewFeatureArgs_CORS_Empty(t *testing.T) {
	s := newTestServer()

	args := s.appendNewFeatureArgs(nil)
	for _, f := range []string{"--cors-origins", "--cors-methods", "--cors-headers", "--cors-credentials"} {
		if containsArg(args, f) {
			t.Errorf("期望空配置时不传递 %s，实际 args: %v", f, args)
		}
	}
}

// TestAppendNewFeatureArgs_NoPrefillAssistant 验证 PrefillAssistant=false 时传递 --no-prefill-assistant
func TestAppendNewFeatureArgs_NoPrefillAssistant(t *testing.T) {
	s := newTestServer()
	s.config.PrefillAssistant = false

	args := s.appendNewFeatureArgs(nil)
	if !containsArg(args, "--no-prefill-assistant") {
		t.Errorf("期望包含 --no-prefill-assistant，实际 args: %v", args)
	}
}

// ===== appendCPUMoeArgs 测试 =====

// TestAppendCPUMoeArgs_OpOffloadTrue 验证 OpOffload=true 传递 --op-offload
func TestAppendCPUMoeArgs_OpOffloadTrue(t *testing.T) {
	s := newTestServer()
	trueVal := true
	s.config.OpOffload = &trueVal

	args := s.appendCPUMoeArgs(nil)
	if !containsArg(args, "--op-offload") {
		t.Errorf("期望包含 --op-offload，实际 args: %v", args)
	}
}

// TestAppendCPUMoeArgs_OpOffloadFalse 验证 OpOffload=false 传递 --no-op-offload
// 风险：开关方向错误会导致算子卸载行为与用户预期相反。
func TestAppendCPUMoeArgs_OpOffloadFalse(t *testing.T) {
	s := newTestServer()
	falseVal := false
	s.config.OpOffload = &falseVal

	args := s.appendCPUMoeArgs(nil)
	if !containsArg(args, "--no-op-offload") {
		t.Errorf("期望包含 --no-op-offload，实际 args: %v", args)
	}
}

// TestAppendCPUMoeArgs_OpOffloadNil 验证 OpOffload=nil 不传递相关参数
func TestAppendCPUMoeArgs_OpOffloadNil(t *testing.T) {
	s := newTestServer()
	s.config.OpOffload = nil

	args := s.appendCPUMoeArgs(nil)
	if containsArg(args, "--op-offload") || containsArg(args, "--no-op-offload") {
		t.Errorf("期望 nil 时不传递 op-offload 参数，实际 args: %v", args)
	}
}

// TestAppendCPUMoeArgs_CPUMoe 验证 CPUMoe=true 传递 --cpu-moe
func TestAppendCPUMoeArgs_CPUMoe(t *testing.T) {
	s := newTestServer()
	s.config.CPUMoe = true

	args := s.appendCPUMoeArgs(nil)
	if !containsArg(args, "--cpu-moe") {
		t.Errorf("期望包含 --cpu-moe，实际 args: %v", args)
	}
}

// TestAppendRuntimeArgs_DirectIO 验证 DirectIO=true 时传 --load-mode dio（迁移自 --direct-io）
func TestAppendRuntimeArgs_DirectIO(t *testing.T) {
	s := newTestServer()
	s.config.DirectIO = true

	args := s.appendRuntimeArgs(nil)
	if got := argValue(args, "--load-mode"); got != "dio" {
		t.Errorf("期望 --load-mode dio，实际 %q，args: %v", got, args)
	}
	if containsArg(args, "--direct-io") {
		t.Errorf("废弃参数 --direct-io 不应再传递（已迁移到 --load-mode），实际 args: %v", args)
	}
}

// TestLoadMode_Precedence 验证下发优先级：DirectIO > Mlock > !Mmap。
func TestLoadMode_Precedence(t *testing.T) {
	tests := []struct {
		name     string
		directIO bool
		mlock    bool
		mmap     bool
		want     string
	}{
		{"默认mmap", false, false, true, "mmap"},
		{"仅Mmap开", false, false, true, "mmap"},
		{"关闭Mmap", false, false, false, "none"},
		{"仅Mlock", false, true, true, "mlock"},
		{"Mlock且关Mmap_优先级Mlock", false, true, false, "mlock"},
		{"仅DirectIO", true, false, true, "dio"},
		{"DirectIO且Mlock_优先级DirectIO", true, true, true, "dio"},
		{"全开_优先级DirectIO", true, true, false, "dio"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sc := &ServerConfig{DirectIO: tc.directIO, Mlock: tc.mlock, Mmap: tc.mmap}
			if got := sc.LoadMode(); got != tc.want {
				t.Errorf("LoadMode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAppendRuntimeArgs_DefaultMmap 验证 默认负载模式 mmap 不额外传 --load-mode（上游默认值）
func TestAppendRuntimeArgs_DefaultMmap(t *testing.T) {
	s := newTestServer()
	s.config.Mmap = true

	args := s.appendRuntimeArgs(nil)
	if got := argValue(args, "--load-mode"); got != "" {
		t.Errorf("期望默认 mmap 时省略 --load-mode（换取上游默认），实际 %q，args: %v", got, args)
	}
}

// ===== appendLoraArgs 测试 =====

// TestAppendLoraArgs_SinglePath 验证单个 LoRA 路径正确传递
func TestAppendLoraArgs_SinglePath(t *testing.T) {
	s := newTestServer()
	s.config.LoraPaths = "/path/to/lora.gguf"

	args := s.appendLoraArgs(nil)
	if !containsArg(args, "--lora") {
		t.Errorf("期望包含 --lora，实际 args: %v", args)
	}
	if got := argValue(args, "--lora"); got != "/path/to/lora.gguf" {
		t.Errorf("期望 --lora=/path/to/lora.gguf，实际: %q", got)
	}
	// 启动时加载但不应用（scale=0）
	if !containsArg(args, "--lora-init-without-apply") {
		t.Errorf("期望包含 --lora-init-without-apply，实际 args: %v", args)
	}
}

// TestAppendLoraArgs_MultiplePaths 验证多个 LoRA 路径分别传递 --lora
func TestAppendLoraArgs_MultiplePaths(t *testing.T) {
	s := newTestServer()
	s.config.LoraPaths = "/path/a.gguf, /path/b.gguf"

	args := s.appendLoraArgs(nil)
	// 统计 --lora 出现次数
	count := 0
	for _, a := range args {
		if a == "--lora" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("期望 2 个 --lora，实际 %d 个，args: %v", count, args)
	}
}

// TestAppendLoraArgs_EmptyPath 验证空路径不传递 --lora
func TestAppendLoraArgs_EmptyPath(t *testing.T) {
	s := newTestServer()
	s.config.LoraPaths = ""

	args := s.appendLoraArgs(nil)
	if containsArg(args, "--lora") {
		t.Errorf("期望空路径不传递 --lora，实际 args: %v", args)
	}
}

// TestAppendLoraArgs_SlotSaveDefaultPath 验证 SlotSaveEnabled=true 且路径为空时使用默认路径
func TestAppendLoraArgs_SlotSaveDefaultPath(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	s.config.SlotSaveEnabled = true
	s.config.SlotSavePath = ""

	args := s.appendLoraArgs(nil)
	expectedPath := filepath.Join(tmpDir, "slots")
	if got := argValue(args, "--slot-save-path"); got != expectedPath {
		t.Errorf("期望 --slot-save-path=%q，实际: %q", expectedPath, got)
	}
}

// ===== appendSpecNgramArgs 测试 =====

// TestAppendSpecNgramArgs_NgramMod 验证 ngram-mod 模式正确传递参数
func TestAppendSpecNgramArgs_NgramMod(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-mod"
	s.config.SpecNgramModNMin = 2
	s.config.SpecNgramModNMax = 4
	s.config.SpecNgramModNMatch = 3

	args := s.appendSpecNgramArgs(nil)
	if got := argValue(args, "--spec-ngram-mod-n-min"); got != "2" {
		t.Errorf("期望 --spec-ngram-mod-n-min=2，实际: %q", got)
	}
	if got := argValue(args, "--spec-ngram-mod-n-max"); got != "4" {
		t.Errorf("期望 --spec-ngram-mod-n-max=4，实际: %q", got)
	}
	if got := argValue(args, "--spec-ngram-mod-n-match"); got != "3" {
		t.Errorf("期望 --spec-ngram-mod-n-match=3，实际: %q", got)
	}
}

// TestAppendSpecNgramArgs_NgramSimple 验证 ngram-simple 模式正确传递参数
func TestAppendSpecNgramArgs_NgramSimple(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-simple"
	s.config.SpecNgramSimpleSizeN = 3
	s.config.SpecNgramSimpleSizeM = 2
	s.config.SpecNgramSimpleMinHits = 1

	args := s.appendSpecNgramArgs(nil)
	if got := argValue(args, "--spec-ngram-simple-size-n"); got != "3" {
		t.Errorf("期望 --spec-ngram-simple-size-n=3，实际: %q", got)
	}
}

// TestAppendSpecNgramArgs_UnrelatedSpecType 验证不相关的 spec-type 不传递 ngram 参数
func TestAppendSpecNgramArgs_UnrelatedSpecType(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "draft-eagle3"
	s.config.SpecNgramModNMin = 2 // 设置了 ngram 参数，但 spec-type 不匹配

	args := s.appendSpecNgramArgs(nil)
	if containsArg(args, "--spec-ngram-mod-n-min") {
		t.Errorf("期望不相关 spec-type 不传递 ngram 参数，实际 args: %v", args)
	}
}

// ===== appendDraftGpuArgs 测试 =====

// TestAppendDraftGpuArgs_MtpDisabled 验证 mtpFallbackDisabled=true 时跳过所有参数
func TestAppendDraftGpuArgs_MtpDisabled(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftNgl = 10
	s.mtpFallbackDisabled.Store(true)

	args := s.appendDraftGpuArgs(nil)
	if containsArg(args, "--spec-draft-ngl") {
		t.Errorf("期望 mtpFallbackDisabled=true 时跳过 --spec-draft-ngl，实际 args: %v", args)
	}
}

// TestAppendDraftGpuArgs_SpecDraftNgl 验证 SpecDraftNgl 正确传递
func TestAppendDraftGpuArgs_SpecDraftNgl(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftNgl = 10

	args := s.appendDraftGpuArgs(nil)
	if got := argValue(args, "--spec-draft-ngl"); got != "10" {
		t.Errorf("期望 --spec-draft-ngl=10，实际: %q", got)
	}
}

// ===== appendMediaOfflineArgs 测试 =====

// TestAppendMediaOfflineArgs_MediaPathExist 验证媒体路径存在时传递 --media-path
func TestAppendMediaOfflineArgs_MediaPathExist(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	// 创建媒体目录
	mediaDir := filepath.Join(tmpDir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatalf("创建测试目录失败: %v", err)
	}
	s.config.MediaPath = "media"

	args := s.appendMediaOfflineArgs(nil)
	if got := argValue(args, "--media-path"); got != mediaDir {
		t.Errorf("期望 --media-path=%q，实际: %q", mediaDir, got)
	}
}

// TestAppendMediaOfflineArgs_MediaPathNotExist 验证媒体路径不存在时跳过参数
// 风险：指向不存在的目录会导致 llama-server 启动失败。
func TestAppendMediaOfflineArgs_MediaPathNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	s.config.MediaPath = "nonexistent-dir"

	args := s.appendMediaOfflineArgs(nil)
	if containsArg(args, "--media-path") {
		t.Errorf("期望目录不存在时跳过 --media-path，实际 args: %v", args)
	}
}

// TestAppendMediaOfflineArgs_MediaPathIsFile 验证媒体路径是文件而非目录时跳过参数
func TestAppendMediaOfflineArgs_MediaPathIsFile(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	// 创建一个文件而非目录
	filePath := filepath.Join(tmpDir, "mediafile")
	if err := os.WriteFile(filePath, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	s.config.MediaPath = "mediafile"

	args := s.appendMediaOfflineArgs(nil)
	if containsArg(args, "--media-path") {
		t.Errorf("期望路径是文件时跳过 --media-path，实际 args: %v", args)
	}
}

// ===== buildStartArgs 集成测试 =====

// TestBuildStartArgs_FullChain 验证完整参数链不丢失关键基础参数
// 风险：参数链拆分后某个子函数被遗漏，导致 llama-server 缺少必传参数。
func TestBuildStartArgs_FullChain(t *testing.T) {
	s := newTestServer()
	s.config.Port = 9999
	s.config.ModelsDir = "/test/models"

	args := s.buildStartArgs()

	// 验证基础参数存在
	if got := argValue(args, "--port"); got != "9999" {
		t.Errorf("期望 --port=9999，实际: %q", got)
	}
	if got := argValue(args, "--models-dir"); got != "/test/models" {
		t.Errorf("期望 --models-dir=/test/models，实际: %q", got)
	}
	if !containsArg(args, "--fit") {
		t.Errorf("期望包含 --fit，实际 args: %v", args)
	}
}

// ===== appendSamplingArgs DRY 采样器嵌套分支测试 =====
// DRY 采样器参数存在层层嵌套的条件分支：
//   DryMultiplier > 0 才进入外层
//     ├─ DryBase > 0 才传 --dry-base
//     ├─ DryAllowedLength > 0 才传 --dry-allowed-length
//     ├─ DrySequenceBreaker != "" 才传 --dry-sequence-breaker（支持逗号分隔多个）
//     └─ DryPenaltyLastN > 0 才传 --dry-penalty-last-n
// 风险：嵌套分支中某个条件写错（如把 > 0 写成 != 0），会导致参数误传或漏传。

// TestAppendSamplingArgs_MinP 验证 min-p 正值传递
func TestAppendSamplingArgs_MinP(t *testing.T) {
	s := newTestServer()
	s.config.MinP = 0.05

	args := s.appendSamplingArgs(nil)
	if got := argValue(args, "--min-p"); got != "0.05" {
		t.Errorf("期望 --min-p=0.05，实际: %q", got)
	}
}

// TestAppendSamplingArgs_MinPZero 验证 min-p=0 时不传递（appendFloatArg 的 val > 0 条件）
func TestAppendSamplingArgs_MinPZero(t *testing.T) {
	s := newTestServer()
	s.config.MinP = 0

	args := s.appendSamplingArgs(nil)
	if containsArg(args, "--min-p") {
		t.Errorf("期望 min-p=0 时不传递，实际 args: %v", args)
	}
}

// TestAppendSamplingArgs_DryFullChain 验证 DRY 全参数链正确传递
func TestAppendSamplingArgs_DryFullChain(t *testing.T) {
	s := newTestServer()
	s.config.DryMultiplier = 1.5
	s.config.DryBase = 2.0
	s.config.DryAllowedLength = 3
	s.config.DrySequenceBreaker = "x,y"
	s.config.DryPenaltyLastN = 100

	args := s.appendSamplingArgs(nil)
	if got := argValue(args, "--dry-multiplier"); got != "1.50" {
		t.Errorf("期望 --dry-multiplier=1.50，实际: %q", got)
	}
	if got := argValue(args, "--dry-base"); got != "2.00" {
		t.Errorf("期望 --dry-base=2.00，实际: %q", got)
	}
	if got := argValue(args, "--dry-allowed-length"); got != "3" {
		t.Errorf("期望 --dry-allowed-length=3，实际: %q", got)
	}
	if got := argValue(args, "--dry-penalty-last-n"); got != "100" {
		t.Errorf("期望 --dry-penalty-last-n=100，实际: %q", got)
	}
	// 验证 sequence-breaker 出现两次（x 和 y）
	count := 0
	for _, a := range args {
		if a == "--dry-sequence-breaker" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("期望 2 个 --dry-sequence-breaker，实际 %d 个，args: %v", count, args)
	}
}

// TestAppendSamplingArgs_DryMultiplierOnly 验证仅 DryMultiplier > 0 时只传 --dry-multiplier
// 风险：嵌套条件错误导致其他 DRY 参数被误传。
func TestAppendSamplingArgs_DryMultiplierOnly(t *testing.T) {
	s := newTestServer()
	s.config.DryMultiplier = 1.0
	// 其他 DRY 参数保持默认 0/""

	args := s.appendSamplingArgs(nil)
	if got := argValue(args, "--dry-multiplier"); got != "1.00" {
		t.Errorf("期望 --dry-multiplier=1.00，实际: %q", got)
	}
	// 其他 DRY 参数不应传递
	for _, flag := range []string{"--dry-base", "--dry-allowed-length", "--dry-sequence-breaker", "--dry-penalty-last-n"} {
		if containsArg(args, flag) {
			t.Errorf("期望仅 DryMultiplier 时不传递 %s，实际 args: %v", flag, args)
		}
	}
}

// TestAppendSamplingArgs_DryMultiplierZero 验证 DryMultiplier=0 时跳过所有 DRY 参数
// 风险：外层条件失效会导致 DRY 参数被无条件传递，干扰采样行为。
func TestAppendSamplingArgs_DryMultiplierZero(t *testing.T) {
	s := newTestServer()
	s.config.DryMultiplier = 0
	// 即使其他参数有值，也不应传递
	s.config.DryBase = 2.0
	s.config.DryAllowedLength = 3
	s.config.DryPenaltyLastN = 100

	args := s.appendSamplingArgs(nil)
	for _, flag := range []string{"--dry-multiplier", "--dry-base", "--dry-allowed-length", "--dry-sequence-breaker", "--dry-penalty-last-n"} {
		if containsArg(args, flag) {
			t.Errorf("期望 DryMultiplier=0 时不传递 %s，实际 args: %v", flag, args)
		}
	}
}

// TestAppendSamplingArgs_DrySequenceBreakerWithSpaces 验证带空格的 breaker 被正确 trim
// 风险：未 trim 空格会导致空字符串被当作 breaker 传递，llama-server 可能报错。
func TestAppendSamplingArgs_DrySequenceBreakerWithSpaces(t *testing.T) {
	s := newTestServer()
	s.config.DryMultiplier = 1.0
	// 包含空格和空段（逗号前后有空格，末尾有逗号产生空段）
	s.config.DrySequenceBreaker = " x , y , "

	args := s.appendSamplingArgs(nil)
	count := 0
	for _, a := range args {
		if a == "--dry-sequence-breaker" {
			count++
		}
	}
	// 应只传递非空段：x 和 y
	if count != 2 {
		t.Errorf("期望 trim 后 2 个非空 breaker，实际 %d 个，args: %v", count, args)
	}
}

// TestAppendSamplingArgs_GrpAttn 验证分组注意力参数正确传递
func TestAppendSamplingArgs_GrpAttn(t *testing.T) {
	s := newTestServer()
	s.config.GrpAttnN = 4
	s.config.GrpAttnW = 256

	args := s.appendSamplingArgs(nil)
	if got := argValue(args, "--grp-attn-n"); got != "4" {
		t.Errorf("期望 --grp-attn-n=4，实际: %q", got)
	}
	if got := argValue(args, "--grp-attn-w"); got != "256" {
		t.Errorf("期望 --grp-attn-w=256，实际: %q", got)
	}
}

// ===== appendValidatedCacheTypeIf 测试 =====
// 此函数用于推测解码的 draft cache-type，比 appendValidatedCacheType 多一个 skip 条件。
// 风险：skip 条件判断错误会导致 mtpFallbackDisabled 时仍传递参数，引发启动失败。

// TestAppendValidatedCacheTypeIf_SkipTrue 验证 skip=true 时跳过参数
func TestAppendValidatedCacheTypeIf_SkipTrue(t *testing.T) {
	s := newTestServer()
	args := s.appendValidatedCacheTypeIf(nil, "--spec-draft-type-k", "f16", true)
	if containsArg(args, "--spec-draft-type-k") {
		t.Errorf("期望 skip=true 时跳过参数，实际 args: %v", args)
	}
}

// TestAppendValidatedCacheTypeIf_EmptyType 验证空类型不传递
func TestAppendValidatedCacheTypeIf_EmptyType(t *testing.T) {
	s := newTestServer()
	args := s.appendValidatedCacheTypeIf(nil, "--spec-draft-type-k", "", false)
	if containsArg(args, "--spec-draft-type-k") {
		t.Errorf("期望空类型不传递，实际 args: %v", args)
	}
}

// TestAppendValidatedCacheTypeIf_Valid 验证合法类型正确传递
func TestAppendValidatedCacheTypeIf_Valid(t *testing.T) {
	s := newTestServer()
	args := s.appendValidatedCacheTypeIf(nil, "--spec-draft-type-k", "q8_0", false)
	if got := argValue(args, "--spec-draft-type-k"); got != "q8_0" {
		t.Errorf("期望 --spec-draft-type-k=q8_0，实际: %q", got)
	}
}

// TestAppendValidatedCacheTypeIf_Invalid 验证无效类型被跳过
func TestAppendValidatedCacheTypeIf_Invalid(t *testing.T) {
	s := newTestServer()
	args := s.appendValidatedCacheTypeIf(nil, "--spec-draft-type-k", "q4_k", false)
	if containsArg(args, "--spec-draft-type-k") {
		t.Errorf("期望无效类型被跳过，实际 args: %v", args)
	}
}

// ===== appendSpecNgramArgs 补充分支测试 =====
// 覆盖 ngram-map-k / ngram-map-k4v / ngram-cache 三种模式及零值跳过分支。

// TestAppendSpecNgramArgs_NgramMapK 验证 ngram-map-k 模式正确传递参数
func TestAppendSpecNgramArgs_NgramMapK(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-map-k"
	s.config.SpecNgramMapKSizeN = 3
	s.config.SpecNgramMapKSizeM = 2
	s.config.SpecNgramMapKMinHits = 1

	args := s.appendSpecNgramArgs(nil)
	if got := argValue(args, "--spec-ngram-map-k-size-n"); got != "3" {
		t.Errorf("期望 --spec-ngram-map-k-size-n=3，实际: %q", got)
	}
	if got := argValue(args, "--spec-ngram-map-k-size-m"); got != "2" {
		t.Errorf("期望 --spec-ngram-map-k-size-m=2，实际: %q", got)
	}
	if got := argValue(args, "--spec-ngram-map-k-min-hits"); got != "1" {
		t.Errorf("期望 --spec-ngram-map-k-min-hits=1，实际: %q", got)
	}
}

// TestAppendSpecNgramArgs_NgramMapK4V 验证 ngram-map-k4v 模式正确传递参数
func TestAppendSpecNgramArgs_NgramMapK4V(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-map-k4v"
	s.config.SpecNgramMapK4VSizeN = 4
	s.config.SpecNgramMapK4VSizeM = 3
	s.config.SpecNgramMapK4VMinHits = 2

	args := s.appendSpecNgramArgs(nil)
	if got := argValue(args, "--spec-ngram-map-k4v-size-n"); got != "4" {
		t.Errorf("期望 --spec-ngram-map-k4v-size-n=4，实际: %q", got)
	}
	if got := argValue(args, "--spec-ngram-map-k4v-size-m"); got != "3" {
		t.Errorf("期望 --spec-ngram-map-k4v-size-m=3，实际: %q", got)
	}
	if got := argValue(args, "--spec-ngram-map-k4v-min-hits"); got != "2" {
		t.Errorf("期望 --spec-ngram-map-k4v-min-hits=2，实际: %q", got)
	}
}

// TestAppendSpecNgramArgs_NgramModZeroValues 验证 ngram-mod 模式下零值不传递
// 风险：> 0 条件失效会导致 0 值被传递，llama-server 可能误解为禁用。
func TestAppendSpecNgramArgs_NgramModZeroValues(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-mod"
	// 所有 ngram 参数保持默认 0

	args := s.appendSpecNgramArgs(nil)
	for _, flag := range []string{"--spec-ngram-mod-n-min", "--spec-ngram-mod-n-max", "--spec-ngram-mod-n-match"} {
		if containsArg(args, flag) {
			t.Errorf("期望零值不传递 %s，实际 args: %v", flag, args)
		}
	}
}

// TestAppendSpecNgramArgs_EmptySpecType 验证空 spec-type 不传递任何 ngram 参数
func TestAppendSpecNgramArgs_EmptySpecType(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = ""
	// 即使设置了参数也不应传递
	s.config.SpecNgramModNMin = 2

	args := s.appendSpecNgramArgs(nil)
	if len(args) != 0 {
		t.Errorf("期望空 spec-type 不传递任何参数，实际 args: %v", args)
	}
}

// ===== appendSpecLookupArgs 测试 =====
// lookup-cache 参数仅在 ngram-cache 模式下传递。
// 风险：模式判断错误会导致其他模式下误传 --lookup-cache-*，引发启动失败。

// TestAppendSpecLookupArgs_NgramCache 验证 ngram-cache 模式正确传递 lookup-cache
func TestAppendSpecLookupArgs_NgramCache(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	s.config.SpecType = "ngram-cache"
	// 创建 lookup-cache-static 文件
	staticFile := filepath.Join(tmpDir, "static.bin")
	if err := os.WriteFile(staticFile, []byte("cache"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	s.config.LookupCacheStatic = "static.bin"
	s.config.LookupCacheDynamic = "dynamic.bin"

	args := s.appendSpecLookupArgs(nil)
	if got := argValue(args, "--lookup-cache-static"); got != staticFile {
		t.Errorf("期望 --lookup-cache-static=%q，实际: %q", staticFile, got)
	}
	// dynamic 不经过 resolvePath，直接传递
	if got := argValue(args, "--lookup-cache-dynamic"); got != "dynamic.bin" {
		t.Errorf("期望 --lookup-cache-dynamic=dynamic.bin，实际: %q", got)
	}
}

// TestAppendSpecLookupArgs_OtherSpecType 验证非 ngram-cache 模式不传递 lookup-cache
func TestAppendSpecLookupArgs_OtherSpecType(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-mod"
	s.config.LookupCacheStatic = "static.bin"
	s.config.LookupCacheDynamic = "dynamic.bin"

	args := s.appendSpecLookupArgs(nil)
	if containsArg(args, "--lookup-cache-static") || containsArg(args, "--lookup-cache-dynamic") {
		t.Errorf("期望非 ngram-cache 模式不传递 lookup-cache，实际 args: %v", args)
	}
}

// TestAppendSpecLookupArgs_EmptyPaths 验证空路径不传递
func TestAppendSpecLookupArgs_EmptyPaths(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-cache"

	args := s.appendSpecLookupArgs(nil)
	if len(args) != 0 {
		t.Errorf("期望空路径不传递任何参数，实际 args: %v", args)
	}
}

// ===== appendSpecDraftModelArgs 测试 =====
// draft 模型路径仅在 draft-eagle3/draft-dflash/draft-simple 三种模式下传递。

// TestAppendSpecDraftModelArgs_Eagle3 验证 draft-eagle3 模式正确传递
func TestAppendSpecDraftModelArgs_Eagle3(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	s.config.SpecType = "draft-eagle3"
	s.config.SpecDraftModel = "draft.gguf"

	args := s.appendSpecDraftModelArgs(nil)
	expected := filepath.Join(tmpDir, "draft.gguf")
	if got := argValue(args, "--spec-draft-model"); got != expected {
		t.Errorf("期望 --spec-draft-model=%q，实际: %q", expected, got)
	}
}

// TestAppendSpecDraftModelArgs_Dflash 验证 draft-dflash 模式正确传递
func TestAppendSpecDraftModelArgs_Dflash(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "draft-dflash"
	s.config.SpecDraftModel = "dflash.gguf"

	args := s.appendSpecDraftModelArgs(nil)
	if !containsArg(args, "--spec-draft-model") {
		t.Errorf("期望 draft-dflash 模式传递 --spec-draft-model，实际 args: %v", args)
	}
}

// TestAppendSpecDraftModelArgs_Simple 验证 draft-simple 模式正确传递
func TestAppendSpecDraftModelArgs_Simple(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "draft-simple"
	s.config.SpecDraftModel = "simple.gguf"

	args := s.appendSpecDraftModelArgs(nil)
	if !containsArg(args, "--spec-draft-model") {
		t.Errorf("期望 draft-simple 模式传递 --spec-draft-model，实际 args: %v", args)
	}
}

// TestAppendSpecDraftModelArgs_OtherSpecType 验证非 draft 模式不传递
// 风险：模式判断错误会导致 ngram 模式下误传 --spec-draft-model。
func TestAppendSpecDraftModelArgs_OtherSpecType(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-mod"
	s.config.SpecDraftModel = "draft.gguf"

	args := s.appendSpecDraftModelArgs(nil)
	if containsArg(args, "--spec-draft-model") {
		t.Errorf("期望非 draft 模式不传递 --spec-draft-model，实际 args: %v", args)
	}
}

// TestAppendSpecDraftModelArgs_EmptyModel 验证空路径不传递
func TestAppendSpecDraftModelArgs_EmptyModel(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "draft-eagle3"
	s.config.SpecDraftModel = ""

	args := s.appendSpecDraftModelArgs(nil)
	if containsArg(args, "--spec-draft-model") {
		t.Errorf("期望空路径不传递，实际 args: %v", args)
	}
}

// ===== appendDraftThreadsArgs 测试 =====
// draft 线程参数受 mtpFallbackDisabled 控制，spec-default 始终传递。

// TestAppendDraftThreadsArgs_Normal 验证正常情况下线程参数正确传递
func TestAppendDraftThreadsArgs_Normal(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftThreads = 4
	s.config.SpecDraftThreadsBatch = 2

	args := s.appendDraftThreadsArgs(nil)
	if got := argValue(args, "--spec-draft-threads"); got != "4" {
		t.Errorf("期望 --spec-draft-threads=4，实际: %q", got)
	}
	if got := argValue(args, "--spec-draft-threads-batch"); got != "2" {
		t.Errorf("期望 --spec-draft-threads-batch=2，实际: %q", got)
	}
}

// TestAppendDraftThreadsArgs_MtpDisabled 验证 mtpFallbackDisabled 时跳过线程参数
func TestAppendDraftThreadsArgs_MtpDisabled(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftThreads = 4
	s.config.SpecDraftThreadsBatch = 2
	s.mtpFallbackDisabled.Store(true)

	args := s.appendDraftThreadsArgs(nil)
	if containsArg(args, "--spec-draft-threads") || containsArg(args, "--spec-draft-threads-batch") {
		t.Errorf("期望 mtpFallbackDisabled 时跳过线程参数，实际 args: %v", args)
	}
}

// TestAppendDraftThreadsArgs_SpecDefault 验证 spec-default 始终传递（不受 mtpDisabled 影响）
func TestAppendDraftThreadsArgs_SpecDefault(t *testing.T) {
	s := newTestServer()
	s.config.SpecDefault = true
	s.mtpFallbackDisabled.Store(true)

	args := s.appendDraftThreadsArgs(nil)
	if !containsArg(args, "--spec-default") {
		t.Errorf("期望 spec-default 不受 mtpDisabled 影响，实际 args: %v", args)
	}
}

// ===== appendDraftGpuArgs 补充分支测试 =====

// TestAppendDraftGpuArgs_SpecDraftDevice 验证 SpecDraftDevice 正确传递
func TestAppendDraftGpuArgs_SpecDraftDevice(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftDevice = "cuda1"

	args := s.appendDraftGpuArgs(nil)
	if got := argValue(args, "--spec-draft-device"); got != "cuda1" {
		t.Errorf("期望 --spec-draft-device=cuda1，实际: %q", got)
	}
}

// TestAppendDraftGpuArgs_SpecDraftPSplit 验证 SpecDraftPSplit 正确传递
func TestAppendDraftGpuArgs_SpecDraftPSplit(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftPSplit = 0.5

	args := s.appendDraftGpuArgs(nil)
	if got := argValue(args, "--spec-draft-p-split"); got != "0.50" {
		t.Errorf("期望 --spec-draft-p-split=0.50，实际: %q", got)
	}
}

// TestAppendDraftGpuArgs_SpecDraftPMin 验证 SpecDraftPMin 正确传递
func TestAppendDraftGpuArgs_SpecDraftPMin(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftPMin = 0.1

	args := s.appendDraftGpuArgs(nil)
	if got := argValue(args, "--spec-draft-p-min"); got != "0.10" {
		t.Errorf("期望 --spec-draft-p-min=0.10，实际: %q", got)
	}
}

// TestAppendDraftGpuArgs_BackendSamplingTrue 验证 SpecDraftBackendSampling=true 传递正向开关
func TestAppendDraftGpuArgs_BackendSamplingTrue(t *testing.T) {
	s := newTestServer()
	trueVal := true
	s.config.SpecDraftBackendSampling = &trueVal

	args := s.appendDraftGpuArgs(nil)
	if !containsArg(args, "--spec-draft-backend-sampling") {
		t.Errorf("期望包含 --spec-draft-backend-sampling，实际 args: %v", args)
	}
}

// TestAppendDraftGpuArgs_BackendSamplingFalse 验证 SpecDraftBackendSampling=false 传递负向开关
// 风险：开关方向错误会导致采样行为与预期相反。
func TestAppendDraftGpuArgs_BackendSamplingFalse(t *testing.T) {
	s := newTestServer()
	falseVal := false
	s.config.SpecDraftBackendSampling = &falseVal

	args := s.appendDraftGpuArgs(nil)
	if !containsArg(args, "--no-spec-draft-backend-sampling") {
		t.Errorf("期望包含 --no-spec-draft-backend-sampling，实际 args: %v", args)
	}
}

// TestAppendDraftGpuArgs_BackendSamplingNil 验证 nil 时不传递
func TestAppendDraftGpuArgs_BackendSamplingNil(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftBackendSampling = nil

	args := s.appendDraftGpuArgs(nil)
	if containsArg(args, "--spec-draft-backend-sampling") || containsArg(args, "--no-spec-draft-backend-sampling") {
		t.Errorf("期望 nil 时不传递，实际 args: %v", args)
	}
}

// ===== appendSwitchArgs 补充分支测试 =====

// TestAppendSwitchArgs_JinjaNil 验证 Jinja=nil 时默认传递 --jinja（兼容旧配置）
// 风险：nil 处理错误会导致旧配置升级后 Jinja2 被意外关闭。
func TestAppendSwitchArgs_JinjaNil(t *testing.T) {
	s := newTestServer()
	s.config.Jinja = nil

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--jinja") {
		t.Errorf("期望 nil 默认开启 --jinja，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_JinjaTrue 验证 Jinja=true 传递 --jinja
func TestAppendSwitchArgs_JinjaTrue(t *testing.T) {
	s := newTestServer()
	jinjaTrue := true
	s.config.Jinja = &jinjaTrue

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--jinja") {
		t.Errorf("期望包含 --jinja，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_JinjaFalse 验证 Jinja=false 传递 --no-jinja
func TestAppendSwitchArgs_JinjaFalse(t *testing.T) {
	s := newTestServer()
	jinjaFalse := false
	s.config.Jinja = &jinjaFalse

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--no-jinja") {
		t.Errorf("期望包含 --no-jinja，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_CachePromptTrue 验证 CachePrompt=true 传递 --cache-prompt
func TestAppendSwitchArgs_CachePromptTrue(t *testing.T) {
	s := newTestServer()
	cacheTrue := true
	s.config.CachePrompt = &cacheTrue

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--cache-prompt") {
		t.Errorf("期望包含 --cache-prompt，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_CachePromptFalse 验证 CachePrompt=false 传递 --no-cache-prompt
func TestAppendSwitchArgs_CachePromptFalse(t *testing.T) {
	s := newTestServer()
	cacheFalse := false
	s.config.CachePrompt = &cacheFalse

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--no-cache-prompt") {
		t.Errorf("期望包含 --no-cache-prompt，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_CachePromptNil 验证 CachePrompt=nil 不传递相关参数
func TestAppendSwitchArgs_CachePromptNil(t *testing.T) {
	s := newTestServer()
	s.config.CachePrompt = nil

	args := s.appendSwitchArgs(nil)
	if containsArg(args, "--cache-prompt") || containsArg(args, "--no-cache-prompt") {
		t.Errorf("期望 nil 时不传递 cache-prompt 参数，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_Metrics 验证 Metrics=true 传递 --metrics
func TestAppendSwitchArgs_Metrics(t *testing.T) {
	s := newTestServer()
	s.config.Metrics = true

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--metrics") {
		t.Errorf("期望包含 --metrics，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_Verbose 验证 Verbose=true 传递 --verbose
func TestAppendSwitchArgs_Verbose(t *testing.T) {
	s := newTestServer()
	s.config.Verbose = true

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--verbose") {
		t.Errorf("期望包含 --verbose，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_Embedding 验证 Embedding=true 传递 --embedding
// 风险：未启用 embedding 会导致 RAG 知识库的 /v1/embeddings 接口不可用。
func TestAppendSwitchArgs_Embedding(t *testing.T) {
	s := newTestServer()
	s.config.Embedding = true

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--embedding") {
		t.Errorf("期望包含 --embedding，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_Pooling 验证 Pooling 非空时传递 --pooling
func TestAppendSwitchArgs_Pooling(t *testing.T) {
	s := newTestServer()
	s.config.Pooling = "mean"

	args := s.appendSwitchArgs(nil)
	if got := argValue(args, "--pooling"); got != "mean" {
		t.Errorf("期望 --pooling=mean，实际: %q", got)
	}
}

// ===== appendServiceArgs 补充分支测试 =====

// TestAppendServiceArgs_Reranker 验证配置了 reranker 模型时传递 --rerank
func TestAppendServiceArgs_Reranker(t *testing.T) {
	s := newTestServer()
	s.config.RerankerModelPath = "/path/to/reranker.gguf"

	args := s.appendServiceArgs(nil)
	if !containsArg(args, "--rerank") {
		t.Errorf("期望包含 --rerank，实际 args: %v", args)
	}
}

// TestAppendServiceArgs_NoReranker 验证未配置 reranker 时不传递 --rerank
func TestAppendServiceArgs_NoReranker(t *testing.T) {
	s := newTestServer()
	s.config.RerankerModelPath = ""

	args := s.appendServiceArgs(nil)
	if containsArg(args, "--rerank") {
		t.Errorf("期望未配置 reranker 时不传递 --rerank，实际 args: %v", args)
	}
}

// TestAppendServiceArgs_Device 验证 Device 非空时传递 --device
func TestAppendServiceArgs_Device(t *testing.T) {
	s := newTestServer()
	s.config.Device = "cuda0"

	args := s.appendServiceArgs(nil)
	if got := argValue(args, "--device"); got != "cuda0" {
		t.Errorf("期望 --device=cuda0，实际: %q", got)
	}
}

// TestAppendServiceArgs_Timeout 验证始终传递 --timeout 900
func TestAppendServiceArgs_Timeout(t *testing.T) {
	s := newTestServer()

	args := s.appendServiceArgs(nil)
	if got := argValue(args, "--timeout"); got != "900" {
		t.Errorf("期望 --timeout=900，实际: %q", got)
	}
}

// ===== appendSpecBasicArgs 补充分支测试 =====
// 覆盖 SpecDraftNMax/SpecDraftNMin > 0 的正常路径（之前只测了 mtpDisabled 分支）。

// TestAppendSpecBasicArgs_DraftNMax 验证 SpecDraftNMax > 0 正确传递
func TestAppendSpecBasicArgs_DraftNMax(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftNMax = 10

	args := s.appendSpecBasicArgs(nil)
	if got := argValue(args, "--spec-draft-n-max"); got != "10" {
		t.Errorf("期望 --spec-draft-n-max=10，实际: %q", got)
	}
}

// TestAppendSpecBasicArgs_DraftNMin 验证 SpecDraftNMin > 0 正确传递
func TestAppendSpecBasicArgs_DraftNMin(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftNMin = 2

	args := s.appendSpecBasicArgs(nil)
	if got := argValue(args, "--spec-draft-n-min"); got != "2" {
		t.Errorf("期望 --spec-draft-n-min=2，实际: %q", got)
	}
}

// TestAppendSpecBasicArgs_DraftNZero 验证零值不传递
func TestAppendSpecBasicArgs_DraftNZero(t *testing.T) {
	s := newTestServer()
	s.config.SpecDraftNMax = 0
	s.config.SpecDraftNMin = 0

	args := s.appendSpecBasicArgs(nil)
	if containsArg(args, "--spec-draft-n-max") || containsArg(args, "--spec-draft-n-min") {
		t.Errorf("期望零值不传递，实际 args: %v", args)
	}
}

// TestAppendSpecBasicArgs_MtpDisabled 验证 mtpFallbackDisabled 时跳过 spec 参数
// 风险：MTP 不支持时仍传递 spec 参数会导致 llama-server 启动失败。
func TestAppendSpecBasicArgs_MtpDisabled(t *testing.T) {
	s := newTestServer()
	s.config.SpecType = "ngram-mod"
	s.config.SpecDraftNMax = 10
	s.config.SpecDraftNMin = 2
	s.mtpFallbackDisabled.Store(true)

	args := s.appendSpecBasicArgs(nil)
	for _, flag := range []string{"--spec-type", "--spec-draft-n-max", "--spec-draft-n-min"} {
		if containsArg(args, flag) {
			t.Errorf("期望 mtpDisabled 时不传递 %s，实际 args: %v", flag, args)
		}
	}
}

// ===== appendLoraArgs 补充分支测试 =====

// TestAppendLoraArgs_SlotSaveCustomPath 验证 SlotSavePath 非空时使用自定义路径
// 风险：自定义路径被忽略会导致 KV 缓存写入错误位置。
func TestAppendLoraArgs_SlotSaveCustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	s.config.SlotSaveEnabled = true
	customPath := filepath.Join(tmpDir, "my-slots")
	s.config.SlotSavePath = customPath

	args := s.appendLoraArgs(nil)
	if got := argValue(args, "--slot-save-path"); got != customPath {
		t.Errorf("期望 --slot-save-path=%q，实际: %q", customPath, got)
	}
}

// TestAppendLoraArgs_SlotSaveDisabled 验证 SlotSaveEnabled=false 时不传递
func TestAppendLoraArgs_SlotSaveDisabled(t *testing.T) {
	s := newTestServer()
	s.config.SlotSaveEnabled = false
	s.config.SlotSavePath = "/some/path"

	args := s.appendLoraArgs(nil)
	if containsArg(args, "--slot-save-path") {
		t.Errorf("期望 SlotSaveEnabled=false 时不传递，实际 args: %v", args)
	}
}

// TestAppendLoraArgs_CacheReuse 验证 CacheReuse > 0 正确传递
func TestAppendLoraArgs_CacheReuse(t *testing.T) {
	s := newTestServer()
	s.config.CacheReuse = 4

	args := s.appendLoraArgs(nil)
	if got := argValue(args, "--cache-reuse"); got != "4" {
		t.Errorf("期望 --cache-reuse=4，实际: %q", got)
	}
}

// TestAppendLoraArgs_LoraWithEmptySegments 验证 LoRA 路径含空段时只传递非空段
// 风险：未 trim 空段会导致空字符串被当作路径传递，llama-server 报错。
func TestAppendLoraArgs_LoraWithEmptySegments(t *testing.T) {
	s := newTestServer()
	// 包含空格和空段
	s.config.LoraPaths = " /a.gguf , , /b.gguf , "

	args := s.appendLoraArgs(nil)
	count := 0
	for _, a := range args {
		if a == "--lora" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("期望 trim 后 2 个非空 --lora，实际 %d 个，args: %v", count, args)
	}
}
