// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

// HubProvider 模型下载源的提供商标识。
//
// 生活类比：像"外卖平台"——豆芽从不同"外卖平台"（ModelScope、HF 镜像）
// 拿同一个"菜品"（GGUF 模型），每家平台的"菜单格式"和"取货地址"略有不同，
// 但都能送到同一个"家"（models 目录）。
type HubProvider string

const (
	// HubModelScope 魔搭社区（阿里出品，国内快，中文友好）
	HubModelScope HubProvider = "modelscope"
	// HubHFMirror HF 国内镜像站（HuggingFace 的加速镜像）
	HubHFMirror HubProvider = "hfmirror"
)

// IsValidHubProvider 判断是否为受支持的下载源。
func IsValidHubProvider(p HubProvider) bool {
	return p == HubModelScope || p == HubHFMirror
}

// ProviderDisplayName 返回下载源的中文展示名（用于日志与错误提示）。
func ProviderDisplayName(p HubProvider) string {
	switch p {
	case HubModelScope:
		return "ModelScope 魔搭社区"
	case HubHFMirror:
		return "HF 镜像"
	default:
		return string(p)
	}
}

// 各站点的 API 基地址与直链前缀。
const (
	modelScopeBase      = "https://modelscope.cn"
	modelScopeSearchURL = modelScopeBase + "/openapi/v1/models"
	modelScopeFilesURL  = modelScopeBase + "/api/v1/models/%s/repo/files"
	modelScopeDLPrefix  = modelScopeBase + "/models/%s/resolve/master/"

	hfMirrorBase     = "https://hf-mirror.com"
	hfMirrorSearch   = hfMirrorBase + "/api/models"
	hfMirrorTree     = hfMirrorBase + "/api/models/%s/tree/main"
	hfMirrorDLPrefix = hfMirrorBase + "/%s/resolve/main/"
)

// hubSearchHTTPClient 用于搜索/列目录等轻量 API 请求（带超时，避免卡死）。
// 生活类比：打电话查菜单（查列表）——不能占线太久。
var hubSearchHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
	},
}

// HubModel 站点上的一个模型仓库。
type HubModel struct {
	Provider  HubProvider `json:"provider"`  // 来源站点
	RepoID    string      `json:"repo_id"`   // 仓库 ID，如 "Qwen/Qwen3-8B-GGUF"
	Name      string      `json:"name"`      // 展示名
	Downloads int64       `json:"downloads"` // 下载量
	Likes     int64       `json:"likes"`     // 点赞/标星数
	// MainFileSize 主 .gguf 文件大小（字节）：取仓库内最小的主 .gguf（入门量化档），
	// 代表"跑起来至少要下多大"。搜索过滤阶段顺带捕获，用于按模型大小排序与列表展示；
	// 查询失败/未知为 0，排序时沉底。
	MainFileSize int64 `json:"main_file_size"`
}

// HubFile 仓库内的一个可下载文件。
type HubFile struct {
	Provider HubProvider `json:"provider"`  // 来源站点
	RepoID   string      `json:"repo_id"`   // 所属仓库 ID
	Path     string      `json:"path"`      // 仓库内相对路径（basename），如 "Qwen3-8B-Q4_K_M.gguf"
	Size     int64       `json:"size"`      // 文件大小（字节），未知为 0
	IsGGUF   bool        `json:"is_gguf"`   // 是否为 .gguf 模型文件
	IsMmproj bool        `json:"is_mmproj"` // 是否为 MMProj 多模态投影文件
	URL      string      `json:"url"`       // 直链下载地址
}

// ModelDownloadProgress 模型下载进度信息（通过回调推送给调用方再转为前端事件）。
type ModelDownloadProgress struct {
	Provider   HubProvider `json:"provider"`
	RepoID     string      `json:"repo_id"`
	FilePath   string      `json:"file_path"` // 当前正在下载的文件
	TotalBytes int64       `json:"total_bytes"`
	Downloaded int64       `json:"downloaded"`
	Percent    float64     `json:"percent"`
	Status     string      `json:"status"` // "downloading"/"completed"/"failed"
	Error      string      `json:"error"`
}

