package main

import (
	"context"

	"douya/internal/apperror"
	"douya/internal/chat"
	"douya/internal/llm"

	zlog "github.com/rs/zerolog/log"
)

// slotActionDesc 描述 slot 操作的中文动作（用于错误日志和返回信息）
var slotActionDesc = map[string]string{
	"save":    "保存",
	"restore": "恢复",
	"erase":   "删除",
}

// validSlotActions 是 operateSlot 允许的合法 action 白名单。
// 生活类比：就像电梯按钮只认"上/下/停"三个指令，按别的键一律不响应，避免误触引发危险。
var validSlotActions = map[string]bool{
	"save":    true,
	"restore": true,
	"erase":   true,
}

// operateSlot 执行 slot 的 save/restore/erase 操作（v9744+ 新格式）。
// 生活类比：就像文件夹的"另存为/打开/删除"三个按钮，背后都是调用同一个文件管理器，只是动作参数不同。
// filename 用于给 save/restore 指定缓存文件名；erase 不需要文件名（传空串即可）。
// RF-1 修复：复用 client.OperateSlot，消除与 internal/llm/client.go 的重复实现，
// 仅在此层追加中文错误描述和成功日志，保持用户可见行为不变。
func (a *App) operateSlot(slotID int, action, filename string) error {
	if !validSlotActions[action] {
		return apperror.Newf(apperror.KindInvalidInput, "非法操作: %s，仅支持 save/restore/erase", action)
	}
	if err := validateNonNegativeInt("slot ID", slotID); err != nil {
		return err
	}
	if a.getClient() == nil {
		return apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutMedium)
	defer cancel()

	// 委托给 client.OperateSlot（已实现白名单校验、URL 转义、auth header、状态码判断）
	if err := a.getClient().OperateSlot(ctx, slotID, action, filename); err != nil {
		return apperror.Wrapf(apperror.KindInternal, "%s slot 失败", err, slotActionDesc[action])
	}

	zlog.Info().Int("slot_id", slotID).Str("action", action).Str("filename", filename).Msg("[app] slot operation succeeded")
	return nil
}

// SaveSlot 保存当前对话 slot 的 KV 缓存到磁盘。
// 调用 llama-server 的 POST /slots/{id}?action=save 端点；缓存文件名取自对话 ID，
// 与自动流程命名一致，保证"恢复"能找到对应文件。
func (a *App) SaveSlot(convID string) error {
	if err := validateNonEmpty("对话 ID", convID); err != nil {
		return err
	}
	return a.operateSlot(0, "save", chat.SlotFileName(convID))
}

// RestoreSlot 从磁盘恢复当前对话 slot 的 KV 缓存。
// 调用 llama-server 的 POST /slots/{id}?action=restore 端点；需提供与保存一致的对话 ID。
func (a *App) RestoreSlot(convID string) error {
	if err := validateNonEmpty("对话 ID", convID); err != nil {
		return err
	}
	return a.operateSlot(0, "restore", chat.SlotFileName(convID))
}

// EraseSlot 删除指定 slot 的 KV 缓存文件。
// 调用 llama-server 的 POST /slots/{id}?action=erase 端点（v9744+ 新增）。
func (a *App) EraseSlot(slotID int) error {
	return a.operateSlot(slotID, "erase", "")
}

// GetSlots 获取所有 slot 的状态信息
func (a *App) GetSlots() ([]llm.SlotInfo, error) {
	if a.getClient() == nil {
		return nil, apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutShort)
	defer cancel()
	return a.getClient().GetSlots(ctx)
}

// Tokenize 对文本进行分词，返回 token ID 列表
func (a *App) Tokenize(text string) ([]int, error) {
	if a.getClient() == nil {
		return nil, apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if err := validateNonEmpty("文本", text); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutShort)
	defer cancel()
	return a.getClient().Tokenize(ctx, text)
}
