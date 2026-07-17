// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

// modelStatus 表示 /v1/models 返回的模型状态字段。
type modelStatus struct {
	Value    string `json:"value"`
	Failed   bool   `json:"failed,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

// modelEntry 表示 /v1/models 列表中的单个模型条目。
type modelEntry struct {
	ID     string      `json:"id"`
	Status modelStatus `json:"status"`
}

// modelsListResponse 表示 /v1/models 端点的响应结构。
type modelsListResponse struct {
	Data []modelEntry `json:"data"`
}

// modelLoadPollState 封装 WaitForModelLoaded 轮询过程中的可变状态。
// 将多个相关的状态变量集中管理，便于在子函数间传递和更新。
//
// 生活类比：就像质检员检查产品时的检查记录表——记录当前是第几次检查、
// 连续合格次数、是否见过产品上线等，每次检查后更新记录表。
type modelLoadPollState struct {
	pollCount           int       // 成功轮询次数
	stableCount         int       // 连续状态稳定次数（loaded/sleeping）
	vramSeenOccupied    bool      // 是否曾检测到 VRAM 被占用
	vramReleaseCount    int       // VRAM 释放确认计数
	modelSeenBefore     bool      // 模型是否曾出现在列表中
	lastDetailedLogTime time.Time // 上次详细日志时间
	startTime           time.Time // 开始时间（用于计算耗时）
}

// WaitForModelLoaded 等待指定模型加载完成（状态变为 loaded 或 sleeping）。
//
// 拆分说明：原 202 行函数按职责拆为调度器 + 4 子函数：
//   - pollModelsEndpoint: 单次 HTTP 轮询并解析响应
//   - evaluateModelState: 评估模型状态（loaded/failed/unloaded/loading）
//   - checkVRAMCrash: VRAM 释放检测（子进程崩溃诊断）
//   - notifyPollProgress: 进度回调与详细日志
//
// 生活类比：就像等快递——主循环不停查物流（poll），收到物流信息后判断是否到货（evaluate），
// 同时关注快递是否丢失（checkVRAMCrash），并定期通知收件人进度（notifyProgress）。
func (c *Client) WaitForModelLoaded(ctx context.Context, modelName string, timeout time.Duration, onProgress ...func(pollCount int, status string)) error {
	deadline := time.Now().Add(timeout)
	pollClient := c.pollClient

	state := &modelLoadPollState{
		startTime:           time.Now(),
		lastDetailedLogTime: time.Now(),
	}

	const requiredStablePolls = 2                 // 连续 2 次轮询状态稳定才认为真正就绪
	const stableInterval = 500 * time.Millisecond // 稳定性检查间隔
	const detailedLogInterval = 30 * time.Second  // 详细日志间隔
	const vramReleaseThreshold = 3                // VRAM 释放确认阈值

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		raw, ok := c.pollModelsEndpoint(ctx, pollClient)
		if !ok {
			time.Sleep(pollRetryInterval)
			continue
		}

		state.pollCount++

		// 首次轮询时记录所有模型 ID 和状态，帮助诊断模型名不匹配问题
		if state.pollCount == 1 {
			logFirstPollResult(modelName, raw)
		}

		// 进度回调与详细日志
		notifyPollProgress(state, onProgress, modelName, raw, detailedLogInterval)

		// 评估模型状态
		found, modelLoaded, shouldReturn, err := evaluateModelState(state, modelName, raw, pollClient, requiredStablePolls, stableInterval)
		if err != nil {
			return err
		}
		if shouldReturn {
			return nil
		}

		// VRAM 释放检测：子进程崩溃后 VRAM 会被操作系统回收
		if !modelLoaded {
			if crashErr := checkVRAMCrash(state, modelName, vramReleaseThreshold); crashErr != nil {
				return crashErr
			}
		}

		// 模型未找到处理
		if !found {
			if handleModelNotFound(state, modelName, raw) {
				return apperror.Newf(apperror.KindUnavailable, "model %s disappeared from model list (process crashed)", modelName)
			}
			time.Sleep(pollRetryInterval)
		}
	}

	return apperror.Newf(apperror.KindTimeout, "model %s did not become loaded within %v", modelName, timeout)
}

// pollModelsEndpoint 执行单次 /v1/models 轮询并解析响应。
// 返回 (parsed, ok)：ok=false 表示请求或解析失败，调用者应重试。
func (c *Client) pollModelsEndpoint(ctx context.Context, pollClient *http.Client) (modelsListResponse, bool) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", http.NoBody)
	if err != nil {
		return modelsListResponse{}, false
	}
	c.setAuthHeader(httpReq)

	resp, err := pollClient.Do(httpReq)
	if err != nil {
		return modelsListResponse{}, false
	}
	body, readErr := readBody(resp.Body)
	resp.Body.Close() // 立即关闭：readBody 已读完数据，避免循环内 body 堆积

	if resp.StatusCode != http.StatusOK || readErr != nil {
		return modelsListResponse{}, false
	}

	var raw modelsListResponse
	if json.Unmarshal(body, &raw) != nil {
		return modelsListResponse{}, false
	}
	return raw, true
}

// logFirstPollResult 记录首次轮询的所有模型 ID 和状态，帮助诊断模型名不匹配问题。
func logFirstPollResult(modelName string, raw modelsListResponse) {
	allModels := make([]string, 0, len(raw.Data))
	for _, d := range raw.Data {
		allModels = append(allModels, fmt.Sprintf("%s=%s", d.ID, d.Status.Value))
	}
	log.Info().Str("target", modelName).Strs("models", allModels).Msg("[client] WaitForModelLoaded: first poll result")
}

// notifyPollProgress 执行进度回调，并在加载耗时较长时记录详细日志。
func notifyPollProgress(state *modelLoadPollState, onProgress []func(int, string), modelName string, raw modelsListResponse, detailedLogInterval time.Duration) {
	if len(onProgress) == 0 || onProgress[0] == nil {
		return
	}

	statusValue := "polling"
	for _, d := range raw.Data {
		if d.ID == modelName || FuzzyMatchModelID(d.ID, modelName) {
			statusValue = d.Status.Value
			break
		}
	}
	onProgress[0](state.pollCount, statusValue)

	// 详细日志：加载超过 30 秒时每 30 秒记录一次状态
	now := time.Now()
	if now.Sub(state.lastDetailedLogTime) >= detailedLogInterval {
		log.Info().
			Str("model", modelName).
			Str("status", statusValue).
			Int("polls", state.pollCount).
			Dur("elapsed", now.Sub(state.startTime)).
			Msg("[client] WaitForModelLoaded: long-running load")
		state.lastDetailedLogTime = now
	}
}

// evaluateModelState 评估目标模型的状态，更新稳定性计数。
// 返回值：
//   - found: 是否在列表中找到目标模型
//   - modelLoaded: 模型是否处于 loaded/sleeping 状态
//   - shouldReturn: 是否应立即返回（模型已稳定就绪）
//   - err: 致命错误（模型加载失败/崩溃等）
func evaluateModelState(state *modelLoadPollState, modelName string, raw modelsListResponse, pollClient *http.Client, requiredStablePolls int, stableInterval time.Duration) (found, modelLoaded, shouldReturn bool, err error) {
	for _, d := range raw.Data {
		if d.ID != modelName && !FuzzyMatchModelID(d.ID, modelName) {
			continue
		}
		found = true
		state.modelSeenBefore = true

		// 检测 failed 字段：子进程崩溃后路由器可能将状态设为 unloaded+failed
		if d.Status.Failed {
			return found, false, false, apperror.Newf(apperror.KindUnavailable, "model %s failed to load (exit_code=%d)", modelName, d.Status.ExitCode)
		}

		// 每 10 次轮询记录一次状态，帮助排查加载卡住的问题
		if state.pollCount%10 == 1 {
			log.Debug().Str("model", modelName).Str("status", d.Status.Value).Int("poll", state.pollCount).Msg("[client] WaitForModelLoaded: polling")
		}

		switch d.Status.Value {
		case "loaded", "sleeping":
			// loaded = 模型已加载就绪
			// sleeping = 模型已加载但处于休眠状态（仍在 VRAM 中，新请求会自动唤醒）
			modelLoaded = true
			state.stableCount++
			if state.stableCount >= requiredStablePolls {
				log.Info().Str("model", modelName).Str("status", d.Status.Value).Int("polls", state.pollCount).Msg("[client] WaitForModelLoaded: model is stable")
				shouldReturn = true
				return found, modelLoaded, shouldReturn, err
			}
			log.Debug().Str("model", modelName).Str("status", d.Status.Value).Int("stable", state.stableCount).Int("required", requiredStablePolls).Msg("[client] WaitForModelLoaded: stability check")
			// 稳定性检查期间使用较长间隔
			time.Sleep(stableInterval)
		case "failed":
			err = apperror.Newf(apperror.KindUnavailable, "model %s failed to load", modelName)
			return found, modelLoaded, shouldReturn, err
		case "unloaded":
			// unloaded 可能是子进程崩溃（exit_code != 0），也可能是初始状态
			if d.Status.ExitCode != 0 {
				err = apperror.Newf(apperror.KindUnavailable, "model %s crashed during loading (exit_code=%d)", modelName, d.Status.ExitCode)
				return found, modelLoaded, shouldReturn, err
			}
			// 模型曾经加载后又卸载了（子进程崩溃），重置稳定性计数
			if state.stableCount > 0 {
				log.Warn().Str("model", modelName).Int("previous_stable", state.stableCount).Msg("[client] WaitForModelLoaded: model became unloaded after being loaded (child process crash?)")
				state.stableCount = 0
			}
			time.Sleep(pollRetryInterval)
		default:
			// loading 等其他状态，继续等待
			if state.stableCount > 0 {
				log.Debug().Str("model", modelName).Str("status", d.Status.Value).Int("previous_stable", state.stableCount).Msg("[client] WaitForModelLoaded: model left loaded state, resetting stability")
				state.stableCount = 0
			}
			time.Sleep(pollRetryInterval)
		}
		return found, modelLoaded, shouldReturn, err
	}
	return found, modelLoaded, shouldReturn, err
}

// checkVRAMCrash 通过 VRAM 释放检测子进程崩溃。
// 子进程崩溃后 VRAM 会被操作系统回收，连续多次检测到释放则确认崩溃。
// 返回 error 表示检测到崩溃，nil 表示正常。
func checkVRAMCrash(state *modelLoadPollState, modelName string, vramReleaseThreshold int) error {
	vramFree, vramErr := checkVRAMFree()
	if vramErr != nil {
		return nil
	}

	if !vramFree {
		// VRAM 被占用，子进程在运行
		if !state.vramSeenOccupied {
			log.Debug().Str("model", modelName).Msg("[client] WaitForModelLoaded: VRAM occupied detected")
		}
		state.vramSeenOccupied = true
		state.vramReleaseCount = 0
		return nil
	}

	// VRAM 空闲
	if !state.vramSeenOccupied {
		return nil
	}

	// VRAM 从占用变为空闲，可能子进程崩溃
	state.vramReleaseCount++
	log.Warn().Str("model", modelName).Int("release_count", state.vramReleaseCount).Int("threshold", vramReleaseThreshold).Msg("[client] WaitForModelLoaded: VRAM released after being occupied (possible crash)")
	if state.vramReleaseCount >= vramReleaseThreshold {
		return apperror.Newf(apperror.KindUnavailable, "model %s crashed during loading (VRAM released after being occupied)", modelName)
	}
	return nil
}

// handleModelNotFound 处理模型未在列表中找到的情况。
// 返回 true 表示模型曾出现后消失（子进程崩溃），应返回错误。
func handleModelNotFound(state *modelLoadPollState, modelName string, raw modelsListResponse) bool {
	if state.modelSeenBefore {
		log.Warn().Str("model", modelName).Msg("[client] WaitForModelLoaded: model disappeared from list (process crashed)")
		return true
	}
	// 模型名称未匹配，记录日志帮助调试
	if state.pollCount <= 5 {
		ids := make([]string, 0, len(raw.Data))
		for _, d := range raw.Data {
			ids = append(ids, d.ID)
		}
		log.Debug().Str("model", modelName).Strs("available_ids", ids).Int("poll", state.pollCount).Msg("[client] WaitForModelLoaded: model not found in response, retrying")
	}
	return false
}
