// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"
)

// procGetDiskFreeSpaceExW 是 Windows API GetDiskFreeSpaceExW 的延迟加载过程。
// 用法与 process_windows.go 中的 kernel32 调用方式一致：LazyDLL 延迟加载，
// 首次调用时才加载 kernel32.dll 并解析函数地址。
var procGetDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// llamaServerExe 是 llama.cpp 服务端可执行文件名。
const llamaServerExe = "llama-server.exe"

// errCUDAFlatLayout 表示 CUDA 后端使用旧版扁平布局，
// 即 llama-server.exe 直接解压在 runtime 根目录，而非 cuda 子目录。
//
// 生活类比：就像有些老型号发动机直接摆在车库正中间（不分车位），
// 旧版 CUDA 后端也是直接把文件铺在 runtime/ 根目录里，没有单独的 cuda/ 子目录。
// 这不是真正的错误，而是一种兼容旧版的特殊情况，调用方应据此走根目录路径。
var errCUDAFlatLayout = errors.New("CUDA 后端使用旧版扁平布局（直接解压在 runtime 根目录）")

// EnsureBackendInstalled 确保指定后端已安装到 runtime 目录，返回 llama-server.exe 的绝对路径。
//
// 生活类比：就像提车前要确认发动机已经装好——如果装好了直接开走（返回路径），
// 如果没装，就从仓库（zip 包）里取出零件组装（解压），装好后再开走。
//
// 流程：
//  1. 如果 bt == BackendAuto，返回错误（应先调用 ResolveBackendType 解析成具体后端）
//  2. 检查目标目录是否已有 llama-server.exe（幂等：已安装则直接返回路径）
//  3. 对于 CUDA，额外检查 runtime 根目录的旧版扁平布局
//  4. 若未安装，查找对应 zip 包
//  5. 检查磁盘空间（估算：zip 大小 × 2）
//  6. 解压 zip 到目标子目录
//  7. 验证解压后 llama-server.exe 存在
//
// 参数：
//   - bt: 后端类型（不能是 BackendAuto）
//   - runtimeDir: runtime 目录的绝对路径
//
// 返回：llama-server.exe 的绝对路径，或错误
func EnsureBackendInstalled(bt BackendType, runtimeDir string) (string, error) {
	// auto 后端未解析前没有具体路径信息，拒绝处理
	if bt == BackendAuto {
		return "", fmt.Errorf("BackendAuto 需先通过 ResolveBackendType 解析成具体后端")
	}

	info := GetBackendInfo(bt)
	destDir := filepath.Join(runtimeDir, info.Subdir)
	serverPath := filepath.Join(destDir, llamaServerExe)

	// 步骤 2：幂等检查——目标目录已有 llama-server.exe，直接返回
	if _, err := os.Stat(serverPath); err == nil {
		log.Debug().
			Str("backend", bt.String()).
			Str("path", serverPath).
			Msg("[backend] 后端已安装，跳过解压")
		return serverPath, nil
	}

	// 步骤 3：CUDA 旧版扁平布局兼容——runtime 根目录直接有 llama-server.exe
	if bt == BackendCUDA {
		rootServerPath := filepath.Join(runtimeDir, llamaServerExe)
		if _, err := os.Stat(rootServerPath); err == nil {
			log.Info().
				Str("path", rootServerPath).
				Msg("[backend] CUDA 使用旧版扁平布局（runtime 根目录），直接复用")
			return rootServerPath, nil
		}
	}

	// 步骤 4：查找 zip 包
	zipPath, err := findBackendZip(bt, runtimeDir)
	if err != nil {
		if errors.Is(err, errCUDAFlatLayout) {
			// CUDA 旧版扁平布局：findBackendZip 已确认根目录有 llama-server.exe
			rootServerPath := filepath.Join(runtimeDir, llamaServerExe)
			log.Info().
				Str("path", rootServerPath).
				Msg("[backend] CUDA zip 包缺失但根目录已有 llama-server.exe，复用旧版布局")
			return rootServerPath, nil
		}
		return "", fmt.Errorf("查找 %s 后端 zip 包失败: %w", info.DisplayName, err)
	}

	// 步骤 5：检查磁盘空间（估算：zip 大小 × 2）
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return "", fmt.Errorf("读取 zip 文件信息失败: %w", err)
	}
	requiredBytes := zipInfo.Size() * 2
	if err := checkDiskSpace(destDir, requiredBytes); err != nil {
		return "", fmt.Errorf("磁盘空间不足，无法解压 %s: %w", info.DisplayName, err)
	}

	// 步骤 6：解压
	log.Info().
		Str("backend", bt.String()).
		Str("zip", zipPath).
		Str("dest", destDir).
		Msg("[backend] 开始解压后端 zip 包")
	if err := extractBackendZip(zipPath, destDir); err != nil {
		return "", fmt.Errorf("解压 %s 失败: %w", zipPath, err)
	}

	// 步骤 7：验证 llama-server.exe 存在
	if _, err := os.Stat(serverPath); err != nil {
		return "", fmt.Errorf("解压完成但 %s 不存在，zip 包可能损坏", serverPath)
	}

	log.Info().
		Str("backend", bt.String()).
		Str("path", serverPath).
		Msg("[backend] 后端安装完成")
	return serverPath, nil
}

