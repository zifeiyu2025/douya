// Package main 配置字段同步验证工具
//
// 用途：检测 Go Config struct 与前端 TS Config interface / DEFAULT_CONFIG 之间的字段漂移。
//
// 生活类比：就像超市进货单和货架陈列的核对——进货单（Go struct）上有的商品，
// 货架（TS interface）上也应该有；货架上的商品，进货单上也应该有。
// 如果两边不一致，就会有人买不到东西或买到过期商品。
//
// 运行方式：go run ./cmd/checkconfig
// 退出码：0=一致，1=有差异（CI 中会用这个退出码阻断合入）
package main

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"douya/internal/config"
)

// ignoredFields 是允许在 TS 端不存在的 Go 字段（如内部使用的版本号）
// 生活类比：进货单上有"内部损耗记录"，但不会摆在货架上给顾客看
var ignoredFields = map[string]bool{
	"version": true, // Go 内部版本号，TS Config interface 不需要暴露给前端
}

func main() {
	// 1. 用反射读取 Go Config struct 的所有字段和 json tag
	goFields := extractGoFields()

	// 2. 读取 TS 文件，提取 Config interface 和 DEFAULT_CONFIG 的字段名
	tsFile, err := os.ReadFile("frontend/src/types/chat.ts")
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法读取 TS 文件: %v\n", err)
		os.Exit(1)
	}
	tsContent := string(tsFile)

	tsConfigFields := extractTSConfigFields(tsContent)
	tsDefaultFields := extractTSDefaultFields(tsContent)

	// 3. 对比三处字段是否一致
	hasDiff := false

	// 检查 Go → TS Config interface
	if diff := compareFields("Go Config struct", "TS Config interface", goFields, tsConfigFields); len(diff) > 0 {
		hasDiff = true
	}

	// 检查 Go → TS DEFAULT_CONFIG
	if diff := compareFields("Go Config struct", "TS DEFAULT_CONFIG", goFields, tsDefaultFields); len(diff) > 0 {
		hasDiff = true
	}

	// 检查 TS Config interface → TS DEFAULT_CONFIG（两者必须完全一致）
	if diff := compareFields("TS Config interface", "TS DEFAULT_CONFIG", tsConfigFields, tsDefaultFields); len(diff) > 0 {
		hasDiff = true
	}

	if hasDiff {
		fmt.Println("\n配置字段存在漂移，请同步 Go Config / TS Config / DEFAULT_CONFIG 三处。")
		os.Exit(1)
	}

	fmt.Printf("✓ 配置字段同步检查通过（Go %d 字段，TS Config %d 字段，DEFAULT_CONFIG %d 字段）\n",
		len(goFields), len(tsConfigFields), len(tsDefaultFields))
}

// extractGoFields 用反射读取 Config struct 的 json tag
func extractGoFields() map[string]bool {
	fields := make(map[string]bool)
	t := reflect.TypeOf(config.Config{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		// json tag 格式为 "model_path" 或 "model_path,omitempty"
		name := strings.Split(jsonTag, ",")[0]
		if name != "" {
			fields[name] = true
		}
	}
	return fields
}

// extractTSConfigFields 从 TS 文件中提取 Config interface 的字段名
// 匹配形如 "  field_name: type" 的行（在 interface Config { } 块内）
func extractTSConfigFields(content string) map[string]bool {
	return extractTSFields(content, "export interface Config {", "}")
}

// extractTSDefaultFields 从 TS 文件中提取 DEFAULT_CONFIG 的字段名
// 匹配形如 "  field_name: value" 的行（在 DEFAULT_CONFIG = { } 块内）
func extractTSDefaultFields(content string) map[string]bool {
	return extractTSFields(content, "export const DEFAULT_CONFIG: Config = {", "}")
}

// extractTSFields 通用提取函数：从指定的起始标记和结束标记之间提取字段名
func extractTSFields(content string, startMarker string, endMarker string) map[string]bool {
	fields := make(map[string]bool)

	// 找到起始标记的位置
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		fmt.Fprintf(os.Stderr, "未找到标记: %s\n", startMarker)
		os.Exit(1)
	}
	// 从起始标记后开始查找
	content = content[startIdx+len(startMarker):]

	// 找到结束标记（第一个独立的 "}"）
	endIdx := strings.Index(content, "}")
	if endIdx == -1 {
		fmt.Fprintf(os.Stderr, "未找到结束标记 }\n")
		os.Exit(1)
	}
	block := content[:endIdx]

	// 用正则提取字段名：匹配 "  field_name:" 或 "  field_name :" 的行
	// 忽略注释行（以 // 或 * 开头）和空行
	re := regexp.MustCompile(`^\s+([a-z_][a-z0-9_]*)\s*:`)
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		// 跳过注释行
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || trimmed == "" {
			continue
		}
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			fields[matches[1]] = true
		}
	}
	return fields
}

// compareFields 对比两个字段集合，打印差异并返回差异列表
func compareFields(nameA string, nameB string, setA map[string]bool, setB map[string]bool) []string {
	var missingInB []string // A 有但 B 没有
	var missingInA []string // B 有但 A 没有

	for field := range setA {
		if !setB[field] && !isIgnored(field, nameA, nameB) {
			missingInB = append(missingInB, field)
		}
	}
	for field := range setB {
		if !setA[field] && !isIgnored(field, nameB, nameA) {
			missingInA = append(missingInA, field)
		}
	}

	sort.Strings(missingInB)
	sort.Strings(missingInA)

	if len(missingInB) > 0 {
		fmt.Printf("\n⚠ %s 有但 %s 缺少的字段:\n", nameA, nameB)
		for _, f := range missingInB {
			fmt.Printf("  - %s\n", f)
		}
	}
	if len(missingInA) > 0 {
		fmt.Printf("\n⚠ %s 有但 %s 缺少的字段:\n", nameB, nameA)
		for _, f := range missingInA {
			fmt.Printf("  - %s\n", f)
		}
	}

	return append(missingInB, missingInA...)
}

// isIgnored 判断字段是否在特定对比方向中被忽略
func isIgnored(field string, sourceName string, targetName string) bool {
	// version 字段在 Go 中存在，但 TS Config interface / DEFAULT_CONFIG 中不需要
	// 因为 version 是内部字段，由后端管理，前端不设置
	if ignoredFields[field] && sourceName == "Go Config struct" {
		return true
	}
	return false
}
