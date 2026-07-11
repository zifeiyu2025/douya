// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import "testing"

// TestFormatBytes_GB 验证 GB 级别格式化（1位小数）
func TestFormatBytes_GB(t *testing.T) {
	const gb = uint64(1024 * 1024 * 1024)
	cases := []struct {
		name  string
		bytes uint64
		want  string
	}{
		{"1 GB", 1 * gb, "1.0 GB"},
		{"1.5 GB", 3 * gb / 2, "1.5 GB"},
		{"10 GB", 10 * gb, "10.0 GB"},
		{"2 GB", 2 * gb, "2.0 GB"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatBytes(c.bytes)
			if got != c.want {
				t.Errorf("FormatBytes(%d) 期望 %q，实际 %q", c.bytes, c.want, got)
			}
		})
	}
}

// TestFormatBytes_MB 验证 MB 级别格式化（0位小数）
func TestFormatBytes_MB(t *testing.T) {
	const mb = uint64(1024 * 1024)
	cases := []struct {
		name  string
		bytes uint64
		want  string
	}{
		{"1 MB", 1 * mb, "1 MB"},
		{"500 MB", 500 * mb, "500 MB"},
		{"1023 MB", 1023 * mb, "1023 MB"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatBytes(c.bytes)
			if got != c.want {
				t.Errorf("FormatBytes(%d) 期望 %q，实际 %q", c.bytes, c.want, got)
			}
		})
	}
}

// TestFormatBytes_Bytes 验证小于 1MB 的字节数格式化
func TestFormatBytes_Bytes(t *testing.T) {
	cases := []struct {
		name  string
		bytes uint64
		want  string
	}{
		{"0 字节", 0, "0 B"},
		{"1 字节", 1, "1 B"},
		{"1023 字节", 1023, "1023 B"},
		{"刚好 1MB 以下", 1024*1024 - 1, "1048575 B"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FormatBytes(c.bytes)
			if got != c.want {
				t.Errorf("FormatBytes(%d) 期望 %q，实际 %q", c.bytes, c.want, got)
			}
		})
	}
}

// TestFormatBytes_Boundary 验证边界值
// 刚好 1GB 和 1MB 的边界应正确分类
func TestFormatBytes_Boundary(t *testing.T) {
	const gb = uint64(1024 * 1024 * 1024)
	const mb = uint64(1024 * 1024)

	// 刚好 1GB
	got := FormatBytes(gb)
	if got != "1.0 GB" {
		t.Errorf("FormatBytes(1GB) 期望 '1.0 GB'，实际 %q", got)
	}

	// 刚好 1MB
	got = FormatBytes(mb)
	if got != "1 MB" {
		t.Errorf("FormatBytes(1MB) 期望 '1 MB'，实际 %q", got)
	}

	// 1GB - 1 字节 → 1023.999... MB，%.0f 四舍五入为 "1024 MB"
	got = FormatBytes(gb - 1)
	if got != "1024 MB" {
		t.Errorf("FormatBytes(1GB-1) 期望 '1024 MB'（四舍五入），实际 %q", got)
	}
}
