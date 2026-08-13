// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

// procGetDiskFreeSpaceExW 是 Windows API GetDiskFreeSpaceExW 的延迟加载过程。
// 用法与 process_windows.go 中的 kernel32 调用方式一致：LazyDLL 延迟加载，
// 首次调用时才加载 kernel32.dll 并解析函数地址。
var procGetDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// llamaServerExe 是 llama.cpp 服务端可执行文件名。
const llamaServerExe = "llama-server.exe"

// EnsureBackendInstalled 确保指定后端已安装到 runtime 目录，返回 llama-server.exe 的绝对路径。
//
// 生活类比：就像提车前要确认发动机已经装好——如果装好了直接开走（返回路径），
// 如果没装，就从仓库（zip 包）里取出零件组装（解压），装好后再开走。
//
// 流程：
//  1. 如果 bt == BackendAuto，返回错误（应先调用 ResolveBackendType 解析成具体后端）
//  2. 检查目标子目录是否已有 llama-server.exe（幂等：已安装则直接返回路径）
//  3. 若未安装，查找对应 zip 包
//  4. 检查磁盘空间（估算：zip 大小 × 2）
//  5. 解压 zip 到目标子目录
//  6. 验证解压后 llama-server.exe 存在
//
// 参数：
//   - bt: 后端类型（不能是 BackendAuto）
//   - runtimeDir: runtime 目录的绝对路径
//   - progressCB: 解压进度回调（可为 nil）
//
// 返回：llama-server.exe 的绝对路径，或错误
func EnsureBackendInstalled(bt BackendType, runtimeDir string, progressCB ExtractProgressFunc) (string, error) {
	// auto 后端未解析前没有具体路径信息，拒绝处理
	if bt == BackendAuto {
		return "", apperror.New(apperror.KindInvalidInput, "BackendAuto 需先通过 ResolveBackendType 解析成具体后端")
	}

	info := GetBackendInfo(bt)
	destDir := filepath.Join(runtimeDir, info.Subdir)
	serverPath := filepath.Join(destDir, llamaServerExe)

	// 步骤 2：幂等检查——目标子目录已有 llama-server.exe 且后端专属 DLL 存在，直接返回
	//
	// 模块化后端（CUDA/Vulkan/SYCL/HIP）的官方包只含后端 DLL，需要先解压 CPU 包作为基础。
	// 如果只检查 llama-server.exe，CPU 包已解压但后端包未解压时会误判为已安装。
	// 因此模块化后端额外检查后端专属 DLL（如 ggml-cuda.dll）是否存在。
	if _, err := os.Stat(serverPath); err == nil {
		if info.BackendDLL != "" {
			// 模块化后端：额外检查后端专属 DLL
			backendDLLPath := filepath.Join(destDir, info.BackendDLL)
			if _, dllErr := os.Stat(backendDLLPath); dllErr != nil {
				// 后端 DLL 不存在，需要继续解压后端包
				log.Debug().
					Str("backend", bt.String()).
					Str("missing_dll", info.BackendDLL).
					Msg("[backend] llama-server.exe 已存在但后端专属 DLL 缺失，继续解压后端包")
			} else {
				log.Debug().
					Str("backend", bt.String()).
					Str("path", serverPath).
					Msg("[backend] 后端已安装（含专属 DLL），跳过解压")
				return serverPath, nil
			}
		} else {
			// 完整后端（CPU/OpenVINO）：llama-server.exe 存在即视为已安装
			log.Debug().
				Str("backend", bt.String()).
				Str("path", serverPath).
				Msg("[backend] 后端已安装，跳过解压")
			return serverPath, nil
		}
	}

	// 步骤 3：查找 zip 包
	zipPath, err := findBackendZip(bt, runtimeDir)
	if err != nil {
		return "", apperror.Wrapf(apperror.KindNotFound, "查找 %s 后端 zip 包失败", err, info.DisplayName)
	}

	// 步骤 4：检查磁盘空间（估算：zip 大小 × 2）
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return "", apperror.Wrap(apperror.KindNotFound, "读取 zip 文件信息失败", err)
	}
	requiredBytes := zipInfo.Size() * 2
	if err := checkDiskSpace(destDir, requiredBytes); err != nil {
		return "", apperror.Wrapf(apperror.KindInternal, "磁盘空间不足，无法解压 %s", err, info.DisplayName)
	}

	// 步骤 5：解压（带进度回调 + 失败重试）
	// P4 改进：原实现解压失败直接删除 zip，网络抖动或磁盘瞬时问题时用户需手动重下。
	// 新逻辑：先验证 zip 完整性（central directory 可读），完整则重试 1 次（可能是临时文件锁），
	// 重试仍失败或 zip 本身损坏才删除。
	log.Info().
		Str("backend", bt.String()).
		Str("zip", zipPath).
		Str("dest", destDir).
		Msg("[backend] 开始解压后端 zip 包")
	if err := extractBackendZipWithRetry(zipPath, destDir, progressCB); err != nil {
		// F-4 修复：解压失败时删除损坏的 zip 文件，避免下次启动反复尝试解压同一文件。
		// 生活类比：收到一个破损的快递包裹，拆不开就扔掉，下次重新下单，
		// 而不是每次经过仓库都尝试拆同一个破损包裹。
		log.Warn().
			Err(err).
			Str("zip", zipPath).
			Msg("[backend] 解压失败（含重试），删除可能损坏的 zip 文件")
		if removeErr := os.Remove(zipPath); removeErr != nil {
			log.Error().Err(removeErr).Str("zip", zipPath).Msg("[backend] 删除损坏 zip 文件失败")
		}
		return "", apperror.Wrapf(apperror.KindInternal, "解压 %s 失败（zip 包可能已损坏，已自动删除，请重新下载）", err, filepath.Base(zipPath))
	}

	// 步骤 6：验证关键文件存在
	// 完整后端（CPU/OpenVINO）：验证 llama-server.exe
	// 模块化后端（CUDA/Vulkan/SYCL/HIP）：验证后端专属 DLL（后端包只含此 DLL）
	verifyPath := serverPath
	if info.BackendDLL != "" {
		verifyPath = filepath.Join(destDir, info.BackendDLL)
	}
	if _, err := os.Stat(verifyPath); err != nil {
		// 同样删除不完整的 zip 包
		log.Warn().
			Err(err).
			Str("zip", zipPath).
			Str("expected", verifyPath).
			Msg("[backend] 解压后关键文件不存在，zip 包可能不完整，删除 zip 文件")
		if removeErr := os.Remove(zipPath); removeErr != nil {
			log.Error().Err(removeErr).Str("zip", zipPath).Msg("[backend] 删除不完整 zip 文件失败")
		}
		return "", apperror.Newf(apperror.KindInternal, "解压完成但 %s 不存在，zip 包可能损坏（已自动删除，请重新下载）", verifyPath)
	}

	log.Info().
		Str("backend", bt.String()).
		Str("path", serverPath).
		Msg("[backend] 后端安装完成")
	return serverPath, nil
}