// SearchModels 在指定下载源搜索模型。query 为空时返回该源的热门/默认列表。
// page 为页码，从 1 起；每页约 30 条，用于配合前端"加载更多"逐页拉取。
func SearchModels(ctx context.Context, provider HubProvider, query string, page int) ([]HubModel, error) {
	if !IsValidHubProvider(provider) {
		return nil, apperror.Newf(apperror.KindInvalidInput, "不支持的下载源: %s", provider)
	}
	switch provider {
	case HubModelScope:
		return searchModelScope(ctx, query, page)
	case HubHFMirror:
		return searchHFMirror(ctx, query, page)
	default:
		return nil, apperror.Newf(apperror.KindInvalidInput, "不支持的下载源: %s", provider)
	}
}

// FilterModelsWithGGUF 并发检查每个候选仓库是否含主 .gguf 文件，过滤掉不含的仓库。
//
// 生活类比：搜索出一堆"候选菜品"后，让几个"试吃员"同时去看每个饭店有没有招牌菜（主 .gguf）。
// 没有招牌菜的饭店就不出现在名单里；某个饭店暂时联系不上（查询失败）时保守保留，避免误删有效选项。
//
// maxConcurrent 控制并发查文件列表的仓库数上限，避免几十个仓库对下载源发起过多请求。
func FilterModelsWithGGUF(ctx context.Context, provider HubProvider, models []HubModel, maxConcurrent int) []HubModel {
	if len(models) == 0 {
		return models
	}
	if maxConcurrent <= 0 {
		maxConcurrent = len(models)
	}
	if maxConcurrent > 8 {
		maxConcurrent = 8
	}

	keep := make([]bool, len(models))
	// 各仓库的主 .gguf 大小（过滤阶段顺带捕获，供按大小排序与前端展示）
	mainSizes := make([]int64, len(models))
	// 每个仓库单独一个限时 context，避免个别慢请求拖慢整批
	perCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 用固定大小的 worker 池并发检查，控制对下载源的请求压力
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent)
	for i, m := range models {
		wg.Add(1)
		go func(i int, m HubModel) {
			defer wg.Done()
			defer func() { <-sem }()
			sem <- struct{}{}

			// 查询失败（网络/接口）时保留该仓库，不能武断判定为无 .gguf
			files, err := ListModelFiles(perCtx, provider, m.RepoID)
			if err != nil {
				log.Warn().Err(err).Str("repo", m.RepoID).Msg("[modelhub] 过滤时查询文件列表失败，保留该仓库")
				keep[i] = true
				return
			}
			// 只剔除"确实列出文件且没有主 .gguf"的仓库；顺带记录最小的主 .gguf（入门量化档）
			for _, f := range files {
				if f.IsGGUF && !f.IsMmproj {
					keep[i] = true
					if f.Size > 0 && (mainSizes[i] == 0 || f.Size < mainSizes[i]) {
						mainSizes[i] = f.Size
					}
				}
			}
		}(i, m)
	}
	wg.Wait()

	filtered := make([]HubModel, 0, len(models))
	for i, m := range models {
		if keep[i] {
			m.MainFileSize = mainSizes[i]
			filtered = append(filtered, m)
		}
	}
	// 按模型大小升序排序：小的在前（低配机器优先可跑），大小未知（查询失败）的沉底，
	// 同大小时下载量高的在前。搜索接口本身按热度返回，这里仅对当页结果重排。
	sortModelsByMainSize(filtered)
	return filtered
}

// sortModelsByMainSize 按主文件大小升序对模型列表排序（未知大小沉底，同大小下载量优先）。
func sortModelsByMainSize(models []HubModel) {
	sort.SliceStable(models, func(a, b int) bool {
		as, bs := models[a].MainFileSize, models[b].MainFileSize
		switch {
		case as == 0 && bs == 0:
			return models[a].Downloads > models[b].Downloads
		case as == 0:
			return false
		case bs == 0:
			return true
		case as != bs:
			return as < bs
		default:
			return models[a].Downloads > models[b].Downloads
		}
	})
}

// ListModelFiles 列出指定仓库内的文件（用于挑选 .gguf 主文件与 MMProj 投影文件）。
func ListModelFiles(ctx context.Context, provider HubProvider, repoID string) ([]HubFile, error) {
	if !IsValidHubProvider(provider) {
		return nil, apperror.Newf(apperror.KindInvalidInput, "不支持的下载源: %s", provider)
	}
	if repoID = strings.TrimSpace(repoID); repoID == "" {
		return nil, apperror.New(apperror.KindInvalidInput, "仓库 ID 不能为空")
	}
	switch provider {
	case HubModelScope:
		return listModelScopeFiles(ctx, repoID)
	case HubHFMirror:
		return listHFMirrorFiles(ctx, repoID)
	default:
		return nil, apperror.Newf(apperror.KindInvalidInput, "不支持的下载源: %s", provider)
	}
}

