package main

import (
	"container/list"
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
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

// appStartTime 记录应用启动时间，供 Health() 计算 uptime 使用
// 在 main() 第一行赋值，保证记录的是最早可观测时间点
var appStartTime time.Time

type LocalFileLoader struct {
	http.Handler
	// baseDir 是本地文件服务的基目录，所有请求路径都会被限制在此目录之下，
	// 防止攻击者通过绝对路径或路径遍历读取任意位置的文件。
	baseDir string
	// cache 是本地文件的 LRU 缓存，避免重复读取磁盘
	// 生活类比：像书桌上的书架，常看的图片放书架上随手可取，书架满了就把最久没看的收起来
	cache *fileLRUCache
}

// fileCacheEntry 是 LRU 缓存的单个条目，记录文件字节内容和修改时间
type fileCacheEntry struct {
	path  string
	data  []byte
	mtime time.Time
	size  int
}

// fileLRUCache 是一个简单的 LRU 缓存实现（map + 双向链表）
// 不引入外部依赖，限制最大项数和最大字节数，超限时淘汰最久未访问的条目
type fileLRUCache struct {
	mu       sync.Mutex
	maxItems int        // 最大条目数
	maxSize  int        // 最大总字节数
	curSize  int        // 当前总字节数
	ll       *list.List // 双向链表，front 为最近访问
	items    map[string]*list.Element
}

// newFileLRUCache 创建一个 LRU 缓存
func newFileLRUCache(maxItems, maxSize int) *fileLRUCache {
	return &fileLRUCache{
		maxItems: maxItems,
		maxSize:  maxSize,
		ll:       list.New(),
		items:    make(map[string]*list.Element),
	}
}

// get 查找缓存条目，命中时将其移到链表头部（标记为最近访问）
func (c *fileLRUCache) get(path string) (*fileCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[path]; ok {
		c.ll.MoveToFront(elem)
		return elem.Value.(*fileCacheEntry), true
	}
	return nil, false
}

// put 添加或更新缓存条目，并在项数或字节数超限时淘汰最久未访问的条目
func (c *fileLRUCache) put(path string, data []byte, mtime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 若已存在，先移除旧条目并扣减其大小
	if elem, ok := c.items[path]; ok {
		old := elem.Value.(*fileCacheEntry)
		c.curSize -= old.size
		c.ll.Remove(elem)
		delete(c.items, path)
	}
	entry := &fileCacheEntry{path: path, data: data, mtime: mtime, size: len(data)}
	elem := c.ll.PushFront(entry)
	c.items[path] = elem
	c.curSize += entry.size
	// 淘汰最久未访问的条目，直到项数和字节数都满足限制
	for c.curSize > c.maxSize || c.ll.Len() > c.maxItems {
		back := c.ll.Back()
		if back == nil {
			break
		}
		old := back.Value.(*fileCacheEntry)
		c.curSize -= old.size
		c.ll.Remove(back)
		delete(c.items, old.path)
	}
}

// clear 清空缓存中所有条目，释放内存。
// 生活类比：像下班后把书架上所有暂存的书都收回书库，腾空书架。
// 适用于窗口最小化到托盘或内存压力较大时主动释放缓存。
func (c *fileLRUCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.curSize = 0
}

// ClearCache 清空本地文件缓存，释放内存。
// 在窗口最小化到托盘等场景调用，避免缓存长期占用内存。
func (h *LocalFileLoader) ClearCache() {
	if h.cache != nil {
		h.cache.clear()
	}
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

	// 优先查 LRU 缓存：命中且文件 mtime 未变时直接返回，避免重复磁盘读取
	fileData, err := h.loadFileWithCache(finalPath)
	if err != nil {
		res.WriteHeader(http.StatusNotFound)
		return
	}

	// 安全：SVG 文件可能包含 JavaScript，强制作为附件下载而非内联渲染
	// 基于 GO-XSS-001 安全实践：不将可能含脚本的活跃格式作为 HTML 内容提供
	if ext == ".svg" {
		res.Header().Set("Content-Type", "image/svg+xml")
		// 提取文件名用于 Content-Disposition，改善下载体验
		// RFC 6266: filename* 支持非 ASCII 字符，filename 作回退
		svgName := filepath.Base(cleaned)
		res.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, svgName))
		_, _ = res.Write(fileData)
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
	_, _ = res.Write(fileData)
}

