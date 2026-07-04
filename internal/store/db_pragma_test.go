// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"testing"
)

// TestDBPRAGMA_Config 验证 SQLite 连接的关键 PRAGMA 配置正确。
//
// 业务场景：WAL 模式下 synchronous=FULL 是过度保守，NORMAL 已能保证一致性。
// 补全 cache_size、mmap_size 等 PRAGMA 可显著提升读写性能。
//
// 修复前：仅配置 _journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000
// 修复后：补全 _synchronous=NORMAL&_cache_size=-65536&_mmap_size=268435456
func TestDBPRAGMA_Config(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	cases := []struct {
		name     string
		pragma   string
		expected string
	}{
		{"journal_mode", "PRAGMA journal_mode", "wal"},
		{"synchronous", "PRAGMA synchronous", "1"}, // 1 = NORMAL
		{"foreign_keys", "PRAGMA foreign_keys", "1"},
		{"busy_timeout", "PRAGMA busy_timeout", "5000"},
		{"cache_size", "PRAGMA cache_size", "-65536"}, // 64MB
		{"wal_autocheckpoint", "PRAGMA wal_autocheckpoint", "1000"},
		{"mmap_size", "PRAGMA mmap_size", "268435456"}, // 256MB
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got string
			err := db.QueryRow(c.pragma).Scan(&got)
			if err != nil {
				t.Fatalf("%s 查询失败: %v", c.name, err)
			}
			if got != c.expected {
				t.Errorf("%s 期望 %q，实际 %q", c.name, c.expected, got)
			}
		})
	}
}