// EnsureCPUBaseInstalled 确保 CPU 包已解压到指定子目录，作为模块化后端的基础。
//
// 生活类比：模块化后端（CUDA/Vulkan/SYCL/HIP）的官方包就像只含发动机本体的"裸机"，
// 需要一个"底盘"来承载——CPU 包就是那个底盘，提供 llama-server.exe + 核心 DLL。
// 先把底盘装好（解压 CPU 包），再装发动机（解压后端 DLL），车才能跑。
//
// 幂等：目标目录已有 llama-server.exe 则跳过（CPU 基础已就位）。
// 注意：CPU zip 包不会被删除，因为完整后端（CPU 后端）可能也需要它。
//
// 参数：
//   - destSubdir: 目标子目录名（如 "cuda"），CPU 包内容解压到 runtime/{destSubdir}/
//   - runtimeDir: runtime 目录的绝对路径
//   - progressCB: 解压进度回调（可为 nil）
func EnsureCPUBaseInstalled(destSubdir, runtimeDir string, progressCB ExtractProgressFunc) error {
	destDir := filepath.Join(runtimeDir, destSubdir)
	serverPath := filepath.Join(destDir, llamaServerExe)

	// 幂等检查：llama-server.exe 已存在则跳过（CPU 基础包已解压过）
	if _, err := os.Stat(serverPath); err == nil {
		log.Debug().Str("dest", destDir).Msg("[backend] CPU 基础包已就位，跳过")
		return nil
	}

	// 查找 CPU zip 包
	zipPath, err := findBackendZip(BackendCPU, runtimeDir)
	if err != nil {
		return apperror.Wrapf(apperror.KindNotFound, "查找 CPU 基础包失败: %v", err)
	}

	// 检查磁盘空间
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return apperror.Wrap(apperror.KindNotFound, "读取 CPU zip 文件信息失败", err)
	}
	if err := checkDiskSpace(destDir, zipInfo.Size()*2); err != nil {
		return apperror.Wrapf(apperror.KindInternal, "磁盘空间不足，无法解压 CPU 基础包: %v", err)
	}

	// 解压 CPU 包到目标子目录
	log.Info().
		Str("zip", zipPath).
		Str("dest", destDir).
		Msg("[backend] 解压 CPU 基础包到后端子目录")
	if err := extractBackendZipWithRetry(zipPath, destDir, progressCB); err != nil {
		log.Warn().
			Err(err).
			Str("zip", zipPath).
			Msg("[backend] CPU 基础包解压失败，删除可能损坏的 zip")
		if removeErr := os.Remove(zipPath); removeErr != nil {
			log.Error().Err(removeErr).Str("zip", zipPath).Msg("[backend] 删除损坏 CPU zip 失败")
		}
		return apperror.Wrapf(apperror.KindInternal, "解压 CPU 基础包失败: %v", err)
	}

	// 验证 llama-server.exe 存在
	if _, err := os.Stat(serverPath); err != nil {
		log.Warn().
			Err(err).
			Str("zip", zipPath).
			Str("expected", serverPath).
			Msg("[backend] CPU 基础包解压后 llama-server.exe 不存在，删除 zip")
		if removeErr := os.Remove(zipPath); removeErr != nil {
			log.Error().Err(removeErr).Str("zip", zipPath).Msg("[backend] 删除不完整 CPU zip 失败")
		}
		return apperror.Newf(apperror.KindInternal, "CPU 基础包解压后 %s 不存在，zip 包可能损坏", serverPath)
	}

	log.Info().Str("dest", destDir).Msg("[backend] CPU 基础包安装完成")
	return nil
}

