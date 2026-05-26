package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed app.ico
var iconData []byte

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

	mutexHandle, isFirst := tryAcquireMutex()
	if !isFirst {
		activateExistingWindow()
		fmt.Println("豆芽已在运行，激活已有窗口")
		return
	}
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
			Assets: assets,
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
		println("Error:", err.Error())
	}
}
