// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"os"
	"path/filepath"
	"testing"

	"douya/internal/system"
)

// makeFakeBackend 在指定 runtime 目录下伪造一个已安装后端
// （创建 llama-server.exe 占位文件），返回该文件的期望绝对路径。
func makeFakeBackend(t *testing.T, runtimeDir string, bt BackendType) string {
	t.Helper()
	subdir := GetBackendInfo(bt).Subdir
	if subdir == "" {
		t.Fatalf("后端 %s 无子目录，无法伪造", bt)
	}
	dir := filepath.Join(runtimeDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, llamaServerExe)
	if err := os.WriteFile(p, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestFindInstalledBackend 验证多目录按优先级查找：
// 目录顺序即优先级（包内置在前、数据目录在后），命中即返回；全部未命中返回空串。
func TestFindInstalledBackend(t *testing.T) {
	bundled := t.TempDir()
	data := t.TempDir()

	// 两目录均未安装：返回空串（调用方将走下载流程）
	if got := FindInstalledBackend(BackendVulkan, []string{bundled, data}); got != "" {
		t.Errorf("两目录均未安装时应返回空串，实际 %q", got)
	}

	// 仅数据目录已安装：应从第二优先级目录命中
	wantInData := makeFakeBackend(t, data, BackendVulkan)
	if got := FindInstalledBackend(BackendVulkan, []string{bundled, data}); got != wantInData {
		t.Errorf("应命中数据目录中的引擎\n got  = %q\n want = %q", got, wantInData)
	}

	// 内置目录也安装后：优先返回内置目录（第一优先级）
	wantInBundled := makeFakeBackend(t, bundled, BackendVulkan)
	if got := FindInstalledBackend(BackendVulkan, []string{bundled, data}); got != wantInBundled {
		t.Errorf("内置目录优先级更高\n got  = %q\n want = %q", got, wantInBundled)
	}

	// auto 无具体子目录：永远返回空串
	if got := FindInstalledBackend(BackendAuto, []string{bundled, data}); got != "" {
		t.Errorf("BackendAuto 应返回空串，实际 %q", got)
	}

	// 空字符串目录项应被安全跳过
	if got := FindInstalledBackend(BackendVulkan, []string{"", bundled}); got != wantInBundled {
		t.Errorf("空目录项应被跳过\n got  = %q\n want = %q", got, wantInBundled)
	}
}

// TestIsBackendInstalledIn 验证多目录安装判定：任一目录命中即为 true。
func TestIsBackendInstalledIn(t *testing.T) {
	bundled := t.TempDir()
	data := t.TempDir()
	dirs := []string{bundled, data}

	if IsBackendInstalledIn(BackendCPU, dirs) {
		t.Error("未安装时应返回 false")
	}
	makeFakeBackend(t, bundled, BackendCPU)
	if !IsBackendInstalledIn(BackendCPU, dirs) {
		t.Error("内置目录命中时应返回 true")
	}
}

// TestResolveBackendTypeWithRuntimeDirs 验证多目录预校验解析。
// 核心场景（微软商店认证失败根因）：引擎只存在于候选目录之一时必须被识别，
// 让 AMD/Intel 机器开箱即用 Vulkan，不再触发首启下载弹窗。
func TestResolveBackendTypeWithRuntimeDirs(t *testing.T) {
	bundled := t.TempDir()
	data := t.TempDir()
	dirs := []string{bundled, data}

	intel := &system.HardwareInfo{GPUVendor: "intel"}
	nvidia := &system.HardwareInfo{GPUVendor: "nvidia"}

	// Intel 核显 + auto + Vulkan 未装：保持原推断 Vulkan（交给下载流程，不误回 CPU）
	if got := ResolveBackendTypeWithRuntimeDirs(intel, "auto", dirs); got != BackendVulkan {
		t.Errorf("Intel+auto 未装时应保持 Vulkan 推断，实际 %v", got)
	}

	// 商店核心场景：仅内置目录装有 Vulkan → Intel 用户直接用 Vulkan（开箱即用）
	makeFakeBackend(t, bundled, BackendVulkan)
	if got := ResolveBackendTypeWithRuntimeDirs(intel, "auto", dirs); got != BackendVulkan {
		t.Errorf("Intel+auto 且内置 Vulkan 已装时应选 Vulkan，实际 %v", got)
	}

	// N 卡 + auto + 仅内置 Vulkan：仍保持 CUDA 推断（原生 CUDA 优先策略不变，
	// 商店模式的"N 卡首启例外"由应用层 installBackend 的商店兜底逻辑处理）
	if got := ResolveBackendTypeWithRuntimeDirs(nvidia, "auto", dirs); got != BackendCUDA {
		t.Errorf("N卡+auto 应保持 CUDA 推断（不被内置 Vulkan 抢占），实际 %v", got)
	}

	// 手动指定的后端不参与安装预校验与回退（尊重用户选择）
	if got := ResolveBackendTypeWithRuntimeDirs(intel, "cpu", dirs); got != BackendCPU {
		t.Errorf("手动指定 cpu 应原样返回，实际 %v", got)
	}

	// 无 GPU + auto：CPU 即兜底，无需检查
	if got := ResolveBackendTypeWithRuntimeDirs(nil, "auto", dirs); got != BackendCPU {
		t.Errorf("无硬件信息时应回退 CPU，实际 %v", got)
	}
}