// GetModelDownloadURL 构造模型的某个文件的直链下载地址。
// 支持子目录路径：将每一个路径段单独转义后用 "/" 连接，避免把 "/" 误编码导致 404。
func GetModelDownloadURL(provider HubProvider, repoID, filePath string) string {
	if !IsValidHubProvider(provider) {
		return ""
	}
	filePath = strings.Trim(filepath.ToSlash(filepath.Clean(filePath)), "/")
	segs := strings.Split(filePath, "/")
	enc := make([]string, 0, len(segs))
	for _, s := range segs {
		if s != "" {
			enc = append(enc, url.PathEscape(s))
		}
	}
	joined := strings.Join(enc, "/")
	switch provider {
	case HubModelScope:
		return fmt.Sprintf(modelScopeDLPrefix, repoID) + joined
	case HubHFMirror:
		return fmt.Sprintf(hfMirrorDLPrefix, repoID) + joined
	default:
		return ""
	}
}

// ============ ModelScope ============

// modelscopeSearchResp 定义 ModelScope 新版 OpenAPI 搜索接口（/openapi/v1/models）的响应结构。
// 生活类比：外卖平台换新菜单了，豆芽按新菜单的栏目（id/display_name/downloads/likes）取数据。
// 该接口为公开只读，无需登录即可调用；旧接口 /api/v1/dolphin/models 已废弃（404）。
type modelscopeSearchResp struct {
	Success bool `json:"success"`
	Data    struct {
		Models []struct {
			ID          string `json:"id"`           // 仓库 ID，如 "Qwen/Qwen3-8B-GGUF"
			DisplayName string `json:"display_name"` // 展示名
			Downloads   int64  `json:"downloads"`    // 下载量
			Likes       int64  `json:"likes"`        // 点赞/标星数
		} `json:"models"`
	} `json:"data"`
}

func searchModelScope(ctx context.Context, query string, page int) ([]HubModel, error) {
	u, _ := url.Parse(modelScopeSearchURL)
	q := u.Query()
	// 按关键词搜索，按下载量排序，每页 30 条
	if query != "" {
		q.Set("search", query)
	}
	q.Set("sort", "downloads")
	q.Set("page_size", "30")
	if page > 1 {
		q.Set("page_number", strconv.Itoa(page))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "创建 ModelScope 搜索请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)

	resp, err := hubSearchHTTPClient.Do(req)
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindUnavailable, "请求 ModelScope 搜索失败", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperror.Newf(apperror.KindUnavailable, "ModelScope 搜索返回非 200 状态码: %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "读取 ModelScope 搜索响应失败", err)
	}
	var parsed modelscopeSearchResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// 解析失败不阻塞：记日志返回空结果，避免接口变动导致功能不可用
		log.Warn().Err(err).Msg("[modelhub] ModelScope search response unmarshal failed")
		return nil, nil
	}

	models := make([]HubModel, 0, len(parsed.Data.Models))
	for _, it := range parsed.Data.Models {
		repoID := strings.TrimSpace(it.ID)
		if repoID == "" {
			continue
		}
		name := strings.TrimSpace(it.DisplayName)
		if name == "" {
			name = repoID
		}
		models = append(models, HubModel{
			Provider:  HubModelScope,
			RepoID:    repoID,
			Name:      name,
			Downloads: it.Downloads,
			Likes:     it.Likes,
		})
	}
	return models, nil
}

// modelscopeFilesResp 定义 ModelScope 文件列表接口的宽松响应结构。
type modelscopeFilesResp struct {
	Code int `json:"Code"`
	Data struct {
		Files []struct {
			Name string `json:"Name"`
			Path string `json:"Path"`
			Size int64  `json:"Size"`
			Type string `json:"Type"`
		} `json:"Files"`
	} `json:"Data"`
}

