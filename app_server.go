package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"douya/internal/config"
	"douya/internal/httputil"
	"douya/internal/llm"
	"douya/internal/store"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) buildServerConfig() *llm.ServerConfig {
	cfg := a.getConfig()
	absServerPath := resolvePath(cfg.LlamaServerPath)
	modelsDir := filepath.Join(appDir(), "models")

	sp := system.CalculateSmartParams(a.hwInfo, resolvePath(cfg.ModelPath))
	zlog.Info().Str("models_dir", modelsDir).Int("gpu_layers", sp.GPULayers).Int("threads", sp.Threads).Bool("flash", sp.FlashAttn).Str("cache_k", sp.CacheTypeK).Str("cache_v", sp.CacheTypeV).Bool("mlock", sp.Mlock).Bool("mmproj_offload", sp.MmprojOffload).Msg("[smart-params] params")

	// reasoning_format 不再硬编码设置：
	// llama-server 默认值 COMMON_REASONING_FORMAT_DEEPSEEK 已能正确处理所有模型的思考内容分离
	// （包括 DeepSeek-R1 的 <think> 标签、Gemma 4 的 <|channel>thought 标签、Qwen3 的思考标签等）
	// 仅在用户手动配置时才传值
	reasoningFormat := cfg.ReasoningFormat

	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if _, err := os.Stat(presetPath); err != nil {
		presetPath = ""
	}

	// GPU层数：用户设置优先，否则用智能参数
	gpuLayers := "auto"
	if cfg.GPULayers > 0 {
		gpuLayers = fmt.Sprintf("%d", cfg.GPULayers)
	} else if sp.GPULayers > 0 {
		gpuLayers = fmt.Sprintf("%d", sp.GPULayers)
	}

	// Flash Attention：用户设置优先，支持 on/off/auto 三值
	flashAttn := ""
	if sp.FlashAttn {
		flashAttn = "on" // 智能参数默认
	}
	if cfg.FlashAttn != nil {
		if *cfg.FlashAttn {
			flashAttn = "on"
		} else {
			flashAttn = "off"
		}
	}

	// Mlock：用户设置优先
	mlock := sp.Mlock
	if cfg.Mlock != nil {
		mlock = *cfg.Mlock
	}

	// 线程数：用户设置优先
	threads := sp.Threads
	if cfg.Threads > 0 {
		threads = cfg.Threads
	}

	// Batch Size：用户设置优先
	batchSize := sp.BatchSize
	if cfg.BatchSize > 0 {
		batchSize = cfg.BatchSize
	}
	ubatchSize := sp.UBatchSize
	if cfg.BatchSize > 0 {
		ubatchSize = batchSize / 2
	}

	// 上下文长度：用户设置优先，否则用智能参数
	contextSize := sp.ContextSize
	if cfg.ContextSize > 0 {
		contextSize = cfg.ContextSize
	}

	// SleepIdleSeconds：尊重用户显式设置
	// -1 表示禁用空闲休眠（与 llama.cpp 默认值对齐），0 视为未设置也禁用
	// server.go 中通过 > 0 判断是否传参，所以 -1/0 都不会传 --sleep-idle-seconds
	sleepIdle := cfg.SleepIdleSeconds
	modelsMax := cfg.ModelsMax
	if modelsMax <= 0 {
		modelsMax = 1
	}

	serverCfg := &llm.ServerConfig{
		ModelsDir:              modelsDir,
		ServerPath:             absServerPath,
		Port:                   cfg.Port,
		GPULayers:              gpuLayers,
		Threads:                threads,
		FlashAttn:              flashAttn,
		CacheTypeK:             sp.CacheTypeK,
		CacheTypeV:             sp.CacheTypeV,
		Mlock:                  mlock,
		MmprojAuto:             cfg.MmprojAuto,
		MmprojOffload:          sp.MmprojOffload,
		KVUnified:              cfg.KVUnified,
		CacheIdleSlots:         cfg.CacheIdleSlots,
		CacheRAM:               cfg.CacheRAM,
		ImageMinTokens:         cfg.ImageMinTokens,
		ImageMaxTokens:         cfg.ImageMaxTokens,
		FitTarget:              cfg.FitTarget,
		FitCtx:                 cfg.FitCtx,
		Reasoning:              cfg.Reasoning,
		ReasoningBudget:        cfg.ReasoningBudget,
		ReasoningFormat:        reasoningFormat,
		ReasoningBudgetMessage: cfg.ReasoningBudgetMessage,
		APIBase:                cfg.APIBase,
		AppDir:                 appDir(),
		ModelsPreset:           presetPath,
		ModelsMax:              modelsMax,
		SleepIdleSeconds:       sleepIdle,
		Mmap:                   cfg.Mmap,
		KVOffload:              cfg.KVOffload,
		ContextShift:           cfg.ContextShift,
		MinP:                   cfg.MinP,
		DryMultiplier:          cfg.DryMultiplier,
		DryBase:                cfg.DryBase,
		DryAllowedLength:       cfg.DryAllowedLength,
		DrySequenceBreaker:     cfg.DrySequenceBreaker,
		DryPenaltyLastN:        cfg.DryPenaltyLastN,
		GrpAttnN:               cfg.GrpAttnN,
		GrpAttnW:               cfg.GrpAttnW,
		Jinja:                  cfg.Jinja,
		CachePrompt:            cfg.CachePrompt,
		Metrics:                cfg.Metrics,
		Verbose:                cfg.Verbose,
		SpecDraftThreads:       cfg.SpecDraftThreads,
		SpecDraftThreadsBatch:  cfg.SpecDraftThreadsBatch,
		SpecDefault:            cfg.SpecDefault,
		Device:                 cfg.Device,
		Parallel:               cfg.Parallel,
		SpecType:               cfg.SpecType,
		SpecDraftNMax:          cfg.SpecDraftNMax,
		SpecDraftNMin:          cfg.SpecDraftNMin,
		CacheTypeKDraft:        cfg.CacheTypeKDraft,
		CacheTypeVDraft:        cfg.CacheTypeVDraft,
		SpecNgramModNMin:      cfg.SpecNgramModNMin,
		SpecNgramModNMax:      cfg.SpecNgramModNMax,
		SpecNgramModNMatch:    cfg.SpecNgramModNMatch,
		SpecNgramSimpleSizeN:   cfg.SpecNgramSimpleSizeN,
		SpecNgramSimpleSizeM:   cfg.SpecNgramSimpleSizeM,
		SpecNgramSimpleMinHits: cfg.SpecNgramSimpleMinHits,
		SpecNgramMapKSizeN:     cfg.SpecNgramMapKSizeN,
		SpecNgramMapKSizeM:     cfg.SpecNgramMapKSizeM,
		SpecNgramMapKMinHits:   cfg.SpecNgramMapKMinHits,
		SpecNgramMapK4VSizeN:   cfg.SpecNgramMapK4VSizeN,
		SpecNgramMapK4VSizeM:   cfg.SpecNgramMapK4VSizeM,
		SpecNgramMapK4VMinHits: cfg.SpecNgramMapK4VMinHits,
		LookupCacheStatic:     cfg.LookupCacheStatic,
		LookupCacheDynamic:    cfg.LookupCacheDynamic,
		SpecDraftModel:         cfg.SpecDraftModel,
		Embedding:              true, // 启用 embedding API（RAG 知识库需要）
		Pooling:                "mean", // 聊天模型 pooling=none 不兼容 OAI embedding API
		ExposeServer:           cfg.ExposeServer,
		SwaFull:              cfg.SwaFull,
		CtxCheckpoints:       cfg.CtxCheckpoints,
		CheckpointMinStep:    cfg.CheckpointMinStep,
		Tools:                cfg.Tools,
		PrefillAssistant:     cfg.PrefillAssistant,
		SlotPromptSimilarity: cfg.SlotPromptSimilarity,
		SkipChatParsing:      cfg.SkipChatParsing,
		APIPrefix:            cfg.APIPrefix,
		SimpleIO:             cfg.SimpleIO,
		BatchSize:            batchSize,
		UBatchSize:           ubatchSize,
		ContextSize:          contextSize,
		SlotSavePath:         cfg.SlotSavePath,
		SlotSaveEnabled:      cfg.SlotSaveEnabled,
		CacheReuse:           cfg.CacheReuse,
		SpecDraftNgl:         cfg.SpecDraftNgl,
		SpecDraftDevice:      cfg.SpecDraftDevice,
		SpecDraftPSplit:      cfg.SpecDraftPSplit,
		SpecDraftPMin:        cfg.SpecDraftPMin,
		SpecDraftBackendSampling: cfg.SpecDraftBackendSampling,
		MtmdBatchMaxTokens:   cfg.MtmdBatchMaxTokens,
		AdaptiveTarget:       cfg.AdaptiveTarget,
		AdaptiveDecay:        cfg.AdaptiveDecay,
		Tags:                 cfg.Tags,
		MediaPath:            cfg.MediaPath,
		Offline:              cfg.Offline,
		Repack:               cfg.Repack,
		Agent:                cfg.Agent,
		UIMcpProxy:           cfg.UIMcpProxy,
		BackendSampling:      cfg.BackendSampling,
		SsePingInterval:      cfg.SsePingInterval,
		LoraPaths:            cfg.LoraPaths,
		RerankerModelPath:   cfg.RerankerModelPath,
	}

	if cfg.CacheTypeK != "" {
		serverCfg.CacheTypeK = cfg.CacheTypeK
	}
	if cfg.CacheTypeV != "" {
		serverCfg.CacheTypeV = cfg.CacheTypeV
	}
	if cfg.CacheTypeKDraft != "" {
		serverCfg.CacheTypeKDraft = cfg.CacheTypeKDraft
	}
	if cfg.CacheTypeVDraft != "" {
		serverCfg.CacheTypeVDraft = cfg.CacheTypeVDraft
	}
	if cfg.SpecType == "" && sp.SpecType != "" {
		serverCfg.SpecType = sp.SpecType
		serverCfg.SpecDraftNMax = sp.SpecDraftNMax
		serverCfg.SpecDraftNMin = sp.SpecDraftNMin
		if serverCfg.CacheTypeKDraft == "" {
			serverCfg.CacheTypeKDraft = sp.CacheTypeKDraft
		}
		if serverCfg.CacheTypeVDraft == "" {
			serverCfg.CacheTypeVDraft = sp.CacheTypeVDraft
		}
		if serverCfg.SpecNgramModNMin == 0 && sp.NgramModNMin > 0 {
			serverCfg.SpecNgramModNMin = sp.NgramModNMin
		}
		if serverCfg.SpecNgramModNMax == 0 && sp.NgramModNMax > 0 {
			serverCfg.SpecNgramModNMax = sp.NgramModNMax
		}
		if serverCfg.SpecNgramModNMatch == 0 && sp.NgramModNMatch > 0 {
			serverCfg.SpecNgramModNMatch = sp.NgramModNMatch
		}
	}

	// Eagle3 自动启用：模型支持 Eagle3（Qwen3.5/3.6）且用户配置了 draft 模型，
	// 但未显式设置 SpecType 时，自动启用 draft-eagle3
	// 生活类比：就像检测到你插了耳机就自动切换音频输出到耳机一样
	if serverCfg.SpecType == "" && sp.SupportsEagle3 && cfg.SpecDraftModel != "" {
		serverCfg.SpecType = "draft-eagle3"
		zlog.Info().Str("draft_model", cfg.SpecDraftModel).Msg("[smart-params] Eagle3 supported and draft model configured, auto-enabling draft-eagle3")
	}

	// 推理模式自动推荐：用户未设置时使用智能参数
	// 生活类比：就像你没手动调空调温度时，汽车自动用舒适温度一样
	if serverCfg.Reasoning == "" && sp.ReasoningMode != "" {
		serverCfg.Reasoning = sp.ReasoningMode
	}
	if serverCfg.ReasoningBudget == 0 && sp.ReasoningBudget != 0 {
		serverCfg.ReasoningBudget = sp.ReasoningBudget
	}

	if a.db != nil {
		if a.encKey != nil {
			if key, err := store.GetEncryptedSetting(a.db, "server_api_key", a.encKey); err == nil && key != "" {
				serverCfg.APIKey = key
			}
		} else {
			if key, err := store.GetSetting(a.db, "server_api_key"); err == nil && key != "" {
				serverCfg.APIKey = key
			}
		}
	}

	return serverCfg
}

