package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"douya/internal/apperror"
	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"

	zlog "github.com/rs/zerolog/log"
)

// getConfig 在读锁保护下获取 config 指针快照，调用方仅用于读取字段，不应修改返回值。
// nil 兜底：极端情况下（如配置尚未加载完成）返回默认配置副本，避免调用方解引用 panic。
func (a *App) getConfig() *config.Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	if a.config == nil {
		return config.DefaultConfig()
	}
	return a.config
}

// setConfig 在写锁保护下整体替换 config 指针。
func (a *App) setConfig(cfg *config.Config) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.config = cfg
}

// updateConfig 在写锁保护下复制-修改-替换 config 指针。
// mutate 对副本做修改；若 mutate 返回错误则不提交替换。
//
// 生活类比：员工改档案时先把原件复印一份，改好后确认无误再替换原件；
// 修改过程不对原件产生任何影响。
//
// P3.5 重构：统一 6 处重复的"copy *cfg → mutate → setConfig"模式。
func (a *App) updateConfig(mutate func(cfg *config.Config) error) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if a.config == nil {
		a.config = config.DefaultConfig()
	}
	newCfg := *a.config
	if err := mutate(&newCfg); err != nil {
		return err
	}
	a.config = &newCfg
	return nil
}

func (a *App) GetConfig() *config.Config {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if a.config == nil {
		cfgPath := filepath.Join(appDir(), "config.json")
		cfg, err := config.Load(cfgPath)
		if err != nil || cfg == nil {
			cfg = config.DefaultConfig()
		}
		a.config = cfg
	}
	return a.config
}

func (a *App) UpdateConfig(cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return apperror.Wrap(apperror.KindInvalidConfig, "配置验证失败", err)
	}

	// 检测性能模式是否变化：性能模式影响 smartparams（ctx-size 等），
	// 变化后需要重新生成 router-preset.ini 以保持一致性
	oldPerformanceMode := a.getConfig().PerformanceMode
	performanceModeChanged := oldPerformanceMode != cfg.PerformanceMode

	if a.service != nil {
		a.service.UpdateConfig(cfg)
	}
	a.setConfig(cfg)

	a.setClient(llm.NewClient(cfg.APIBase, a.getServerAPIKey()))

	searchChain := a.buildSearchChain()

	if a.service != nil {
		a.service.UpdateClient(a.getClient())
		a.service.UpdateSearchChain(searchChain)
	}

	// 性能模式变化时异步重新生成 preset 文件（不阻塞配置保存）
	// 修复一致性缺口：原实现 preset 只在启动/重载模型时生成，切换性能模式后 preset 中的
	// ctx-size 等参数与实际启动参数不一致
	if performanceModeChanged && a.service != nil {
		go func() {
			// 防止 panic 导致整个进程崩溃（preset 生成涉及文件 IO）
			defer recoverLog("[config] 性能模式 preset 生成 goroutine panic")
			if err := a.generatePresetFile(); err != nil {
				zlog.Warn().Err(err).Msg("[config] 性能模式切换后重新生成 preset 文件失败")
			} else {
				zlog.Info().Str("old", oldPerformanceMode).Str("new", cfg.PerformanceMode).
					Msg("[config] 性能模式切换，preset 文件已重新生成")
			}
		}()
	}

	return config.Save(filepath.Join(appDir(), "config.json"), cfg)
}

// HasServerAPIKey 返回是否已设置 API Key（不暴露实际密钥值给前端）
func (a *App) HasServerAPIKey() bool {
	return a.getServerAPIKey() != ""
}

// getServerAPIKey 内部方法，获取实际的 API Key 值
// 当 ServerAPIKeyEnabled 为 false 时返回空字符串，不发送 API Key
func (a *App) getServerAPIKey() string {
	if !a.getConfig().ServerAPIKeyEnabled {
		return ""
	}
	// 优先尝试加密读取（兼容已加密数据），失败时回退到明文读取（兼容旧版明文数据）
	// 生活类比：先试着用钥匙打开加密快递柜，没钥匙或柜里没东西时，再去普通货架找
	var value string
	if v, err := a.service.GetEncryptedSetting("server_api_key"); err == nil {
		value = v
	}
	if value == "" {
		if v, err := a.service.GetSetting("server_api_key"); err == nil {
			value = v
		}
	}
	return value
}

func (a *App) SetServerAPIKey(key string) error {
	if err := validateNonEmpty("API Key", key); err != nil {
		return err
	}
	if err := validateStringLength("API Key", key, 256); err != nil {
		return err
	}
	return a.service.SetEncryptedSetting("server_api_key", key)
}