// findBackendZip 在 runtimeDir 下查找指定后端的 zip 包。
//
// 生活类比：在仓库里按货品标签（glob 模式）找对应零件箱——
// 比如 CUDA 的标签是 "llama-*-bin-win-cuda-1[23]*-x64.zip"。
//
// 匹配规则：
//   - 使用 filepath.Glob 匹配 GetBackendInfo(bt).ZipPattern
//   - 匹配到多个时，按文件名排序取第一个（版本排序由文件名前缀保证）
//   - 没匹配到时返回明确错误
func findBackendZip(bt BackendType, runtimeDir string) (string, error) {
	info := GetBackendInfo(bt)
	if info.ZipPattern == "" {
		return "", apperror.Newf(apperror.KindInvalidConfig, "后端 %s 没有定义 ZipPattern", info.DisplayName)
	}

	pattern := filepath.Join(runtimeDir, info.ZipPattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", apperror.Wrapf(apperror.KindInternal, "glob 匹配失败 (%s)", err, pattern)
	}

	if len(matches) == 0 {
		return "", apperror.Newf(apperror.KindNotFound, "未找到 %s 后端的 zip 包，请从官方发布页下载", info.DisplayName)
	}

	// 多个匹配时按文件名排序取第一个（版本递增由文件名前缀保证；
	// 同时兼容历史 "b\d+" 与语义版本 "v\d+\.\d+\.\d+" 两种命名）
	first := matches[0]
	if len(matches) > 1 {
		log.Warn().
			Str("pattern", info.ZipPattern).
			Strs("matches", matches).
			Str("selected", first).
			Msg("[backend] 匹配到多个 zip 包，取排序后第一个")
	}
	return first, nil
}