func (a *App) startServerAndWatch(srv *llm.Server, ctx context.Context) {
	// 推送首次启动进度：准备启动引擎
	runtime.EventsEmit(ctx, "server:switchProgress", map[string]string{
		"stage": "preparing",
	})

	if err := srv.Start(); err != nil {
		zlog.Error().Err(err).Msg("start llama-server failed")
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running:     false,
			ModelReady:  false,
			Error:       fmt.Sprintf("启动 llama-server 失败: %v", err),
		})
		return
	}

	if err := srv.WaitForReady(60e9); err != nil {
		zlog.Error().Err(err).Msg("wait for server ready failed")
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running:     false,
			ModelReady:  false,
			Error:       fmt.Sprintf("llama-server 未就绪: %v", err),
		})
		return
	}

	// 推送首次启动进度：引擎已就绪，准备加载模型
	runtime.EventsEmit(ctx, "server:switchProgress", map[string]string{
		"stage": "loading",
	})

	a.presetsMu.RLock()
	presetsSnapshot := make([]llm.ModelPreset, len(a.presets))
	copy(presetsSnapshot, a.presets)
	a.presetsMu.RUnlock()

	foundDefault := false
	for _, p := range presetsSnapshot {
		if p.Alias == "default" {
			a.currentModelMu.Lock()
			a.currentModelName = p.Name
			a.currentModelMu.Unlock()
			if a.client != nil {
				a.client.SetCurrentModel(p.Name)
			}
			foundDefault = true
			break
		}
	}
	if !foundDefault && len(presetsSnapshot) > 0 {
		a.currentModelMu.Lock()
		a.currentModelName = presetsSnapshot[0].Name
		a.currentModelMu.Unlock()
		if a.client != nil {
			a.client.SetCurrentModel(presetsSnapshot[0].Name)
		}
		a.currentModelMu.RLock()
		zlog.Info().Str("model", a.currentModelName).Msg("[server] no default preset found, using first model")
		a.currentModelMu.RUnlock()
	}

	a.currentModelMu.RLock()
	modelForDetect := a.currentModelName
	a.currentModelMu.RUnlock()

	// 推送首次启动进度：检测模型能力
	runtime.EventsEmit(ctx, "server:switchProgress", map[string]string{
		"stage":       "detecting",
		"targetModel": modelForDetect,
	})

	if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
		zlog.Error().Err(err).Msg("detect model architecture failed")
	}

	// 启动后自动加载默认模型
	if modelForDetect != "" && a.client != nil {
		zlog.Info().Str("model", modelForDetect).Msg("[server] auto-loading default model")
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running:     true,
			ModelReady:  false,
			Switching:   true,
			SwitchingTo: modelForDetect,
		})

		// 在 LoadModel 之前启动 SSE 监听，确保捕获完整加载进度
		sseCancel := a.tryWatchModelLoadProgress(ctx, modelForDetect)
		defer sseCancel()

		loadErr := a.client.LoadModel(ctx, modelForDetect)
		if loadErr != nil && !isAlreadyRunningError(loadErr) {
			// 非预期错误（非 "already running"），报告失败
			zlog.Error().Err(loadErr).Str("model", modelForDetect).Msg("[server] auto-load default model failed")
			runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
				Running:    false,
				ModelReady: false,
				Error:      fmt.Sprintf("默认模型加载失败: %v（可手动切换模型）", loadErr),
			})
		} else {
			// LoadModel 成功或返回 "already running"（模型可能还在 LOADING 状态）
			// 必须等待模型状态变为 loaded 才能认为就绪
			if loadErr != nil {
				zlog.Info().Str("model", modelForDetect).Msg("[server] model is already running/loading, waiting for loaded state")
			}

			// 首次加载进度回调：每 5 次轮询推送一次进度
			lastProgressStage := ""
			progressCallback := func(pollCount int, status string) {
				if pollCount%5 != 1 && status == lastProgressStage {
					return
				}
				lastProgressStage = status
				stage := "loading"
				switch status {
				case "loaded", "sleeping":
					stage = "waiting"
				case "failed":
					stage = "failed"
				case "unloaded":
					stage = "retrying"
				}
				runtime.EventsEmit(ctx, "server:switchProgress", map[string]string{
					"stage":       stage,
					"targetModel": modelForDetect,
				})
			}

			if err := a.client.WaitForModelLoaded(ctx, modelForDetect, 300*time.Second, progressCallback); err != nil {
				// 加载失败，尝试去掉 mmproj 重试（mmproj 不兼容会导致子进程崩溃）
				if a.tryReloadWithoutMmproj(ctx, modelForDetect, progressCallback) {
					// 重试成功
				} else {
					// 重试也失败或不适用，启动后台 goroutine 继续等待
					zlog.Warn().Err(err).Str("model", modelForDetect).Msg("[server] auto-load default model timed out, continuing to wait in background")
					go func() {
						defer func() {
							if r := recover(); r != nil {
								zlog.Error().Interface("panic", r).Str("model", modelForDetect).Msg("model load goroutine panic")
							}
						}()
						// 后台继续等待，不设超时（依赖 WatchWithCallback 检测崩溃）
						if bgErr := a.client.WaitForModelLoaded(ctx, modelForDetect, 600*time.Second, progressCallback); bgErr != nil {
							zlog.Error().Err(bgErr).Str("model", modelForDetect).Msg("[server] auto-load default model background wait also failed")
							// 修复：将 Running 改为 false，与 Error 字段保持语义一致
							// 此前 Running: true 会导致前端 `!status.running && status.error` 条件失效，错误被静默丢弃
							runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
								Running:    false,
								ModelReady: false,
								Error:      fmt.Sprintf("默认模型加载失败: %v（可手动切换模型）", bgErr),
							})
						} else {
							zlog.Info().Str("model", modelForDetect).Msg("[server] default model loaded and ready (background)")
							a.serverReady.Store(true)
							// 模型加载完成后重新检测架构，因为首次检测时 mmproj 可能尚未加载
							if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
								zlog.Warn().Err(err).Msg("[server] re-detect model architecture after background load failed")
							}
							a.emitSwitchSuccess(modelForDetect)
						}
					}()
				}
			} else {
				zlog.Info().Str("model", modelForDetect).Msg("[server] default model loaded and ready")
				a.serverReady.Store(true)
				// 模型加载完成后重新检测架构，因为首次检测时 mmproj 可能尚未加载
				if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
					zlog.Warn().Err(err).Msg("[server] re-detect model architecture after load failed")
				}
				a.emitSwitchSuccess(modelForDetect)
			}
		}
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	a.serverMu.Lock()
	a.watchCancel = watchCancel
	a.serverMu.Unlock()
	go srv.WatchWithCallback(watchCtx, func(status llm.ServerStatus) {
		if a.isSwitching.Load() {
			return
		}
		a.currentModelMu.RLock()
		curModel := a.currentModelName
		a.currentModelMu.RUnlock()
		if status.Running {
			var caps llm.ModelCapabilities
			if a.service != nil {
				caps = a.service.GetModelCapabilities()
			} else {
				caps = llm.ModelCapabilities{TextInput: true}
			}
			status.Capabilities = &caps
			status.CurrentModel = curModel
		} else {
			a.serverReady.Store(false)
		}
		status.ModelReady = a.serverReady.Load()
		runtime.EventsEmit(ctx, "server:status", status)
	}, func() {
		a.currentModelMu.RLock()
		modelForDetect2 := a.currentModelName
		a.currentModelMu.RUnlock()
		if err := a.service.DetectModelArchitectureForModel(modelForDetect2); err != nil {
			zlog.Error().Err(err).Msg("detect model architecture after restart failed")
		}
		// 重启后重新加载当前模型，加载完成后才设置 serverReady
		if modelForDetect2 != "" && a.client != nil {
			zlog.Info().Str("model", modelForDetect2).Msg("[server] reloading model after restart")
			loadErr := a.client.LoadModel(ctx, modelForDetect2)
			if loadErr != nil && !isAlreadyRunningError(loadErr) {
				zlog.Error().Err(loadErr).Str("model", modelForDetect2).Msg("[server] reload model after restart failed")
				runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
					Running:    false,
					ModelReady: false,
					Error:      fmt.Sprintf("重启后模型加载失败: %v", loadErr),
				})
			} else {
				if loadErr != nil {
					zlog.Info().Str("model", modelForDetect2).Msg("[server] model is already running/loading after restart, waiting for loaded state")
				}
				if err := a.client.WaitForModelLoaded(ctx, modelForDetect2, 120*time.Second); err != nil {
					zlog.Error().Err(err).Str("model", modelForDetect2).Msg("[server] reload model wait after restart failed")
					runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
						Running:    false,
						ModelReady: false,
						Error:      fmt.Sprintf("重启后模型加载超时: %v", err),
					})
				} else {
					zlog.Info().Str("model", modelForDetect2).Msg("[server] model reloaded and ready after restart")
					a.serverReady.Store(true)
					// 模型加载完成后重新检测架构，因为首次检测时 mmproj 可能尚未加载
					if err := a.service.DetectModelArchitectureForModel(modelForDetect2); err != nil {
						zlog.Warn().Err(err).Msg("[server] re-detect model architecture after restart load failed")
					}
					runtime.EventsEmit(ctx, "server:status", a.runningStatus())
				}
			}
		} else {
			a.serverReady.Store(true)
		}
	})

	// 路由模式下子进程崩溃监控：主进程不会崩溃，但子进程可能崩溃
	// 定期检查模型状态，如果发现模型从 loaded/sleeping 变为 unloaded/failed，自动重新加载
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		consecutiveCrashes := 0
		const maxConsecutiveCrashes = 3

		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
			}

			// 不在切换中且 serverReady 为 true 时才检查
			if a.isSwitching.Load() || !a.serverReady.Load() {
				continue
			}

			a.currentModelMu.RLock()
			modelName := a.currentModelName
			a.currentModelMu.RUnlock()
			if modelName == "" || a.client == nil {
				continue
			}

			status, err := a.client.GetModelStatus(watchCtx, modelName)
			if err != nil {
				// 查询失败可能是暂时的网络问题，跳过
				continue
			}

			switch status.Status {
			case "loaded", "sleeping":
				// 模型正常运行，重置崩溃计数
				if consecutiveCrashes > 0 {
					zlog.Info().Str("model", modelName).Msg("[router-monitor] model recovered, resetting crash count")
					consecutiveCrashes = 0
				}
			case "unloaded", "failed":
				consecutiveCrashes++
				exitInfo := ""
				if status.ExitCode != 0 {
					exitInfo = fmt.Sprintf(" (exit_code=%d)", status.ExitCode)
				}
				zlog.Warn().
					Str("model", modelName).
					Str("status", status.Status).
					Bool("failed", status.Failed).
					Int("crash_count", consecutiveCrashes).
					Msg("[router-monitor] model became unloaded/failed" + exitInfo)

				// 获取 stderr 诊断信息
				stderrHint := a.getServerStderrHint()
				if stderrHint != "" {
					zlog.Warn().Str("stderr_hint", stderrHint).Msg("[router-monitor] server stderr hint")
				}

				if consecutiveCrashes > maxConsecutiveCrashes {
					zlog.Error().Str("model", modelName).Msg("[router-monitor] model keeps crashing, giving up auto-reload")
					a.serverReady.Store(false)
					runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
					Running:    false,
					ModelReady: false,
					Error:      fmt.Sprintf("模型 %s 反复崩溃，请检查模型文件是否损坏或显存是否充足", modelName),
				})
					continue
				}

				// 自动重新加载模型
				zlog.Info().Str("model", modelName).Msg("[router-monitor] attempting to reload crashed model")
				a.serverReady.Store(false)
				runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
				Running:     false,
				ModelReady:  false,
				Switching:   true,
				SwitchingTo: modelName,
			})

				loadErr := a.client.LoadModel(watchCtx, modelName)
				if loadErr != nil && !isAlreadyRunningError(loadErr) {
					zlog.Error().Err(loadErr).Str("model", modelName).Msg("[router-monitor] reload failed")
					runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
						Running:    false,
						ModelReady: false,
						Error:      fmt.Sprintf("模型重新加载失败: %v", loadErr),
					})
					continue
				}

				if err := a.client.WaitForModelLoaded(watchCtx, modelName, 120*time.Second); err != nil {
					zlog.Error().Err(err).Str("model", modelName).Msg("[router-monitor] reload wait failed")
					runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
						Running:    false,
						ModelReady: false,
						Error:      fmt.Sprintf("模型重新加载超时: %v", err),
					})
				} else {
					zlog.Info().Str("model", modelName).Msg("[router-monitor] model reloaded successfully")
					a.serverReady.Store(true)
					a.emitSwitchSuccess(modelName)
				}
			}
		}
	}()
}

