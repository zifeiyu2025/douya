// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIsRedundantFile_LlamaExe 验证 llama.exe（独立工具）被精确匹配为冗余。
//
// 生活类比：llama.exe 是独立的命令行工具（就像买工具箱送的备用螺丝刀），
// 豆芽只用 llama-server.exe，所以 llama.exe 是冗余的，解压时直接扔掉。
//
// 关键：必须精确匹配，不能用前缀匹配，否则会误伤 llama-server.exe 和 llama.dll。
func TestIsRedundantFile_LlamaExe(t *testing.T) {
	if !isRedundantFile("llama.exe") {
		t.Error("llama.exe 应为冗余文件")
	}
}

// TestIsRedundantFile_LlamaServerExe 验证 llama-server.exe 不是冗余文件。
//
// 这是豆芽的核心可执行文件，绝对不能被误判为冗余。
func TestIsRedundantFile_LlamaServerExe(t *testing.T) {
	if isRedundantFile("llama-server.exe") {
		t.Error("llama-server.exe 不应为冗余文件（豆芽核心可执行文件）")
	}
}

// TestIsRedundantFile_LlamaDLL 验证 llama.dll 等核心 DLL 不是冗余文件。
//
// 这些是 llama-server 运行必需的动态库，误删会导致启动失败。
//
// 特别注意：新版官方预编译包把 ggml-cpu.dll 拆分为架构特定的
// ggml-cpu-haswell.dll、ggml-cpu-alderlake.dll 等，这些也必须保留，
// 不能被冗余过滤逻辑误删。
func TestIsRedundantFile_LlamaDLL(t *testing.T) {
	coreFiles := []string{
		"llama.dll",
		"llama-server-impl.dll",
		"llama-common.dll",
		"ggml.dll",
		"ggml-base.dll",
		"ggml-cuda.dll",
		"ggml-hip.dll",
		"ggml-vulkan.dll",
		"mtmd.dll",
		// 自编译版产出的统一 CPU 库
		"ggml-cpu.dll",
		// 官方预编译包产出的架构特定 CPU 库（共 14 个，列举部分代表）
		"ggml-cpu-haswell.dll",
		"ggml-cpu-alderlake.dll",
		"ggml-cpu-sse42.dll",
		"ggml-cpu-zen4.dll",
		"ggml-cpu-skylakex.dll",
		"ggml-cpu-sapphirerapids.dll",
	}
	for _, f := range coreFiles {
		if isRedundantFile(f) {
			t.Errorf("核心文件 %q 不应被误判为冗余", f)
		}
	}
}

// TestIsRedundantFile_RedundantPrefixes 验证所有冗余前缀都能正确匹配。
//
// 生活类比：llama-cli、llama-quantize 等都是 llama.cpp 附带的命令行工具，
// 豆芽用不到，解压时按前缀匹配直接跳过（覆盖 exe 和对应的 impl.dll）。
func TestIsRedundantFile_RedundantPrefixes(t *testing.T) {
	// 冗余工具的 exe 和 impl.dll 都应被匹配
	redundantCases := []string{
		"llama-batched-bench.exe",
		"llama-bench.exe",
		"llama-cli.exe",
		"llama-completion.exe",
		"llama-fit-params.exe",
		"llama-gemma3-cli.exe",
		"llama-gguf-split.exe",
		"llama-imatrix.exe",
		"llama-llava-cli.exe",
		"llama-minicpmv-cli.exe",
		"llama-mtmd-cli.exe",
		"llama-mtmd-debug.exe",
		"llama-perplexity.exe",
		"llama-quantize.exe",
		"llama-qwen2vl-cli.exe",
		"llama-results.exe",
		"llama-template-analysis.exe",
		"llama-tokenize.exe",
		"llama-tts.exe",
		"llama-cvector-generator.exe",
		"llama-export-lora.exe",
		"ggml-rpc-server.exe",
		// 对应的 impl.dll 也应被匹配
		"llama-cli-impl.dll",
		"llama-quantize-impl.dll",
		"llama-bench-impl.dll",
	}
	for _, f := range redundantCases {
		if !isRedundantFile(f) {
			t.Errorf("冗余文件 %q 应被识别为冗余", f)
		}
	}
}

