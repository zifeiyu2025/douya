// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"douya/internal/pdfutil"
)

// extractPDFText 提取 PDF 文本内容。
// 委托给 pdfutil 包，失败时返回提示文本。
func extractPDFText(data []byte) string {
	return pdfutil.ExtractText(data)
}