func (a *App) GetServerStatus() llm.ServerStatus {
	a.serverMu.Lock()
	srv := a.server
	a.serverMu.Unlock()
	if srv != nil {
		status := srv.Status()
		if status.Running {
			if a.service != nil {
				caps := a.service.GetModelCapabilities()
				status.Capabilities = &caps
			}
			a.currentModelMu.RLock()
			status.CurrentModel = a.currentModelName
			a.currentModelMu.RUnlock()
		}
		status.ModelReady = a.serverReady.Load()
		if a.isSwitching.Load() {
			status.Switching = true
			a.switchingToMu.RLock()
			status.SwitchingTo = a.switchingTo
			a.switchingToMu.RUnlock()
		}
		return status
	}
	return llm.ServerStatus{Running: false, Error: "server not initialized"}
}

func (a *App) runningStatus() llm.ServerStatus {
	caps := a.service.GetModelCapabilities()
	a.currentModelMu.RLock()
	modelName := a.currentModelName
	a.currentModelMu.RUnlock()
	return llm.ServerStatus{
		Running:      true,
		ModelReady:   a.serverReady.Load(),
		CurrentModel: modelName,
		Capabilities: &caps,
	}
}

func (a *App) StopGeneration() {
	if a.service != nil {
		a.service.StopGeneration()
	}
}