// GetModelParams 读取指定模型的专属生成参数。
// 返回 nil 表示该模型未保存过参数，前端应显示"未保存"状态。
// modelName 为空时返回 nil。
func (a *App) GetModelParams(modelName string) (*chat.ModelParams, error) {
	if a.service == nil {
		return nil, apperror.New(apperror.KindUnavailable, "服务未初始化")
	}
	return a.service.GetModelParams(modelName)
}

// SaveModelParams 将当前全局 Config 中的生成参数保存为指定模型的专属预设。
// 调用后，切换到该模型时会自动恢复这些参数。
// modelName 为空时返回错误。
func (a *App) SaveModelParams(modelName string) error {
	if a.service == nil {
		return apperror.New(apperror.KindUnavailable, "服务未初始化")
	}
	if strings.TrimSpace(modelName) == "" {
		return apperror.New(apperror.KindInvalidInput, "模型名不能为空")
	}
	cfg := a.getConfig()
	if cfg == nil {
		return apperror.New(apperror.KindInternal, "配置未加载")
	}
	params := chat.ModelParamsFromConfig(cfg)
	return a.service.SetModelParams(modelName, params)
}

// ClearModelParams 清除指定模型的专属生成参数。
// 清除后切换到该模型将使用全局默认参数。
func (a *App) ClearModelParams(modelName string) error {
	if a.service == nil {
		return apperror.New(apperror.KindUnavailable, "服务未初始化")
	}
	return a.service.ClearModelParams(modelName)
}

// HasModelParams 检查指定模型是否已保存过专属生成参数。
// 前端用于显示"已保存/未保存"状态标记。
func (a *App) HasModelParams(modelName string) bool {
	if a.service == nil {
		return false
	}
	return a.service.HasModelParams(modelName)
}

// PathCheckResult 启动路径检查的结构化结果
//
// 生活类比：像出门前的行李检查清单，分成"必需品"（runtime）和"重要物品"（models）两类。
// 必需品缺失出不了门（终止启动），重要物品缺失可以出门但影响体验（警告但继续）。
type PathCheckResult struct {
	// RuntimeMissing runtime 目录缺失的文件列表（每项格式 "类型: 路径"）
	// 非空表示 AI 推理引擎不完整，必须修复才能启动
	RuntimeMissing []string
	// ModelsDir 模型目录路径
	ModelsDir string
	// ModelsEmpty models 目录是否为空（无 .gguf 文件）
	// 为 true 时警告但不阻止启动，允许用户进入应用后续下载模型
	ModelsEmpty bool
	// ModelsDirMissing models 目录本身不存在（比"空"更严重，但仍不阻止启动）
	ModelsDirMissing bool
}

// HasRuntimeIssues 是否存在 runtime 问题（需要终止启动）
func (r PathCheckResult) HasRuntimeIssues() bool {
	return len(r.RuntimeMissing) > 0
}

// HasModelIssues 是否存在 models 问题（警告但不终止）
func (r PathCheckResult) HasModelIssues() bool {
	return r.ModelsEmpty || r.ModelsDirMissing
}

