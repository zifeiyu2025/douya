package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/secrets"

	"fyne.io/systray"
	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 初始化应用级 rootCtx：生命周期贯穿整个 App 运行期，
	// shutdownInternal 会 rootCancel() 通知所有被跟踪的长生命周期 goroutine 退出。
	// 必须在任何 trackedGo 调用之前完成初始化。
	a.rootCtx, a.rootCancel = context.WithCancel(context.Background())

	a.cleanupOrphanProcesses()
	a.initHardware()

	cfgPath, err := a.loadAndValidateConfig(ctx)
	if err != nil {
		a.forceQuit()
		return
	}

	runtimeDir, _, err := a.ensureDirectories(ctx)
	if err != nil {
		// 目录创建失败：ensureDirectories 内部已弹窗提示用户
		a.forceQuit()
		return
	}

	if a.installBackend(ctx, runtimeDir) {
		return
	}

	a.handleMissingModels(ctx)

	dbPath := filepath.Join(appDir(), "data", "douya.db")

	if err := a.loadSecrets(ctx); err != nil {
		a.forceQuit()
		return
	}

	if err := a.initDatabase(ctx, dbPath); err != nil {
		a.forceQuit()
		return
	}

	// 提前创建 chat.Service（用 nil client/searchChain 占位），供后续 getServerAPIKey /
	// buildSearchChain 等通过 service 访问 settings，避免 App 层直接 import store（QUAL-3）。
	// 后续 UpdateClient / UpdateSearchChain 会填充真实依赖。
	a.service = chat.NewService(nil, nil, a.db, a.getConfig(), secrets.NewCipher(a.encKey), appDir())
	a.service.SetHostContext(ctx)
	a.service.SetEventPublisher(newWailsChatEventPublisher(ctx))

	a.migrateSearchEngines(cfgPath)

	a.buildService(ctx)

	a.cleanupOrphanSessions(ctx)

	// startServerAndWatch 是一次性启动流程（同步执行模型检测/加载后返回），
	// 内部会通过 trackedGo 启动 watcher/health 等长生命周期 goroutine。
	// 此处不直接 trackedGo：它使用 Wails ctx 而非 rootCtx，且 shutdown 不必等待模型加载完成。
	// P2.2 修复：startServerWithAutoFallback / autoLoadDefaultModel 内部已加入
	// a.exiting 检查与 rootCtx 派生 context，用户退出时能立即中断启动，避免拉起孤儿进程。
	go a.startServerAndWatch(a.server, ctx)
}

// shutdownInternal 是合并后的统一关闭逻辑，由 shutdown 和 PrepareShutdown 复用。
// stopOnce.Do 保证幂等：无论调用多少次，关闭逻辑只执行一次。
//
// 资源释放顺序（生活类比：下班关店流程）：
//  1. 停止生成（让正在进行的对话停下来）
//  2. rootCancel（广播"下班了"，通知被跟踪的长生命周期 goroutine 退出）
//  3. watchCancel（取消 server watch ctx，watcher/health 监听随之退出）
//  4. g.Wait（在门口签到表前等所有被跟踪 goroutine 出来，避免锁门时还有人留在里面）
//  5. srv.Stop + CloseJob（关闭 llama-server 进程）
//  6. ragVS.Close（关闭知识库向量存储）
//  7. db.Close（关闭数据库）
//
// waitForServerStop 当前实现下不影响行为（srv.Stop 本身为同步阻塞），
// 保留该参数以匹配任务规约签名，并为未来"异步停止"差异预留扩展点。
// teardown 是统一的资源释放流程，由 shutdownInternal 与 GracefulExit 共用。
// stopOnce 保证幂等：无论调用多少次，关闭逻辑只执行一次。
//
// emitProgress 可选：GracefulExit 传入以推送 EventShutdownProgress 进度；
// shutdownInternal 传 nil（无前端进度展示需求）。
//
// 资源释放顺序（生活类比：下班关店流程）：
//  1. 停止生成（让正在进行的对话停下来）
//  2. rootCancel（广播"下班了"，通知被跟踪的长生命周期 goroutine 退出）
//  3. watchCancel（取消 server watch ctx，watcher/health 监听随之退出）
//  4. g.Wait（在门口签到表前等所有被跟踪 goroutine 出来，避免锁门时还有人留在里面）
//  5. srv.Stop + CloseJob（关闭 llama-server 进程）
//  6. ragVS.Close（关闭知识库向量存储）
//  7. db.Close（关闭数据库）
func (a *App) teardown(emitProgress func(stage string)) {
	a.stopOnce.Do(func() {
		if a.service != nil {
			a.service.StopGeneration()
		}

		if emitProgress != nil {
			emitProgress("stopping_generation")
		}

		// 1. 取消应用级 rootCtx，通知所有被跟踪的长生命周期 goroutine 退出
		if a.rootCancel != nil {
			a.rootCancel()
		}

		// 2. 取消 server watch ctx（watcher/health 监听依赖此 ctx 退出）
		a.serverMu.Lock()
		if a.watchCancel != nil {
			a.watchCancel()
			a.watchCancel = nil
		}
		srv := a.server
		a.serverMu.Unlock()

		// 3. 等待被跟踪的 goroutine 退出，确保关闭底层资源后不会被访问
		a.g.Wait()

		// 4. 停止 llama-server 进程（同步：taskkill + 3s 超时 + force kill）
		if srv != nil {
			if emitProgress != nil {
				emitProgress("stopping_server")
			}
			if err := srv.Stop(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: stop server failed")
			}
			srv.CloseJob()
		}

		// 5. MCP 服务器无需主动断开（由 llama-server 进程退出时自动清理子进程）

		// 6. 关闭 RAG 向量库
		if a.ragVS != nil {
			if emitProgress != nil {
				emitProgress("closing_rag")
			}
			if err := a.ragVS.Close(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: close RAG vector store failed")
			}
		}

		// 7. 关闭数据库
		if a.db != nil {
			if emitProgress != nil {
				emitProgress("closing_db")
			}
			if err := a.db.Close(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: close database failed")
			}
		}

		if emitProgress != nil {
			emitProgress("done")
		}
	})
}