// StopThinking 发送推理控制请求，强制结束当前思考块。
// 用户在流式推理过程中点击"直接回答"按钮时调用。
// 生活类比：就像你在考试时监考老师说"思考时间到，开始答题"，模型会立即结束思考开始输出答案。
func (a *App) StopThinking() error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	if a.service == nil {
		return fmt.Errorf("服务未初始化")
	}
	completionID := a.service.GetCurrentCompletionID()
	if completionID == "" {
		return fmt.Errorf("当前没有正在进行的推理")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.client.StopThinking(ctx, completionID)
}

// SaveSlot 保存当前 slot 的 KV 缓存到磁盘。
// 调用 llama-server 的 POST /slots/{id}?action=save 端点（v9744+ 新格式）。
func (a *App) SaveSlot(slotID int) error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/slots/%d?action=save", a.client.BaseURL(), slotID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("创建保存 slot 请求失败: %w", err)
	}
	a.client.SetAuthHeader(req)

	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("保存 slot 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := httputil.ReadBodyLimited(resp.Body, 1024*1024) // 限制 1MB，错误响应体通常很小
		return fmt.Errorf("保存 slot 返回状态 %d: %s", resp.StatusCode, string(body))
	}

	zlog.Info().Int("slot_id", slotID).Msg("[app] SaveSlot: KV cache saved successfully")
	return nil
}

// RestoreSlot 从磁盘恢复 slot 的 KV 缓存。
// 调用 llama-server 的 POST /slots/{id}?action=restore 端点（v9744+ 新格式）。
func (a *App) RestoreSlot(slotID int) error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/slots/%d?action=restore", a.client.BaseURL(), slotID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("创建恢复 slot 请求失败: %w", err)
	}
	a.client.SetAuthHeader(req)

	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("恢复 slot 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := httputil.ReadBodyLimited(resp.Body, 1024*1024) // 限制 1MB，错误响应体通常很小
		return fmt.Errorf("恢复 slot 返回状态 %d: %s", resp.StatusCode, string(body))
	}

	zlog.Info().Int("slot_id", slotID).Msg("[app] RestoreSlot: KV cache restored successfully")
	return nil
}

// EraseSlot 删除指定 slot 的 KV 缓存文件。
// 调用 llama-server 的 POST /slots/{id}?action=erase 端点（v9744+ 新增）。
func (a *App) EraseSlot(slotID int) error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/slots/%d?action=erase", a.client.BaseURL(), slotID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("创建删除 slot 请求失败: %w", err)
	}
	a.client.SetAuthHeader(req)

	resp, err := a.client.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("删除 slot 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := httputil.ReadBodyLimited(resp.Body, 1024*1024)
		return fmt.Errorf("删除 slot 返回状态 %d: %s", resp.StatusCode, string(body))
	}

	zlog.Info().Int("slot_id", slotID).Msg("[app] EraseSlot: KV cache erased successfully")
	return nil
}

// DeleteModel 删除模型（从 llama-server 的模型列表中移除并卸载）
func (a *App) DeleteModel(modelName string) error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.client.DeleteModel(ctx, modelName)
}

// DownloadModel 触发模型下载（非阻塞，进度通过 /models/sse 跟踪）
func (a *App) DownloadModel(modelName string) error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.client.DownloadModel(ctx, modelName)
}

