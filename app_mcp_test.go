// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import "testing"

// =============================================================================
// matchMCPServerForTool 测试
// 覆盖 BUG-9 修复：server 名含下划线时的归属判断、前缀重叠时的长度优先规则。
// =============================================================================

// TestMatchMCPServerForTool_ServerNameWithUnderscore 含下划线 server 名不会被首下划线误拆分。
// 回归 BUG-9：旧实现用首下划线拆分，会把 "my_server_foo" 误归给 "my"。
func TestMatchMCPServerForTool_ServerNameWithUnderscore(t *testing.T) {
	serverNames := []string{"my_server", "echo"}
	got, ok := matchMCPServerForTool("my_server_foo", serverNames)
	if !ok || got != "my_server" {
		t.Fatalf("工具 my_server_foo 应归给 my_server（含下划线 server 名），实际归给 %q (ok=%v)", got, ok)
	}
}

// TestMatchMCPServerForTool_SimpleServer 普通 server 名正常匹配。
func TestMatchMCPServerForTool_SimpleServer(t *testing.T) {
	serverNames := []string{"echo", "calc"}
	got, ok := matchMCPServerForTool("calc_add", serverNames)
	if !ok || got != "calc" {
		t.Fatalf("工具 calc_add 应归给 calc，实际归给 %q (ok=%v)", got, ok)
	}
}

// TestMatchMCPServerForTool_PrefixOverlapLongestWins server 名互为前缀时，长名称优先。
// 覆盖场景：server "echo" 与 "echo_x"，工具 "echo_x_foo" 必须归给 "echo_x"。
func TestMatchMCPServerForTool_PrefixOverlapLongestWins(t *testing.T) {
	serverNames := []string{"echo", "echo_x"}
	got, ok := matchMCPServerForTool("echo_x_foo", serverNames)
	if !ok || got != "echo_x" {
		t.Fatalf("工具 echo_x_foo 应归给更长的 echo_x，实际归给 %q (ok=%v)", got, ok)
	}
}

// TestMatchMCPServerForTool_OrderIndependent 匹配结果不依赖 serverNames 的传入顺序。
func TestMatchMCPServerForTool_OrderIndependent(t *testing.T) {
	tools := []struct {
		name      string
		server    []string
		want      string
		wantMatch bool
	}{
		{"echo_x_foo", []string{"echo_x", "echo"}, "echo_x", true},
		{"echo_x_foo", []string{"echo", "echo_x"}, "echo_x", true},
		{"echo_foo", []string{"echo_x", "echo"}, "echo", true},
	}
	for _, tc := range tools {
		got, ok := matchMCPServerForTool(tc.name, tc.server)
		if ok != tc.wantMatch || got != tc.want {
			t.Errorf("match(%q, %v) = (%q, %v)，期望 (%q, %v)",
				tc.name, tc.server, got, ok, tc.want, tc.wantMatch)
		}
	}
}

// TestMatchMCPServerForTool_NoMatch 无任何 server 前缀匹配时返回 false。
func TestMatchMCPServerForTool_NoMatch(t *testing.T) {
	serverNames := []string{"echo", "calc"}
	if _, ok := matchMCPServerForTool("random_tool", serverNames); ok {
		t.Fatal("random_tool 不应匹配任何已配置 server")
	}
	if _, ok := matchMCPServerForTool("search", serverNames); ok {
		t.Fatal("内置工具 search 不应匹配任何 MCP server")
	}
}

// TestMatchMCPServerForTool_EmptyEdge 空工具名 / 空 server 名等边界情况。
func TestMatchMCPServerForTool_EmptyEdge(t *testing.T) {
	// 空工具名
	if _, ok := matchMCPServerForTool("", []string{"echo"}); ok {
		t.Fatal("空工具名不应匹配")
	}
	// 空 server 名列表
	if _, ok := matchMCPServerForTool("echo_foo", nil); ok {
		t.Fatal("空 server 列表不应匹配")
	}
	// 空 server 名（空前缀）应被忽略，工具仍归给正常 server
	if got, ok := matchMCPServerForTool("echo_foo", []string{"", "echo"}); !ok || got != "echo" {
		t.Fatalf("空 server 名应被忽略，工具应归给 echo，实际 (%q, %v)", got, ok)
	}
	// 名字恰好等于 server 名（缺 "_" 后缀）不算匹配
	if _, ok := matchMCPServerForTool("echo", []string{"echo"}); ok {
		t.Fatal("工具名仅等于 server 名、缺下划线后缀，不应匹配")
	}
}