// TestIsRedundantFile_NormalFiles 验证普通文件不被误判为冗余。
func TestIsRedundantFile_NormalFiles(t *testing.T) {
	normalCases := []string{
		"README.txt",
		"LICENSE",
		"config.json",
		"ggml-common.dll",
		"llama-server.exe",
		"some-random-file.txt",
	}
	for _, f := range normalCases {
		if isRedundantFile(f) {
			t.Errorf("普通文件 %q 不应被误判为冗余", f)
		}
	}
}

// TestIsRedundantFile_WithPath 验证含目录路径的文件名能正确匹配。
//
// zip 包中的文件名可能含目录路径（如 "path/to/llama-cli.exe"），
// isRedundantFile 应取 basename 后再判断，不被目录路径干扰。
func TestIsRedundantFile_WithPath(t *testing.T) {
	cases := []string{
		"subdir/llama-cli.exe",
		"path/to/llama-quantize.exe",
		"deep/nested/dir/llama-bench.exe",
		"subdir/llama.exe",
	}
	for _, f := range cases {
		if !isRedundantFile(f) {
			t.Errorf("含路径的冗余文件 %q 应被识别为冗余", f)
		}
	}

	// 含路径但非冗余的文件
	if isRedundantFile("subdir/llama-server.exe") {
		t.Error("含路径的 llama-server.exe 不应被误判为冗余")
	}
}

// TestExtractBackendZip_NormalExtraction 验证正常 zip 包能完整解压。
//
// 生活类比：打开一个正常零件箱，里面的零件应该全部摆到指定位置。
func TestExtractBackendZip_NormalExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test-backend.zip")
	destDir := filepath.Join(tmpDir, "extracted")

	// 构造测试 zip：包含 llama-server.exe 和核心 DLL
	files := map[string]string{
		"llama-server.exe": "fake exe content",
		"llama.dll":        "fake dll content",
		"ggml.dll":         "fake ggml content",
		"README.txt":       "test readme",
	}
	if err := createTestZip(zipPath, files); err != nil {
		t.Fatalf("创建测试 zip 失败: %v", err)
	}

	if err := extractBackendZip(zipPath, destDir, nil); err != nil {
		t.Fatalf("extractBackendZip 失败: %v", err)
	}

	// 验证所有文件都被解压
	for filename, expectedContent := range files {
		path := filepath.Join(destDir, filename)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("文件 %q 未被解压: %v", filename, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("文件 %q 内容 = %q, want %q", filename, string(content), expectedContent)
		}
	}
}

// TestExtractBackendZip_RedundantSkipped 验证冗余文件被跳过不解压。
//
// 生活类比：搬家时只带走日常用品，那些用不到的专用工具直接扔掉，不占新家的空间。
func TestExtractBackendZip_RedundantSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test-backend.zip")
	destDir := filepath.Join(tmpDir, "extracted")

	// zip 包含冗余文件和必需文件
	files := map[string]string{
		"llama-server.exe":    "core exe",
		"llama.dll":           "core dll",
		"llama-cli.exe":       "redundant",
		"llama-quantize.exe":  "redundant",
		"llama-cli-impl.dll":  "redundant impl",
		"llama-bench.exe":     "redundant",
		"llama.exe":           "redundant standalone",
		"ggml-rpc-server.exe": "redundant",
	}
	if err := createTestZip(zipPath, files); err != nil {
		t.Fatalf("创建测试 zip 失败: %v", err)
	}

	if err := extractBackendZip(zipPath, destDir, nil); err != nil {
		t.Fatalf("extractBackendZip 失败: %v", err)
	}

	// 必需文件应存在
	mustExist := []string{"llama-server.exe", "llama.dll"}
	for _, f := range mustExist {
		path := filepath.Join(destDir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("必需文件 %q 应被解压", f)
		}
	}

	// 冗余文件不应存在
	mustNotExist := []string{
		"llama-cli.exe", "llama-quantize.exe", "llama-cli-impl.dll",
		"llama-bench.exe", "llama.exe", "ggml-rpc-server.exe",
	}
	for _, f := range mustNotExist {
		path := filepath.Join(destDir, f)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("冗余文件 %q 不应被解压", f)
		}
	}
}

