// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// slotOpTimeout 是 slot save/restore/erase 操作的超时时间。
// save 涉及磁盘写入，restore 涉及磁盘读取和 KV 重建，给 30s 余量。
const slotOpTimeout = 30 * time.Second

// slotIDDefault 是豆芽默认使用的 slot 编号。
// 豆芽以单 slot 模式运行（--parallel 1），slot 0 始终存在。
const slotIDDefault = 0

// maxSlotFileNameLen 限制槽位缓存文件名的最大长度，防止超长文件名触发系统限制。
const maxSlotFileNameLen = 64

// SlotFileName 根据对话 ID 生成安全、稳定的槽位缓存文件名。
// 自动流程（会话内 save/restore）与手动按钮共用同一命名，保证两边保存的文件能被正确找到。
//
// 生活类比：像给文档起文件名一样，对话 ID 里的字母/数字/横线保留，其余一律换成下划线，
// 再截断到一定长度，确保既能对上号又不会被系统当成非法路径。
func SlotFileName(convID string) string {
	var b strings.Builder
	for _, r := range convID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := b.String()
	if name == "" {
		name = "default"
	}
	if len(name) > maxSlotFileNameLen {
		name = name[len(name)-maxSlotFileNameLen:]
	}
	return name
}

// tryRestoreSlot 在流式请求开始前尝试恢复指定对话的 KV 缓存。
// 触发条件：用户启用了 SlotSaveEnabled 且上次保存的对话 ID 与当前一致。
// 失败时仅记录日志，不阻塞主流程（llama-server 会自动 fallback 到重新 prefill）。
//
// 生活类比：像图书馆借书证，如果你上次借的书还在登记簿上（同对话 ID），
// 就直接凭登记簿取回书（restore），不用重新登记。如果登记簿上不是这本书，
// 或者管理员找不到登记簿（restore 失败），那就重新走一遍借书流程。
func (s *Service) tryRestoreSlot(ctx context.Context, convID string) {
	cfg := s.getConfigSnapshot()
	if cfg == nil || !cfg.SlotSaveEnabled {
		return
	}

	s.lastSavedSlotMu.RLock()
	savedID := s.lastSavedConvID
	s.lastSavedSlotMu.RUnlock()

	// 不是同一个对话，跳过 restore
	// （slot 0 当前可能保存着其他对话的 KV，直接 restore 会把错误缓存塞回去）
	if savedID == "" || savedID != convID {
		return
	}

	client := s.getClientSnapshot()
	if client == nil {
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, slotOpTimeout)
	defer cancel()

	if err := client.OperateSlot(opCtx, slotIDDefault, "restore", SlotFileName(convID)); err != nil {
		// restore 失败不阻塞主流程：llama-server 会自动重新 prefill
		log.Warn().Err(err).Str("conv_id", convID).Msg("[slot] restore failed, will fallback to full prefill")
		return
	}

	log.Info().Str("conv_id", convID).Msg("[slot] restored KV cache from disk, skipping redundant prefill")
}

// trySaveSlot 在流式请求成功结束后保存当前对话的 KV 缓存。
// 触发条件：用户启用了 SlotSaveEnabled。
// 失败时仅记录日志，不影响主流程（下次请求会重新 prefill，与未启用时行为一致）。
//
// 注意：必须在生成成功完成后调用，避免在生成失败时保存半截 KV。
func (s *Service) trySaveSlot(ctx context.Context, convID string) {
	cfg := s.getConfigSnapshot()
	if cfg == nil || !cfg.SlotSaveEnabled {
		return
	}

	client := s.getClientSnapshot()
	if client == nil {
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, slotOpTimeout)
	defer cancel()

	if err := client.OperateSlot(opCtx, slotIDDefault, "save", SlotFileName(convID)); err != nil {
		log.Warn().Err(err).Str("conv_id", convID).Msg("[slot] save failed, next request will re-prefill")
		return
	}

	s.lastSavedSlotMu.Lock()
	s.lastSavedConvID = convID
	s.lastSavedSlotMu.Unlock()

	log.Info().Str("conv_id", convID).Msg("[slot] saved KV cache to disk for fast restore on next turn")
}

// ClearSavedSlot 清除所有 slot 缓存。
// 在模型切换时调用：旧模型的 KV 缓存对新模型毫无价值，必须清除避免误用。
//
// 生活类比：换了一本完全不同的书，原来的读书笔记就该撕掉重写，
// 否则对照不上反而误导。erase 操作会让磁盘上的缓存文件也被清理掉。
func (s *Service) ClearSavedSlot(ctx context.Context) {
	// 无论是否启用 SlotSaveEnabled，都先清空内存中的 lastSavedConvID
	// 防止"启用→切换→关闭→再启用"等边界场景下出现 stale 状态
	s.lastSavedSlotMu.Lock()
	oldID := s.lastSavedConvID
	s.lastSavedConvID = ""
	s.lastSavedSlotMu.Unlock()

	cfg := s.getConfigSnapshot()
	if cfg == nil || !cfg.SlotSaveEnabled {
		return
	}

	// 没有保存过任何 slot，磁盘上也不会有缓存文件，跳过 erase
	if oldID == "" {
		return
	}

	client := s.getClientSnapshot()
	if client == nil {
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, slotOpTimeout)
	defer cancel()

	if err := client.OperateSlot(opCtx, slotIDDefault, "erase", ""); err != nil {
		// erase 失败不影响正确性，新模型加载后 slot 0 会被重新初始化
		log.Warn().Err(err).Msg("[slot] erase failed during model switch, stale cache may persist on disk")
		return
	}

	log.Info().Str("old_conv_id", oldID).Msg("[slot] erased KV cache during model switch")
}