// findBackendZip 在 runtimeDir 下查找指定后端的 zip 包。
//
// 生活类比：在仓库里按货品标签（glob 模式）找对应零件箱——
// 比如 CUDA 的标签是 "llama-b*-bin-win-cuda-cu*-x64.zip"。
//
// 匹配规则：
//   - 使用 filepath.Glob 匹配 GetBackendInfo(bt).ZipPattern
//   - 匹配到多个时，按文件名排序取第一个（版本排序由文件名前缀保证）
//   - 没匹配到时返回明确错误
//
// CUDA 特殊处理：如果 zip 包不存在但 runtime 根目录已有 llama-server.exe
// （旧版直接解压的情况），返回 errCUDAFlatLayout 标记，调用方据此走根目录路径。
func findBackendZip(bt BackendType, runtimeDir string) (string, error) {
	info := GetBackendInfo(bt)
	if info.ZipPattern == "" {
		return "", fmt.Errorf("后端 %s 没有定义 ZipPattern", info.DisplayName)
	}

	pattern := filepath.Join(runtimeDir, info.ZipPattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob 匹配失败 (%s): %w", pattern, err)
	}

	if len(matches) == 0 {
		// CUDA 旧版扁平布局兼容：zip 包缺失但根目录已有 llama-server.exe
		if bt == BackendCUDA {
			rootServerPath := filepath.Join(runtimeDir, llamaServerExe)
			if _, statErr := os.Stat(rootServerPath); statErr == nil {
				log.Info().
					Str("runtime", runtimeDir).
					Msg("[backend] CUDA zip 包缺失，检测到根目录已有 llama-server.exe（旧版扁平布局）")
				return "", errCUDAFlatLayout
			}
		}
		return "", fmt.Errorf("未找到 %s 后端的 zip 包，请从官方发布页下载", info.DisplayName)
	}

	// 多个匹配时按文件名排序取第一个（llama-b* 前缀保证版本递增排序）
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
func extractBackendZip(zipPath, destDir string) error {
	// 打开 zip 文件
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer r.Close()

	// 创建目标目录（MkdirAll 幂等，已存在不报错）
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 清理 destDir 路径，用于后续 zip slip 检查
	cleanDest := filepath.Clean(destDir)

	// skippedRedundant 统计本次解压跳过的冗余文件数
	// 生活类比：搬家时记一下扔掉了多少件旧物，事后心里有数
	skippedRedundant := 0

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
			return fmt.Errorf("zip slip 检测到：路径 %q 逃逸出目标目录 %q", f.Name, destDir)
		}

		// 幂等检查：目标文件已存在且大小相同时跳过
		if existingInfo, statErr := os.Stat(destPath); statErr == nil {
			if existingInfo.Size() == int64(f.UncompressedSize64) {
				log.Debug().
					Str("file", f.Name).
					Msg("[backend] 文件已存在且大小相同，跳过")
				continue
			}
		}

		// 解压并写入文件
		if err := extractZipFile(f, destPath); err != nil {
			return fmt.Errorf("解压 %s 失败: %w", f.Name, err)
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
		return fmt.Errorf("打开 zip 内文件失败: %w", err)
	}
	defer rc.Close()

	// 确保父目录存在（兼容含子目录路径的 zip）
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("创建父目录失败: %w", err)
	}

	// 创建目标文件（覆盖已存在的文件）
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer out.Close()

	// 拷贝文件内容
	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("写入文件内容失败: %w", err)
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
		return fmt.Errorf("路径转换 UTF-16 失败: %w", err)
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
		return fmt.Errorf("获取磁盘剩余空间失败: %w", callErr)
	}

	if int64(freeBytesAvailable) < requiredBytes {
		return fmt.Errorf("磁盘空间不足：需要 %d 字节，可用 %d 字节", requiredBytes, freeBytesAvailable)
	}

	return nil
}

// IsBackendInstalled 检查指定后端是否已安装到 runtime 目录。
//
// 生活类比：检查发动机是不是已经装在车上了——看子目录有没有 llama-server.exe，
// 对于 CUDA 旧版还要看根目录是不是直接放着。
//
// 检查位置：
//   - 标准布局：{runtimeDir}/{subdir}/llama-server.exe
//   - CUDA 旧版扁平布局：{runtimeDir}/llama-server.exe
func IsBackendInstalled(bt BackendType, runtimeDir string) bool {
	info := GetBackendInfo(bt)
	// auto 后端没有具体子目录，视为未安装
	if info.Subdir == "" {
		return false
	}

	// 标准布局：检查子目录下的 llama-server.exe
	subdirPath := filepath.Join(runtimeDir, info.Subdir, llamaServerExe)
	if _, err := os.Stat(subdirPath); err == nil {
		return true
	}

	// CUDA 旧版扁平布局：检查根目录的 llama-server.exe
	if bt == BackendCUDA {
		rootPath := filepath.Join(runtimeDir, llamaServerExe)
		if _, err := os.Stat(rootPath); err == nil {
			return true
		}
	}

	return false
}
