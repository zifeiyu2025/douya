// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"path/filepath"
	"testing"
)

// TestPathWithinDir 验证路径边界判断：exe 在 runtimeDir 内为 true，
// 兄弟目录（runtime_evil）或上级目录为 false。
// P2.5 回归：纯字符串前缀匹配会误判 `...\runtime_evil\...`，改用 filepath.Rel 判断。
func TestPathWithinDir(t *testing.T) {
	dir := `C:\Users\test\AppData\Roaming\douya\runtime`

	cases := []struct {
		name string
		exe  string
		want bool
	}{
		{name: "精确位于 runtime 下", exe: filepath.Join(dir, "llama-server.exe"), want: true},
		{name: "runtime 子目录下", exe: filepath.Join(dir, "cuda", "llama-server.exe"), want: true},
		{name: "兄弟目录 runtime_evil 误判", exe: `C:\Users\test\AppData\Roaming\douya\runtime_evil\llama-server.exe`, want: false},
		{name: "上级目录", exe: `C:\Users\test\AppData\Roaming\douya\llama-server.exe`, want: false},
		{name: "完全无关目录", exe: `D:\elsewhere\llama-server.exe`, want: false},
		{name: "大小写不同盘符", exe: `c:\Users\test\AppData\Roaming\douya\runtime\llama-server.exe`, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathWithinDir(dir, tc.exe); got != tc.want {
				t.Errorf("pathWithinDir(%q, %q) = %v, want %v", dir, tc.exe, got, tc.want)
			}
		})
	}
}
