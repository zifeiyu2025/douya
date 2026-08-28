// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/UserExistsError/conpty"
	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

// Start 启动 llama-server 进程。
//
// 拆分说明：原 181 行函数按启动流程拆为调度器 + 4 子函数：
//   - prepareProcessEnv: 构建子进程环境变量（PATH 注入 + 安全变量传递）
//   - bindProcessToJobObject: 将进程绑定到 Job Object（父进程退出时自动 kill 子进程）
//   - startWithExecCmd: exec.Cmd 启动路径（ConPTY 不可用时的回退方案）
//   - startWithConPTYSuccess: ConPTY 启动成功路径（原生终端输出）
//
// 生活类比：就像启动一辆汽车——先检查安全带（API Key 校验），停掉正在运行的引擎（stop existing），
// 准备好燃油（env），然后尝试一键启动（ConPTY）；如果一键启动失败，就用钥匙手动启动（exec.Cmd）。
//
// 注意：调用此函数时不应持有 s.mu，子函数会在返回前释放锁。
func (s *Server) Start() error {
	// 安全校验：开启局域网暴露时必须启用 API Key，防止局域网内未授权设备调用本地算力
	if s.config.ExposeServer && (!s.config.ServerAPIKeyEnabled || s.config.APIKey == "") {
		return apperror.New(apperror.KindInvalidConfig, "开启局域网暴露必须先启用服务 API Key 并设置密钥")
	}
	s.mu.Lock()

	// 如果服务器已在运行，先停止（重启场景）
	if s.status.Running && s.isAlive() {
		log.Info().Msg("stopping existing model server before starting new one...")
		s.mu.Unlock()
		if err := s.stopInternal(); err != nil {
			log.Error().Err(err).Msg("stop existing server before restart")
		}
		s.mu.Lock()
	}

	args := s.buildStartArgs()
	log.Info().Str("server_path", s.config.ServerPath).Strs("args", args).Msg("[server] DIAG full command line")
	// 安全：API Key 通过环境变量传递，而非命令行参数
	// 基于 GO-CONFIG-001 安全实践：避免命令行参数被同权限进程通过 tasklist/WMI 读取
	// 安全加固：仅当项目配置显式开启 ServerAPIKeyEnabled 时才注入 LLAMA_API_KEY
	if s.config.ServerAPIKeyEnabled && s.config.APIKey != "" {
		s.cmdEnv = append(s.cmdEnv, "LLAMA_API_KEY="+s.config.APIKey)
	}

	// llama-server.exe 与 DLL 同目录（runtime/），直接用 exe 所在目录作为工作目录
	runtimeDir := filepath.Dir(s.config.ServerPath)
	env := prepareProcessEnv(runtimeDir, s.cmdEnv, s.config.ServerAPIKeyEnabled)

	s.cmd = exec.Command(s.config.ServerPath, args...)
	s.cmd.Dir = runtimeDir
	// Agent 模式：内置工具（read_file/exec_shell_command 等 uses_cwd）的相对路径
	// 以进程工作目录为基准。用户配置了 AgentCwd 时切到该目录，让 Agent 直接
	// 在用户的项目文件夹里干活；目录无效则保持引擎目录（不阻断启动）。
	if s.config.Agent && s.config.AgentCwd != "" {
		if info, statErr := os.Stat(s.config.AgentCwd); statErr == nil && info.IsDir() {
			s.cmd.Dir = s.config.AgentCwd
		} else {
			s.cmdEnv = append(s.cmdEnv, "LLAMA_AGENT_CWD_INVALID=1")
		}
	}

	s.stderrBuf = NewRingBuffer(500) // 增大缓冲区到 500 行，便于控制台查看历史
	if s.onLog != nil {
		s.stderrBuf.SetOnChange(s.onLog)
	}
	s.lastStartTime.Store(time.Now().UnixNano())

	// P4.5 修复（移除 crashDegradeLevel 重置）：此前此处把崩溃降级级别清零，
	// 会破坏 WatchWithCallback 的降级链升级（1→2）。
	// 流程：崩溃 → 设置 level=1 → Start() 构建参数（应用降级）→ 立即清零 →
	// 再次崩溃时读到 level=0 → 重新设 level=1，永远到不了 level 2（gpu-layers auto）。
	// 真正需要清空的场景：
	//   - 启动成功后重置：已在 WatchWithCallback 重启成功分支处理（server.go）
	//   - 手动/回退重建 server：initServer 每次创建全新 Server（level 从 0 开始）
	// 因此此处无需清零，删掉可让降级链正确升级。

	// 尝试用 ConPTY 启动（获得原生终端输出：ANSI 颜色码、进度条）
	// 生活类比：ConPTY 就像一个"虚拟显示器"，让 llama-server 以为自己在真正的终端里运行
	pty, ptyErr := startWithConPTY(s.config.ServerPath, args, runtimeDir, env, 120, 40)
	if ptyErr != nil {
		log.Warn().Err(ptyErr).Msg("ConPTY unavailable, falling back to exec.Cmd")
		s.pty = nil
		s.startWithExecCmd(args, runtimeDir, env)
		return nil
	}

	// ConPTY 启动成功
	s.startWithConPTYSuccess(pty, args, runtimeDir, env)
	return nil
}