// CountTokens 估算消息列表的 token 数量
func (a *App) CountTokens(messages []llm.ChatMessage) (int, error) {
	if a.client == nil {
		return 0, fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.client.CountTokens(ctx, messages)
}

// GetLoraAdapters 获取当前加载的 LoRA 适配器列表
func (a *App) GetLoraAdapters() ([]llm.LoraAdapter, error) {
	if a.client == nil {
		return nil, fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.client.GetLoraAdapters(ctx)
}

// SetLoraAdapters 设置 LoRA 适配器（运行时热切换）
func (a *App) SetLoraAdapters(adapters []llm.LoraAdapter) error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.client.SetLoraAdapters(ctx, adapters)
}

// GetSlots 获取所有 slot 的状态信息
func (a *App) GetSlots() ([]llm.SlotInfo, error) {
	if a.client == nil {
		return nil, fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.client.GetSlots(ctx)
}

// Tokenize 对文本进行分词，返回 token ID 列表
func (a *App) Tokenize(text string) ([]int, error) {
	if a.client == nil {
		return nil, fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.client.Tokenize(ctx, text)
}

// ApplyTemplate 对消息列表应用聊天模板，返回格式化后的字符串
func (a *App) ApplyTemplate(messages []llm.ChatMessage) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.client.ApplyTemplate(ctx, messages)
}

// AnthropicMessages 代理 Anthropic Messages API
// 前端传入原始 JSON 请求体字符串，后端透传到 /v1/messages 端点，返回原始 JSON 响应体字符串
func (a *App) AnthropicMessages(body string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	respBody, err := a.client.AnthropicMessages(ctx, []byte(body))
	if err != nil {
		return "", err
	}
	return string(respBody), nil
}

// AnthropicCountTokens 代理 Anthropic token 计数
// 前端传入原始 JSON 请求体字符串，后端透传到 /v1/messages/count_tokens 端点，返回原始 JSON 响应体字符串
func (a *App) AnthropicCountTokens(body string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	respBody, err := a.client.AnthropicCountTokens(ctx, []byte(body))
	if err != nil {
		return "", err
	}
	return string(respBody), nil
}

// BuiltInTools 代理内置工具
// 前端传入原始 JSON 请求体字符串，后端透传到 /tools 端点，返回原始 JSON 响应体字符串
func (a *App) BuiltInTools(body string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	respBody, err := a.client.BuiltInTools(ctx, []byte(body))
	if err != nil {
		return "", err
	}
	return string(respBody), nil
}

// GetServerLogs 获取 llama-server 控制台的最近日志
func (a *App) GetServerLogs() string {
	if a.server == nil {
		return ""
	}
	return a.server.LastOutput()
}

// GetTerminalHistory 获取终端历史日志（纯文本，用于 xterm.js 初始化时回显）
func (a *App) GetTerminalHistory() string {
	if a.server == nil {
		return ""
	}
	return a.server.LastOutput()
}

// ResizeTerminal 调整 ConPTY 终端尺寸（前端 xterm.js 尺寸变化时调用）
func (a *App) ResizeTerminal(cols, rows int) error {
	if a.server == nil {
		return nil
	}
	return a.server.ResizeTerminal(cols, rows)
}

// IsConPTYMode 返回当前是否使用 ConPTY 模式（前端据此决定用 xterm.js 还是文本日志）
func (a *App) IsConPTYMode() bool {
	if a.server == nil {
		return false
	}
	return a.server.IsConPTYMode()
}

func (a *App) GetAvailableModels() ([]llm.ModelOption, error) {
	a.presetsMu.RLock()
	presetsCopy := make([]llm.ModelPreset, len(a.presets))
	copy(presetsCopy, a.presets)
	a.presetsMu.RUnlock()

	options := make([]llm.ModelOption, 0, len(presetsCopy))

	modelStatuses := map[string]string{}
	if a.client != nil && a.serverReady.Load() {
		if models, err := a.client.GetModelsList(a.ctx); err == nil {
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

// tryWatchModelLoadProgress 尝试通过 /models/sse 端点实时监听模型加载进度
// 将进度通过 runtime.EventsEmit 推送到前端（事件名 modelLoadProgress）
// 如果 SSE 连接失败，静默回退到轮询方式，不影响主流程
// 返回一个 cancel 函数，调用方可提前终止 SSE 监听
func (a *App) tryWatchModelLoadProgress(ctx context.Context, modelName string) context.CancelFunc {
	sseCtx, sseCancel := context.WithCancel(ctx)

	go func() {
		defer sseCancel()

		err := a.client.WatchModelLoadProgress(sseCtx, modelName, func(event llm.ModelLoadEvent) {
			// 推送实时加载进度到前端
			runtime.EventsEmit(ctx, "modelLoadProgress", map[string]interface{}{
				"model":    event.Model,
				"status":   event.Status,
				"progress": event.ProgressPercent,
			})
		})

		if err != nil {
			// SSE 连接失败，静默回退到轮询方式
			zlog.Debug().Err(err).Str("model", modelName).Msg("[sse] /models/sse unavailable, falling back to polling")
		}
	}()

	return sseCancel
}

// emitSwitchingStatus emits a server status event indicating a model switch is in progress.
func (a *App) emitSwitchingStatus(modelName string) {
	runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
		Running:     false,
		ModelReady:  false,
		Switching:   true,
		SwitchingTo: modelName,
	})
}

// emitSwitchSuccess emits a server status event indicating the model switch succeeded.
func (a *App) emitSwitchSuccess(modelName string) {
	caps := a.service.GetModelCapabilities()
	runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
		Running:      true,
		ModelReady:   true,
		CurrentModel: modelName,
		Capabilities: &caps,
	})
}

// emitSwitchProgress emits a progress event for model switch.
func (a *App) emitSwitchProgress(stage, targetModel string) {
	runtime.EventsEmit(a.ctx, "server:switchProgress", map[string]interface{}{
		"stage":       stage,
		"targetModel": targetModel,
	})
}

// tryReloadWithoutMmproj 尝试去掉 mmproj 后重新加载模型
// 当模型加载失败（通常因 mmproj 不兼容导致子进程崩溃）时调用
// 返回 true 表示重试成功，模型已加载就绪
func (a *App) tryReloadWithoutMmproj(ctx context.Context, modelName string, progressCallback func(int, string)) bool {
	// 检查该模型是否有 mmproj，如果没有则不适用此重试策略
	if !a.regeneratePresetWithoutMmproj(modelName) {
		return false
	}

	// 通知路由器重新加载 preset 文件
	if err := a.client.ReloadPresets(ctx); err != nil {
		zlog.Warn().Err(err).Msg("[server] failed to reload presets after removing mmproj")
		return false
	}

	// 等待一小段时间让路由器处理 preset 重载
	time.Sleep(2 * time.Second)

	// 重新加载模型（不带 mmproj）
	zlog.Info().Str("model", modelName).Msg("[server] retrying model load without mmproj")
	if err := a.client.LoadModel(ctx, modelName); err != nil && !isAlreadyRunningError(err) {
		zlog.Error().Err(err).Str("model", modelName).Msg("[server] retry load model (without mmproj) failed")
		return false
	}

	// 等待模型加载
	if err := a.client.WaitForModelLoaded(ctx, modelName, 300*time.Second, progressCallback); err != nil {
		zlog.Error().Err(err).Str("model", modelName).Msg("[server] retry load model (without mmproj) timed out")
		return false
	}

	// 重试成功
	zlog.Info().Str("model", modelName).Msg("[server] model loaded successfully without mmproj (text-only mode)")
	a.serverReady.Store(true)
	a.emitSwitchSuccess(modelName)

	// 通知前端多模态不可用
	runtime.EventsEmit(ctx, "server:mmprojUnavailable", map[string]string{
		"model": modelName,
		"hint":  "多模态投影器不兼容，已切换为纯文本模式",
	})

	return true
}

// regeneratePresetWithoutMmproj 重新生成不含指定模型 mmproj 的 preset 文件
// 当模型因 mmproj 不兼容导致子进程崩溃时调用，让模型以纯文本模式加载
// 返回 true 表示成功去掉了 mmproj 并重新生成了 preset 文件
func (a *App) regeneratePresetWithoutMmproj(modelName string) bool {
	a.presetsMu.Lock()
	defer a.presetsMu.Unlock()

	found := false
	for i := range a.presets {
		if a.presets[i].Name == modelName {
			if a.presets[i].MmprojPath == "" {
				// 该模型本来就没有 mmproj，不需要重试
				return false
			}
			zlog.Info().Str("model", modelName).Str("mmproj", a.presets[i].MmprojPath).
				Msg("[preset] removing mmproj from preset due to loading failure, will retry in text-only mode")
			a.presets[i].MmprojPath = ""
			a.presets[i].MmprojVision = false
			a.presets[i].MmprojAudio = false
			a.presets[i].MmprojVideo = false
			found = true
			break
		}
	}

	if !found {
		return false
	}

	// 重新生成 preset 文件
	var globalDefaults map[string]string
	if a.hwInfo != nil {
		defaultModelPath := ""
		if len(a.presets) > 0 {
			defaultModelPath = a.presets[0].ModelPath
			for _, p := range a.presets {
				if p.Alias == "default" {
					defaultModelPath = p.ModelPath
					break
				}
			}
		}
		sp := system.CalculateSmartParams(a.hwInfo, defaultModelPath)
		globalDefaults = map[string]string{
			"ctx-size": fmt.Sprintf("%d", sp.ContextSize),
		}
	}

	content := llm.GeneratePreset(a.presets, globalDefaults)
	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if err := llm.WritePresetFile(presetPath, content); err != nil {
		zlog.Error().Err(err).Msg("[preset] failed to write preset file without mmproj")
		return false
	}

	zlog.Info().Str("path", presetPath).Msg("[preset] regenerated preset file without mmproj")
	return true
}

func (a *App) generatePresetFile() error {
	modelsDir := filepath.Join(appDir(), "models")
	presets, err := llm.ScanModelsDir(modelsDir)
	if err != nil {
		return fmt.Errorf("scan models dir: %w", err)
	}

	if len(presets) == 0 {
		return fmt.Errorf("no models found in %s", modelsDir)
	}

	defaultModelPath := a.getConfig().ModelPath
	llm.SetDefaultAlias(presets, defaultModelPath)

	presetRelPaths := make(map[string]string, len(presets))
	for i := range presets {
		presetRelPaths[presets[i].Name] = presets[i].ModelPath
	}

	for i := range presets {
		absModelPath := resolvePath(presets[i].ModelPath)
		presets[i].ModelPath = absModelPath

		if presets[i].MmprojPath != "" {
			presets[i].MmprojPath = resolvePath(presets[i].MmprojPath)
			var modalities []string
			if presets[i].MmprojVision {
				modalities = append(modalities, "vision")
			}
			if presets[i].MmprojAudio {
				modalities = append(modalities, "audio")
			}
			if presets[i].MmprojVideo {
				modalities = append(modalities, "video")
			}
			zlog.Info().
				Str("model", presets[i].Name).
				Str("mmproj", presets[i].MmprojPath).
				Strs("modalities", modalities).
				Msg("[preset] mmproj detected")
		}
	}

	a.presetsMu.Lock()
	a.presets = presets
	a.presetRelPaths = presetRelPaths
	a.presetsMu.Unlock()

	var globalDefaults map[string]string
	if a.hwInfo != nil {
		defaultModelPath := ""
		if len(presets) > 0 {
			defaultModelPath = presets[0].ModelPath
			for _, p := range presets {
				if p.Alias == "default" {
					defaultModelPath = p.ModelPath
					break
				}
			}
		}
		sp := system.CalculateSmartParams(a.hwInfo, defaultModelPath)
		globalDefaults = map[string]string{
			"ctx-size":      fmt.Sprintf("%d", sp.ContextSize),
			"mmproj-offload": "1",
			"pooling":       "mean",
		}
		zlog.Info().Int("ctx-size", sp.ContextSize).Msg("[preset] global defaults")
	}

	content := llm.GeneratePreset(presets, globalDefaults)
	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if err := llm.WritePresetFile(presetPath, content); err != nil {
		return fmt.Errorf("write preset file: %w", err)
	}

	zlog.Info().Str("path", presetPath).Int("count", len(presets)).Msg("[preset] generated preset file")
	return nil
}

func (a *App) GetModelCapabilities() llm.ModelCapabilities {
	if a.service == nil {
		return llm.ModelCapabilities{TextInput: true}
	}
	return a.service.GetModelCapabilities()
}

func (a *App) GetSmartParams() *SmartParamsInfo {
	info := &SmartParamsInfo{}

	// 硬件信息
	info.Hardware.CPUCores = a.hwInfo.CPUCores
	info.Hardware.HasGPU = a.hwInfo.HasGPU
	info.Hardware.GPUName = a.hwInfo.GPUName
	info.Hardware.GPUVRAMMB = a.hwInfo.GPUVRAMMB

	// 模型元数据
	cfg := a.getConfig()
	modelPath := resolvePath(cfg.ModelPath)
	if modelPath != "" {
		if meta, err := system.ParseGGUFMetadataCached(modelPath); err == nil && meta != nil {
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
		}
	}

	// 智能参数
	sp := system.CalculateSmartParams(a.hwInfo, modelPath)
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

// SwitchModel 切换模型（主流程编排）
func (a *App) SwitchModel(modelName string) SwitchResult {
	// 预检查
	if errMsg := a.switchPreCheck(); errMsg != "" {
		return SwitchResult{Error: errMsg}
	}

	// VRAM 预检查（不阻塞切换，只是提前警告）
	if vramMsg := a.vramPreCheck(modelName); vramMsg != "" {
		zlog.Warn().Str("model", modelName).Str("vram_msg", vramMsg).Msg("[router] VRAM pre-check warning")
		// 注意：VRAM 预检查只是警告，不阻止切换（估算可能不准确）
		// 但将警告信息传递给前端
		runtime.EventsEmit(a.ctx, "server:switchProgress", map[string]interface{}{
			"stage":   "vram-warning",
			"model":   modelName,
			"message": vramMsg,
		})
	}

	// SpecType 兼容性检查（不阻塞切换，只是提前警告）
	if specMsg := a.specTypeCompatCheck(modelName); specMsg != "" {
		zlog.Warn().Str("model", modelName).Str("spec_msg", specMsg).Msg("[router] SpecType compatibility warning")
		runtime.EventsEmit(a.ctx, "server:switchProgress", map[string]interface{}{
			"stage":   "spec-warning",
			"model":   modelName,
			"message": specMsg,
		})
	}

	// 停止当前生成，记录旧模型，设置切换状态
	previousModel := a.switchPrepare(modelName)

	// 加载新模型
	alreadyRunning, loadErr := a.switchLoadModel(modelName)
	if loadErr != "" {
		return a.handleSwitchFailure(modelName, previousModel, loadErr)
	}

	// 等待模型就绪（已运行的模型跳过）
	if !alreadyRunning {
		if waitErr := a.switchWaitReady(modelName); waitErr != "" {
			return a.handleSwitchFailure(modelName, previousModel, waitErr)
		}
	}

	// 完成切换：更新状态、保存配置、检测架构
	return a.switchFinalize(modelName, previousModel)
}

// switchPreCheck 预检查：服务器是否启动、是否正在切换、VRAM 是否足够
func (a *App) switchPreCheck() string {
	if a.server == nil || a.client == nil {
		return "服务器未启动"
	}
	if !a.isSwitching.CompareAndSwap(false, true) {
		return "正在切换模型中，请稍候。"
	}
	return ""
}

// vramPreCheck VRAM 预检查：估算模型 VRAM 需求，与 GPU VRAM 比较
func (a *App) vramPreCheck(modelName string) string {
	a.presetsMu.RLock()
	var preset *llm.ModelPreset
	for i := range a.presets {
		if a.presets[i].Name == modelName {
			preset = &a.presets[i]
			break
		}
	}
	a.presetsMu.RUnlock()
	if preset == nil {
		return "" // 找不到 preset，跳过预检查
	}

	modelPath := resolvePath(preset.ModelPath)
	mmprojPath := ""
	if preset.MmprojPath != "" {
		mmprojPath = resolvePath(preset.MmprojPath)
	}

	estimated := llm.EstimateModelVRAM(modelPath, mmprojPath)
	gpuVRAM, err := llm.GetGPUVRAMBytes()
	if err != nil {
		zlog.Debug().Err(err).Msg("[vram-check] cannot query GPU VRAM, skip pre-check")
		return ""
	}

	zlog.Info().
		Str("model", modelName).
		Str("estimated", llm.FormatBytes(estimated)).
		Str("gpu_vram", llm.FormatBytes(gpuVRAM)).
		Msg("[vram-check] pre-check")

	if estimated > gpuVRAM {
		return fmt.Sprintf("显存不足：模型预估需要 %s，GPU 仅有 %s。建议使用更小的量化版本或关闭视觉模型。",
			llm.FormatBytes(estimated), llm.FormatBytes(gpuVRAM))
	}
	return ""
}

// specTypeCompatCheck SpecType 兼容性检查：当 llama-server 以推测解码模式运行但目标模型不兼容时发出警告
func (a *App) specTypeCompatCheck(modelName string) string {
	// 获取目标模型的 GGUF 元数据
	a.presetsMu.RLock()
	var modelPath string
	for i := range a.presets {
		if a.presets[i].Name == modelName {
			modelPath = resolvePath(a.presets[i].ModelPath)
			break
		}
	}
	a.presetsMu.RUnlock()
	if modelPath == "" {
		return ""
	}

	meta, err := system.ParseGGUFMetadataCached(modelPath)
	if err != nil {
		return ""
	}

	// 获取当前 llama-server 的 SpecType
	currentSpecType := ""
	if a.server != nil {
		currentSpecType = a.server.GetSpecType()
	}

	// 如果当前是 MTP 模式但目标模型不支持 MTP
	if currentSpecType == "draft-mtp" && !meta.HasMTP {
		return fmt.Sprintf("当前服务器以 MTP 模式运行，但 %s 不支持 MTP。切换可能导致短暂崩溃后自动恢复，建议重启应用以获得最佳体验。", modelName)
	}

	return ""
}

// switchPrepare 停止当前生成、记录旧模型、设置切换状态
func (a *App) switchPrepare(modelName string) string {
	if a.service != nil {
		a.service.StopGeneration()
	}

	a.currentModelMu.RLock()
	previousModel := a.currentModelName
	a.currentModelMu.RUnlock()

	a.switchingToMu.Lock()
	a.switchingTo = modelName
	a.switchingToMu.Unlock()

	a.serverReady.Store(false)

	a.emitSwitchingStatus(modelName)
	a.emitSwitchProgress("loading", modelName)

	return previousModel
}

// switchLoadModel 加载模型，返回 (是否已运行, 错误消息)
func (a *App) switchLoadModel(modelName string) (bool, string) {
	loadTimeout := a.calculateLoadTimeout(modelName)
	zlog.Info().Str("model", modelName).Dur("timeout", loadTimeout).Msg("[router] switch model with dynamic timeout")

	// 在 LoadModel 之前启动 SSE 监听，确保捕获完整加载进度
	sseCancel := a.tryWatchModelLoadProgress(a.ctx, modelName)
	defer sseCancel()

	loadErr := a.client.LoadModel(a.ctx, modelName)
	if loadErr == nil {
		// LoadModel 返回 200 仅表示开始加载，需要等待模型真正就绪
		a.emitSwitchProgress("waiting", modelName)

		if waitErr := a.client.WaitForModelLoaded(a.ctx, modelName, loadTimeout); waitErr != nil {
			// 优先检测 OOM/显存/内存不足，返回明确提示
			if oomMsg := a.detectOOMError(); oomMsg != "" {
				return false, oomMsg
			}
			waitErrStr := waitErr.Error()
			stderrHint := a.getServerStderrHint()
			// 根据错误内容分类：崩溃 vs 超时
			isCrash := strings.Contains(waitErrStr, "failed to load") ||
			strings.Contains(waitErrStr, "crashed") ||
			strings.Contains(waitErrStr, "exit_code") ||
			strings.Contains(waitErrStr, "VRAM released") ||
			strings.Contains(waitErrStr, "disappeared from model list")
			if isCrash {
				if stderrHint != "" {
					return false, fmt.Sprintf("模型加载失败: %v\n\n详细信息: %s", waitErr, stderrHint)
				}
				return false, fmt.Sprintf("模型加载失败: %v", waitErr)
			}
			// 真正的超时
			if stderrHint != "" {
				return false, fmt.Sprintf("模型加载超时: %v\n\n详细信息: %s", waitErr, stderrHint)
			}
			return false, fmt.Sprintf("模型加载超时: %v", waitErr)
		}
		return false, ""
	}

	if isAlreadyRunningError(loadErr) {
		// 模型可能还在 LOADING 状态，必须等待状态变为 loaded
		zlog.Info().Str("model", modelName).Msg("[router] model is already running/loading, waiting for loaded state")
		a.emitSwitchProgress("waiting", modelName)

		if waitErr := a.client.WaitForModelLoaded(a.ctx, modelName, loadTimeout); waitErr != nil {
			if oomMsg := a.detectOOMError(); oomMsg != "" {
				return false, oomMsg
			}
			waitErrStr := waitErr.Error()
			stderrHint := a.getServerStderrHint()
			isCrash := strings.Contains(waitErrStr, "failed to load") ||
			strings.Contains(waitErrStr, "crashed") ||
			strings.Contains(waitErrStr, "exit_code") ||
			strings.Contains(waitErrStr, "VRAM released") ||
			strings.Contains(waitErrStr, "disappeared from model list")
			if isCrash {
				if stderrHint != "" {
					return false, fmt.Sprintf("模型加载失败: %v\n\n详细信息: %s", waitErr, stderrHint)
				}
				return false, fmt.Sprintf("模型加载失败: %v", waitErr)
			}
			if stderrHint != "" {
				return false, fmt.Sprintf("模型加载超时: %v\n\n详细信息: %s", waitErr, stderrHint)
			}
			return false, fmt.Sprintf("模型加载超时: %v", waitErr)
		}
		return true, ""
	}

	// LoadModel 本身失败，也检测 OOM
	if oomMsg := a.detectOOMError(); oomMsg != "" {
		return false, oomMsg
	}
	// 从 server stderr 获取详细错误信息
	stderrHint := a.getServerStderrHint()
	if stderrHint != "" {
		return false, fmt.Sprintf("模型加载失败: %v\n\n详细信息: %s", loadErr, stderrHint)
	}
	return false, fmt.Sprintf("模型加载失败: %v", loadErr)
}

// getServerStderrHint 从 llama-server 的 stderr 缓冲区提取关键错误信息
func (a *App) getServerStderrHint() string {
	if a.server == nil {
		return ""
	}
	stderr := a.server.LastOutput()
	if stderr == "" {
		return ""
	}
	// 提取关键错误行
	lines := strings.Split(stderr, "\n")
	var hints []string
	keywords := []string{"error", "failed", "OOM", "CUDA", "VRAM", "out of memory", "cannot", "invalid", "unsupported"}
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				hints = append(hints, strings.TrimSpace(line))
				break
			}
		}
	}
	if len(hints) == 0 {
		return ""
	}
	// 最多返回 3 条关键信息
	if len(hints) > 3 {
		hints = hints[len(hints)-3:]
	}
	return strings.Join(hints, "\n")
}