func (a *App) shutdownInternal(_ context.Context, waitForServerStop bool) {
	_ = waitForServerStop // 当前实现统一同步停止，保留参数以匹配任务规约签名
	a.teardown(nil)
}

func (a *App) shutdown(ctx context.Context) {
	a.shutdownInternal(ctx, false)
}

// PrepareShutdown 由前端在准备退出时调用（Wails 绑定，无参）。
// 合并后行为与 shutdown 一致：执行完整资源释放，后续 OnShutdown 因 stopOnce 而成为 no-op。
func (a *App) PrepareShutdown() {
	a.shutdownInternal(context.Background(), true)
}

func (a *App) shouldPreventClose() bool {
	return !a.exiting.Load()
}

func (a *App) tryStartExit() bool {
	if a.ctx == nil {
		return false
	}
	return a.exiting.CompareAndSwap(false, true)
}

// forceQuit 用于启动阶段遇到致命错误时强制退出应用。
//
// 与 GracefulExit 的区别：此时 db / server / ragVS 等资源尚未初始化，
// 无需执行资源清理流程，直接退出即可。
//
// 为什么需要先设置 exiting 标志：
//
//	runtime.Quit 会触发 OnBeforeClose → beforeClose，
//	而 beforeClose 在 exiting 为 false 时会返回 true 阻止关闭
//	（根据 CloseAction 配置，可能只是隐藏窗口到托盘）。
//	必须先将 exiting 置为 true，beforeClose 才会放行，Wails 进程才能真正退出。
//
// 为什么还需要 systray.Quit：
//
//	systray.Run 在独立 goroutine 中运行（见 main.go），
//	runtime.Quit 只关闭 Wails 窗口，不影响托盘。
//	不调用 systray.Quit 会导致托盘图标残留，用户仍可操作菜单。
//
// 为什么最后要调用 os.Exit(0)：
//
//	runtime.Quit 和 systray.Quit 都是异步的，发送退出信号后不会立即终止进程。
//	Wails 的关闭流程需要时间（触发 OnBeforeClose、OnShutdown 等回调）。
//	如果不调用 os.Exit，进程可能延迟数秒才退出，导致：
//	1. 单实例互斥体未释放，新进程检测到已有实例而退出（RestartApp 场景）
//	2. 用户看到旧窗口残留，以为应用没有关闭
//	os.Exit(0) 确保进程立即终止，互斥体立即释放。
//	在 forceQuit 场景下无需清理资源（db/server/ragVS 尚未初始化），直接退出是安全的。
func (a *App) forceQuit() {
	a.exiting.Store(true)
	runtime.Quit(a.ctx)
	systray.Quit()
	os.Exit(0)
}

func (a *App) beforeClose(ctx context.Context) bool {
	if !a.shouldPreventClose() {
		return false
	}
	// 根据 close_action 配置决定行为
	cfg := a.getConfig()
	switch cfg.CloseAction {
	case "exit":
		go a.GracefulExit()
		return true // 阻止默认关闭，由 GracefulExit 处理
	case "tray":
		runtime.WindowHide(ctx)
		a.clearFileCache()
		return true
	default: // "ask" 或未设置
		// M5 修复：ALT+F4 / 系统关闭按钮 与前端关闭按钮行为一致，
		// 通过事件通知前端弹出询问对话框，而非直接隐藏到托盘。
		runtime.EventsEmit(ctx, EventWindowCloseRequest, nil)
		return true
	}
}