// TestExtractBackendZip_Idempotent 验证幂等：已存在且大小相同的文件被跳过。
//
// 生活类比：零件已经摆好了，再次解压不会重复摆放（覆盖）相同零件。
func TestExtractBackendZip_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test-backend.zip")
	destDir := filepath.Join(tmpDir, "extracted")

	files := map[string]string{
		"llama-server.exe": "original content",
	}
	if err := createTestZip(zipPath, files); err != nil {
		t.Fatalf("创建测试 zip 失败: %v", err)
	}

	// 第一次解压
	if err := extractBackendZip(zipPath, destDir, nil); err != nil {
		t.Fatalf("第一次解压失败: %v", err)
	}

	// 手动修改文件内容（模拟用户自定义）
	targetPath := filepath.Join(destDir, "llama-server.exe")
	customContent := "user customized content"
	if err := os.WriteFile(targetPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("修改文件失败: %v", err)
	}

	// 注意：幂等跳过的前提是"大小相同"，这里我们改了内容但大小可能不同
	// 所以改用相同大小但不同内容来验证幂等逻辑
	sameSizeContent := strings.Repeat("X", len("original content"))
	if err := os.WriteFile(targetPath, []byte(sameSizeContent), 0o644); err != nil {
		t.Fatalf("修改文件失败: %v", err)
	}

	// 第二次解压：文件大小相同，应跳过不解压
	if err := extractBackendZip(zipPath, destDir, nil); err != nil {
		t.Fatalf("第二次解压失败: %v", err)
	}

	// 验证内容未被覆盖（仍是 sameSizeContent，不是 original content）
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if string(content) != sameSizeContent {
		t.Errorf("幂等检查失败：文件被覆盖, content = %q, want %q", string(content), sameSizeContent)
	}
}

// TestExtractBackendZip_ZipSlip 验证 zip slip 攻击被拦截。
//
// 生活类比：恶意 zip 包里藏了个"../etc/passwd"这样的路径，试图把文件写到目标目录之外。
// extractBackendZip 必须检测并拒绝这种路径逃逸，防止恶意文件覆盖系统文件。
func TestExtractBackendZip_ZipSlip(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "malicious.zip")
	destDir := filepath.Join(tmpDir, "extracted")

	// 构造恶意 zip：包含逃逸路径
	// 注意：这里直接用低层 zip.Writer 构造，跳过 filepath.Clean 的预处理
	maliciousFiles := map[string]string{
		"../../../etc/evil.exe": "malicious content",
	}
	if err := createTestZip(zipPath, maliciousFiles); err != nil {
		t.Fatalf("创建恶意 zip 失败: %v", err)
	}

	err := extractBackendZip(zipPath, destDir, nil)
	if err == nil {
		t.Fatal("zip slip 攻击应被拦截并返回错误，但 extractBackendZip 返回 nil")
	}
	if !strings.Contains(err.Error(), "zip slip") {
		t.Errorf("错误信息应包含 'zip slip', got: %v", err)
	}

	// 验证恶意文件未写到目标目录之外
	evilPath := filepath.Join(tmpDir, "evil.exe")
	if _, statErr := os.Stat(evilPath); statErr == nil {
		t.Error("恶意文件被写到了目标目录之外，zip slip 防护失效")
	}
}

// TestExtractBackendZip_NestedDirectories 验证含子目录的 zip 能正确解压。
//
// 虽然 llama.cpp 的 zip 通常是扁平结构，但代码兼容含子目录路径的 zip。
func TestExtractBackendZip_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "nested.zip")
	destDir := filepath.Join(tmpDir, "extracted")

	files := map[string]string{
		"llama-server.exe":          "core exe",
		"subdir/extra.dll":          "nested dll",
		"deep/nested/path/file.txt": "deep file",
	}
	if err := createTestZip(zipPath, files); err != nil {
		t.Fatalf("创建测试 zip 失败: %v", err)
	}

	if err := extractBackendZip(zipPath, destDir, nil); err != nil {
		t.Fatalf("extractBackendZip 失败: %v", err)
	}

	// 验证嵌套文件被解压
	nestedPath := filepath.Join(destDir, "subdir", "extra.dll")
	if _, err := os.Stat(nestedPath); err != nil {
		t.Errorf("嵌套文件 subdir/extra.dll 应被解压: %v", err)
	}

	deepPath := filepath.Join(destDir, "deep", "nested", "path", "file.txt")
	if _, err := os.Stat(deepPath); err != nil {
		t.Errorf("深层嵌套文件应被解压: %v", err)
	}
}