// detectOOMError 检测 stderr 中是否包含 OOM/显存/内存不足错误
// 返回明确的中文错误提示，若非 OOM 则返回空字符串
func (a *App) detectOOMError() string {
	if a.server == nil {
		return ""
	}
	stderr := a.server.LastOutput()
	if stderr == "" {
		return ""
	}
	lower := strings.ToLower(stderr)

	// CUDA 显存不足模式
	cudaOOMPatterns := []string{
		"cuda error", "cuda_error_out_of_memory", "out of memory",
		"failed to allocate cuda", "failed to allocate gpu",
		"gpu memory", "vram", "not enough gpu memory",
	}
	for _, p := range cudaOOMPatterns {
		if strings.Contains(lower, p) {
			hint := a.getServerStderrHint()
			return fmt.Sprintf("显存不足：模型加载需要的显存超过了 GPU 可用显存。\n详细信息: %s", hint)
		}
	}

	// 系统内存不足模式
	ramOOMPatterns := []string{
		"bad allocation", "cannot allocate memory", "mmap failed",
		"std::bad_alloc", "memory allocation failed",
	}
	for _, p := range ramOOMPatterns {
		if strings.Contains(lower, p) {
			hint := a.getServerStderrHint()
			return fmt.Sprintf("内存不足：系统内存不足以加载模型，可能是物理内存不足或交换空间不够。\n详细信息: %s", hint)
		}
	}

	return ""
}

