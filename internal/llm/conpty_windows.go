//go:build windows
// +build windows

package llm

import (
	"fmt"
	"strings"

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
	parts := []string{quoteIfNeeded(path)}
	for _, arg := range args {
		parts = append(parts, quoteIfNeeded(arg))
	}
	return strings.Join(parts, " ")
}

// quoteIfNeeded 如果字符串包含空格或制表符，用双引号括起
func quoteIfNeeded(s string) string {
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, "\"") {
		return "\"" + s + "\""
	}
	return s
}