// redundantFilePrefixes 是豆芽不需要的工具 exe 及其 impl.dll 的文件名前缀列表
// 这些工具是 llama.cpp 附带的命令行工具，豆芽只使用 llama-server。
//
// 生活类比：就像买了一台整套工具箱回家装电脑，但实际只用得上螺丝刀（llama-server），
// 其他锤子、扳手、电钻（llama-cli、llama-quantize 等）都是用不到的冗余工具，
// 与其占着车库空间，不如在拆箱时直接扔掉。
var redundantFilePrefixes = []string{
	"llama-batched-bench",
	"llama-bench",
	"llama-cli",
	"llama-completion",
	"llama-fit-params",
	"llama-gemma3-cli",
	"llama-gguf-split",
	"llama-imatrix",
	"llama-llava-cli",
	"llama-minicpmv-cli",
	"llama-mtmd-cli",
	"llama-mtmd-debug",
	"llama-perplexity",
	"llama-quantize",
	"llama-qwen2vl-cli",
	"llama-results",
	"llama-template-analysis",
	"llama-tokenize",
	"llama-tts",
	"llama-cvector-generator",
	"llama-export-lora",
	"ggml-rpc-server",
}

// isRedundantFile 判断 zip 中的文件是否为豆芽不需要的冗余文件。
// 返回 true 表示应跳过不解压，false 表示需要解压。
//
// 生活类比：搬家时只带走日常用的锅碗瓢盆，那些一年用不到一次的专用工具就可以扔掉了。
// 注意 llama.exe 是独立的 llama 工具（冗余），但 llama-server.exe、llama.dll 等必需保留，
// 因此对 llama.exe 采用精确匹配，其他冗余工具按前缀匹配（覆盖 exe 和对应的 impl.dll）。
//
// 参数：
//   - filename: zip 条目的文件名（可含目录路径，如 "path/to/llama-cli.exe"）
func isRedundantFile(filename string) bool {
	// 取文件名（不含目录路径），避免目录路径干扰匹配
	name := filepath.Base(filename)

	// 精确匹配 llama.exe（独立的 llama 工具，不是 llama-server）
	// 不能用前缀匹配，否则会误伤 llama-server.exe、llama.dll 等必需文件
	if name == "llama.exe" {
		return true
	}

	// 前缀匹配冗余工具（覆盖 exe 和对应的 -impl.dll）
	for _, prefix := range redundantFilePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// ExtractProgressFunc 是解压进度回调函数类型。
// current 是已解压文件数，total 是总文件数（不含冗余跳过的文件）。
type ExtractProgressFunc func(current, total int)

// extractBackendZipWithRetry 解压 zip 包，失败时若 zip 完整则重试 1 次。
//
// P4 改进：原 extractBackendZip 失败直接返回，调用方会删除 zip 让用户重下。
// 新逻辑：解压失败时先验证 zip central directory 是否可读（判断 zip 本身是否损坏），
// 若 zip 完整（可能是临时文件锁/磁盘瞬时问题），清理 destDir 后重试 1 次；
// 若 zip 损坏或重试仍失败，返回错误由调用方删除 zip。
//
// 生活类比：拆快递拆不开时，先检查包裹本身是不是破损的（zip 完整性）。
// 如果包裹完好，可能是剪刀卡住了（临时问题），换个剪刀再试一次；
// 如果包裹本身破损，直接拒收（删除 zip）。
func extractBackendZipWithRetry(zipPath, destDir string, progressCB ExtractProgressFunc) error {
	err := extractBackendZip(zipPath, destDir, progressCB)
	if err == nil {
		return nil
	}

	// 解压失败：验证 zip 是否损坏（central directory 是否可读）
	log.Warn().Err(err).Str("zip", zipPath).Msg("[backend] 首次解压失败，验证 zip 完整性")
	zipReader, verifyErr := zip.OpenReader(zipPath)
	if verifyErr != nil {
		// zip 本身损坏（无法打开），不重试
		log.Warn().Err(verifyErr).Str("zip", zipPath).Msg("[backend] zip 文件损坏（无法读取 central directory），不重试")
		return err
	}
	zipReader.Close()

	// zip 完整：清理 destDir 后重试 1 次（可能是临时文件锁或部分写入残留）
	log.Info().Str("zip", zipPath).Str("dest", destDir).Msg("[backend] zip 完整，清理目标目录后重试解压")
	if cleanErr := os.RemoveAll(destDir); cleanErr != nil {
		log.Warn().Err(cleanErr).Str("dest", destDir).Msg("[backend] 清理目标目录失败，尝试直接重试")
	}
	if mkdirErr := os.MkdirAll(destDir, 0o755); mkdirErr != nil {
		return apperror.Wrapf(apperror.KindInternal, "重试前创建目录失败 (原始错误: %v)", mkdirErr, err)
	}

	retryErr := extractBackendZip(zipPath, destDir, progressCB)
	if retryErr == nil {
		log.Info().Str("zip", zipPath).Msg("[backend] 重试解压成功")
		return nil
	}
	// 重试仍失败：返回最后一次错误（包含重试上下文）
	return apperror.Wrapf(apperror.KindInternal, "重试解压仍失败 (首次错误: %v)", retryErr, err)
}

// extractBackendZip 将 zip 包解压到 destDir。
//
// 生活类比：打开零件箱，把里面的零件一个个摆放到指定位置（destDir），
// 同时把不需要的冗余工具（llama-cli、llama-quantize 等）直接丢弃，不占地方。
//
// 安全与幂等设计：
//   - zip slip 防护：清理目标路径并验证其仍在 destDir 下，防止恶意 zip 写到 destDir 之外
//   - 幂等检查：目标文件已存在且大小相同时跳过，避免重复写入
//   - 冗余文件跳过：通过 isRedundantFile 识别并跳过豆芽不需要的工具 exe 及其 impl.dll
//   - 扁平结构假设：llama.cpp 的 zip 包是扁平结构，但代码兼容含子目录路径的 zip
//
// 参数：
//   - zipPath: zip 文件路径
//   - destDir: 解压目标目录（不存在时会创建）
//   - progressCB: 解压进度回调（可为 nil），按文件数计算百分比
func extractBackendZip(zipPath, destDir string, progressCB ExtractProgressFunc) error {
	// 打开 zip 文件
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "打开 zip 失败", err)
	}
	defer r.Close()

	// 创建目标目录（MkdirAll 幂等，已存在不报错）
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return apperror.Wrap(apperror.KindInternal, "创建目标目录失败", err)
	}

	// 清理 destDir 路径，用于后续 zip slip 检查
	cleanDest := filepath.Clean(destDir)

	// 第一遍遍历：统计需要解压的文件总数（排除冗余和目录）
	totalFiles := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if isRedundantFile(f.Name) {
			continue
		}
		totalFiles++
	}

	// skippedRedundant 统计本次解压跳过的冗余文件数
	// 生活类比：搬家时记一下扔掉了多少件旧物，事后心里有数
	skippedRedundant := 0
	extractedFiles := 0

	for _, f := range r.File {
		// 跳过目录条目（MkdirAll 已保证目录存在）
		if f.FileInfo().IsDir() {
			continue
		}

		// 跳过冗余文件（豆芽不需要的工具 exe 及其 impl.dll）
		// 注意：此检查放在 zip slip 之前，避免对冗余文件做无谓的路径构造
		if isRedundantFile(f.Name) {
			skippedRedundant++
			continue
		}

		// 构造目标路径并做 zip slip 安全检查
		destPath := filepath.Join(destDir, f.Name)
		cleanDestPath := filepath.Clean(destPath)
		// 验证清理后的路径仍在 destDir 下（或是 destDir 本身）
		if cleanDestPath != cleanDest &&
			!strings.HasPrefix(cleanDestPath, cleanDest+string(os.PathSeparator)) {
			return apperror.Newf(apperror.KindInvalidInput, "zip slip 检测到：路径 %q 逃逸出目标目录 %q", f.Name, destDir)
		}

		// 幂等检查：目标文件已存在且大小相同时跳过
		if existingInfo, statErr := os.Stat(destPath); statErr == nil {
			if existingInfo.Size() == int64(f.UncompressedSize64) {
				log.Debug().
					Str("file", f.Name).
					Msg("[backend] 文件已存在且大小相同，跳过")
				extractedFiles++
				continue
			}
		}

		// 解压并写入文件
		if err := extractZipFile(f, destPath); err != nil {
			return apperror.Wrapf(apperror.KindInternal, "解压 %s 失败", err, f.Name)
		}
		extractedFiles++

		// 推送解压进度（按文件数计算）
		if progressCB != nil && totalFiles > 0 {
			progressCB(extractedFiles, totalFiles)
		}
	}

	// 解压完成后记录跳过的冗余文件数（仅在有跳过时记录，减少日志噪音）
	if skippedRedundant > 0 {
		log.Debug().
			Int("skipped", skippedRedundant).
			Str("backend", destDir).
			Msg("[backend] 解压时跳过冗余文件")
	}

	return nil
}

