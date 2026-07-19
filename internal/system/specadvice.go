// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"fmt"
	"net/url"
	"strings"
)

// BuildSidecarURL 为 Eagle3/DFlash 推测解码的 sidecar 模型构造 hf-mirror.com 下载链接。
//
// 豆芽不直接下载 sidecar 模型，而是引导用户前往 hf-mirror.com（国内镜像）手动下载。
// - 有 HF repo 时：直接指向仓库文件列表页，用户可在其中查找 eagle3-/dflash- 前缀的文件
// - 无 HF repo 时：使用站内搜索兜底，关键词为 "<架构> <sidecar>"
//
// 返回值：
//   - url:  下载链接（不支持的 sidecar 类型返回空串）
//   - desc: 人类可读的名称（"Eagle3" 或 "DFlash"），用于提示文案
//
// 生活类比：像快递柜的「取件码页面」——豆芽不帮你送货上门（不下载文件），
// 但给你一张写有地址的便签（链接），让你自己去快递柜取（浏览器下载）。
func BuildSidecarURL(hfRepo, arch, sidecar string) (urlStr, desc string) {
	switch strings.ToLower(sidecar) {
	case "eagle3":
		desc = "Eagle3"
	case "dflash":
		desc = "DFlash"
	default:
		// 不支持的 sidecar 类型：返回空串，避免前端展示无效链接
		return "", ""
	}

	// 有 HF repo 时直接指向仓库文件列表页
	if hfRepo != "" {
		return fmt.Sprintf("https://hf-mirror.com/%s/tree/main", hfRepo), desc
	}

	// 无 HF repo 时用搜索兜底（arch 已是模型架构字符串）
	// 用 url.QueryEscape 编码空格为 +，符合 RFC 3986 application/x-www-form-urlencoded 约定
	keyword := strings.TrimSpace(fmt.Sprintf("%s %s", arch, strings.ToLower(sidecar)))
	return fmt.Sprintf("https://hf-mirror.com/search?search_keyword=%s", url.QueryEscape(keyword)), desc
}

// SpecAdvice 表示针对推测解码的智能提醒。
//
// 豆芽检测到当前模型支持 Eagle3 推测解码（如 Qwen3.5/3.6）但用户未配置 draft 模型时，
// 会生成此建议，前端在设置界面静态显示 + 模型加载后弹通知，引导用户前往
// hf-mirror.com 下载对应的 sidecar 模型。
//
// 生活类比：像手机检测到你连接了不支持快充的充电器，会弹出「建议使用原装充电器以获得快充体验」，
// 但不会强制你换充电器——只是提醒，最终决定权在用户。
type SpecAdvice struct {
	// Sidecar 推测解码类型："eagle3" 或 "dflash"
	Sidecar string
	// Desc 人类可读名称："Eagle3" 或 "DFlash"
	Desc string
	// DownloadURL hf-mirror.com 下载链接（仓库页或搜索页）
	DownloadURL string
	// Reason 触发建议的原因（用于日志和前端展示）
	// 例如 "模型支持 Eagle3 推测解码，但未配置 draft 模型"
	Reason string
}

// EvaluateSpecAdvice 根据模型能力和用户配置生成推测解码建议。
//
// 参数：
//   - supportsEagle3: 模型是否支持 Eagle3 推测解码（来自 GGUF 元数据）
//   - hfRepo:         GGUF 元数据中的 general.source.huggingface.repository
//   - arch:           模型架构字符串（无 HF repo 时用于搜索兜底）
//   - specDraftModel: 用户当前配置的 draft 模型路径（空串表示未配置）
//   - adviceEnabled:  用户是否开启了推测解码建议开关
//
// 返回值：
//   - *SpecAdvice: 需要提醒时返回非空指针；无需提醒时返回 nil
//
// 触发条件（全部满足才提醒）：
//  1. adviceEnabled == true（用户开启了开关）
//  2. supportsEagle3 == true（模型支持）
//  3. specDraftModel == ""（用户未配置 draft 模型）
//
// 设计权衡：只在模型支持且用户未配置时提醒，避免打扰已经配置好的用户。
// 如果用户明确关闭了 adviceEnabled，则不再打扰（尊重用户选择）。
func EvaluateSpecAdvice(supportsEagle3 bool, hfRepo, arch, specDraftModel string, adviceEnabled bool) *SpecAdvice {
	// 用户关闭了开关：不提醒
	if !adviceEnabled {
		return nil
	}
	// 模型不支持：不提醒
	if !supportsEagle3 {
		return nil
	}
	// 用户已配置 draft 模型：不提醒
	if specDraftModel != "" {
		return nil
	}

	// 模型支持 Eagle3 且用户未配置 draft：生成建议
	urlStr, desc := BuildSidecarURL(hfRepo, arch, "eagle3")
	return &SpecAdvice{
		Sidecar:     "eagle3",
		Desc:        desc,
		DownloadURL: urlStr,
		Reason:      fmt.Sprintf("模型支持 %s 推测解码，但未配置 draft 模型", desc),
	}
}
