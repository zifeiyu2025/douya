// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// containsArg 检查 args 切片中是否存在指定的 flag（如 "--split-mode"）。
// 生活类比：在购物清单里找某个商品名，找到返回 true，找不到返回 false。
func containsArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// argValue 返回指定 flag 后面紧跟的值。
// 若 flag 不存在或后面没有值，返回空字符串。
// 生活类比：在购物清单里找某商品名后面写的数量。
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// newTestServer 创建用于测试 buildStartArgs 的 Server 实例。
// 使用最小化配置，仅填充必填字段，避免触发其他参数分支干扰断言。
func newTestServer() *Server {
	return &Server{
		config: &ServerConfig{
			ModelsDir:   "/tmp/models",
			AppDir:      "/tmp/appdir",
			Port:        8080,
			ContextSize: 2048,
		},
	}
}

// ===== 多 GPU 参数分支测试（--split-mode / --tensor-split / --main-gpu） =====

// TestAppendServiceArgs_SplitMode_Empty 验证 SplitMode 为空时不传递 --split-mode 参数。
// 生活类比：用户没说怎么分蛋糕，就让厨师按默认方式切（llama.cpp 默认 layer 模式）。
func TestAppendServiceArgs_SplitMode_Empty(t *testing.T) {
	s := newTestServer()
	s.config.SplitMode = ""

	args := s.appendServiceArgs(nil)
	if containsArg(args, "--split-mode") {
		t.Errorf("期望 SplitMode 为空时不传递 --split-mode，实际 args: %v", args)
	}
}

// TestAppendServiceArgs_SplitMode_Layer 验证 SplitMode=layer 时正确传递 --split-mode layer
func TestAppendServiceArgs_SplitMode_Layer(t *testing.T) {
	s := newTestServer()
	s.config.SplitMode = "layer"

	args := s.appendServiceArgs(nil)
	if !containsArg(args, "--split-mode") {
		t.Fatalf("期望包含 --split-mode，实际 args: %v", args)
	}
	if got := argValue(args, "--split-mode"); got != "layer" {
		t.Errorf("期望 --split-mode=layer，实际得到: %q", got)
	}
}

// TestAppendServiceArgs_SplitMode_Row 验证 SplitMode=row 时正确传递 --split-mode row
func TestAppendServiceArgs_SplitMode_Row(t *testing.T) {
	s := newTestServer()
	s.config.SplitMode = "row"

	args := s.appendServiceArgs(nil)
	if got := argValue(args, "--split-mode"); got != "row" {
		t.Errorf("期望 --split-mode=row，实际得到: %q", got)
	}
}

// TestAppendServiceArgs_SplitMode_Tensor 验证 SplitMode=tensor 时正确传递 --split-mode tensor
func TestAppendServiceArgs_SplitMode_Tensor(t *testing.T) {
	s := newTestServer()
	s.config.SplitMode = "tensor"

	args := s.appendServiceArgs(nil)
	if got := argValue(args, "--split-mode"); got != "tensor" {
		t.Errorf("期望 --split-mode=tensor，实际得到: %q", got)
	}
}

// TestAppendServiceArgs_SplitMode_None 验证 SplitMode=none 时正确传递 --split-mode none
func TestAppendServiceArgs_SplitMode_None(t *testing.T) {
	s := newTestServer()
	s.config.SplitMode = "none"

	args := s.appendServiceArgs(nil)
	if got := argValue(args, "--split-mode"); got != "none" {
		t.Errorf("期望 --split-mode=none，实际得到: %q", got)
	}
}

// TestAppendServiceArgs_TensorSplit_Empty 验证 TensorSplit 为空时不传递 --tensor-split
func TestAppendServiceArgs_TensorSplit_Empty(t *testing.T) {
	s := newTestServer()
	s.config.TensorSplit = ""

	args := s.appendServiceArgs(nil)
	if containsArg(args, "--tensor-split") {
		t.Errorf("期望 TensorSplit 为空时不传递 --tensor-split，实际 args: %v", args)
	}
}

// TestAppendServiceArgs_TensorSplit_Valid 验证合法的 TensorSplit 正确传递
// 生活类比：两张卡 3:1 分配，就像蛋糕切 4 份，第一块拿 3 份第二块拿 1 份。
func TestAppendServiceArgs_TensorSplit_Valid(t *testing.T) {
	s := newTestServer()
	s.config.TensorSplit = "3,1"

	args := s.appendServiceArgs(nil)
	if !containsArg(args, "--tensor-split") {
		t.Fatalf("期望包含 --tensor-split，实际 args: %v", args)
	}
	if got := argValue(args, "--tensor-split"); got != "3,1" {
		t.Errorf("期望 --tensor-split=3,1，实际得到: %q", got)
	}
}

// TestAppendServiceArgs_MainGPU_Negative 验证 MainGPU=-1 时不传递 --main-gpu
// 生活类比：用户没指定哪块卡当主力，就让 llama.cpp 自己挑默认的。
func TestAppendServiceArgs_MainGPU_Negative(t *testing.T) {
	s := newTestServer()
	s.config.MainGPU = -1

	args := s.appendServiceArgs(nil)
	if containsArg(args, "--main-gpu") {
		t.Errorf("期望 MainGPU=-1 时不传递 --main-gpu，实际 args: %v", args)
	}
}