// extractZipFile 解压 zip 中的单个文件到 destPath。
// 调用方负责做 zip slip 安全检查和目录创建。
func extractZipFile(f *zip.File, destPath string) error {
	// 打开 zip 内文件
	rc, err := f.Open()
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "打开 zip 内文件失败", err)
	}
	defer rc.Close()

	// 确保父目录存在（兼容含子目录路径的 zip）
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return apperror.Wrap(apperror.KindInternal, "创建父目录失败", err)
	}

	// 创建目标文件（覆盖已存在的文件）
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "创建目标文件失败", err)
	}
	defer out.Close()

	// 拷贝文件内容
	if _, err := io.Copy(out, rc); err != nil {
		return apperror.Wrap(apperror.KindInternal, "写入文件内容失败", err)
	}
	return nil
}

// checkDiskSpace 检查指定路径所在磁盘是否有足够的剩余空间。
//
// 生活类比：装新发动机前，先看看车库还剩多少空间——空间不够就别装了，免得装一半卡住。
//
// 参数：
//   - path: 任意路径（绝对路径或相对路径），函数会取其所在磁盘
//   - requiredBytes: 需要的字节数
//
// 返回：空间足够返回 nil，不足返回错误
func checkDiskSpace(path string, requiredBytes int64) error {
	// GetDiskFreeSpaceExW 要求路径必须存在，否则返回 "path not found"。
	// 对于尚未创建的目标目录，向上查找最近的存在父目录来代表同一磁盘。
	checkPath := path
	for {
		if _, err := os.Stat(checkPath); err == nil {
			break
		}
		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			// 已到达根目录，停止向上查找
			break
		}
		checkPath = parent
	}

	// 路径转换为 UTF-16 指针（Windows API 要求）
	pathPtr, err := syscall.UTF16PtrFromString(checkPath)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "路径转换 UTF-16 失败", err)
	}

	var freeBytesAvailable uint64
	var totalBytes uint64
	var totalFreeBytes uint64

	// Windows API: 获取磁盘剩余空间
	// 返回值 ret=0 表示失败，此时 callErr 包含错误信息
	ret, _, callErr := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return apperror.Wrap(apperror.KindInternal, "获取磁盘剩余空间失败", callErr)
	}

	if int64(freeBytesAvailable) < requiredBytes {
		return apperror.Newf(apperror.KindInternal, "磁盘空间不足：需要 %d 字节，可用 %d 字节", requiredBytes, freeBytesAvailable)
	}

	return nil
}