// prepareProcessEnv 构建子进程环境变量。
// 1. 将 runtimeDir 注入 PATH（确保 DLL 可被找到）
// 2. 过滤敏感环境变量（最小权限原则，防止泄露给子进程）
// 3. 追加安全传递的环境变量（如 LLAMA_API_KEY，仅当 ServerAPIKeyEnabled 时）
//
// serverAPIKeyEnabled：控制 LLAMA_API_KEY 是否可从父进程环境透传。
// 当为 false 时，父进程的 LLAMA_API_KEY 将被视为敏感变量并过滤；
// 已配置的 API Key 仍通过 cmdEnv 显式传入（不受此参数影响）。
//
// 安全实践（SEC-003）：原实现仅过滤 PATH，其余环境变量原样继承。
// 改为黑名单过滤敏感前缀（*_SECRET/*_TOKEN/*_PASSWORD/*_CREDENTIAL/*_KEY），
// LLAMA_API_KEY 仅当项目配置显式开启 ServerAPIKeyEnabled 时才允许从父进程透传，
// 已配置的密钥通过 cmdEnv 显式注入。
func prepareProcessEnv(runtimeDir string, cmdEnv []string, serverAPIKeyEnabled bool) []string {
	currentPath := os.Getenv("PATH")
	newPath := runtimeDir
	if currentPath != "" {
		newPath = runtimeDir + ";" + currentPath
	}
	env := os.Environ()
	filtered := make([]string, 0, len(env)+len(cmdEnv)+1)
	for _, e := range env {
		// PATH 单独处理（注入 runtimeDir）
		if strings.HasPrefix(e, "PATH=") {
			continue
		}
		// 过滤敏感环境变量（按前缀匹配 KEY=VALUE 中的 KEY 部分）
		if isSensitiveEnvVar(e, serverAPIKeyEnabled) {
			continue
		}
		filtered = append(filtered, e)
	}
	filtered = append(filtered, "PATH="+newPath)
	// 追加安全传递的环境变量（如 API Key，已由调用方显式传入）
	filtered = append(filtered, cmdEnv...)
	return filtered
}

// sensitiveEnvVarPrefixes 敏感环境变量前缀黑名单。
// 命中任一前缀的环境变量不会传递给 llama-server 子进程。
var sensitiveEnvVarPrefixes = []string{
	"_SECRET=",
	"_TOKEN=",
	"_PASSWORD=",
	"_CREDENTIAL=",
	"_PASSPHRASE=",
	"AWS_SECRET_ACCESS_KEY=",
	"AWS_SESSION_TOKEN=",
	"GH_TOKEN=",
	"GITHUB_TOKEN=",
	"GITLAB_TOKEN=",
	"DOCKER_PASSWORD=",
	"KUBE_TOKEN=",
}

