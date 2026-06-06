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
	cleaned := filepath.Clean(filePath)
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

	fileData, err := os.ReadFile(cleaned)
	if err != nil {
		res.WriteHeader(http.StatusNotFound)
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
	case ".svg":
		res.Header().Set("Content-Type", "image/svg+xml")
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

	// 初始化日志系统
	logger.Init()
	log.Info().Msg("豆芽启动中...")

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
			Handler: &LocalFileLoader{},
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