// IsBackendInstalled 检查指定后端是否已安装到 runtime 目录。
//
// 生活类比：检查发动机是不是已经装在车位上了——看对应子目录有没有 llama-server.exe。
//
// 检查位置：{runtimeDir}/{subdir}/llama-server.exe
func IsBackendInstalled(bt BackendType, runtimeDir string) bool {
	info := GetBackendInfo(bt)
	// auto 后端没有具体子目录，视为未安装
	if info.Subdir == "" {
		return false
	}

	// 检查子目录下的 llama-server.exe
	subdirPath := filepath.Join(runtimeDir, info.Subdir, llamaServerExe)
	if _, err := os.Stat(subdirPath); err == nil {
		return true
	}

	return false
}

// cudartZipPattern 匹配 CUDA 后端附带的 cudart zip 包文件名。
// 全量适配：同时匹配 CUDA 12.x 和 13.x
// 例如：cudart-llama-bin-win-cuda-12.4-x64.zip、cudart-llama-bin-win-cuda-13.3-x64.zip
const cudartZipPattern = "cudart-llama-bin-win-cuda-1[23]*-x64.zip"

// EnsureCudartInstalled 确保 cudart 包已解压到 runtime/cuda/ 目录。
//
// CUDA 后端需要主包（llama-server.exe 等）和 cudart 包（cudart64_*.dll 等厂商 DLL）一起才能运行。
// 主包通过 EnsureBackendInstalled 解压，cudart 包通过本函数解压到同一目录。
//
// 幂等：如果 cudart64_*.dll 已存在，说明 cudart 包已解压，直接返回。
//
// 生活类比：买了电脑后又买了套外设配件包，开机前要先把配件包拆开装上键盘鼠标，
// 否则电脑开机后发现缺键盘鼠标又让你重新买——其实配件就在快递盒里没拆。
//
// 参数：
//   - runtimeDir: runtime 目录的绝对路径
//   - progressCB: 解压进度回调（可为 nil）
func EnsureCudartInstalled(runtimeDir string, progressCB ExtractProgressFunc) error {
	cudaSubdir := filepath.Join(runtimeDir, "cuda")

	// 幂等检查：如果 cudart64_*.dll 已存在，说明 cudart 包已解压
	matches, _ := filepath.Glob(filepath.Join(cudaSubdir, "cudart64_*.dll"))
	if len(matches) > 0 {
		log.Debug().Str("dir", cudaSubdir).Msg("[backend] cudart 包已解压，跳过")
		return nil
	}

	// 查找 cudart zip 包
	pattern := filepath.Join(runtimeDir, cudartZipPattern)
	zipMatches, err := filepath.Glob(pattern)
	if err != nil {
		return apperror.Wrapf(apperror.KindInternal, "glob 匹配 cudart 包失败 (%s)", err, pattern)
	}
	if len(zipMatches) == 0 {
		return apperror.New(apperror.KindNotFound, "未找到 cudart zip 包，请从官方发布页下载")
	}

	zipPath := zipMatches[0]

	// 解压到 cuda 子目录（与主包相同的目录）
	log.Info().
		Str("zip", zipPath).
		Str("dest", cudaSubdir).
		Msg("[backend] 开始解压 cudart zip 包")
	if err := extractBackendZip(zipPath, cudaSubdir, progressCB); err != nil {
		// F-4 修复：解压失败时删除损坏的 zip 文件，避免下次启动反复尝试解压同一文件
		log.Warn().
			Err(err).
			Str("zip", zipPath).
			Msg("[backend] cudart 解压失败，删除可能损坏的 zip 文件")
		if removeErr := os.Remove(zipPath); removeErr != nil {
			log.Error().Err(removeErr).Str("zip", zipPath).Msg("[backend] 删除损坏 cudart zip 文件失败")
		}
		return apperror.Wrap(apperror.KindInternal, "解压 cudart 包失败（zip 包可能已损坏，已自动删除）", err)
	}

	// 验证 cudart64_*.dll 存在
	matches, _ = filepath.Glob(filepath.Join(cudaSubdir, "cudart64_*.dll"))
	if len(matches) == 0 {
		log.Warn().
			Str("zip", zipPath).
			Str("dest", cudaSubdir).
			Msg("[backend] 解压后 cudart64_*.dll 不存在，zip 包可能不完整")
		if removeErr := os.Remove(zipPath); removeErr != nil {
			log.Error().Err(removeErr).Str("zip", zipPath).Msg("[backend] 删除不完整 cudart zip 文件失败")
		}
		return apperror.New(apperror.KindInternal, "解压完成但 cudart64_*.dll 不存在，zip 包可能损坏（已自动删除）")
	}

	log.Info().
		Str("dir", cudaSubdir).
		Msg("[backend] cudart 包安装完成")
	return nil
}
