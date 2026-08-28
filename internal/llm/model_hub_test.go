// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import "testing"

// TestSortModelsByMainSize 验证搜索结果按主文件大小升序排序：
// 小的在前、未知大小（查询失败保守保留的仓库）沉底、同大小下载量高的在前。
func TestSortModelsByMainSize(t *testing.T) {
	models := []HubModel{
		{RepoID: "big", MainFileSize: 8 << 30, Downloads: 999},
		{RepoID: "unknown-b", MainFileSize: 0, Downloads: 10},
		{RepoID: "small", MainFileSize: 1 << 30, Downloads: 1},
		{RepoID: "unknown-a", MainFileSize: 0, Downloads: 100},
		{RepoID: "mid", MainFileSize: 4 << 30, Downloads: 500},
		{RepoID: "tie-small-popular", MainFileSize: 1 << 30, Downloads: 50},
	}

	sortModelsByMainSize(models)

	want := []string{
		"tie-small-popular", // 1GB，同大小下载量高者在前
		"small",             // 1GB
		"mid",               // 4GB
		"big",               // 8GB
		"unknown-a",         // 未知大小沉底，按下载量排
		"unknown-b",
	}
	for i, w := range want {
		if models[i].RepoID != w {
			t.Fatalf("第 %d 位应为 %s，实际 %s（完整顺序: %v）", i, w, models[i].RepoID, repoIDs(models))
		}
	}
}

func repoIDs(models []HubModel) []string {
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.RepoID)
	}
	return ids
}