// calculateLoadTimeout 根据模型文件大小动态计算加载超时
// 基础 180 秒 + 每GB 30秒，上限 600 秒（10分钟）
func (a *App) calculateLoadTimeout(modelName string) time.Duration {
	const (
		baseTimeout = 180 * time.Second
		perGB       = 30 * time.Second
		maxTimeout  = 600 * time.Second // 10分钟上限，避免前端长时间卡死
	)

	fileSize := a.getModelFileSize(modelName)
	if fileSize <= 0 {
		// 无法获取大小时，使用保守的 300 秒（与首次加载一致）
		return 300 * time.Second
	}

	fileSizeGB := float64(fileSize) / (1024 * 1024 * 1024)
	timeout := baseTimeout + time.Duration(fileSizeGB*float64(perGB))
	if timeout > maxTimeout {
		timeout = maxTimeout
	}
	return timeout
}

// getModelFileSize 获取模型文件大小（字节）
func (a *App) getModelFileSize(modelName string) int64 {
	a.presetsMu.RLock()
	defer a.presetsMu.RUnlock()
	for _, p := range a.presets {
		if p.Name == modelName {
			if info, err := os.Stat(p.ModelPath); err == nil {
				return info.Size()
			}
		}
	}
	return 0
}

// switchWaitReady 等待模型就绪（含 mmproj 回退检测）
// 注意：不需要在此调用 WaitForModelLoaded，因为调用方 switchLoadModel 已确保模型加载完成
func (a *App) switchWaitReady(modelName string) string {
	// 等待 mmproj 等后加载初始化完成
	// 使用指数退避：200ms → 300ms → 500ms → 800ms
	propsCtx, propsCancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer propsCancel()
	backoffs := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 600 * time.Millisecond, 800 * time.Millisecond, time.Second}
	var lastProps *llm.ServerProps
	for i := 0; i < 10; i++ {
		props, propsErr := a.client.GetServerProps(propsCtx, modelName)
		if propsErr == nil {
			lastProps = props
			// 不需要 mmproj — 立即退出
			if !props.Modalities.Vision && !props.Modalities.Audio {
				break
			}
			// mmproj 已加载 — 退出
			if i > 0 {
				break
			}
			// 首次成功但有 mmproj — 可能仍在加载，再检查一次
			continue
		}
		select {
		case <-propsCtx.Done():
			return ""
		case <-time.After(backoffs[min(i, len(backoffs)-1)]):
		}
	}
	// 缓存 props 结果，供 DetectModelArchitectureForModel 复用
	if lastProps != nil {
		a.service.SetCachedProps(lastProps)
	}

	return ""
}

