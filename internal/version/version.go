// Package version 提供应用版本信息的统一来源。
//
// 生活类比：版本号就像身份证号，全应用任何地方需要报版本时，
// 都应该来这里查，而不是各自手写一份（避免不一致）。
package version

import "regexp"

// Version 当前应用版本号（每次发版时更新）
const Version = "0.12.13"

// versionPattern 语义化版本号正则（X.Y.Z 格式，可选预发布后缀如 -beta.1）
//
// 生活类比：身份证号的格式校验规则——必须是 18 位且符合特定模式。
// 版本号必须符合 X.Y.Z 格式（如 0.10.7），否则发版会出问题。
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)

// IsValid 验证版本号是否符合语义化版本格式（X.Y.Z）
//
// 用法：
//
//	if !version.IsValid(version.Version) {
//	    log.Fatal("版本号格式错误")
//	}
//
// 在测试中用于确保发版前版本号格式正确。
func IsValid(v string) bool {
	return versionPattern.MatchString(v)
}
