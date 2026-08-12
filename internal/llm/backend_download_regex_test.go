// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"regexp"
	"testing"
)

// TestCudaReleaseAssetRegex 验证 CUDA 后端 release asset 正则匹配官方资产名。
func TestCudaReleaseAssetRegex(t *testing.T) {
	re := regexp.MustCompile(GetBackendInfo(BackendCUDA).ReleaseAssetRegex)

	tests := []struct {
		asset   string
		matches bool
	}{
		// 官方实际资产名（b10228 release）
		{"llama-b10228-bin-win-cuda-13.3-x64.zip", true}, // CUDA 13.3 ✅
		{"llama-b10228-bin-win-cuda-12.4-x64.zip", true}, // CUDA 12.4 ✅ (新支持)
		{"llama-b10167-bin-win-cuda-13.3-x64.zip", true}, // 旧版本号 ✅
		{"llama-b10167-bin-win-cuda-12.4-x64.zip", true}, // 旧版本 12.4 ✅
		// 不应匹配的
		{"llama-b10228-bin-win-cuda-14.0-x64.zip", false}, // CUDA 14 不在范围
		{"llama-b10228-bin-win-cuda-11.8-x64.zip", false}, // CUDA 11 不在范围
		{"llama-b10228-bin-win-cuda-x64.zip", false},      // 无版本号
		{"llama-b10228-bin-win-vulkan-x64.zip", false},    // 非_cuda
		{"llama-b10228-bin-win-cpu-x64.zip", false},       // 非_cuda
		{"cudart-llama-bin-win-cuda-13.3-x64.zip", false}, // cudart 包（不同前缀）
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			got := re.MatchString(tt.asset)
			if got != tt.matches {
				t.Errorf("MatchString(%q) = %v, 期望 %v", tt.asset, got, tt.matches)
			}
		})
	}
}

// TestCudartAssetRegex 验证 cudart 包正则匹配官方资产名。
func TestCudartAssetRegex(t *testing.T) {
	tests := []struct {
		asset   string
		matches bool
	}{
		// 官方实际资产名（b10228 release）
		{"cudart-llama-bin-win-cuda-13.3-x64.zip", true}, // CUDA 13.3 ✅
		{"cudart-llama-bin-win-cuda-12.4-x64.zip", true}, // CUDA 12.4 ✅ (新支持)
		// 不应匹配的
		{"cudart-llama-bin-win-cuda-14.0-x64.zip", false}, // CUDA 14 不在范围
		{"cudart-llama-bin-win-cuda-11.8-x64.zip", false}, // CUDA 11 不在范围
		{"llama-b10228-bin-win-cuda-13.3-x64.zip", false}, // 主后端包（不同前缀）
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			got := cudartAssetRegex.MatchString(tt.asset)
			if got != tt.matches {
				t.Errorf("MatchString(%q) = %v, 期望 %v", tt.asset, got, tt.matches)
			}
		})
	}
}

// TestCudaMajorVersion 验证从 asset 名提取 CUDA 大版本号。
func TestCudaMajorVersion(t *testing.T) {
	tests := []struct {
		asset    string
		expected int
	}{
		{"llama-b10228-bin-win-cuda-13.3-x64.zip", 13}, // CUDA 13.3 → 13
		{"llama-b10228-bin-win-cuda-12.4-x64.zip", 12}, // CUDA 12.4 → 12
		{"cudart-llama-bin-win-cuda-13.3-x64.zip", 13}, // cudart 13.3 → 13
		{"cudart-llama-bin-win-cuda-12.4-x64.zip", 12}, // cudart 12.4 → 12
		{"llama-b10228-bin-win-vulkan-x64.zip", 0},     // 非 CUDA → 0
		{"llama-b10228-bin-win-cpu-x64.zip", 0},        // 非 CUDA → 0
		{"", 0},                                        // 空字符串 → 0
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			got := cudaMajorVersion(tt.asset)
			if got != tt.expected {
				t.Errorf("cudaMajorVersion(%q) = %d, 期望 %d", tt.asset, got, tt.expected)
			}
		})
	}
}

// TestOtherBackendRegexes 验证其他后端正则仍正常匹配官方资产名。
func TestOtherBackendRegexes(t *testing.T) {
	tests := []struct {
		backend BackendType
		asset   string
		matches bool
	}{
		// HIP / ROCm (AMD)
		{BackendHIP, "llama-b10228-bin-win-hip-radeon-x64.zip", true},
		{BackendHIP, "llama-b10369-bin-win-rocm-7.14-x64.zip", true},
		{BackendHIP, "llama-b10228-bin-win-cuda-13.3-x64.zip", false},
		// SYCL (Intel)
		{BackendSYCL, "llama-b10228-bin-win-sycl-x64.zip", true},
		// Vulkan (跨厂商)
		{BackendVulkan, "llama-b10228-bin-win-vulkan-x64.zip", true},
		// OpenVINO (Intel) - 官方资产名含年份版本号
		{BackendOpenVINO, "llama-b10228-bin-win-openvino-2026.2.1-x64.zip", true},
		{BackendOpenVINO, "llama-b10228-bin-win-openvino-2024.1-x64.zip", true},
		// CPU
		{BackendCPU, "llama-b10228-bin-win-cpu-x64.zip", true},
		{BackendCPU, "llama-b10228-bin-win-cpu-arm64.zip", false}, // arm64 不支持
	}

	for _, tt := range tests {
		t.Run(string(tt.backend)+"_"+tt.asset, func(t *testing.T) {
			info := GetBackendInfo(tt.backend)
			re := regexp.MustCompile(info.ReleaseAssetRegex)
			got := re.MatchString(tt.asset)
			if got != tt.matches {
				t.Errorf("%s 正则 %q 匹配 %q = %v, 期望 %v",
					info.DisplayName, info.ReleaseAssetRegex, tt.asset, got, tt.matches)
			}
		})
	}
}