// TestEnsureBackendInstalled_AutoRejected 验证 auto 后端被拒绝处理。
//
// auto 后端需要先通过 ResolveBackendType 解析成具体后端，直接调用 EnsureBackendInstalled 应报错。
func TestEnsureBackendInstalled_AutoRejected(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := EnsureBackendInstalled(BackendAuto, tmpDir, nil)
	if err == nil {
		t.Error("BackendAuto 应被拒绝，返回错误")
	}
	if !strings.Contains(err.Error(), "BackendAuto") {
		t.Errorf("错误信息应提及 BackendAuto, got: %v", err)
	}
}

// TestEnsureBackendInstalled_AlreadyInstalled 验证已安装后端直接返回路径。
//
// 生活类比：发动机已经装好了，提车时直接开走，不用再装一遍。
func TestEnsureBackendInstalled_AlreadyInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	// 模拟 CPU 后端已安装：在 runtime/cpu/ 下创建 llama-server.exe
	cpuDir := filepath.Join(tmpDir, "cpu")
	if err := os.MkdirAll(cpuDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	serverPath := filepath.Join(cpuDir, "llama-server.exe")
	if err := os.WriteFile(serverPath, []byte("fake exe"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	got, err := EnsureBackendInstalled(BackendCPU, tmpDir, nil)
	if err != nil {
		t.Fatalf("已安装的 CPU 后端应直接返回, got error: %v", err)
	}
	if got != serverPath {
		t.Errorf("返回路径 = %q, want %q", got, serverPath)
	}
}

// TestEnsureBackendInstalled_CUDAStandardLayout 验证 CUDA 标准布局（runtime/cuda/子目录）。
//
// 新版目录结构：所有后端都在各自子目录下，CUDA 后端位于 runtime/cuda/。
// CUDA 是模块化后端，幂等检查需要同时存在 llama-server.exe 和 ggml-cuda.dll。
func TestEnsureBackendInstalled_CUDAStandardLayout(t *testing.T) {
	tmpDir := t.TempDir()
	// 模拟标准布局：llama-server.exe + ggml-cuda.dll 在 runtime/cuda/ 子目录
	cudaDir := filepath.Join(tmpDir, "cuda")
	if err := os.MkdirAll(cudaDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	cudaServerPath := filepath.Join(cudaDir, "llama-server.exe")
	if err := os.WriteFile(cudaServerPath, []byte("fake exe"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	// CUDA 是模块化后端，幂等检查额外验证后端专属 DLL
	cudaDLLPath := filepath.Join(cudaDir, "ggml-cuda.dll")
	if err := os.WriteFile(cudaDLLPath, []byte("fake dll"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	got, err := EnsureBackendInstalled(BackendCUDA, tmpDir, nil)
	if err != nil {
		t.Fatalf("CUDA 标准布局应直接返回, got error: %v", err)
	}
	if got != cudaServerPath {
		t.Errorf("返回路径 = %q, want %q", got, cudaServerPath)
	}
}

// TestEnsureBackendInstalled_ZipMissing 验证 zip 包缺失时报错。
//
// 生活类比：发动机没装、仓库里也没有零件箱（zip），应明确报错让用户下载。
func TestEnsureBackendInstalled_ZipMissing(t *testing.T) {
	tmpDir := t.TempDir()
	// 不放任何 zip 包，也不放 llama-server.exe
	_, err := EnsureBackendInstalled(BackendCPU, tmpDir, nil)
	if err == nil {
		t.Error("zip 包缺失应返回错误")
	}
	// CPU 后端没有旧版扁平布局兼容，错误信息应提示下载
	if !strings.Contains(err.Error(), "未找到") && !strings.Contains(err.Error(), "zip") {
		t.Errorf("错误信息应提示 zip 包缺失, got: %v", err)
	}
}

// TestEnsureBackendInstalled_FromZip 验证从 zip 包解压安装后端。
//
// 生活类比：发动机没装，但仓库里有零件箱（zip），应自动解压组装。
func TestEnsureBackendInstalled_FromZip(t *testing.T) {
	tmpDir := t.TempDir()
	// 构造 CPU 后端的 zip 包
	zipPath := filepath.Join(tmpDir, "llama-b10166-bin-win-cpu-x64.zip")
	files := map[string]string{
		"llama-server.exe": "fake exe content",
		"llama.dll":        "fake dll",
	}
	if err := createTestZip(zipPath, files); err != nil {
		t.Fatalf("创建测试 zip 失败: %v", err)
	}

	got, err := EnsureBackendInstalled(BackendCPU, tmpDir, nil)
	if err != nil {
		t.Fatalf("从 zip 解压安装失败: %v", err)
	}

	// 验证返回的路径正确
	wantPath := filepath.Join(tmpDir, "cpu", "llama-server.exe")
	if got != wantPath {
		t.Errorf("返回路径 = %q, want %q", got, wantPath)
	}

	// 验证文件确实被解压
	if _, err := os.Stat(got); err != nil {
		t.Errorf("解压后的文件不存在: %s", got)
	}
}

// TestIsBackendInstalled 验证后端安装状态检查。
func TestIsBackendInstalled(t *testing.T) {
	tmpDir := t.TempDir()

	// 初始状态：所有后端都未安装
	if IsBackendInstalled(BackendCPU, tmpDir) {
		t.Error("空 runtime 目录，CPU 后端不应已安装")
	}

	// 安装 CPU 后端：在 runtime/cpu/ 创建 llama-server.exe
	cpuDir := filepath.Join(tmpDir, "cpu")
	if err := os.MkdirAll(cpuDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	cpuServerPath := filepath.Join(cpuDir, "llama-server.exe")
	if err := os.WriteFile(cpuServerPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	// CPU 后端应已安装
	if !IsBackendInstalled(BackendCPU, tmpDir) {
		t.Error("CPU 后端已安装 llama-server.exe，应返回 true")
	}

	// CUDA 后端未安装（无 cuda/ 子目录，无根目录 llama-server.exe）
	if IsBackendInstalled(BackendCUDA, tmpDir) {
		t.Error("CUDA 后端未安装，应返回 false")
	}

	// auto 后端永远返回 false（没有具体子目录）
	if IsBackendInstalled(BackendAuto, tmpDir) {
		t.Error("auto 后端应永远返回 false（无具体子目录）")
	}
}

// TestIsBackendInstalled_CUDAStandardLayout 验证 CUDA 标准布局检测。
//
// 新版目录结构：CUDA 后端在 runtime/cuda/ 子目录下。
func TestIsBackendInstalled_CUDAStandardLayout(t *testing.T) {
	tmpDir := t.TempDir()
	// 在 runtime/cuda/ 子目录放 llama-server.exe（标准布局）
	cudaDir := filepath.Join(tmpDir, "cuda")
	if err := os.MkdirAll(cudaDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	cudaServerPath := filepath.Join(cudaDir, "llama-server.exe")
	if err := os.WriteFile(cudaServerPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	// CUDA 后端应识别标准布局为已安装
	if !IsBackendInstalled(BackendCUDA, tmpDir) {
		t.Error("CUDA 标准布局（runtime/cuda/ 有 llama-server.exe）应返回 true")
	}
}

// TestIsBackendInstalled_RootNotRecognized 验证根目录的 llama-server.exe 不再被识别为已安装。
//
// 旧版扁平布局已废弃：根目录有 llama-server.exe 但子目录没有时，应返回 false。
// 这确保用户必须把后端文件放到对应子目录下。
func TestIsBackendInstalled_RootNotRecognized(t *testing.T) {
	tmpDir := t.TempDir()
	// 在 runtime 根目录直接放 llama-server.exe（旧版扁平布局，已废弃）
	rootServerPath := filepath.Join(tmpDir, "llama-server.exe")
	if err := os.WriteFile(rootServerPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	// CUDA 后端不应再识别根目录的 llama-server.exe 为已安装
	if IsBackendInstalled(BackendCUDA, tmpDir) {
		t.Error("CUDA 后端不应再识别根目录的 llama-server.exe（扁平布局已废弃）")
	}
}

// createTestZip 创建测试用 zip 包。
//
// 参数：
//   - path: zip 文件路径
//   - files: 文件名 → 内容 的映射
func createTestZip(path string, files map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte(content)); err != nil {
			return err
		}
	}
	return nil
}
