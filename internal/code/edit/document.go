package edit

import (
	"bytes"
	"os"
	"sync"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// InputEncoding 是源码编码类型。
type InputEncoding = gotreesitter.InputEncoding

const (
	// InputEncodingUTF8 表示 UTF-8 编码。
	InputEncodingUTF8 = gotreesitter.InputEncodingUTF8
	// InputEncodingUTF16 表示 UTF-16 编码。
	InputEncodingUTF16 = gotreesitter.InputEncodingUTF16
)

// Document 表示一个打开的文档，封装了语法树、源码和文件元数据。
type Document struct {
	mu       sync.Mutex
	uri      string
	language *gotreesitter.Language
	source   []byte
	tree     *gotreesitter.Tree
	parser   *gotreesitter.Parser
	bound    *gotreesitter.BoundTree
	lineIdx  *LineIndex
	encoding InputEncoding

	// 文件系统追踪
	origFilePath   string      // 解析后的文件系统路径
	origFileMode   os.FileMode // 原始文件权限
	origModTime    time.Time   // 打开时的 mtime（初始参考值）
	origFileSize   int64       // 打开时的文件大小
	lastSavedMTime time.Time   // 上次保存的 mtime（冲突检测基准）

	// 脏状态
	dirty      bool
	createdAt  time.Time
	modifiedAt time.Time
	version    int // 编辑版本号，每次编辑递增
}

// URI 返回文档的标识符。
func (d *Document) URI() string { return d.uri }

// Language 返回文档的语言。
func (d *Document) Language() *gotreesitter.Language { return d.language }

// Source 返回当前源码。
func (d *Document) Source() []byte { return d.source }

// Tree 返回当前语法树。
func (d *Document) Tree() *gotreesitter.Tree { return d.tree }

// Bound 返回便捷封装的 BoundTree。
func (d *Document) Bound() *gotreesitter.BoundTree { return d.bound }

// LineIndex 返回文档的行索引。
func (d *Document) LineIndex() *LineIndex { return d.lineIdx }

// Dirty 返回文档是否有未保存的更改。
func (d *Document) Dirty() bool { return d.dirty }

// Version 返回当前编辑版本号。
func (d *Document) Version() int { return d.version }

// OrigFilePath 返回文件系统路径。
func (d *Document) OrigFilePath() string { return d.origFilePath }

// Encoding 返回文档编码。
func (d *Document) Encoding() InputEncoding { return d.encoding }

// Lock 锁定文档。
func (d *Document) Lock() { d.mu.Lock() }

// Unlock 解锁文档。
func (d *Document) Unlock() { d.mu.Unlock() }

// detectLanguage 根据文件名或源码自动检测语言。
func detectLanguage(filePath string, source []byte, languageName string) (*gotreesitter.Language, error) {
	if languageName != "" {
		entry := grammars.DetectLanguageByName(languageName)
		if entry == nil {
			return nil, ErrUnsupportedLanguage
		}
		return entry.Language(), nil
	}

	entry := grammars.DetectLanguage(filePath)
	if entry == nil {
		// 根据 shebang 检测
		if firstLine := firstLineOf(source); firstLine != "" {
			entry = grammars.DetectLanguageByShebang(firstLine)
		}
	}
	if entry == nil {
		return nil, ErrUnsupportedLanguage
	}
	return entry.Language(), nil
}

// firstLineOf 返回源码的第一行（用于 shebang 检测）。
func firstLineOf(source []byte) string {
	for i, b := range source {
		if b == '\n' {
			return string(source[:i])
		}
	}
	return string(source)
}

// resolveURI 将 URI 解析为文件系统路径。
func resolveURI(uri string) (string, error) {
	if len(uri) > 7 && uri[:7] == "file://" {
		path := uri[7:]
		if len(path) > 0 && path[0] == '/' {
			return path, nil
		}
		return path, nil
	}
	if len(uri) > 0 && uri[0] == '/' {
		return uri, nil
	}
	return "", ErrNonFileURI
}

// detectEncoding 检测源码编码（基于 BOM）。
func detectEncoding(source []byte) InputEncoding {
	if len(source) >= 2 {
		if source[0] == 0xFE && source[1] == 0xFF {
			return InputEncodingUTF16
		}
		if source[0] == 0xFF && source[1] == 0xFE {
			return InputEncodingUTF16
		}
	}
	return InputEncodingUTF8
}

// detectLineEnding 检测源码的行尾风格。
// 返回 "\n" (LF) 或 "\r\n" (CRLF)。
func detectLineEnding(source []byte) string {
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			if i > 0 && source[i-1] == '\r' {
				return "\r\n"
			}
			return "\n"
		}
	}
	return "\n"
}

// normalizeLineEndings 将文本的行尾转换为文档风格。
func (d *Document) normalizeLineEndings(text []byte) []byte {
	switch detectLineEnding(d.source) {
	case "\r\n":
		// 先统一为 \n，再转 \r\n，避免重复转换
		text = bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
		text = bytes.ReplaceAll(text, []byte("\n"), []byte("\r\n"))
	default:
		// 统一为 \n
		text = bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
	}
	return text
}

// successResult 构建编辑操作的成功结果。
func (d *Document) successResult(newSource []byte, oldLen int) *EditResult {
	result := &EditResult{
		Success:   true,
		NewSource: string(newSource),
		ByteDiff:  len(newSource) - oldLen,
	}

	root := d.tree.RootNode()
	if root != nil && root.HasError() {
		result.ParseErrors = collectErrors(root, d.language)
	}

	return result
}
