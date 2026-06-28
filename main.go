package main

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"fyne.io/systray"
	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"douya/internal/logger"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed app.ico
var iconData []byte

type LocalFileLoader struct {
	http.Handler
	// baseDir 是本地文件服务的基目录，所有请求路径都会被限制在此目录之下，
	// 防止攻击者通过绝对路径或路径遍历读取任意位置的文件。
	baseDir string
}

// allowedFileExts 定义 LocalFileLoader 允许提供的文件扩展名白名单
var allowedFileExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".svg": true,
}

func (h *LocalFileLoader) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	filePath := strings.TrimPrefix(req.URL.Path, "/local-file/")
	if filePath == "" {
		res.WriteHeader(http.StatusBadRequest)
		return
	}

	// 安全：清理路径并阻止路径遍历
	// 基于 GO-PATH-001 安全实践
	cleaned := filepath.Clean(filePath)

	// 安全：拒绝绝对路径，防止攻击者构造 C:/Windows/System32/xxx.png 这类
	// 绝对路径读取任意位置允许扩展名的文件
	if filepath.IsAbs(cleaned) {
		res.WriteHeader(http.StatusForbidden)
		return
	}

	// 安全：拒绝包含 ".." 的路径遍历尝试
	if strings.Contains(cleaned, "..") {
		res.WriteHeader(http.StatusForbidden)
		return
	}

	// 安全：仅允许白名单中的文件扩展名
	ext := strings.ToLower(filepath.Ext(cleaned))
	if !allowedFileExts[ext] {
		res.WriteHeader(http.StatusForbidden)
		return
	}

	// 安全：将请求路径限制在基目录之下，防止越界读取。
	// 用 filepath.Join 拼接后再次 Clean，并用 filepath.Rel 验证最终路径
	// 仍未逃逸出基目录（纵深防御，即使前面的检查被绕过也能拦住）。
	finalPath := filepath.Clean(filepath.Join(h.baseDir, cleaned))
	rel, err := filepath.Rel(h.baseDir, finalPath)
	if err != nil {
		res.WriteHeader(http.StatusForbidden)
		return
	}
	// rel 为 ".." 或以 ".." + 路径分隔符开头，说明最终路径已逃出基目录
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		res.WriteHeader(http.StatusForbidden)
		return
	}

	fileData, err := os.ReadFile(finalPath)
	if err != nil {
		res.WriteHeader(http.StatusNotFound)
		return
	}

	// 安全：SVG 文件可能包含 JavaScript，强制作为附件下载而非内联渲染
	// 基于 GO-XSS-001 安全实践：不将可能含脚本的活跃格式作为 HTML 内容提供
	if ext == ".svg" {
		res.Header().Set("Content-Type", "image/svg+xml")
		res.Header().Set("Content-Disposition", "attachment")
		res.Write(fileData)
		return
	}

	switch ext {
	case ".jpg", ".jpeg":
		res.Header().Set("Content-Type", "image/jpeg")
	case ".png":
		res.Header().Set("Content-Type", "image/png")
	case ".gif":
		res.Header().Set("Content-Type", "image/gif")
	case ".webp":
		res.Header().Set("Content-Type", "image/webp")
	case ".bmp":
		res.Header().Set("Content-Type", "image/bmp")
	}
	res.Write(fileData)
}

func isWailsBindingsProcess() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	exeName := filepath.Base(exePath)
	return exeName == "wailsbindings.exe"
}

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	pCreateMutex         = kernel32.NewProc("CreateMutexW")
	user32               = syscall.NewLazyDLL("user32.dll")
	pFindWindow          = user32.NewProc("FindWindowW")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pShowWindow          = user32.NewProc("ShowWindow")
)

const mutexName = "Global\\DouyaAI_SingleInstance"

func tryAcquireMutex() (uintptr, bool) {
	mutexNamePtr, _ := syscall.UTF16PtrFromString(mutexName)
	handle, _, err := pCreateMutex.Call(0, 1, uintptr(unsafe.Pointer(mutexNamePtr)))
	if handle == 0 {
		return 0, false
	}
	isFirst := true
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok && errno == syscall.ERROR_ALREADY_EXISTS {
			isFirst = false
		}
	}
	return handle, isFirst
}

func activateExistingWindow() {
	titlePtr, _ := syscall.UTF16PtrFromString("豆芽 - AI 聊天助手")
	hwnd, _, _ := pFindWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd != 0 {
		pShowWindow.Call(hwnd, 9)
		pSetForegroundWindow.Call(hwnd)
	}
}

func main() {
	if isWailsBindingsProcess() {
		return
	}

	// 初始化日志系统（同时输出到控制台和文件）
	logDir := filepath.Join(appDir(), "data", "logs")
	logger.Init(logDir)
	log.Info().Str("logDir", logDir).Msg("豆芽启动中...")

	mutexHandle, isFirst := tryAcquireMutex()
	if !isFirst {
		activateExistingWindow()
		log.Info().Msg("检测到已有实例运行，激活已有窗口")
		return
	}
	log.Info().Msg("单实例互斥体获取成功")
	if mutexHandle != 0 {
		defer syscall.CloseHandle(syscall.Handle(mutexHandle))
	}

	app := NewApp()

	go systray.Run(app.onSystrayReady, app.onSystrayExit)

	err := wails.Run(&options.App{
		Title:     "豆芽 - AI 聊天助手",
		Width:     1200,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: &LocalFileLoader{baseDir: appDir()},
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
		Frameless: true,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	systray.Quit()

	if err != nil {
		log.Error().Err(err).Msg("Wails 运行失败")
	}
}