// loadFileWithCache 优先从 LRU 缓存加载文件内容。
// 缓存命中且 os.Stat 返回的 mtime 与缓存中一致时，直接返回缓存数据；
// 缓存未命中或 mtime 变化时，从磁盘读取并回填缓存。
// 生活类比：像查字典——先把要查的词在脑子里回想（查缓存），想不起来再翻书（读磁盘）并记住。
func (h *LocalFileLoader) loadFileWithCache(finalPath string) ([]byte, error) {
	// 缓存未启用（cache 为 nil）时直接读磁盘
	if h.cache == nil {
		return os.ReadFile(finalPath)
	}
	// 1. 查缓存：命中且 mtime 未变则直接返回
	if entry, ok := h.cache.get(finalPath); ok {
		info, statErr := os.Stat(finalPath)
		if statErr == nil && info.ModTime().Equal(entry.mtime) {
			return entry.data, nil
		}
	}
	// 2. 缓存未命中或 mtime 已变，从磁盘读取
	fileData, err := os.ReadFile(finalPath)
	if err != nil {
		return nil, err
	}
	// 3. 回填缓存（需要 mtime，Stat 失败则不缓存但仍返回数据）
	info, statErr := os.Stat(finalPath)
	if statErr == nil {
		h.cache.put(finalPath, fileData, info.ModTime())
	}
	return fileData, nil
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
	// L-4：unsafe.Pointer 用于 Windows API (CreateMutexW) 调用，无内存安全替代方案。
	// 参数类型已校验：mutexNamePtr 是 *uint16（UTF16 字符串），符合 CreateMutexW 签名。
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
	// L-4：unsafe.Pointer 用于 Windows API (FindWindowW) 调用，激活已有窗口实例。
	titlePtr, _ := syscall.UTF16PtrFromString("豆芽 - AI 聊天助手")
	hwnd, _, _ := pFindWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd != 0 {
		_, _, _ = pShowWindow.Call(hwnd, 9)
		_, _, _ = pSetForegroundWindow.Call(hwnd)
	}
}

func main() {
	// wailsbuild 会编译并运行 wailsbindings.exe 来生成前端绑定。
	// 该进程只需执行下方的 wails.Run（Wails 在 bindings 构建标签下自动生成绑定后退出），
	// 不能启动日志/单实例互斥/系统托盘，否则会干扰正在运行的真实实例（此前这些启动逻辑不加条件，
	// 导致绑定进程的 main 直接 return，wailsjs 绑定文件长期未被重新生成）。
	// 生活类比：绑定生成进程像"登记员只来抄写清单"，抄完就走，不用开灯烧水。
	isBindingsProcess := isWailsBindingsProcess()

	if !isBindingsProcess {
		// 记录应用启动时间（供 Health() 计算 uptime）
		appStartTime = time.Now()

		// 初始化日志系统（同时输出到控制台和文件）
		logDir := filepath.Join(appDir(), "data", "logs")
		logger.Init(logDir)
		log.Info().Str("logDir", logDir).Msg("豆芽启动中...")
	}

	var mutexHandle uintptr
	isFirst := true
	if !isBindingsProcess {
		mutexHandle, isFirst = tryAcquireMutex()
	}
	if !isFirst {
		// 重启场景（DOUYA_RESTART=1）：旧实例正在退出，此处等待其释放互斥后再重试，
		// 避免新进程因"检测到已有实例"被直接退出，导致"重启变退出"。
		if os.Getenv("DOUYA_RESTART") == "1" {
			// 关闭首次失败获得的句柄（避免泄漏），进入轮询等待
			if mutexHandle != 0 {
				_ = syscall.CloseHandle(syscall.Handle(mutexHandle))
				mutexHandle = 0
			}
			log.Info().Msg("[restart] 等待旧实例释放互斥体...")
			deadline := time.Now().Add(20 * time.Second)
			for !isFirst && time.Now().Before(deadline) {
				time.Sleep(200 * time.Millisecond)
				h2, ok2 := tryAcquireMutex()
				if ok2 {
					mutexHandle = h2
					isFirst = true
					break
				}
				if h2 != 0 {
					_ = syscall.CloseHandle(syscall.Handle(h2))
				}
			}
		}
		if !isFirst {
			activateExistingWindow()
			log.Info().Msg("检测到已有实例运行，激活已有窗口")
			return
		}
	}
	if !isBindingsProcess {
		log.Info().Msg("单实例互斥体获取成功")
		if mutexHandle != 0 {
			defer func() { _ = syscall.CloseHandle(syscall.Handle(mutexHandle)) }()
		}
	}

	app := NewApp()

	// 提前创建 LocalFileLoader，保存引用以便托盘最小化时清理缓存
	// 绑定生成进程不创建（避免调用 appDir() 在临时目录产生默认配置等副作用）
	var fileLoader http.Handler
	if !isBindingsProcess {
		localFileLoader := &LocalFileLoader{
			baseDir: appDir(),
			// LRU 缓存：最多 100 个文件，总大小上限 50MB，超限淘汰最久未访问的
			cache: newFileLRUCache(100, 50*1024*1024),
		}
		app.fileLoader = localFileLoader
		fileLoader = localFileLoader

		go systray.Run(app.onSystrayReady, app.onSystrayExit)
	}

	err := wails.Run(&options.App{
		Title:     "豆芽 - AI 聊天助手",
		Width:     1200,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: fileLoader,
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind: []any{
			app,
		},
		Frameless: true,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if !isBindingsProcess {
		systray.Quit()
	}

	if err != nil {
		log.Error().Err(err).Msg("Wails 运行失败")
	}
}
