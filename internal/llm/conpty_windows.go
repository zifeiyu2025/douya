//go:build windows

package llm

import (
	"fmt"
	"strings"
	"syscall"

	"github.com/UserExistsError/conpty"
)

// startWithConPTY 使用 Windows ConPTY（伪控制台）启动 llama-server
// ConPTY 让子进程以为在真实终端中运行，从而输出完整的 ANSI 颜色码和进度条
// 返回 ConPty 实例，调用方负责 Close()
func startWithConPTY(path string, args []string, dir string, env []string, width, height int) (*conpty.ConPty, error) {
	if !conpty.IsConPtyAvailable() {
		return nil, conpty.ErrConPtyUnsupported
	}
	// 构建命令行字符串（路径和参数需要正确引号）
	cmdLine := buildCommandLine(path, args)
	pty, err := conpty.Start(cmdLine,
		conpty.ConPtyDimensions(width, height),
		conpty.ConPtyWorkDir(dir),
		conpty.ConPtyEnv(env),
	)
	if err != nil {
		return nil, fmt.Errorf("conpty start failed: %w", err)
	}
	return pty, nil
}

// buildCommandLine 将程序路径和参数列表组合为 Windows 命令行字符串
// 生活类比：就像在命令行里输入完整命令，路径和参数有空格时要用引号括起来
func buildCommandLine(path string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, quoteIfNeeded(path))
	for _, arg := range args {
		parts = append(parts, quoteIfNeeded(arg))
	}
	return strings.Join(parts, " ")
}

// quoteIfNeeded 对 Windows 命令行参数进行正确转义
// 安全实践：使用 syscall.EscapeArg 自动处理 Windows 命令行转义（见安全审查 #24），
// 对不含特殊字符的字符串返回原值，行为与旧版"仅在需要时加引号"兼容。
func quoteIfNeeded(s string) string {
	return syscall.EscapeArg(s)
}