// clearFileCache 清空本地文件 LRU 缓存，释放内存。
// 在窗口最小化到托盘时调用——用户不查看图片时无需占用 50MB 缓存。
func (a *App) clearFileCache() {
	if a.fileLoader != nil {
		a.fileLoader.ClearCache()
	}
}

func (a *App) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
	}
}

// HandleCloseRequest 处理前端关闭按钮点击，返回 "tray" 或 "exit" 表示应执行的操作
func (a *App) HandleCloseRequest() string {
	cfg := a.getConfig()
	switch cfg.CloseAction {
	case "exit":
		return "exit"
	case "tray":
		return "tray"
	default: // "ask" 或未设置
		return "ask"
	}
}

// SetCloseAction 设置关闭行为并持久化
func (a *App) SetCloseAction(action string) {
	// P3.5 重构：updateConfig 统一"复制→修改副本→替换指针"模式
	var cfg *config.Config
	if err := a.updateConfig(func(c *config.Config) error {
		c.CloseAction = action
		cfg = c
		return nil
	}); err != nil {
		zlog.Warn().Err(err).Msg("[SetCloseAction] 配置更新失败")
		return
	}
	// 保存前校验，失败记录日志但不阻塞保存（避免阻塞关闭动作设置功能）
	if err := cfg.Validate(); err != nil {
		zlog.Warn().Err(err).Msg("[SetCloseAction] 配置校验失败，仍保存")
	}
	if err := config.Save(filepath.Join(appDir(), "config.json"), cfg); err != nil {
		zlog.Warn().Err(err).Msg("[SetCloseAction] 配置保存失败")
	}
}

func (a *App) GracefulExit() {
	if !a.tryStartExit() {
		return
	}

	runtime.WindowShow(a.ctx)

	go func() {
		// L-1：优雅关闭流程涉及 DB/进程/事件多类资源，panic 会中断关闭导致资源泄漏
		defer recoverLog("[shutdown] GracefulExit panic, exit forced")

		// P3.2 重构：GracefulExit 与 shutdownInternal 共用 teardown，
		// 仅额外传入进度回调推送 EventShutdownProgress。
		a.teardown(func(stage string) {
			messages := map[string]string{
				"stopping_generation": "正在停止生成...",
				"stopping_server":     "正在关闭服务...",
				"closing_rag":         "正在关闭知识库...",
				"closing_db":          "正在关闭数据库...",
				"done":                "再见 👋",
			}
			msg, ok := messages[stage]
			if !ok {
				msg = stage
			}
			runtime.EventsEmit(a.ctx, EventShutdownProgress, map[string]any{
				"stage":   stage,
				"message": msg,
			})
		})

		runtime.Quit(a.ctx)
		systray.Quit()
	}()
}

// RestartApp 重启应用：先启动新进程，再退出当前进程。
// 通过临时 bat 脚本延迟启动新进程，确保旧进程完全退出后再启动新的，
// 避免端口/文件锁冲突。
//
// 生活类比：换班时，先让接班的人到岗准备好，老员工再下班，
// 中间留几秒钟交接时间，避免两人同时操作同一个岗位（端口/文件冲突）。
func (a *App) RestartApp() {
	exe, err := os.Executable()
	if err != nil {
		zlog.Error().Err(err).Msg("[restart] 获取可执行文件路径失败")
		a.forceQuit()
		return
	}

	// 安全校验 exe 路径：清理路径分隔符，检查是否包含 shell 元字符
	exe = filepath.Clean(exe)
	if strings.ContainsAny(exe, "&|><^()") {
		zlog.Warn().Str("exe", exe).Msg("[restart] exe 路径包含特殊字符，重启可能失败")
	}

	// 创建临时 bat 脚本：等待 2 秒后启动新进程，然后删除自身
	// 权限设为 0600，仅文件所有者可读写，避免被其他用户篡改
	batPath := filepath.Join(filepath.Dir(exe), "restart_douya.bat")
	batContent := fmt.Sprintf("@echo off\r\ntimeout /t 2 /nobreak >nul\r\nstart \"\" %q\r\ndel \"%%~f0\"\r\n", exe)
	if err := os.WriteFile(batPath, []byte(batContent), 0o600); err != nil {
		zlog.Error().Err(err).Msg("[restart] 创建重启脚本失败")
		a.forceQuit()
		return
	}

	// 异步启动 bat 脚本（不等待其完成）
	cmd := exec.Command("cmd", "/c", batPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		zlog.Error().Err(err).Msg("[restart] 启动重启脚本失败")
		os.Remove(batPath)
		a.forceQuit()
		return
	}

	zlog.Info().Str("exe", exe).Msg("[restart] 重启脚本已启动，即将退出当前进程")

	// 短暂等待确保 bat 脚本已开始执行，然后退出当前进程
	go func() {
		// 防止 panic 导致整个进程崩溃
		defer recoverLog("[restart] 退出等待 goroutine panic")
		time.Sleep(500 * time.Millisecond)
		a.forceQuit()
	}()
}
