package rag

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"douya/internal/pdfutil"
)

func TestParseFileFromBytes_TextFile(t *testing.T) {
	data := []byte("Hello, World!")
	text, err := ParseFileFromBytes(data, "test.txt")
	if err != nil {
		t.Fatalf("ParseFileFromBytes .txt failed: %v", err)
	}
	if text != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", text)
	}
}

func TestParseFileFromBytes_MarkdownFile(t *testing.T) {
	data := []byte("# Title\n\nSome content")
	text, err := ParseFileFromBytes(data, "doc.md")
	if err != nil {
		t.Fatalf("ParseFileFromBytes .md failed: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty text for markdown file")
	}
}

func TestParseFileFromBytes_CodeFile(t *testing.T) {
	data := []byte("func main() { fmt.Println(\"hello\") }")
	text, err := ParseFileFromBytes(data, "main.go")
	if err != nil {
		t.Fatalf("ParseFileFromBytes .go failed: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty text for code file")
	}
}

func TestParseFileFromBytes_NonUTF8Text(t *testing.T) {
	// 非 UTF-8 的二进制数据
	data := []byte{0x80, 0x81, 0x82, 0x83}
	_, err := ParseFileFromBytes(data, "binary.txt")
	if err == nil {
		t.Error("expected error for non-UTF-8 text file, got nil")
	}
}

func TestParseFileFromBytes_EmptyPDF(t *testing.T) {
	// 空数据
	data := []byte{}
	text := pdfutil.ExtractTextWithFallback(data, "")
	if text != "" {
		t.Errorf("expected empty string for empty data, got %q", text)
	}
}

func TestParseFileFromBytes_InvalidPDF(t *testing.T) {
	// 非 PDF 头部的数据
	data := []byte("This is not a PDF")
	text := pdfutil.ExtractTextWithFallback(data, "")
	if text != "" {
		t.Errorf("expected empty string for non-PDF data, got %q", text)
	}
}

func TestParseFileFromBytes_PDFWithText(t *testing.T) {
	// 最小 PDF 结构，包含括号文本
	data := []byte("%PDF-1.4\n(Hello PDF World)\nendobj")
	text := pdfutil.ExtractTextWithFallback(data, "")
	if text == "" {
		t.Error("expected non-empty text from PDF with parenthesized text")
	}
}

func TestParseFileFromBytes_EmptyDOCX(t *testing.T) {
	// 空数据不是有效的 zip
	_, err := ParseFileFromBytes([]byte{}, "test.docx")
	if err == nil {
		t.Error("expected error for empty docx, got nil")
	}
}

func TestParseFileFromBytes_InvalidDOCX(t *testing.T) {
	// 不是 zip 格式的数据
	_, err := ParseFileFromBytes([]byte("not a zip"), "test.docx")
	if err == nil {
		t.Error("expected error for invalid docx, got nil")
	}
}

func TestParseFileFromBytes_UnknownExtension(t *testing.T) {
	// 未知扩展名应回退到 parseAsText
	data := []byte("plain text content")
	text, err := ParseFileFromBytes(data, "readme.xyz")
	if err != nil {
		t.Fatalf("ParseFileFromBytes unknown ext failed: %v", err)
	}
	if text != "plain text content" {
		t.Errorf("expected 'plain text content', got %q", text)
	}
}

func TestParseFileFromBytes_NoExtension(t *testing.T) {
	// 无扩展名应回退到 parseAsText
	data := []byte("no extension file")
	text, err := ParseFileFromBytes(data, "Makefile")
	if err != nil {
		t.Fatalf("ParseFileFromBytes no ext failed: %v", err)
	}
	if text != "no extension file" {
		t.Errorf("expected 'no extension file', got %q", text)
	}
}

func TestStripXMLTags(t *testing.T) {
	input := `<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`
	result := stripXMLTags(input)
	// 每个标签被替换为一个空格，6个标签产生6个空格
	expected := "   Hello   "
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestStripXMLTags_EntityDecoding(t *testing.T) {
	input := `<t>&amp; &lt; &gt; &quot; &apos;</t>`
	result := stripXMLTags(input)
	if result != " & < > \" ' " {
		t.Errorf("expected decoded entities, got %q", result)
	}
}

func TestCleanWhitespace(t *testing.T) {
	input := "Hello\r\n\r\n\r\nWorld"
	result := cleanWhitespace(input)
	// cleanWhitespace 会 trim 空行，所以连续换行之间的空行被去掉
	if result != "Hello\nWorld" {
		t.Errorf("expected 'Hello\\nWorld', got %q", result)
	}
}

func TestCleanWhitespace_MultipleSpaces(t *testing.T) {
	input := "Hello    World"
	result := cleanWhitespace(input)
	if result != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", result)
	}
}

func TestParseDOCX_ZipBomb(t *testing.T) {
	// 构造高压缩比 ZIP：word/document.xml 解压后超过 100MB 限制
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	fw, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("创建 zip 条目失败: %v", err)
	}

	// 写入超过 100MB 的重复内容，压缩后体积很小
	// 使用 1MB chunk 避免一次性分配过大内存
	chunk := make([]byte, 1024*1024) // 1MB
	for i := range chunk {
		chunk[i] = 'a'
	}

	totalSize := int64(101 * 1024 * 1024) // 101MB，超过 100MB 限制
	written := int64(0)
	for written < totalSize {
		n := int64(len(chunk))
		if written+n > totalSize {
			n = totalSize - written
		}
		if _, err := fw.Write(chunk[:n]); err != nil {
			t.Fatalf("写入 zip 条目失败: %v", err)
		}
		written += n
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip writer 失败: %v", err)
	}

	// 验证 parseDOCX 拦截了 zip bomb
	_, err = parseDOCX(zipBuf.Bytes())
	if err == nil {
		t.Fatal("预期返回错误（zip bomb 拦截），但得到 nil")
	}

	expectedErr := "DOCX 解压内容超过 100MB 限制"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("预期错误包含 %q，实际得到 %q", expectedErr, err.Error())
	}
}