func listModelScopeFiles(ctx context.Context, repoID string) ([]HubFile, error) {
	apiURL := fmt.Sprintf(modelScopeFilesURL, url.PathEscape(repoID)) + "?Revision=master"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, http.NoBody)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "创建 ModelScope 文件列表请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)

	resp, err := hubSearchHTTPClient.Do(req)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "请求 ModelScope 文件列表失败", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperror.Newf(apperror.KindUnavailable, "ModelScope 文件列表返回非 200 状态码: %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "读取 ModelScope 文件列表响应失败", err)
	}
	var parsed modelscopeFilesResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		log.Warn().Err(err).Str("repo", repoID).Msg("[modelhub] ModelScope files unmarshal failed")
		return nil, apperror.Wrap(apperror.KindUnavailable, "解析 ModelScope 文件列表失败", err)
	}

	return normalizeFiles(HubModelScope, repoID, fileEntriesFromMSScope(parsed)), nil
}

type fileEntry struct {
	Path string
	Size int64
}

func fileEntriesFromMSScope(r modelscopeFilesResp) []fileEntry {
	entries := make([]fileEntry, 0, len(r.Data.Files))
	for _, f := range r.Data.Files {
		name := f.Path
		if name == "" {
			name = f.Name
		}
		if name == "" {
			continue
		}
		entries = append(entries, fileEntry{Path: name, Size: f.Size})
	}
	return entries
}

// ============ HF 镜像 ============

type hfModelMeta struct {
	ID        string `json:"id"`
	Downloads int64  `json:"downloads"`
	Likes     int64  `json:"likes"`
}

func searchHFMirror(ctx context.Context, query string, page int) ([]HubModel, error) {
	u, _ := url.Parse(hfMirrorSearch)
	q := u.Query()
	q.Set("search", query)
	q.Set("limit", "30")
	// HF 搜索接口用 full=... 复杂，常用 p 字段分页偏移。这里用 p 表示页数（从 1 起）。
	if page > 1 {
		q.Set("p", strconv.Itoa(page))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "创建 HF 镜像搜索请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)

	resp, err := hubSearchHTTPClient.Do(req)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "请求 HF 镜像搜索失败", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperror.Newf(apperror.KindUnavailable, "HF 镜像搜索返回非 200 状态码: %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "读取 HF 镜像搜索响应失败", err)
	}
	var items []hfModelMeta
	if err := json.Unmarshal(raw, &items); err != nil {
		// 兼容旧版/接口变动：可能返回对象包裹数组
		var obj struct {
			Models []hfModelMeta `json:"models"`
		}
		if err2 := json.Unmarshal(raw, &obj); err2 != nil || obj.Models == nil {
			log.Warn().Err(err).Msg("[modelhub] HF mirror search response unmarshal failed")
			return nil, nil
		}
		items = obj.Models
	}

	models := make([]HubModel, 0, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		models = append(models, HubModel{
			Provider:  HubHFMirror,
			RepoID:    id,
			Name:      id,
			Downloads: it.Downloads,
			Likes:     it.Likes,
		})
	}
	return models, nil
}

type hfTreeEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Type string `json:"type"`
}

func listHFMirrorFiles(ctx context.Context, repoID string) ([]HubFile, error) {
	// 注意：repoID 形如 "Qwen/Qwen3-8B-GGUF"，其 "/" 必须原样保留，
	// 不能经 url.PathEscape 转成 "%2F"，否则 HF 接口返回 400 导致一个文件都列不出来。
	treeURL := fmt.Sprintf(hfMirrorTree, repoID) + "?recursive=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, treeURL, http.NoBody)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "创建 HF 镜像文件列表请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)

	resp, err := hubSearchHTTPClient.Do(req)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "请求 HF 镜像文件列表失败", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperror.Newf(apperror.KindUnavailable, "HF 镜像文件列表返回非 200 状态码: %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "读取 HF 镜像文件列表响应失败", err)
	}
	var entries []hfTreeEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		log.Warn().Err(err).Str("repo", repoID).Msg("[modelhub] HF mirror tree unmarshal failed")
		return nil, apperror.Wrap(apperror.KindUnavailable, "解析 HF 镜像文件列表失败", err)
	}

	entries2 := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		// 仅保留文件（跳过目录，如子目录中的文件需到子目录继续展开，这里先只取根目录文件）
		if e.Type != "" && e.Type == "directory" {
			continue
		}
		if e.Path == "" {
			continue
		}
		entries2 = append(entries2, fileEntry{Path: e.Path, Size: e.Size})
	}
	return normalizeFiles(HubHFMirror, repoID, entries2), nil
}

// ============ 统一加工 ============

