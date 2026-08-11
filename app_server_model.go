package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"douya/internal/apperror"
	"douya/internal/llm"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
)

// DeleteModel 删除模型（从 llama-server 的模型列表中移除并卸载）
func (a *App) DeleteModel(modelName string) error {
	if a.getClient() == nil {
		return apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if err := validateNonEmpty("模型名称", modelName); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutMedium)
	defer cancel()
	return a.getClient().DeleteModel(ctx, modelName)
}

// DownloadModel 触发模型下载（非阻塞，进度通过 /models/sse 跟踪）
func (a *App) DownloadModel(modelName string) error {
	if a.getClient() == nil {
		return apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if err := validateNonEmpty("模型名称", modelName); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutMedium)
	defer cancel()
	return a.getClient().DownloadModel(ctx, modelName)
}

// CountTokens 估算消息列表的 token 数量
func (a *App) CountTokens(messages []llm.ChatMessage) (int, error) {
	if a.getClient() == nil {
		return 0, apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutShort)
	defer cancel()
	return a.getClient().CountTokens(ctx, messages)
}

// GetLoraAdapters 获取当前加载的 LoRA 适配器列表
func (a *App) GetLoraAdapters() ([]llm.LoraAdapter, error) {
	if a.getClient() == nil {
		return nil, apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutShort)
	defer cancel()
	return a.getClient().GetLoraAdapters(ctx)
}

// SetLoraAdapters 设置 LoRA 适配器（运行时热切换）
func (a *App) SetLoraAdapters(adapters []llm.LoraAdapter) error {
	if a.getClient() == nil {
		return apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutMedium)
	defer cancel()
	return a.getClient().SetLoraAdapters(ctx, adapters)
}

// GetLastPromptTokens 返回最近一次请求的 prompt_tokens（来自 llama-server usage）。
// 这是真实的上下文已用 token 数（含系统提示词+历史消息+RAG+搜索结果等）。
// 前端用于持久化显示总上下文 token 用量，而非仅显示输入框文本的 token 数。
func (a *App) GetLastPromptTokens() int {
	if a.service == nil {
		return 0
	}
	return a.service.LastPromptTokens()
}

