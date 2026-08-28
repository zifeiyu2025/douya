// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"douya/internal/apperror"
	"douya/internal/llm"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SearchHubModels 在下载源搜索模型（第 page 页，从 1 起），并过滤掉无主 .gguf 的仓库。
//
// 生活类比：在"外卖平台"上按页搜"菜品"——第 1 页是下载量最高的一批，点"加载更多"再看下一页。
// 每页仍会检查是否含主 .gguf，剔除点开也下载不了的仓库。
func (a *App) SearchHubModels(provider string, query string, page int) ([]llm.HubModel, error) {
	prov, err := resolveHubProvider(provider)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	searchCtx, searchCancel := context.WithTimeout(context.Background(), apiTimeoutMedium)
	defer searchCancel()
	models, err := llm.SearchModels(searchCtx, prov, strings.TrimSpace(query), page)
	if err != nil {
		return nil, err
	}

	// 过滤阶段：需要逐个仓库查文件列表，耗时较长，给独立且更宽的超时预算
	filterCtx, filterCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer filterCancel()
	return llm.FilterModelsWithGGUF(filterCtx, prov, models, 8), nil
}

// ListHubModelFiles 列出指定仓库内的文件（用于挑选 .gguf 主文件与可选 MMProj）。
func (a *App) ListHubModelFiles(provider string, repoID string) ([]llm.HubFile, error) {
	prov, err := resolveHubProvider(provider)
	if err != nil {
		return nil, err
	}
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return nil, apperror.New(apperror.KindInvalidInput, "仓库 ID 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutMedium)
	defer cancel()
	return llm.ListModelFiles(ctx, prov, repoID)
}

// DownloadHubModel 开始下载模型（异步，进度通过 model:downloadProgress 事件推送）。
//
// mainFile 为必选的 .gguf 主文件；mmprojFile 可选，为同一仓库的 MMProj 投影文件。
// 下载完成后自动刷新模型列表，使新模型出现在顶部下拉框。
func (a *App) DownloadHubModel(provider string, repoID string, mainFile string, mmprojFile string) error {
	prov, err := resolveHubProvider(provider)
	if err != nil {
		return err
	}
	repoID = strings.TrimSpace(repoID)
	mainFile = strings.TrimSpace(mainFile)
	mmprojFile = strings.TrimSpace(mmprojFile)
	if repoID == "" {
		return apperror.New(apperror.KindInvalidInput, "仓库 ID 不能为空")
	}
	if mainFile == "" {
		return apperror.New(apperror.KindInvalidInput, "请先选择一个 .gguf 主文件")
	}
	mainBase, err := sanitizeGGUFName(mainFile)
	if err != nil {
		return err
	}
	var mmprojBase string
	if mmprojFile != "" {
		mmprojBase, err = sanitizeGGUFName(mmprojFile)
		if err != nil {
			return err
		}
	}

	// 防重复下载：同一文件同时只能有一个下载任务
	dlKey := fmt.Sprintf("%s/%s/%s", prov, repoID, mainBase)
	a.downloadMu.Lock()
	if a.downloadingModels == nil {
		a.downloadingModels = make(map[string]bool)
	}
	if a.downloadingModels[dlKey] {
		a.downloadMu.Unlock()
		return apperror.Newf(apperror.KindInvalidInput, "模型 %s 正在下载中，请勿重复触发", mainBase)
	}
	a.downloadingModels[dlKey] = true
	a.downloadMu.Unlock()

	clearDownload := func() {
		a.downloadMu.Lock()
		delete(a.downloadingModels, dlKey)
		a.downloadMu.Unlock()
	}

	zlog.Info().
		Str("provider", string(prov)).
		Str("repo", repoID).
		Str("file", mainBase).
		Msg("[modelhub] 开始下载模型")

	// 异步下载 + 收尾（goroutine 内 panic 防护）
	go func() {
		defer clearDownload()
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Str("file", mainBase).Msg("[modelhub] 下载 goroutine panic")
				a.emitModelDownloadProgress(llm.ModelDownloadProgress{
					Provider: prov, RepoID: repoID, FilePath: mainBase,
					Status: "failed", Error: fmt.Sprintf("下载模型发生内部错误：%v", r),
				})
				a.emitModelDownloadComplete(repoID, mainBase, false, fmt.Sprintf("下载模型发生内部错误：%v", r))
			}
		}()

		modelsDir := filepath.Join(appDir(), "models")
		if err := os.MkdirAll(modelsDir, 0o755); err != nil {
			zlog.Error().Err(err).Str("dir", modelsDir).Msg("[modelhub] 创建 models 目录失败")
			a.emitModelDownloadComplete(repoID, mainBase, false, err.Error())
			return
		}

		emitErr := a.downloadSingle(prov, repoID, mainBase, modelsDir)
		if emitErr != nil {
			a.emitModelDownloadComplete(repoID, mainBase, false, emitErr.Error())
			return
		}

		// 可选 MMProj 文件
		if mmprojBase != "" {
			if emitErr := a.downloadSingle(prov, repoID, mmprojBase, modelsDir); emitErr != nil {
				a.emitModelDownloadComplete(repoID, mmprojBase, false, emitErr.Error())
				return
			}
		}

		// 下载完成：刷新模型列表，让新模型出现在下拉框
		a.refreshModelsAfterDownload()

		zlog.Info().Str("file", mainBase).Msg("[modelhub] 模型下载完成")

		// 微软商店认证 10.12.10 修复：下载完成后自动激活新模型（ReloadPresets + 切换加载），
		// 不再要求"重启应用生效"——审核员装完模型即点对话，重启兜底对他们不可见。
		// activateMode 告知前端后续走向：auto=后端自动加载，listed=已入列表可手动切换，restart=需重启（引擎未就绪）。
		activateMode := a.downloadedModelActivateMode()
		a.emitModelDownloadComplete(repoID, mainBase, true, "", activateMode)
		if activateMode == "auto" {
			a.trackedGo(func() {
				a.autoActivateDownloadedModel(mainBase)
			})
		}
	}()

	return nil
}