// normalizeFiles 将各站点的文件条目统一加工为 HubFile 列表。
// 仅保留 .gguf 文件（模型主文件与 MMProj 投影文件），便于前端挑选。
// Path 保留仓库内的相对路径（含子目录），以保证子目录文件的下载直链正确。
func normalizeFiles(provider HubProvider, repoID string, entries []fileEntry) []HubFile {
	files := make([]HubFile, 0, len(entries))
	for _, e := range entries {
		rel := strings.Trim(filepath.ToSlash(filepath.Clean(e.Path)), "/")
		if rel == "" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(rel), ".gguf") {
			continue
		}
		baseName := filepath.Base(rel)
		files = append(files, HubFile{
			Provider: provider,
			RepoID:   repoID,
			Path:     rel,
			Size:     e.Size,
			IsGGUF:   true,
			IsMmproj: isMmprojName(strings.ToLower(baseName)),
			URL:      GetModelDownloadURL(provider, repoID, rel),
		})
	}
	return files
}

// isMmprojName 判断文件名是否为 MMProj 多模态投影文件。
// 常见命名：mmproj-x.gguf / mm_proj-x.gguf / mm-proj-x.gguf 等。
func isMmprojName(lower string) bool {
	return strings.Contains(lower, "mmproj") ||
		strings.Contains(lower, "mm_proj") ||
		strings.Contains(lower, "mm-proj") ||
		strings.Contains(lower, "multimodal-proj")
}

// ============ 通用断点续传下载器 ============

