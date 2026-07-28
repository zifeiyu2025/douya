// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import "testing"

// TestParseGPUArchitecture 测试从 GPU 名称解析微架构代号
//
// 生活类比：就像从车牌号前缀判断车辆品牌，从 GPU 名称中的"RTX 50/40/30"等关键字
// 可以判断它属于哪一代架构。这个测试确保解析逻辑覆盖各种常见命名格式。
func TestParseGPUArchitecture(t *testing.T) {
	tests := []struct {
		name     string
		gpuName  string
		wantArch string
	}{
		// RTX 50 系 → Blackwell
		{"RTX 5090", "NVIDIA GeForce RTX 5090", "Blackwell"},
		{"RTX 5080", "NVIDIA GeForce RTX 5080", "Blackwell"},
		{"RTX 5070 Ti", "RTX 5070 Ti", "Blackwell"},
		{"RTX 5060", "GeForce RTX 5060", "Blackwell"},
		// RTX 40 系 → Ada
		{"RTX 4090", "NVIDIA GeForce RTX 4090", "Ada"},
		{"RTX 4080", "RTX 4080", "Ada"},
		{"RTX 4060", "GeForce RTX 4060", "Ada"},
		// RTX 30 系 → Ampere
		{"RTX 3090", "NVIDIA GeForce RTX 3090", "Ampere"},
		{"RTX 3080 Ti", "RTX 3080 Ti", "Ampere"},
		{"RTX 3060", "GeForce RTX 3060", "Ampere"},
		// GTX 16 系 → Turing
		{"GTX 1660", "GeForce GTX 1660", "Turing"},
		{"GTX 1650", "GTX 1650", "Turing"},
		// 边界情况
		{"空字符串", "", "Unknown"},
		{"未知显卡", "Intel Arc A770", "Unknown"},
		{"AMD 显卡", "AMD Radeon RX 7900 XTX", "Unknown"},
		// 大小写不敏感
		{"小写 rtx 50", "rtx 5090", "Blackwell"},
		{"混合大小写", "Rtx 4090", "Ada"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGPUArchitecture(tt.gpuName)
			if got != tt.wantArch {
				t.Errorf("parseGPUArchitecture(%q) 期望 %q，实际 %q", tt.gpuName, tt.wantArch, got)
			}
		})
	}
}
