package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"douya/internal/apperror"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) onSystrayReady() {
	systray.SetTitle("豆芽")
	systray.SetTooltip("豆芽 - 本地AI 助手")
	systray.SetIcon(iconData)

	systray.SetOnTapped(func() {
		a.ShowWindow()
	})

	mShow := systray.AddMenuItem("显示豆芽", "显示主窗口")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出豆芽", "退出程序")

	// 托盘菜单 goroutine：长期运行的 UI 循环，仅调用 ShowWindow/GracefulExit，
	// 不访问 db/ragVS/server 等被 shutdown 管理的资源，故不纳入 trackedGo 跟踪。
	// 通过 mQuit 点击或 systray.Quit() 退出，已有 recover 保护。
	go func() {
		// L-1：托盘菜单 goroutine 长期运行，panic 会导致菜单失效
		defer recoverLog("[systray] menu goroutine panic")
		for {
			select {
			case <-mShow.ClickedCh:
				a.ShowWindow()
			case <-mQuit.ClickedCh:
				a.GracefulExit()
				return
			}
		}
	}()
}

func (a *App) onSystrayExit() {
	systray.SetIcon([]byte{})
}

func (a *App) SelectImageFile() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择图片",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "图片文件",
				Pattern:     "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.bmp;*.svg",
			},
		},
	})
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "选择文件失败", err)
	}
	// 用户取消了选择
	if filePath == "" {
		return "", nil
	}

	// 解析为绝对路径，便于后续比较与复制
	srcPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "解析源文件路径失败", err)
	}

	// 目标目录：appDir()/data/images/
	imagesDir := filepath.Join(appDir(), "data", "images")

	// 若原文件已经在 images 目录下，无需复制，直接返回相对路径
	if absDir, absErr := filepath.Abs(imagesDir); absErr == nil {
		if strings.HasPrefix(srcPath, absDir+string(filepath.Separator)) {
			if rel, relErr := filepath.Rel(appDir(), srcPath); relErr == nil {
				return filepath.ToSlash(rel), nil
			}
		}
	}

	// 创建目标目录（如不存在）
	if err = os.MkdirAll(imagesDir, 0o755); err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "创建图片目录失败", err)
	}

	// 保留原扩展名
	ext := strings.ToLower(filepath.Ext(srcPath))

	// 计算源文件内容的 SHA256 哈希，作为去重依据。
	// 生活类比：给图片盖一个"身份证号"，内容相同则号码相同。
	// 用哈希前16位作为文件名，相同内容的图片只会保存一份。
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "打开源文件失败", err)
	}

	hasher := sha256.New()
	if _, e := io.Copy(hasher, srcFile); e != nil {
		srcFile.Close()
		return "", apperror.Wrap(apperror.KindInternal, "计算文件哈希失败", e)
	}
	srcFile.Close()

	hashHex := hex.EncodeToString(hasher.Sum(nil))[:16]
	dstName := "bg_" + hashHex + ext
	dstPath := filepath.Join(imagesDir, dstName)

	// 若目标文件已存在，说明内容相同的图片已保存过，直接复用，不再写入
	if _, statErr := os.Stat(dstPath); statErr == nil {
		if rel, relErr := filepath.Rel(appDir(), dstPath); relErr == nil {
			return filepath.ToSlash(rel), nil
		}
	}

	// 目标文件不存在，复制源文件到目标位置（保留原文件）
	srcFile, err = os.Open(srcPath)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "打开源文件失败", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "创建目标文件失败", err)
	}
	if _, e := io.Copy(dstFile, srcFile); e != nil {
		dstFile.Close()
		// 复制失败时清理已创建的空文件
		_ = os.Remove(dstPath)
		return "", apperror.Wrap(apperror.KindInternal, "复制图片失败", e)
	}
	if e := dstFile.Close(); e != nil {
		return "", apperror.Wrap(apperror.KindInternal, "保存图片失败", e)
	}

	// 返回相对路径，并用正斜杠（前端会用 URL 访问，Windows 下分隔符需转为 /）
	rel, err := filepath.Rel(appDir(), dstPath)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "计算相对路径失败", err)
	}
	return filepath.ToSlash(rel), nil
}

// SelectLoraFile 打开文件对话框选择 LoRA 适配器文件
func (a *App) SelectLoraFile() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 LoRA 适配器",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "LoRA 适配器文件",
				Pattern:     "*.gguf;*.bin;*.safetensors",
			},
			{
				DisplayName: "所有文件",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "选择文件失败", err)
	}
	return filePath, nil
}