// DownloadHubFile 从 url 将文件下载到 destPath，支持断点续传与取消。
//
// 生活类比：像下载一部大电影——下到一半断了没关系，再次下载会从断点继续，
// 不用从头再来；中途不想下了也能暂停，已下载的部分会保留供下次续传。
//
// 行为约定：
//   - 若 destPath 或 destPath+".tmp" 已存在且小于 totalSize，从断点恢复（Range 续传）；续传失败退化为重头下载。
//   - 全新/续传均先写 destPath+".tmp"，成功后原子重命名（仅历史暂停路径的断点直接落在 destPath 本体上）。
//   - ctx 取消时保留已下载部分（不删除），供下次续传；进度推送成功回调。
//   - 网络中断/读写失败同样保留 .tmp 断点，配合上层"重试下载"实现断点续传；
//     仅完成后字节数校验不一致（数据可疑）时清理并报错。
func DownloadHubFile(ctx context.Context, fileURL, destPath string, totalSize int64, provider HubProvider, progressCB func(ModelDownloadProgress)) error {
	destPath = filepath.Clean(destPath)

	// 断点探测：优先看目标文件本体（历史暂停路径），其次看失败时保留的 .tmp 部分文件。
	// 两处都要求已知 totalSize 才能确认"部分文件"语义，未知大小则退化为全新下载。
	var resumeFrom int64
	if totalSize > 0 {
		if info, err := os.Stat(destPath); err == nil {
			if info.Size() >= totalSize {
				emitProgress(progressCB, provider, "", destPath, totalSize, totalSize, "completed")
				return nil
			}
			resumeFrom = info.Size()
		} else if info, statErr := os.Stat(destPath + ".tmp"); statErr == nil && info.Size() > 0 {
			resumeFrom = info.Size()
		}
	}

	// Range 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, http.NoBody)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "创建下载请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)
	if resumeFrom > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeFrom))
	}

	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "下载请求失败", err)
	}
	defer resp.Body.Close()

	// 请求了续传但服务端返回 200（忽略 Range）→ 从头下载
	if resumeFrom > 0 && resp.StatusCode != http.StatusPartialContent {
		resumeFrom = 0
	}
	if resumeFrom == 0 && resp.StatusCode != http.StatusOK {
		return apperror.Newf(apperror.KindUnavailable, "下载返回非 200 状态码: %d", resp.StatusCode)
	}
	if resumeFrom > 0 && resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return apperror.Newf(apperror.KindUnavailable, "下载返回非 206 状态码: %d", resp.StatusCode)
	}

	// 确定目标总大小
	if totalSize <= 0 {
		if resp.ContentLength > 0 {
			totalSize = resumeFrom + resp.ContentLength
		} else {
			totalSize = 0 // 未知
		}
	}

	// 打开输出文件
	var out *os.File
	var tmpPath string
	if resumeFrom > 0 {
		if _, statErr := os.Stat(destPath); statErr == nil {
			// 断点在目标文件本体上（历史暂停路径）：直接续写本体
			out, err = os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE, 0o644)
		} else {
			// 断点在上次失败保留的 .tmp 上：续写临时文件，完成后仍走原子重命名
			tmpPath = destPath + ".tmp"
			out, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE, 0o644)
		}
		if err == nil {
			// 关键：把写偏移拨到断点处，否则 Write 会从文件头开始覆盖已有数据
			if _, seekErr := out.Seek(resumeFrom, io.SeekStart); seekErr != nil {
				out.Close()
				return apperror.Wrap(apperror.KindInternal, "定位续传偏移失败", seekErr)
			}
		}
	} else {
		tmpPath = destPath + ".tmp"
		out, err = os.Create(tmpPath)
	}
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "创建下载文件失败", err)
	}
	var outClosed bool
	defer func() {
		if !outClosed && out != nil {
			out.Close()
		}
	}()

	buf := make([]byte, 64*1024)
	var downloaded = resumeFrom
	lastReport := time.Now()

	for {
		select {
		case <-ctx.Done():
			// 取消/中断：保留已下载部分供续传。全新未完成时把 .tmp 改回目标名。
			out.Close()
			outClosed = true
			if tmpPath != "" {
				_ = os.Rename(tmpPath, destPath)
			}
			if progressCB != nil {
				progressCB(ModelDownloadProgress{
					Provider:   provider,
					FilePath:   filepath.Base(destPath),
					TotalBytes: totalSize,
					Downloaded: downloaded,
					Percent:    pct(downloaded, totalSize),
					Status:     "paused",
					Error:      "下载已暂停，已保留断点",
				})
			}
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				outClosed = true
				// 写盘失败保留 .tmp 断点：可能是瞬时错误，重试可从断点续传
				return apperror.Wrap(apperror.KindInternal, "写入文件失败", werr)
			}
			downloaded += int64(n)
			if progressCB != nil && time.Since(lastReport) >= 500*time.Millisecond {
				progressCB(ModelDownloadProgress{
					Provider:   provider,
					FilePath:   filepath.Base(destPath),
					TotalBytes: totalSize,
					Downloaded: downloaded,
					Percent:    pct(downloaded, totalSize),
					Status:     "downloading",
				})
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			outClosed = true
			// 网络中断保留 .tmp 断点：这是最高频的失败场景，重试可从断点续传
			return apperror.Wrap(apperror.KindUnavailable, "读取下载响应失败", readErr)
		}
	}

	if err := out.Close(); err != nil {
		outClosed = true
		return apperror.Wrap(apperror.KindInternal, "关闭下载文件失败", err)
	}
	outClosed = true

	// 字节数完整性校验
	if totalSize > 0 && downloaded != totalSize {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		} else {
			_ = os.Remove(destPath)
		}
		return apperror.Newf(apperror.KindUnavailable, "下载文件不完整：已下载 %d 字节，预期 %d 字节", downloaded, totalSize)
	}

	// 全新下载：临时文件原子重命名为目标文件
	if tmpPath != "" {
		if err := os.Rename(tmpPath, destPath); err != nil {
			_ = os.Remove(tmpPath)
			return apperror.Wrap(apperror.KindInternal, "重命名下载文件失败", err)
		}
	}

	log.Info().
		Str("provider", string(provider)).
		Str("path", destPath).
		Int64("size", totalSize).
		Msg("[modelhub] 模型文件下载完成")

	emitProgress(progressCB, provider, "", destPath, totalSize, totalSize, "completed")
	return nil
}

// ProbeFileSize 通过 HEAD 请求探测下载文件的真实大小（字节）。
//
// 用于激活 DownloadHubFile 的 Range 断点续传与完成度校验：调用方拿到 totalSize 后，
// 失败重试时能从 .tmp 已下载字节处继续，而不是从头再下数 GB。
// 探测失败一律返回 0（未知大小），调用方以无续传模式降级，不阻断下载流程。
func ProbeFileSize(ctx context.Context, fileURL string) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fileURL, http.NoBody)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", githubUA)
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength <= 0 {
		return 0
	}
	return resp.ContentLength
}

func emitProgress(cb func(ModelDownloadProgress), provider HubProvider, repoID, destPath string, total, done int64, status string) {
	if cb == nil {
		return
	}
	cb(ModelDownloadProgress{
		Provider:   provider,
		RepoID:     repoID,
		FilePath:   filepath.Base(destPath),
		TotalBytes: total,
		Downloaded: done,
		Percent:    pct(done, total),
		Status:     status,
	})
}

func pct(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(done) / float64(total) * 100
}