// isSensitiveEnvVar 判断环境变量是否敏感（不传递给子进程）。
// serverAPIKeyEnabled：仅当项目配置明确启用了 ServerAPIKeyEnabled 时，
// LLAMA_API_KEY 才允许从父进程环境透传给 llama-server 子进程。
//
// 过滤策略：
//  1. LLAMA_API_KEY → 受 ServerAPIKeyEnabled 控制
//  2. DOUYA_SKIP_ACL → 应用内部使用，始终过滤（泄露可能导致子进程跳过权限检查）
//  3. 后缀匹配（key 末尾）：_SECRET / _TOKEN / _PASSWORD / _CREDENTIAL / _PASSPHRASE
//     使用 strings.HasSuffix 确保不会出现子串假阳性（如 MY_TOKEN_FORMAT 以 _FORMAT 结尾，不匹配）
//  4. 精确前缀匹配：sensitiveEnvVarPrefixes 列表（每个条目以 = 结尾，确保完整 KEY 名匹配）
//
// 设计取舍：后缀匹配可能导致极少数命名不当的非敏感变量被拦截（如 MY_TOKEN_MAP），
// 但漏传实际的密钥/令牌远比误拦截的风险大，这是有意为之的安全偏保守策略。
//
// 生活类比：就像门禁系统——只有住户登记了"允许访客"时，门卫才放行访客（LLAMA_API_KEY），
// 否则一律拦截，防止未授权访问。
func isSensitiveEnvVar(envEntry string, serverAPIKeyEnabled bool) bool {
	// 提取 KEY 部分（= 之前）
	eqIdx := strings.Index(envEntry, "=")
	if eqIdx <= 0 {
		return false
	}
	key := envEntry[:eqIdx]

	// LLAMA_API_KEY 仅在项目配置显式开启 ServerAPIKeyEnabled 时允许透传
	if key == "LLAMA_API_KEY" {
		return !serverAPIKeyEnabled
	}

	// DOUYA_SKIP_ACL：应用内部环境变量（控制 ACL 收紧跳过），
	// 不应泄露给子进程，否则可能允许子进程跳过权限检查。
	if key == "DOUYA_SKIP_ACL" {
		return true
	}

	// 黑名单：匹配 _SECRET/_TOKEN/_PASSWORD/_CREDENTIAL/_PASSPHRASE 后缀
	upperKey := strings.ToUpper(key)
	for _, suffix := range []string{"_SECRET", "_TOKEN", "_PASSWORD", "_CREDENTIAL", "_PASSPHRASE"} {
		if strings.HasSuffix(upperKey, suffix) {
			return true
		}
	}

	// 精确匹配已知敏感变量
	for _, prefix := range sensitiveEnvVarPrefixes {
		if strings.HasPrefix(envEntry, prefix) {
			return true
		}
	}

	return false
}

// bindProcessToJobObject 将子进程绑定到 Job Object。
// 作用：父进程（豆芽）退出时，Windows 会自动 kill 绑定的子进程（llama-server），
// 避免孤儿进程持续占用 GPU 资源。
func (s *Server) bindProcessToJobObject(pid int) {
	if s.job == nil {
		job, err := CreateJobObject()
		if err != nil {
			log.Error().Err(err).Msg("create job object failed (child process not bound)")
			return
		}
		s.job = job
	}
	if err := s.job.AssignProcess(pid); err != nil {
		log.Error().Err(err).Msg("assign process to job object failed (child process not bound)")
		return
	}
	log.Info().Int("pid", pid).Msg("llama-server bound to job object (will auto-kill on parent exit)")
}

// startWithExecCmd 使用 exec.Cmd 启动 llama-server（ConPTY 不可用时的回退方案）。
//
// 注意：调用此函数时必须已持有 s.mu，函数会在返回前释放锁。
func (s *Server) startWithExecCmd(args []string, runtimeDir string, env []string) {
	s.cmd.Stdout = s.stderrBuf.TeeWriter(os.Stderr)
	s.cmd.Stderr = s.stderrBuf.TeeWriter(os.Stderr)
	s.cmd.Env = env
	s.cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}

	if err := s.cmd.Start(); err != nil {
		enhancedErr := enhanceStartError(err)
		s.status = ServerStatus{Running: false, Error: fmt.Sprintf("启动 llama-server 失败: %v", enhancedErr)}
		// 清理旧 context，避免泄漏（进程未启动，不会有 wait goroutine 消费旧 context）
		s.replaceContext()
		s.mu.Unlock()
		return
	}

	s.bindProcessToJobObject(s.cmd.Process.Pid)
	s.replaceContext()
	s.status = ServerStatus{Running: true}

	go func() {
		// L-3：cmd.Wait 是系统调用，panic 概率极低，但 recover 可防极端情况
		defer func() {
			if r := recover(); r != nil {
				log.Warn().Interface("panic", r).Msg("[server] cmd.Wait goroutine panic")
			}
		}()
		err := s.cmd.Wait()
		// 安全实践（基于 B-1.6）：统一调用 updateStatusAfterExit 构建 status + 错误信息
		s.updateStatusAfterExit(err, 0, false)
	}()

	s.mu.Unlock()
}

