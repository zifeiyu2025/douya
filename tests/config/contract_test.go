package config_test

// 契约测试：Go DefaultConfig() ↔ TS DEFAULT_CONFIG 默认值对齐
//
// 本测试采用"方案 A"：
//  1. Go 测试将 DefaultConfig() 序列化为 JSON，写入 go_default_config.json
//  2. 单独的 Node.js 脚本 compare_defaults.mjs 读取该 JSON，
//     与 frontend/src/types/chat.ts 中的 DEFAULT_CONFIG 逐字段比对
//
// 运行方式：
//   go test ./tests/config/... -run TestContractExportDefaultConfig -count=1
//   node tests/config/compare_defaults.mjs
//
// 当 Go 默认值变更时，本测试会更新导出的 JSON，Node 脚本随即检测到差异并报错，
// 提醒开发者同步修改 TS DEFAULT_CONFIG。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"douya/internal/config"
)

// TestContractExportDefaultConfig 将 Go DefaultConfig() 导出为 JSON 文件，
// 供 Node.js 脚本与 TS DEFAULT_CONFIG 进行跨语言契约比对。
func TestContractExportDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	// 序列化为带缩进的 JSON（便于人工审阅与版本对比）
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("序列化 DefaultConfig 失败: %v", err)
	}

	// 写入到本测试目录下的 go_default_config.json
	outPath := filepath.Join(".", "go_default_config.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		t.Fatalf("写入 %s 失败: %v", outPath, err)
	}

	t.Logf("已导出 Go DefaultConfig JSON 到 %s (%d 字节)", outPath, len(data))

	// 基本健全性检查：确保导出的 JSON 非空且包含关键字段
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析导出的 JSON 失败: %v", err)
	}

	// 检查关键字段存在（覆盖不同类型：string/int/float64/bool/*bool）
	requiredKeys := []string{
		"temperature",        // float64
		"top_k",              // int
		"cache_ram",          // int
		"sleep_idle_seconds", // int
		"cache_prompt",       // *bool
		"jinja",              // *bool
		"reasoning_preserve", // *bool
		"model_path",         // string
		"mmproj_auto",        // bool
	}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("导出的 JSON 缺少关键字段: %s", key)
		}
	}
}

// TestContractDefaultConfigSpotChecks 对几个曾出现不一致的关键字段做硬编码断言。
// 这些值是 Go DefaultConfig() 的"契约锚点"——若 Go 侧修改了它们，
// 本测试会失败，提醒开发者同步更新 TS DEFAULT_CONFIG 和 compare_defaults.mjs。
func TestContractDefaultConfigSpotChecks(t *testing.T) {
	cfg := config.DefaultConfig()

	// 以下是 TS 侧历史曾与 Go 不一致的字段，作为重点回归锚点
	spotChecks := []struct {
		name string
		got  any
		want any
	}{
		// 采样参数（与 llama.cpp 默认值对齐）
		{"temperature", cfg.Temperature, 0.8},
		{"top_k", cfg.TopK, 40},
		// 缓存配置
		{"cache_ram", cfg.CacheRAM, 0},
		// 空闲休眠（-1=禁用）
		{"sleep_idle_seconds", cfg.SleepIdleSeconds, -1},
		// *bool 类型字段：cache_prompt 默认 true（显式启用）
		{"cache_prompt", *cfg.CachePrompt, true},
		// *bool 类型字段：jinja 默认 true（默认开启 Jinja2 模板引擎）
		{"jinja", *cfg.Jinja, true},
		// *bool 类型字段：reasoning_preserve 默认 nil
		{"reasoning_preserve_is_nil", cfg.ReasoningPreserve, (*bool)(nil)},
	}

	for _, sc := range spotChecks {
		t.Run(sc.name, func(t *testing.T) {
			// 对于 *bool 类型的 nil 比较，必须使用 reflect.DeepEqual：
			// 因为 *bool 的 nil 装箱为 interface{} 后，其 (type, value) != (nil, nil)，
			// 直接用 "!= nil" 判断会误判为非 nil。
			if sc.name == "reasoning_preserve_is_nil" {
				if !reflect.DeepEqual(sc.got, sc.want) {
					t.Errorf("%s: 期望 nil，实际 %v", sc.name, sc.got)
				}
				return
			}
			if sc.got != sc.want {
				t.Errorf("%s: 期望 %v，实际 %v", sc.name, sc.want, sc.got)
			}
		})
	}
}
