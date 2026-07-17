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
//
// 安全说明（基于 GO-PATH-001 安全审查 #2）：
//   - 相对路径：强制做 base-dir 包含性检查，检测到遍历（含 ..）时降级为 Base(p)。
//   - 绝对路径：**直接原样返回，不做 base-dir 校验**。
//
// 之所以对绝对路径放行，是因为现有调用方（app.resolvePath、Server.resolvePath）需要支持
// 用户在 config.json 中配置任意绝对路径（如模型文件路径 D:\models\xxx.gguf），
// 强制 base-dir 校验会破坏该功能。
//
// ⚠️ 调用方注意：仅当 p 来自可信来源（本地配置文件、用户通过文件对话框选择的路径等）
// 时才可使用此函数。若 p 可能来自远程 HTTP 请求或其他不可信输入，**不要使用此函数**，
// 应改用严格模式（自行实现 base-dir 强制校验，绝对路径也必须落在 baseDir 之内）。
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