// validatePaths 检查启动所需的关键文件是否缺失，返回分类的结构化结果。
//
// 检查范围：
//   - runtime/ 目录：引擎 exe、6 个核心 DLL、3 个 CUDA 运行时 DLL（仅当 ggml-cuda.dll 存在时）
//   - models/ 目录：是否存在、是否含有 .gguf 模型文件
//
// 生活类比：
//   - runtime 是"发动机舱"——里面有发动机(llama-server.exe)和传动系统(DLL)，缺一不可
//   - models 是"油箱"——空了车也能点火，但跑不起来，需要用户去"加油"（下载模型）
func (a *App) validatePaths() PathCheckResult {
	result := PathCheckResult{}
	baseDir := appDir()

	// ===== 1. 检查 runtime 目录完整性 =====
	cfg := a.getConfig()

	// 解析后端类型：优先用 startup 中缓存的解析结果，否则重新解析
	// 生活类比：检查发动机舱前，先确定这车装的是什么型号的发动机
	resolvedBackend, serverPath := a.resolvedBackendSnapshot()
	if resolvedBackend == "" {
		resolvedBackend = llm.ResolveBackendType(a.hwInfo, cfg.BackendType)
	}

	// 解析 serverPath：优先用 startup 中缓存的路径，否则从配置路径解析
	if serverPath == "" {
		serverPath = resolvePath(cfg.LlamaServerPath)
	}
	if _, err := os.Stat(serverPath); err != nil {
		result.RuntimeMissing = append(result.RuntimeMissing, fmt.Sprintf("引擎程序: %s", serverPath))
	}

	runtimeDir := filepath.Join(baseDir, "runtime")
	if info, err := os.Stat(runtimeDir); err != nil || !info.IsDir() {
		result.RuntimeMissing = append(result.RuntimeMissing, fmt.Sprintf("运行时目录: %s", runtimeDir))
	} else {
		// 根据后端类型校验 DLL
		// 生活类比：不同型号的发动机需要的零件不同，按型号清单逐一检查
		backendInfo := llm.GetBackendInfo(resolvedBackend)

		// DLL 检查目录：runtime/{subdir}/（每个后端都在自己的子目录下）
		// 生活类比：每种发动机都有自己的专属车位，去对应车位检查零件即可
		dllDir := runtimeDir
		if backendInfo.Subdir != "" {
			dllDir = filepath.Join(runtimeDir, backendInfo.Subdir)
		}

		// 校验 RequiredDLLs（核心 DLL + 后端专属 DLL）—— 阻断级检查
		// 注意：coreDLLs 中含 glob 模式条目（如 ggml-cpu*.dll），用于同时适配
		// 自编译版（ggml-cpu.dll）和官方预编译包（ggml-cpu-haswell.dll 等架构特定 DLL）。
		// 含 "*" 的条目用 checkDLLFound 检查至少匹配一个文件，缺失则加入 RuntimeMissing 阻断启动。
		for _, dll := range backendInfo.RequiredDLLs {
			if !checkDLLFound(dllDir, dll) {
				result.RuntimeMissing = append(result.RuntimeMissing, fmt.Sprintf("后端DLL: %s", filepath.Join(dllDir, dll)))
			}
		}

		// 校验 VendorDLLs（厂商运行时 DLL，如 CUDA 的 cudart/cublas）—— 阻断级检查
		// 这些 DLL 来自官方预编译包附带（zip 包内含完整运行时），缺失说明后端包不完整。
		// 生活类比：快递箱里没附电池，说明包裹不完整，直接重新下单（下载），
		// 不去隔壁仓库（系统 PATH）翻找，逻辑简单直接。
		// VendorDLLs 同样支持 glob 模式（如 cudart64_*.dll 兼容 CUDA 12/13）。
		for _, dll := range backendInfo.VendorDLLs {
			if !checkDLLFound(dllDir, dll) {
				result.RuntimeMissing = append(result.RuntimeMissing,
					fmt.Sprintf("厂商DLL: %s", filepath.Join(dllDir, dll)))
			}
		}
	}

	// ===== 2. 检查 models 目录 =====
	result.ModelsDir = filepath.Join(baseDir, "models")
	if info, err := os.Stat(result.ModelsDir); err != nil || !info.IsDir() {
		// models 目录本身不存在
		result.ModelsDirMissing = true
	} else {
		// models/ 目录存在，检查是否有至少一个 .gguf 模型文件
		// 生活类比：厨房建好了，但里面没有食材也没法做菜
		hasModel := false
		entries, err := os.ReadDir(result.ModelsDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".gguf") {
					hasModel = true
					break
				}
			}
		}
		if !hasModel {
			result.ModelsEmpty = true
		}
	}

	return result
}

// checkDLLFound 检查指定目录下是否存在某个 DLL 文件，支持 glob 模式。
//
// 生活类比：仓库管理员查货——清单上写的可能是精确型号（"M8 螺丝"），
// 也可能是通配型号（"M* 螺丝"，匹配 M6/M8/M10 等）。精确型号直接找，
// 通配型号只要找到任意一款就算通过。
//
// 规则：
//   - 名称含 "*"：用 filepath.Glob 匹配，至少一个结果即视为找到
//   - 名称不含 "*"：用 os.Stat 精确匹配
//
// 参数：
//   - dir: 目标目录
//   - name: DLL 文件名（可含 glob 通配符 *）
//
// 返回：找到返回 true，未找到返回 false
func checkDLLFound(dir, name string) bool {
	fullPath := filepath.Join(dir, name)
	if strings.Contains(name, "*") {
		// glob 模式：至少匹配一个文件
		matches, err := filepath.Glob(fullPath)
		return err == nil && len(matches) > 0
	}
	_, err := os.Stat(fullPath)
	return err == nil
}