// startWithConPTYSuccess 处理 ConPTY 启动成功后的后续设置。
//
// 注意：调用此函数时必须已持有 s.mu，函数会在返回前释放锁。
func (s *Server) startWithConPTYSuccess(pty *conpty.ConPty, args []string, runtimeDir string, env []string) {
	s.pty = pty
	s.cmd = nil
	log.Info().Int("pid", pty.Pid()).Msg("llama-server started with ConPTY (native terminal output: ANSI colors + progress bars)")

	s.bindProcessToJobObject(pty.Pid())
	s.replaceContext()
	s.status = ServerStatus{Running: true}

	// 启动 ConPTY 输出读取 goroutine（批量发送到前端 xterm.js）
	go s.readConPTYOutput()

	// 启动等待 goroutine
	go func() {
		// L-3：pty.Wait 是系统调用，recover 保护 pty 路径的状态更新
		defer func() {
			if r := recover(); r != nil {
				log.Warn().Interface("panic", r).Msg("[server] pty.Wait goroutine panic")
			}
		}()
		exitCode, err := pty.Wait(s.ctx)
		// 安全实践（基于 B-1.6）：统一调用 updateStatusAfterExit 构建 status + 错误信息
		s.updateStatusAfterExit(err, exitCode, true)
	}()

	s.mu.Unlock()
}

// updateStatusAfterExit 统一处理进程退出后的 status 更新和错误信息构建
// 安全实践（基于 B-1.6）：消除 startWithExecCmd 和 startWithConPTYSuccess 中 wait goroutine 的重复逻辑
//
// 参数说明：
//   - err：进程退出的 error（nil 表示正常退出）
//   - exitCode：进程退出码（仅 ConPTY 路径有意义，exec.Cmd 路径传 0）
//   - isConPTY：是否为 ConPTY 路径（影响错误信息格式和 DLL 缺失检测）
//
// 行为约定：
//   - 如果 ctx 已取消（s.ctx.Err() != nil），视为正常退出，不记录错误
//   - 否则构建错误信息（含 stderr 尾部输出），ConPTY 路径额外检测 DLL 缺失
//   - 更新 s.status 为 Running: false（带 Error 字段）
//   - M9 修复：进程自然退出（非 Stop 调用）时关闭 ConPTY 句柄。
//     若不关闭，崩溃→自动重启循环会不断残留未释放的 ConPTY 句柄和阻塞的 Read goroutine，
//     导致句柄/ goroutine 累积泄漏。关闭后可解锁阻塞的 pty.Read goroutine 使其退出。
func (s *Server) updateStatusAfterExit(err error, exitCode uint32, isConPTY bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// M9 修复：进程已退出，关闭 ConPTY 句柄释放资源，避免崩溃→自动重启循环累积未
	// 释放的 ConPTY 句柄和阻塞的 Read goroutine。
	// 注意：此处只 Close 不置空——readConPTYOutput 的 startRead 闭包会并发读取 s.pty 字段，
	// 置空会引入字段读写的数据竞争。ConPTY 的 Read 在 Close 后会返回，从而解锁阻塞的
	// 读取 goroutine 使其退出；下一次 watchdog 重启时会通过 startWithConPTYSuccess 覆盖为新实例。
	// Close 幂等：既可由主动 Stop（stopInternal cleanupFn）调用，也可在此自然退出路径调用，无副作用。
	if s.pty != nil {
		_ = s.pty.Close()
	}

	s.status = ServerStatus{Running: false}

	// ctx 已取消时视为正常退出，不记录错误
	if err != nil && s.ctx.Err() == nil {
		var errMsg string
		if isConPTY {
			errMsg = fmt.Sprintf("server exited with error: %v (exit code: %d)", err, exitCode)
		} else {
			errMsg = fmt.Sprintf("server exited with error: %v", err)
		}
		// 附加 stderr 尾部输出
		if s.stderrBuf != nil {
			if tail := s.stderrBuf.String(); tail != "" {
				errMsg += "\n" + tail
			}
		}
		// ConPTY 路径：检测 DLL 缺失导致的立即崩溃
		// P1-1 修复：lastStartTime 已改为 atomic.Int64，用 Load 读取再还原为 time.Time
		startTime := time.Unix(0, s.lastStartTime.Load())
		if isConPTY && exitCode != 0 && startTime.Before(time.Now().Add(-10*time.Second)) {
			if enhanced := enhanceStartError(errors.New(errMsg)); enhanced != nil {
				errMsg = enhanced.Error()
			}
		}
		s.status.Error = errMsg
	}
}
