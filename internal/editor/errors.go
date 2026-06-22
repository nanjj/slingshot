package editor

import "errors"

// 编辑器错误定义
var (
	// ErrDocumentNotReady 文档 tree 未初始化（首次解析失败或未就绪）。
	ErrDocumentNotReady = errors.New("document not ready")

	// ErrUnsupportedLanguage 语言不受支持。
	ErrUnsupportedLanguage = errors.New("unsupported language")

	// ErrNonFileURI 不是文件 URI，无法保存。
	ErrNonFileURI = errors.New("non-file URI, cannot save")

	// ErrDocumentNotFound 文档未找到。
	ErrDocumentNotFound = errors.New("document not found")

	// ErrInvalidPosition 无效的位置。
	ErrInvalidPosition = errors.New("invalid position")

	// ErrNodeNotFound 节点未找到。
	ErrNodeNotFound = errors.New("node not found")

	// ErrFileNotFound 文件未找到。
	ErrFileNotFound = errors.New("file not found")

	// ErrFileModifiedExternally 文件被外部修改。
	ErrFileModifiedExternally = errors.New("file modified externally")

	// ErrEncodingNotSupported 编码不支持。
	ErrEncodingNotSupported = errors.New("encoding not supported")
)
