package editor

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// SyntaxError 描述语法树中的一个错误或缺失节点。
type SyntaxError struct {
	Type     string `json:"type"`     // "error" 或 "missing"
	StartRow uint32 `json:"startRow"`
	StartCol uint32 `json:"startCol"`
	EndRow   uint32 `json:"endRow"`
	EndCol   uint32 `json:"endCol"`
}

// ValidationResult 是 Validate 操作的返回结果。
type ValidationResult struct {
	Valid           bool          `json:"valid"`
	SyntaxErrors    []SyntaxError `json:"syntaxErrors,omitempty"`
	LineEnding      string        `json:"lineEnding"`      // "\n" 或 "\r\n"
	TrailingNewline bool          `json:"trailingNewline"` // 文件末尾是否有换行（POSIX 规范）
	SourceSize      int           `json:"sourceSize"`
	Language        string        `json:"language"`
}

// Validate 验证文档的语法正确性，并返回文档健康状态。
//
// 检查项包括：
//   - 语法树中是否有错误或缺失节点
//   - 行尾风格
//   - 文件末尾是否有换行（POSIX 规范）
func (ed *Editor) Validate(uri string) (*ValidationResult, error) {
	doc, ok := ed.getDocument(uri)
	if !ok {
		return nil, ErrDocumentNotFound
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)
	return doc.validate(), nil
}

func (d *Document) validate() *ValidationResult {
	result := &ValidationResult{
		SourceSize: len(d.source),
	}
	if d.language != nil {
		result.Language = d.language.Name
	}

	// 检测行尾风格
	result.LineEnding = detectLineEnding(d.source)

	// 检测末尾换行（文件非空且以 \n 结尾）
	if len(d.source) > 0 && d.source[len(d.source)-1] == '\n' {
		result.TrailingNewline = true
	}

	// 检测语法错误
	root := d.tree.RootNode()
	if root != nil && root.HasError() {
		result.SyntaxErrors = collectSyntaxErrors(root, d.language)
		result.Valid = false
	} else {
		result.Valid = true
	}

	return result
}

// collectSyntaxErrors 递归遍历语法树，收集所有错误或缺失节点。
func collectSyntaxErrors(node *gotreesitter.Node, lang *gotreesitter.Language) []SyntaxError {
	var errs []SyntaxError
	collectSyntaxErrorsRecursive(node, lang, &errs)
	return errs
}

func collectSyntaxErrorsRecursive(node *gotreesitter.Node, lang *gotreesitter.Language, errs *[]SyntaxError) {
	if node == nil {
		return
	}

	if node.Type(lang) == "ERROR" || node.IsMissing() {
		sp := node.StartPoint()
		ep := node.EndPoint()
		typ := "error"
		if node.IsMissing() {
			typ = "missing"
		}
		*errs = append(*errs, SyntaxError{
			Type:     typ,
			StartRow: sp.Row,
			StartCol: sp.Column,
			EndRow:   ep.Row,
			EndCol:   ep.Column,
		})
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		collectSyntaxErrorsRecursive(node.Child(i), lang, errs)
	}
}

// collectErrors 收集语法树中的错误节点（字符串格式）。
// 与 collectSyntaxErrors 共享递归遍历逻辑。
func collectErrors(node *gotreesitter.Node, lang *gotreesitter.Language) []string {
	errs := collectSyntaxErrors(node, lang)
	strs := make([]string, len(errs))
	for i, e := range errs {
		strs[i] = fmt.Sprintf("%s node at [%d:%d]-[%d:%d]",
			e.Type, e.StartRow, e.StartCol, e.EndRow, e.EndCol)
	}
	return strs
}