// ApplyTemplate 对消息列表应用聊天模板，返回格式化后的字符串
func (a *App) ApplyTemplate(messages []llm.ChatMessage) (string, error) {
	if a.getClient() == nil {
		return "", apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if len(messages) == 0 {
		return "", apperror.New(apperror.KindInvalidInput, "消息列表不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutShort)
	defer cancel()
	return a.getClient().ApplyTemplate(ctx, messages)
}

func (a *App) GetModelCapabilities() llm.ModelCapabilities {
	if a.service == nil {
		return llm.ModelCapabilities{TextInput: true}
	}
	return a.service.GetModelCapabilities()
}

func (a *App) GetSmartParams() *SmartParamsInfo {
	info := &SmartParamsInfo{}

	// 硬件信息（nil 防御：initHardware 前被调用时不 panic）
	if a.hwInfo != nil {
		info.Hardware.CPUCores = a.hwInfo.CPUCores
		info.Hardware.HasGPU = a.hwInfo.HasGPU
		info.Hardware.HasCUDABackend = a.hwInfo.HasCUDABackend
		info.Hardware.GPUName = a.hwInfo.GPUName
		info.Hardware.GPUVRAMMB = a.hwInfo.GPUVRAMMB
	}

	// 模型元数据
	cfg := a.getConfig()
	modelPath := resolvePath(cfg.ModelPath)
	var meta *system.GGUFMetadata
	if modelPath != "" {
		if m, err := system.ParseGGUFMetadataCached(modelPath); err == nil && m != nil {
			meta = m
			info.Model.Architecture = meta.Architecture
			info.Model.BlockCount = meta.BlockCount
			info.Model.EmbeddingLength = meta.EmbeddingLength
			info.Model.ContextLength = meta.ContextLength
			info.Model.FileSizeMB = meta.FileSize / 1024 / 1024
			info.Model.ExpertCount = meta.ExpertCount
			info.Model.ExpertUsed = meta.ExpertUsed
			info.Model.HasMTP = meta.HasMTP
			info.Model.HasReasoning = meta.HasReasoning
			info.Model.NParams = meta.NParams
			info.Model.SizeLabel = meta.SizeLabel
			info.Model.FType = meta.FileType
		}
	}

	// 智能参数
	resolvedBackend, _ := a.resolvedBackendSnapshot()
	sp := system.CalculateSmartParams(a.hwInfo, modelPath, string(resolvedBackend), a.getConfig().PerformanceMode)
	info.Params.GPULayers = sp.GPULayers
	info.Params.Threads = sp.Threads
	info.Params.BatchSize = sp.BatchSize
	info.Params.UBatchSize = sp.UBatchSize
	info.Params.FlashAttn = sp.FlashAttn
	info.Params.CacheTypeK = sp.CacheTypeK
	info.Params.CacheTypeV = sp.CacheTypeV
	info.Params.Mlock = sp.Mlock
	info.Params.MmprojOffload = sp.MmprojOffload
	info.Params.ContextSize = sp.ContextSize
	info.Params.SpecType = sp.SpecType
	info.Params.SpecDraftNMax = sp.SpecDraftNMax
	info.Params.SpecDraftNMin = sp.SpecDraftNMin
	info.Params.NgramModNMin = sp.NgramModNMin
	info.Params.NgramModNMax = sp.NgramModNMax
	info.Params.NgramModNMatch = sp.NgramModNMatch

	// 用户覆盖状态
	info.Overrides.GPULayers = cfg.GPULayers > 0
	info.Overrides.FlashAttn = cfg.FlashAttn != nil
	info.Overrides.Mlock = cfg.Mlock != nil
	info.Overrides.Threads = cfg.Threads > 0
	info.Overrides.BatchSize = cfg.BatchSize > 0
	info.Overrides.ContextSize = cfg.ContextSize != 0
	info.Overrides.CacheTypeK = cfg.CacheTypeK != ""
	info.Overrides.CacheTypeV = cfg.CacheTypeV != ""
	info.Overrides.SpecType = cfg.SpecType != ""

	return info
}

func (a *App) GetAvailableModels() ([]llm.ModelOption, error) {
	a.presetsMu.RLock()
	presetsCopy := make([]llm.ModelPreset, len(a.presets))
	copy(presetsCopy, a.presets)
	a.presetsMu.RUnlock()

	options := make([]llm.ModelOption, 0, len(presetsCopy))

	modelStatuses := map[string]string{}
	if a.getClient() != nil && a.serverReady.Load() {
		if models, err := a.getClient().GetModelsList(a.ctx); err == nil {
			for _, m := range models {
				modelStatuses[m.ID] = m.Status
			}
		}
	}

	for _, p := range presetsCopy {
		isDefault := p.Alias == "default"
		fileName := filepath.Base(p.ModelPath)
		isLoaded, status := findModelMatch(p.Name, modelStatuses)
		options = append(options, llm.ModelOption{
			Name:         p.Name,
			ModelPath:    p.ModelPath,
			FileName:     fileName,
			IsDefault:    isDefault,
			IsLoaded:     isLoaded,
			MmprojVision: p.MmprojVision,
			MmprojAudio:  p.MmprojAudio,
			MmprojVideo:  p.MmprojVideo,
			Status:       status,
		})
	}

	return options, nil
}

func (a *App) ReloadModels() error {
	if a.getClient() == nil {
		return apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if err := a.getClient().ReloadModels(a.ctx); err != nil {
		return apperror.Wrap(apperror.KindInternal, "热重载模型列表失败", err)
	}
	system.InvalidateGGUFCache()
	if err := a.generatePresetFile(); err != nil {
		zlog.Error().Err(err).Msg("[reload] regenerate preset file failed")
	}
	return nil
}

// findModelMatch 在模型状态映射中查找匹配的模型
// 先精确匹配，再模糊匹配（排除 "default" 这种太通用的 ID）
func findModelMatch(name string, statuses map[string]string) (bool, string) {
	if status, ok := statuses[name]; ok {
		return true, status
	}
	for id, status := range statuses {
		if llm.FuzzyMatchModelID(id, name) {
			return true, status
		}
	}
	return false, ""
}

func isAlreadyRunningError(err error) bool {
	if err == nil {
		return false
	}
	// 优先用 errors.Is 精准判断（llm.LoadModel 已类型化为 KindConflict）
	if errors.Is(err, apperror.ErrConflict) {
		return true
	}
	// 兜底：字符串匹配，兼容未类型化的错误路径
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "already running") ||
		strings.Contains(errMsg, "already loaded")
}
