package main

import (
	"fmt"
	"os"

	"douya/internal/chat"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) SendMessage(params chat.SendMessageParams) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪，请检查配置和数据。")
	}

	if !a.serverReady.Load() {
		return fmt.Errorf("AI 服务未启动，请等待服务就绪或检查配置。")
	}

	// 纳入 trackedGo 跟踪：shutdown 时 g.Wait() 会等待本 goroutine 退出，
	// 避免 db/ragVS 关闭后仍访问这些资源导致 panic 或数据损坏。
	a.trackedGo(func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Error().Interface("panic", r).Msg("SendMessage panic")
				convID := a.service.CurrentConvID()
				if convID == "" {
					convID = params.ConversationID
				}
				runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
					Type:           "error",
					Content:        fmt.Sprintf("内部错误: %v", r),
					ConversationID: convID,
				})
			}
		}()
		if err := a.service.SendMessage(a.ctx, params); err != nil {
			zlog.Error().Err(err).Msg("SendMessage error")
			convID := a.service.CurrentConvID()
			if convID == "" {
				convID = params.ConversationID
			}
			runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
				Type:           "error",
				Content:        err.Error(),
				ConversationID: convID,
			})
		}
	})
	return nil
}

func (a *App) GetConversations() ([]*chat.Conversation, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.GetConversations()
}

func (a *App) GetMessages(conversationID string) ([]*chat.Message, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.GetMessages(conversationID)
}

func (a *App) CreateConversation() (*chat.Conversation, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.CreateConversation()
}

func (a *App) RenameConversation(id string, title string) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}
	return a.service.RenameConversation(id, title)
}

func (a *App) DeleteConversation(id string) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}
	return a.service.DeleteConversation(id)
}

func (a *App) SearchMessages(query string) ([]*chat.Message, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.SearchMessages(query)
}

func (a *App) ExportConversation(id string, format string) (string, error) {
	if !a.ready.Load() {
		return "", fmt.Errorf("应用未就绪。")
	}
	return a.service.ExportConversation(id, format)
}

func (a *App) ExportConversationWithDialog(id string, format string) (bool, error) {
	if !a.ready.Load() {
		return false, fmt.Errorf("应用未就绪。")
	}

	content, err := a.service.ExportConversation(id, format)
	if err != nil {
		return false, err
	}

	var defaultName string
	var filterName string
	var filterPattern string
	switch format {
	case "json":
		defaultName = "对话导出.json"
		filterName = "JSON 文件 (*.json)"
		filterPattern = "*.json"
	case "txt", "plain", "plaintext":
		defaultName = "对话导出.txt"
		filterName = "纯文本文件 (*.txt)"
		filterPattern = "*.txt"
	case "csv":
		defaultName = "对话导出.csv"
		filterName = "CSV 文件 (*.csv)"
		filterPattern = "*.csv"
	default:
		defaultName = "对话导出.md"
		filterName = "Markdown 文件 (*.md)"
		filterPattern = "*.md"
	}

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "导出对话",
		Filters: []runtime.FileFilter{
			{
				DisplayName: filterName,
				Pattern:     filterPattern,
			},
		},
	})
	if err != nil {
		return false, err
	}
	if savePath == "" {
		return false, nil
	}

	err = os.WriteFile(savePath, []byte(content), 0o644)
	if err != nil {
		return false, fmt.Errorf("写入文件失败: %w", err)
	}

	return true, nil
}

func (a *App) DeleteMessage(id string) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}
	return a.service.DeleteMessage(id)
}

// CompressConversation 手动触发对话压缩（P2-A3）。
// 用户点击 TokenCounter 旁的"立即压缩"按钮时调用，同步返回压缩结果。
// 注意：此方法会阻塞至摘要生成完成（可能数秒~数十秒），前端需显示 loading。
func (a *App) CompressConversation(convID string) (*chat.CompressResult, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	if !a.serverReady.Load() {
		return nil, fmt.Errorf("AI 服务未启动，请等待服务就绪。")
	}
	return a.service.CompressConversation(convID)
}

func (a *App) RegenerateMessage(userMessageID string, searchMode string) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}

	if !a.serverReady.Load() {
		return fmt.Errorf("AI 服务未启动，请等待服务就绪或检查配置。")
	}

	// 纳入 trackedGo 跟踪：shutdown 时 g.Wait() 会等待本 goroutine 退出，
	// 避免 db/ragVS 关闭后仍访问这些资源导致 panic 或数据损坏。
	a.trackedGo(func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Error().Interface("panic", r).Msg("RegenerateMessage panic")
				convID := a.service.CurrentConvID()
				runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
					Type:           "error",
					Content:        fmt.Sprintf("内部错误: %v", r),
					ConversationID: convID,
				})
			}
		}()
		if err := a.service.RegenerateMessage(userMessageID, searchMode); err != nil {
			zlog.Error().Err(err).Msg("RegenerateMessage error")
			convID := a.service.CurrentConvID()
			runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
				Type:           "error",
				Content:        err.Error(),
				ConversationID: convID,
			})
		}
	})
	return nil
}

func (a *App) GetCleanupResult() []*chat.AbnormalConversation {
	a.cleanupResultMu.Lock()
	defer a.cleanupResultMu.Unlock()
	result := a.cleanupResult
	a.cleanupResult = nil
	return result
}