// downloadSingle 下载单个文件，返回 nil 表示成功、错误表示失败信息。
// 进度通过事件推送。
func (a *App) downloadSingle(prov llm.HubProvider, repoID, fileName, modelsDir string) error {
	url := llm.GetModelDownloadURL(prov, repoID, fileName)
	if url == "" {
		return fmt.Errorf("无法为下载源 %s 构造下载地址", llm.ProviderDisplayName(prov))
	}
	destPath := filepath.Join(modelsDir, fileName)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 探测真实大小：激活 Range 断点续传与完成度校验；探测失败退化为无续传下载（totalSize=0）
	totalSize := llm.ProbeFileSize(ctx, url)
	if st, statErr := os.Stat(destPath); statErr == nil {
		// 已存在同名文件：能确认已完整则跳过；仅是部分文件（上次取消/失败遗留的断点）
		// 则继续往下走 DownloadHubFile 续传。无法确认大小时维持旧行为视为已完成。
		if totalSize <= 0 || st.Size() >= totalSize {
			zlog.Info().Str("file", destPath).Msg("[modelhub] 文件已存在，跳过下载")
			a.emitModelDownloadProgress(llm.ModelDownloadProgress{
				Provider: prov, RepoID: repoID, FilePath: fileName,
				TotalBytes: 0, Downloaded: 0, Status: "completed",
			})
			return nil
		}
	}
	if err := llm.DownloadHubFile(ctx, url, destPath, totalSize, prov, func(p llm.ModelDownloadProgress) {
		a.emitModelDownloadProgress(p)
	}); err != nil {
		return err
	}
	return nil
}

func (a *App) emitModelDownloadProgress(p llm.ModelDownloadProgress) {
	wailsruntime.EventsEmit(a.ctx, EventModelDownloadProgress, p)
}