// TestAppendServiceArgs_MainGPU_Zero 验证 MainGPU=0 时传递 --main-gpu 0
// 注意：0 是合法值（第一块 GPU），不应被 appendIntArg 的 val > 0 条件过滤
func TestAppendServiceArgs_MainGPU_Zero(t *testing.T) {
	s := newTestServer()
	s.config.MainGPU = 0

	args := s.appendServiceArgs(nil)
	if !containsArg(args, "--main-gpu") {
		t.Fatalf("期望包含 --main-gpu（MainGPU=0 是合法值），实际 args: %v", args)
	}
	if got := argValue(args, "--main-gpu"); got != "0" {
		t.Errorf("期望 --main-gpu=0，实际得到: %q", got)
	}
}

// TestAppendServiceArgs_MainGPU_Positive 验证 MainGPU=1 时传递 --main-gpu 1
func TestAppendServiceArgs_MainGPU_Positive(t *testing.T) {
	s := newTestServer()
	s.config.MainGPU = 1

	args := s.appendServiceArgs(nil)
	if got := argValue(args, "--main-gpu"); got != "1" {
		t.Errorf("期望 --main-gpu=1，实际得到: %q", got)
	}
}

// TestAppendServiceArgs_MultiGPU_Combination 验证多 GPU 参数组合正确传递
// 模拟双卡场景：split-mode=layer + tensor-split=3,1 + main-gpu=0
func TestAppendServiceArgs_MultiGPU_Combination(t *testing.T) {
	s := newTestServer()
	s.config.SplitMode = "layer"
	s.config.TensorSplit = "3,1"
	s.config.MainGPU = 0

	args := s.appendServiceArgs(nil)
	if got := argValue(args, "--split-mode"); got != "layer" {
		t.Errorf("期望 --split-mode=layer，实际得到: %q", got)
	}
	if got := argValue(args, "--tensor-split"); got != "3,1" {
		t.Errorf("期望 --tensor-split=3,1，实际得到: %q", got)
	}
	if got := argValue(args, "--main-gpu"); got != "0" {
		t.Errorf("期望 --main-gpu=0，实际得到: %q", got)
	}
}

// ===== 自定义聊天模板文件参数测试（--chat-template-file） =====

// TestAppendSwitchArgs_ChatTemplateFile_Empty 验证 ChatTemplateFile 为空时不传递参数
func TestAppendSwitchArgs_ChatTemplateFile_Empty(t *testing.T) {
	s := newTestServer()
	s.config.ChatTemplateFile = ""

	args := s.appendSwitchArgs(nil)
	if containsArg(args, "--chat-template-file") {
		t.Errorf("期望 ChatTemplateFile 为空时不传递 --chat-template-file，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_ChatTemplateFile_NotExist 验证文件不存在时跳过参数并记录警告
// 生活类比：用户指定的菜谱文件找不到，厨师就不按菜谱做，用食材自带的做法。
func TestAppendSwitchArgs_ChatTemplateFile_NotExist(t *testing.T) {
	s := newTestServer()
	s.config.ChatTemplateFile = "nonexistent-template.jinja"

	args := s.appendSwitchArgs(nil)
	if containsArg(args, "--chat-template-file") {
		t.Errorf("期望文件不存在时跳过 --chat-template-file，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_ChatTemplateFile_IsDir 验证路径是目录时跳过参数
func TestAppendSwitchArgs_ChatTemplateFile_IsDir(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	s.config.ChatTemplateFile = "mydir"
	// 在 AppDir 下创建同名目录
	if err := os.MkdirAll(filepath.Join(tmpDir, "mydir"), 0o755); err != nil {
		t.Fatalf("创建测试目录失败: %v", err)
	}

	args := s.appendSwitchArgs(nil)
	if containsArg(args, "--chat-template-file") {
		t.Errorf("期望路径是目录时跳过 --chat-template-file，实际 args: %v", args)
	}
}

// TestAppendSwitchArgs_ChatTemplateFile_Exist 验证文件存在时正确传递参数
func TestAppendSwitchArgs_ChatTemplateFile_Exist(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	s.config.ChatTemplateFile = "my-template.jinja"
	// 在 AppDir 下创建模板文件
	templatePath := filepath.Join(tmpDir, "my-template.jinja")
	if err := os.WriteFile(templatePath, []byte("{% for message in messages %}{{ message.role }}{% endfor %}"), 0o644); err != nil {
		t.Fatalf("创建测试模板文件失败: %v", err)
	}

	args := s.appendSwitchArgs(nil)
	if !containsArg(args, "--chat-template-file") {
		t.Fatalf("期望文件存在时传递 --chat-template-file，实际 args: %v", args)
	}
	got := argValue(args, "--chat-template-file")
	if !strings.HasSuffix(got, "my-template.jinja") {
		t.Errorf("期望 --chat-template-file 以 my-template.jinja 结尾，实际得到: %q", got)
	}
}

// TestAppendSwitchArgs_ChatTemplateFile_AbsolutePath 验证绝对路径直接使用
func TestAppendSwitchArgs_ChatTemplateFile_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	s := newTestServer()
	s.config.AppDir = tmpDir
	// 使用绝对路径
	absPath := filepath.Join(tmpDir, "abs-template.jinja")
	if err := os.WriteFile(absPath, []byte("template content"), 0o644); err != nil {
		t.Fatalf("创建测试模板文件失败: %v", err)
	}
	s.config.ChatTemplateFile = absPath

	args := s.appendSwitchArgs(nil)
	if got := argValue(args, "--chat-template-file"); got != absPath {
		t.Errorf("期望 --chat-template-file=%q，实际得到: %q", absPath, got)
	}
}