// switchFinalize 完成切换：更新模型名、保存配置、检测架构、发射事件
func (a *App) switchFinalize(modelName, previousModel string) SwitchResult {
	// 更新当前模型名
	a.currentModelMu.Lock()
	a.currentModelName = modelName
	a.currentModelMu.Unlock()
	// 同步更新 client 的当前模型（v9744+ API 需要）
	if a.client != nil {
		a.client.SetCurrentModel(modelName)
	}

	// 更新嵌入模型名（仅在未配置专用嵌入模型时跟随聊天模型切换）
	if a.ragEmbedder != nil && a.getConfig().EmbeddingModel == "" {
		a.ragEmbedder.SetModel(modelName)
	}

	// 保存配置
	a.presetsMu.RLock()
	relPath, hasRelPath := a.presetRelPaths[modelName]
	a.presetsMu.RUnlock()
	if hasRelPath {
		a.configMu.Lock()
		a.config.ModelPath = relPath
		cfg := a.config
		a.configMu.Unlock()
		if err := config.Save(filepath.Join(appDir(), "config.json"), cfg); err != nil {
			zlog.Error().Err(err).Msg("[router] save config after model switch failed")
			runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
				Running:      true,
				ModelReady:   true,
				CurrentModel: modelName,
				Error:        fmt.Sprintf("config save failed, model may revert on restart: %v", err),
			})
		}
	}

	// 检测模型架构
	a.service.SetDetectedModelName(modelName)
	if err := a.service.DetectModelArchitectureForModel(modelName); err != nil {
		zlog.Error().Err(err).Msg("[router] detect model architecture after switch failed")
		runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
			Running:      true,
			ModelReady:   true,
			CurrentModel: modelName,
			Error:        fmt.Sprintf("模型架构检测失败: %v", err),
		})
	}

	// 模型切换后重置 LoRA 适配器为未应用状态（scale=0）
	// 用户可在设置界面重新启用需要的适配器
	if a.client != nil {
		loraCtx, loraCancel := context.WithTimeout(a.ctx, 5*time.Second)
		if adapters, err := a.client.GetLoraAdapters(loraCtx); err == nil && len(adapters) > 0 {
			// 将所有适配器的 scale 设为 0（保留列表，不删除）
			for i := range adapters {
				adapters[i].Scale = 0
			}
			if err := a.client.SetLoraAdapters(loraCtx, adapters); err != nil {
				zlog.Warn().Err(err).Msg("[router] failed to reset lora adapters after model switch")
			} else {
				zlog.Info().Int("count", len(adapters)).Msg("[router] lora adapters reset to scale=0 after model switch")
			}
		}
		loraCancel()
	}

	// 发射完成事件
	a.emitSwitchProgress("done", modelName)

	// 在清除切换状态之前发射成功事件
	// 确保前端在 WatchWithCallback 发出过时状态之前收到成功事件
	a.emitSwitchSuccess(modelName)
	a.serverReady.Store(true)

	// 清除切换状态
	a.isSwitching.Store(false)
	a.switchingToMu.Lock()
	a.switchingTo = ""
	a.switchingToMu.Unlock()

	zlog.Info().Str("model", modelName).Str("previous", previousModel).Msg("[router] model switched")

	a.currentModelMu.RLock()
	resultModel := a.currentModelName
	a.currentModelMu.RUnlock()
	caps := a.service.GetModelCapabilities()
	return SwitchResult{
		Success:       true,
		CurrentModel:  resultModel,
		Capabilities:  &caps,
		PreviousModel: previousModel,
	}
}

// handleSwitchFailure 处理模型切换失败：尝试恢复旧模型，清理状态，返回错误结果
func (a *App) handleSwitchFailure(modelName, previousModel, errMsg string) SwitchResult {
	zlog.Error().Str("error", errMsg).Msg("[router] model switch failed")
	a.emitSwitchProgress("failed", modelName)

	// 注意：isSwitching 在回滚完成后再清除，防止回滚期间用户发起新切换
	a.switchingToMu.Lock()
	a.switchingTo = ""
	a.switchingToMu.Unlock()

	rollbackSuccess := false
	if previousModel != "" && previousModel != modelName {
		zlog.Info().Str("model", previousModel).Msg("[router] attempting to restore model")
		restoreCtx, restoreCancel := context.WithTimeout(a.ctx, 30*time.Second)
		if restoreErr := a.client.LoadModel(restoreCtx, previousModel); restoreErr == nil {
			_ = a.client.WaitForModelLoaded(restoreCtx, previousModel, 30*time.Second)
			a.currentModelMu.Lock()
			a.currentModelName = previousModel
			a.currentModelMu.Unlock()
			a.emitSwitchSuccess(previousModel)
			a.serverReady.Store(true)
			rollbackSuccess = true
		} else {
			zlog.Error().Err(restoreErr).Str("model", previousModel).Msg("[router] failed to restore model")
			runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
				Running:    false,
				ModelReady: false,
				Error:      fmt.Sprintf("%s，恢复旧模型也失败", errMsg),
			})
		}
		restoreCancel()
	} else {
		runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
			Running:    false,
			ModelReady: false,
			Error:      errMsg,
		})
	}

	// 回滚完成后再清除 isSwitching
	a.isSwitching.Store(false)

	return SwitchResult{
		Error:           errMsg,
		PreviousModel:   previousModel,
		RolledBack:      previousModel != "" && previousModel != modelName,
		RollbackSuccess: rollbackSuccess,
	}
}

func (a *App) ReloadModels() error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	if err := a.client.ReloadModels(a.ctx); err != nil {
		return fmt.Errorf("热重载模型列表失败: %w", err)
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
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "already running") ||
		strings.Contains(errMsg, "already loaded")
}