func (a *App) emitModelDownloadComplete(repoID, file string, success bool, errMsg string, activateMode ...string) {
	payload := map[string]any{
		"repo_id": repoID,
		"file":    file,
		"success": success,
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	// activateMode 仅在成功时携带：告知前端下载完成后的走向（auto/listed/restart）
	if success && len(activateMode) > 0 && activateMode[0] != "" {
		payload["activate"] = activateMode[0]
	}
	wailsruntime.EventsEmit(a.ctx, EventModelDownloadComplete, payload)
}

// refreshModelsAfterDownload 下载完成后刷新模型列表，使新模型对顶部下拉框可见。
// 服务已就绪时走完整 ReloadModels（含后端客户端重载）；否则仅重生成 preset 与内存列表。
func (a *App) refreshModelsAfterDownload() {
	system.InvalidateGGUFCache()
	if a.serverReady.Load() && a.getClient() != nil {
		if err := a.getClient().ReloadModels(a.ctx); err != nil {
			zlog.Warn().Err(err).Msg("[modelhub] ReloadModels failed after download")
		}
	}
	if err := a.generatePresetFile(); err != nil {
		zlog.Warn().Err(err).Msg("[modelhub] regenerate preset file failed after download")
	}
	// models 目录已非空：清除首启的"空模型"标志，让 GetServerStatus 恢复真实状态。
	// 若不清除，GetServerStatus 会一直返回"模型目录为空"，前端 missingModels 粘滞、聊天入口不解锁。
	if a.presetCount() > 0 {
		a.modelsEmpty.Store(false)
	}
}

// presetCount 返回当前内存中的模型预设数量（RLock 保护）。
func (a *App) presetCount() int {
	a.presetsMu.RLock()
	defer a.presetsMu.RUnlock()
	return len(a.presets)
}

// downloadedModelActivateMode 判定刚下载完成的模型如何生效：
//   - "auto"：服务已构建且尚无就绪模型（典型为首次使用）——后端自动 ReloadPresets + 加载新模型；
//   - "listed"：已有就绪模型——ReloadModels 已让新模型进入列表，可随时手动切换，不打断当前会话；
//   - "restart"：服务尚未构建完成（如引擎缺失走启动期下载、startup 提前返回）——保持"重启后生效"兜底。
func (a *App) downloadedModelActivateMode() string {
	if !a.ready.Load() {
		return "restart"
	}
	if a.getServer() == nil || a.getClient() == nil {
		return "restart"
	}
	if a.serverReady.Load() {
		return "listed"
	}
	return "auto"
}

// autoActivateDownloadedModel 下载完成后自动加载新模型，免除"重启应用生效"。
// 微软商店认证 10.12.10（Unusable Feature: Chat with AI）的修复主路径：
// 审核员装完模型即尝试对话，若应用要求重启后模型才会加载，会被判定为功能不可用。
func (a *App) autoActivateDownloadedModel(fileName string) {
	// 加载可能耗时数分钟（trackedGo 独立 goroutine），panic 防护避免拖垮进程
	defer recoverLog("[modelhub] auto activate downloaded model panic")

	if a.getServer() == nil || a.getClient() == nil {
		return
	}
	// 用户在下载收尾期间已手动切换并加载了模型：尊重当前会话，不再强行切到新模型
	if a.serverReady.Load() {
		return
	}

	// 让运行中的路由器重新加载 preset 文件，感知刚下载的模型（启动时 preset 还是空表）
	reloadCtx, reloadCancel := context.WithTimeout(context.Background(), apiTimeoutShort)
	if err := a.getClient().ReloadPresets(reloadCtx); err != nil {
		zlog.Warn().Err(err).Msg("[modelhub] ReloadPresets failed before auto activate")
	}
	reloadCancel()

	modelName := a.findPresetNameByFile(fileName)
	if modelName == "" {
		zlog.Warn().Str("file", fileName).Msg("[modelhub] downloaded model not found in presets, skip auto activate")
		return
	}

	zlog.Info().Str("model", modelName).Str("file", fileName).Msg("[modelhub] auto activating downloaded model")
	result := a.SwitchModel(modelName)
	if result.Error != "" {
		// 失败不弹窗不退出：前端下载横幅会展示错误并保留重启兜底，用户也可手动重试
		zlog.Warn().Str("model", modelName).Str("error", result.Error).
			Msg("[modelhub] auto activate downloaded model failed")
		return
	}
	zlog.Info().Str("model", modelName).Msg("[modelhub] downloaded model activated (no restart needed)")
}

// findPresetNameByFile 按模型文件名（basename）在 preset 列表中定位注册名。
// generatePresetFile 已把 ModelPath 解析为绝对路径，basename 与下载落盘的文件名一致。
func (a *App) findPresetNameByFile(fileName string) string {
	a.presetsMu.RLock()
	defer a.presetsMu.RUnlock()
	for _, p := range a.presets {
		if filepath.Base(p.ModelPath) == fileName {
			return p.Name
		}
	}
	return ""
}

// resolveHubProvider 将前端传来的 provider 字符串校验并转换为 HubProvider。
func resolveHubProvider(provider string) (llm.HubProvider, error) {
	p := llm.HubProvider(strings.TrimSpace(provider))
	if !llm.IsValidHubProvider(p) {
		return "", apperror.Newf(apperror.KindInvalidInput, "不支持的下载源: %q（合法值: modelscope/hfmirror）", provider)
	}
	return p, nil
}

// sanitizeGGUFName 校验模型文件名：取 basename、防目录穿越、限制 .gguf 扩展名。
func sanitizeGGUFName(name string) (string, error) {
	base := filepath.Base(filepath.Clean(name))
	if base == "." || base == string(filepath.Separator) || strings.ContainsAny(base, `/\`) {
		return "", apperror.New(apperror.KindInvalidInput, "非法的模型文件名")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".gguf") {
		return "", apperror.Newf(apperror.KindInvalidInput, "仅支持下载 .gguf 文件: %s", name)
	}
	return base, nil
}
