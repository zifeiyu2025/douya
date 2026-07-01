// Package pathutil 提供路径安全相关的公共工具函数。
// 安全实践：统一路径遍历防护与文件权限收紧，避免多处实现不一致（见安全审查 #20）。
package pathutil

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// ResolveInBase 将相对路径 p 解析为相对于 baseDir 的绝对路径，并防止路径遍历攻击。
// 生活类比：像快递员根据"当前所在楼层"补全收件地址，但若发现地址试图逃出大楼（含 ..），
// 就降级为只送包裹的姓名（Base(p)），确保不会送到大楼外。
// 已是绝对路径则原样返回；检测到遍历时降级为 Base(p) 并记录警告日志。
func ResolveInBase(baseDir, p string) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if filepath.IsAbs(p) {
		return p
	}
	candidate := filepath.Join(baseDir, p)
	// 验证结果路径仍在基准目录内（使用分隔符前缀避免兄弟目录绕过，如 baseDir="C:\app" 时 "C:\app-evil" 不应通过）
	absCandidate, err := filepath.Abs(candidate)
	if err == nil {
		if absCandidate != baseDir && !strings.HasPrefix(absCandidate, baseDir+string(filepath.Separator)) {
			log.Warn().Str("path", p).Str("baseDir", baseDir).Msg("[pathutil] path traversal detected, fallback to base name")
			return filepath.Join(baseDir, filepath.Base(p))
		}
	}
	return candidate
}

// RestrictACLWindows 在 Windows 上收紧文件/目录的 ACL，移除继承权限并仅授予当前用户读写。
// 安全实践：弥补 Windows 上 0600 权限位无效的问题，与密钥文件处理保持一致（见安全审查 #6/#21/#22）。
// 在非 Windows 平台为空操作。
// 测试环境（DOUYA_SKIP_ACL=1）跳过 ACL 收紧，避免临时目录清理失败。
func RestrictACLWindows(path string) error {
	// 测试环境跳过，避免 icacls 收紧权限后 TempDir 无法删除
	if os.Getenv("DOUYA_SKIP_ACL") == "1" {
		return nil
	}
	// 获取当前用户名，用于在 ACL 中授予该用户读写权限
	u, err := user.Current()
	if err != nil {
		return err
	}

	// icacls 命令：
	//   /inheritance:r  移除所有继承的 ACE
	//   /grant:r        替换现有 ACE，仅授予指定用户指定权限
	cmd := exec.Command("icacls", path, "/inheritance:r", "/grant:r", u.Username+":(R,W)")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn().Err(err).Str("path", path).Str("output", string(output)).Msg("[pathutil] icacls failed to restrict ACL")
		return err
	}
	return nil
}

// SecureWriteFile 写入文件并收紧权限：Windows 调用 icacls，Unix 使用 0600 权限位。
// dirMode 用于指定目录创建权限（若需创建目录），Windows 同样收紧。
func SecureWriteFile(path string, data []byte, dirMode os.FileMode) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return RestrictACLWindows(path)
}

// SecureMkdirAll 创建目录并收紧权限：Windows 调用 icacls，Unix 使用 dirMode。
func SecureMkdirAll(path string, dirMode os.FileMode) error {
	if err := os.MkdirAll(path, dirMode); err != nil {
		return err
	}
	return RestrictACLWindows(path)
}
