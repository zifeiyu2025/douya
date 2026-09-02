// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"douya/internal/apperror"

	"github.com/rs/zerolog/log"
)

// rawModelInfo 用于解析 /v1/models 和 /v1/models/{name} 返回的模型信息。
// 两个端点的 JSON 结构在模型字段上一致，提取公共部分避免重复定义。
type rawModelInfo struct {
	ID           string    `json:"id"`
	Capabilities []string  `json:"capabilities"`
	Meta         ModelMeta `json:"meta"`
	Architecture struct {
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
}

// toModelInfo 将原始解析结果转换为 ModelInfo。
func (r *rawModelInfo) toModelInfo() *ModelInfo {
	return &ModelInfo{
		Name:            r.ID,
		Capabilities:    r.Capabilities,
		InputModalities: r.Architecture.InputModalities,
		Meta:            r.Meta,
	}
}

// GetModelInfoByName 获取指定模型的信息。
// 优先尝试直接端点 /v1/models/{name}（快速路径），失败则降级到 /v1/models 列表查询。
// 当 modelName 为空时，直接走列表端点并返回第一个模型。
//
// 拆分说明：原 150 行函数拆为调度器 + 2 子函数：
//   - tryDirectModelEndpoint: 直接端点查询（快速路径）
//   - fetchModelInfoFromList: 列表端点查询（降级路径）
//
// 生活类比：就像查电话号码——先查"快捷拨号"（直接端点），查不到再翻"通讯录"（列表端点）。
func (c *Client) GetModelInfoByName(ctx context.Context, modelName string) (*ModelInfo, error) {
	if modelName != "" {
		if info, found, err := c.tryDirectModelEndpoint(ctx, modelName); err != nil {
			return nil, err
		} else if found {
			return info, nil
		}
	}
	return c.fetchModelInfoFromList(ctx, modelName)
}

// tryDirectModelEndpoint 尝试通过 /v1/models/{name} 直接获取模型信息。
// 返回值：
//   - info: 解析成功的模型信息（found=true 时有效）
//   - found: true 表示直接端点命中并成功解析；false 表示需要降级到列表端点
//   - err: 权限错误等不可降级的错误
func (c *Client) tryDirectModelEndpoint(ctx context.Context, modelName string) (info *ModelInfo, found bool, err error) {
	directURL := c.baseURL + "/v1/models/" + url.PathEscape(modelName)
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, directURL, http.NoBody)
	if reqErr != nil {
		return nil, false, reqErr
	}
	c.setAuthHeader(httpReq)

	resp, doErr := c.httpClient.Do(httpReq)
	if doErr != nil {
		// 请求本身失败（网络错误等）：记录诊断日志，降级到列表端点
		log.Debug().Str("model", modelName).Err(doErr).Msg("[client] GetModelInfoByName: direct endpoint request failed, falling back to /v1/models")
		return nil, false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 非 200 状态码：记录诊断日志（含状态码），便于排查
		log.Debug().Str("model", modelName).Int("status", resp.StatusCode).Msg("[client] GetModelInfoByName: direct endpoint non-200, falling back to /v1/models")
		// 401/403 属于权限错误，直接返回而非降级到 /v1/models
		// 原因：权限不足时 /v1/models 同样会失败，降级只会浪费时间并掩盖真正的鉴权问题
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			// 使用 KindPermission 让上层可用 errors.Is(err, apperror.ErrPermission) 精准识别鉴权失败
			return nil, false, apperror.Newf(apperror.KindPermission, "direct endpoint returned %d for model %q", resp.StatusCode, modelName)
		}
		return nil, false, nil
	}

	body, readErr := readBody(resp.Body)
	if readErr != nil {
		// 读取响应体失败：记录诊断日志，降级到列表端点
		log.Debug().Str("model", modelName).Int("status", resp.StatusCode).Err(readErr).Msg("[client] GetModelInfoByName: read body failed, falling back to /v1/models")
		return nil, false, nil
	}

	var target rawModelInfo
	if json.Unmarshal(body, &target) != nil || target.ID == "" {
		// 200 但解析失败：记录诊断日志，便于排查接口返回异常
		log.Debug().Str("model", modelName).Int("status", resp.StatusCode).Msg("[client] GetModelInfoByName: 200 but body parse failed, falling back to /v1/models")
		return nil, false, nil
	}

	caps := target.Capabilities
	if len(caps) == 0 {
		// capabilities 字段可能为对象数组等非字符串数组格式，尝试原始解析
		var rawGeneric struct {
			Capabilities json.RawMessage `json:"capabilities"`
		}
		if json.Unmarshal(body, &rawGeneric) == nil {
			caps = parseCapabilitiesRaw(rawGeneric.Capabilities)
		}
	}
	target.Capabilities = caps

	log.Info().Str("model", target.ID).Strs("caps", caps).Msg("[client] GetModelInfoByName: direct hit")
	return target.toModelInfo(), true, nil
}

// fetchModelInfoFromList 通过 /v1/models 列表端点获取模型信息。
// 当 modelName 非空时，在列表中查找指定模型；为空时返回第一个模型。
func (c *Client) fetchModelInfoFromList(ctx context.Context, modelName string) (*ModelInfo, error) {
	body, err := c.fetchModelsListRaw(ctx)
	if err != nil {
		return nil, err
	}

	log.Debug().Str("raw_response", string(body)).Msg("[client] /v1/models raw response")

	var raw struct {
		Data []rawModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	if len(raw.Data) == 0 {
		return nil, apperror.New(apperror.KindNotFound, "模型列表为空")
	}

	target := &raw.Data[0]
	if modelName != "" {
		// 在 /v1/models 列表中查找指定模型，未找到时返回明确错误而非误用第一个模型
		found := false
		for i := range raw.Data {
			if raw.Data[i].ID == modelName {
				target = &raw.Data[i]
				found = true
				break
			}
		}
		if !found {
			return nil, apperror.Newf(apperror.KindNotFound, "模型 %q 不存在于 /v1/models 列表", modelName)
		}
	}

	caps := target.Capabilities
	if len(caps) == 0 {
		// capabilities 字段可能为对象数组等非字符串数组格式，尝试原始解析
		var rawGeneric struct {
			Data []struct {
				ID           string          `json:"id"`
				Capabilities json.RawMessage `json:"capabilities"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &rawGeneric); err == nil {
			for i := range rawGeneric.Data {
				if rawGeneric.Data[i].ID == target.ID {
					caps = parseCapabilitiesRaw(rawGeneric.Data[i].Capabilities)
					break
				}
			}
		}
	}
	target.Capabilities = caps

	log.Info().Str("model", modelName).Str("target_id", target.ID).Strs("caps", caps).Int("raw_data_count", len(raw.Data)).Msg("[client] GetModelInfoByName")

	return target.toModelInfo(), nil
}
