package editor

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// Editor 是 Treesitter AI Editor 的主入口。
// 每个项目创建一个 Editor 实例，管理项目下所有文档。
// projectRoot 在创建时固定，不可变。
type Editor struct {
	mu          sync.Mutex
	documents   sync.Map // uri -> *Document
	projectRoot string   // 项目根目录（创建时固定）
}

// NewEditor 创建新的编辑器实例。
// projectRoot 是项目根目录，用于将相对 URI 解析为绝对路径。
// 传入空字符串可禁用路径解析（仅接受 file:// 绝对 URI）。
func NewEditor(projectRoot string) *Editor {
	return &Editor{
		projectRoot: projectRoot,
	}
}

// ProjectRoot 返回项目根目录。
// projectRoot 在创建时固定，调用安全无需加锁。
func (ed *Editor) ProjectRoot() string {
	return ed.projectRoot
}

// ─── EditorManager：多项目 Editor 实例管理 ─────────────────────────────────

// EditorManager 管理多个 Editor 实例，每个项目根目录对应一个独立实例。
// 切换项目时切换 Editor 实例，各项目的文档状态、脏页等编辑状态完全隔离。
type EditorManager struct {
	mu      sync.Mutex
	editors map[string]*Editor // keyed by resolved absolute path
	current string             // 当前活跃的 editor key
}

// NewEditorManager 创建 EditorManager。
// initialRoot 为初始项目根目录，会自动创建对应的 Editor 实例。
func NewEditorManager(initialRoot string) (*EditorManager, error) {
	abs, err := filepath.Abs(initialRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve initial root: %w", err)
	}
	em := &EditorManager{
		editors: make(map[string]*Editor),
	}
	em.editors[abs] = NewEditor(abs)
	em.current = abs
	return em, nil
}

// SwitchTo 切换到指定项目根目录的 Editor 实例。
// 如果该目录尚未创建 Editor，则自动创建新实例。
// 返回对应的 Editor，同时设为当前活跃实例。
// 切换后之前项目的编辑状态（已打开的文档、脏页标记、AST 树）保留不变。
func (em *EditorManager) SwitchTo(root string) (*Editor, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	em.mu.Lock()
	defer em.mu.Unlock()
	if ed, ok := em.editors[abs]; ok {
		em.current = abs
		return ed, nil
	}
	ed := NewEditor(abs)
	em.editors[abs] = ed
	em.current = abs
	return ed, nil
}

// Current 返回当前活跃的 Editor 实例。
func (em *EditorManager) Current() *Editor {
	em.mu.Lock()
	defer em.mu.Unlock()
	return em.editors[em.current]
}

// ─── 文档生命周期 ───

// OpenDocument 打开（或创建）一个文档。
//
// source 的行为取决于 URI 类型和文件是否存在：
//
//	file:// URI + 文件存在 → 从磁盘加载（source 被忽略，
//	                          如果 source≠nil 且内容不同，在日志中记录警告）
//	file:// URI + 文件不存在 → 用 source 初始化新建文件
//	                           （source=nil 则创建空文档）
//
// 典型使用模式：
//
//	editor.OpenDocument("file:///main.go", nil, "go")        // 打开已有文件
//	editor.OpenDocument("file:///new.go", genCode, "go")     // AI 生成新文件
//	editor.OpenDocument("scratch:///snippet", code, "go")    // 分析代码片段
func (ed *Editor) OpenDocument(uri string, source []byte, languageName string) error {
	// 1. 关闭同名已有文档（如有）
	if existing, ok := ed.documents.Load(uri); ok {
		doc := existing.(*Document)
		doc.Lock()
		if doc.tree != nil {
			doc.tree.Release()
		}
		if doc.bound != nil {
			doc.bound.Release()
		}
		doc.Unlock()
		ed.documents.Delete(uri)
	}

	// 2. 解析 URI 为文件系统路径（考虑 projectRoot）
	filePath, err := ed.resolveDocumentPath(uri)
	if err != nil {
		filePath = "" // 非文件 URI（如 scratch://），不能 Save
	}

	// 3. 检测文件是否存在
	var fileExists bool
	var fileInfo os.FileInfo
	if filePath != "" {
		fileInfo, fileExists = checkFileExists(filePath)
	}

	// 4. 确定初始源码
	var initSource []byte
	if fileExists {
		initSource, err = os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		if source != nil && !bytes.Equal(source, initSource) {
			log.Printf("warning: OpenDocument %q: file exists on disk, "+
				"source parameter ignored (disk and source differ)", uri)
		}
	} else {
		if source != nil {
			initSource = source
		} else {
			initSource = []byte{}
		}
	}

	// 5. 语言检测（优先用文件路径，回退到 URI 中的路径部分）
	lang, err := detectLanguage(filePath, initSource, languageName)
	if err != nil {
		// 尝试从 URI 提取路径部分用于语言检测（如 scratch:///test.go）
		virtualPath := extractVirtualPath(uri)
		if virtualPath != "" {
			lang, err = detectLanguage(virtualPath, initSource, languageName)
		}
		if err != nil {
			return fmt.Errorf("detect language: %w", err)
		}
	}

	// 6. 创建解析器并解析
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(initSource)
	if err != nil {
		return fmt.Errorf("initial parse: %w", err)
	}

	// 7. 构建 Document
	doc := &Document{
		uri:        uri,
		language:   parser.Language(),
		source:     initSource,
		tree:       tree,
		parser:     parser,
		bound:      gotreesitter.Bind(tree),
		lineIdx:    NewLineIndex(initSource),
		encoding:   detectEncoding(initSource),
		origFilePath: filePath,
		createdAt:  time.Now(),
		modifiedAt: time.Now(),
	}
	if fileExists {
		doc.origFileMode = fileInfo.Mode()
		doc.origFileSize = fileInfo.Size()
		doc.origModTime = fileInfo.ModTime()
		doc.lastSavedMTime = fileInfo.ModTime()
	}

	// 8. 注册文档
	ed.documents.Store(uri, doc)
	return nil
}

// CloseDocument 关闭文档，释放资源。
// 不会自动保存。如果文档 dirty，记录警告。
func (ed *Editor) CloseDocument(uri string) error {
	doc, ok := ed.getDocument(uri)
	if !ok {
		return ErrDocumentNotFound
	}

	doc.Lock()
	defer doc.Unlock()

	if doc.dirty {
		log.Printf("warning: CloseDocument %q: document has unsaved changes, discarding", uri)
	}

	if doc.tree != nil {
		doc.tree.Release()
		doc.tree = nil
	}
	if doc.bound != nil {
		doc.bound.Release()
		doc.bound = nil
	}

	ed.documents.Delete(uri)
	return nil
}

// ─── 只读操作（原 View 方法） ───

// GetStructure 返回根节点的层次化结构。
// maxDepth=-1 表示递归到叶子，maxChildren=-1 表示不限制。
func (ed *Editor) GetStructure(uri string, maxDepth, maxChildren int) (NodeInfo, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return NodeInfo{}, err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if doc.tree == nil {
		return NodeInfo{}, fmt.Errorf("document has no tree")
	}
	root := doc.tree.RootNode()
	return buildStructure(root, doc.language, doc.source, 0, maxDepth, maxChildren), nil
}

// GetNode 按字节位置获取最小节点。
func (ed *Editor) GetNode(uri string, pos uint32) (NodeInfo, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return NodeInfo{}, err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if doc.tree == nil {
		return NodeInfo{}, ErrDocumentNotReady
	}
	root := doc.tree.RootNode()
	node := root.DescendantForByteRange(pos, pos)
	if node == nil {
		return NodeInfo{}, ErrNodeNotFound
	}
	return nodeToInfo(node, doc.language, doc.source), nil
}

// GetNodeAtPoint 按行列位置获取最小节点。
func (ed *Editor) GetNodeAtPoint(uri string, row, col uint32) (NodeInfo, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return NodeInfo{}, err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if doc.tree == nil {
		return NodeInfo{}, ErrDocumentNotReady
	}
	p := gotreesitter.Point{Row: row, Column: col}
	root := doc.tree.RootNode()
	node := root.DescendantForPointRange(p, p)
	if node == nil {
		return NodeInfo{}, ErrNodeNotFound
	}
	return nodeToInfo(node, doc.language, doc.source), nil
}

// GetNodeAtRange 获取覆盖 [startByte, endByte) 的最小节点。
func (ed *Editor) GetNodeAtRange(uri string, startByte, endByte uint32) (NodeInfo, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return NodeInfo{}, err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if doc.tree == nil {
		return NodeInfo{}, ErrDocumentNotReady
	}
	root := doc.tree.RootNode()
	node := root.DescendantForByteRange(startByte, endByte)
	if node == nil {
		return NodeInfo{}, ErrNodeNotFound
	}
	return nodeToInfo(node, doc.language, doc.source), nil
}

// GetDescendantsAt 获取覆盖指定字节位置的所有祖先节点（从内到外）。
func (ed *Editor) GetDescendantsAt(uri string, pos uint32) ([]NodeInfo, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if doc.tree == nil {
		return nil, ErrDocumentNotReady
	}
	root := doc.tree.RootNode()
	leaf := root.DescendantForByteRange(pos, pos)
	if leaf == nil {
		return nil, ErrNodeNotFound
	}

	var result []NodeInfo
	for n := leaf; n != nil; n = n.Parent() {
		result = append(result, nodeToInfo(n, doc.language, doc.source))
	}
	return result, nil
}

// Query 执行 tree-sitter 查询，返回匹配结果。
func (ed *Editor) Query(uri string, pattern string) ([]QueryResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if doc.tree == nil {
		return nil, ErrDocumentNotReady
	}
	q, err := gotreesitter.NewQuery(pattern, doc.language)
	if err != nil {
		return nil, fmt.Errorf("compile query: %w", err)
	}

	matches := q.Execute(doc.tree)
	results := make([]QueryResult, 0, len(matches))

	for _, match := range matches {
		qr := QueryResult{
			Pattern:  match.PatternIndex,
			Captures: make(map[string][]NodeInfo),
		}
		for _, cap := range match.Captures {
			qr.Captures[cap.Name] = append(qr.Captures[cap.Name], nodeToInfo(cap.Node, doc.language, doc.source))
		}
		results = append(results, qr)
	}
	return results, nil
}

// GetText 获取指定范围内的原始文本。
func (ed *Editor) GetText(uri string, startByte, endByte uint32) (string, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return "", err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	if startByte > endByte || endByte > uint32(len(doc.source)) {
		return "", ErrInvalidPosition
	}
	return string(doc.source[startByte:endByte]), nil
}

// GetSource 返回文档的全文。
func (ed *Editor) GetSource(uri string) (string, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return "", err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)
	return string(doc.source), nil
}


// GetLine 获取指定行。
func (ed *Editor) GetLine(uri string, line uint32) (string, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return "", err
	}
	doc.Lock()
	defer doc.Unlock()
	ed.reloadIfExternalModified(doc)

	source := doc.source
	li := doc.lineIdx

	if int(line) >= len(li.offsets) {
		return "", ErrInvalidPosition
	}

	start := li.offsets[line]
	var end uint32
	if int(line)+1 < len(li.offsets) {
		end = li.offsets[line+1]
	} else {
		end = uint32(len(source))
	}

	lineBytes := source[start:end]
	if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
		lineBytes = lineBytes[:len(lineBytes)-1]
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\r' {
			lineBytes = lineBytes[:len(lineBytes)-1]
		}
	}
	return string(lineBytes), nil
}

// ─── 写入操作（原 Edit 方法） ───

// Insert 在指定字节偏移位置插入文本。
func (ed *Editor) Insert(uri string, pos uint32, text string) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	// 将插入文本的行尾转换为文档风格
	text = string(doc.normalizeLineEndings([]byte(text)))

	source := doc.source
	if pos > uint32(len(source)) {
		return nil, ErrInvalidPosition
	}

	inputEdit := gotreesitter.InputEdit{
		StartByte:  pos,
		OldEndByte: pos,
		NewEndByte: pos + uint32(len(text)),
	}
	row, col := doc.lineIdx.ByteToPoint(pos)
	inputEdit.StartPoint = gotreesitter.Point{Row: row, Column: col}
	inputEdit.OldEndPoint = inputEdit.StartPoint

	newSource := make([]byte, 0, len(source)+len(text))
	newSource = append(newSource, source[:pos]...)
	newSource = append(newSource, text...)
	newSource = append(newSource, source[pos:]...)

	endRow, endCol := doc.lineIdx.ByteToPoint(pos + uint32(len(text)))
	inputEdit.NewEndPoint = gotreesitter.Point{Row: endRow, Column: endCol}

	oldLen := len(doc.source)
	err = doc.applyEdit(inputEdit, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// InsertAtPoint 在行列位置插入文本。
func (ed *Editor) InsertAtPoint(uri string, row, col uint32, text string) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	// 将插入文本的行尾转换为文档风格
	text = string(doc.normalizeLineEndings([]byte(text)))

	pos := doc.lineIdx.PointToByte(row, col)
	if pos > uint32(len(doc.source)) {
		return nil, ErrInvalidPosition
	}

	inputEdit := gotreesitter.InputEdit{
		StartByte:   pos,
		OldEndByte:  pos,
		NewEndByte:  pos + uint32(len(text)),
		StartPoint:  gotreesitter.Point{Row: row, Column: col},
		OldEndPoint: gotreesitter.Point{Row: row, Column: col},
	}

	newSource := make([]byte, 0, len(doc.source)+len(text))
	newSource = append(newSource, doc.source[:pos]...)
	newSource = append(newSource, text...)
	newSource = append(newSource, doc.source[pos:]...)

	endRow, endCol := doc.lineIdx.ByteToPoint(pos + uint32(len(text)))
	inputEdit.NewEndPoint = gotreesitter.Point{Row: endRow, Column: endCol}

	oldLen := len(doc.source)
	err = doc.applyEdit(inputEdit, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// InsertBefore 在节点前插入（自动调整缩进）。
func (ed *Editor) InsertBefore(uri string, sel NodeSelector, text string) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	node, err := doc.resolveNode(sel)
	if err != nil {
		return nil, err
	}

	// 将插入文本的行尾转换为文档风格
	text = string(doc.normalizeLineEndings([]byte(text)))

	rw := gotreesitter.NewRewriter(doc.source)
	rw.InsertBefore(node, []byte(text))
	newSource, edits, err := rw.Apply()
	if err != nil {
		return nil, fmt.Errorf("rewriter: %w", err)
	}

	oldLen := len(doc.source)
	err = doc.applyEdits(edits, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// InsertAfter 在节点后插入（自动调整缩进）。
func (ed *Editor) InsertAfter(uri string, sel NodeSelector, text string) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	node, err := doc.resolveNode(sel)
	if err != nil {
		return nil, err
	}

	// 将插入文本的行尾转换为文档风格
	text = string(doc.normalizeLineEndings([]byte(text)))

	rw := gotreesitter.NewRewriter(doc.source)
	rw.InsertAfter(node, []byte(text))
	newSource, edits, err := rw.Apply()
	if err != nil {
		return nil, fmt.Errorf("rewriter: %w", err)
	}

	oldLen := len(doc.source)
	err = doc.applyEdits(edits, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// Replace 替换指定字节范围的内容。
func (ed *Editor) Replace(uri string, startByte, endByte uint32, text string) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	if startByte > endByte || endByte > uint32(len(doc.source)) {
		return nil, ErrInvalidPosition
	}

	// 将替换文本的行尾转换为文档风格
	text = string(doc.normalizeLineEndings([]byte(text)))

	rw := gotreesitter.NewRewriter(doc.source)
	rw.ReplaceRange(startByte, endByte, []byte(text))
	newSource, edits, err := rw.Apply()
	if err != nil {
		return nil, fmt.Errorf("rewriter: %w", err)
	}

	oldLen := len(doc.source)
	err = doc.applyEdits(edits, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// ReplaceNode 替换节点内容。
func (ed *Editor) ReplaceNode(uri string, sel NodeSelector, text string) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	node, err := doc.resolveNode(sel)
	if err != nil {
		return nil, err
	}

	// 将替换文本的行尾转换为文档风格
	text = string(doc.normalizeLineEndings([]byte(text)))

	rw := gotreesitter.NewRewriter(doc.source)
	rw.Replace(node, []byte(text))
	newSource, edits, err := rw.Apply()
	if err != nil {
		return nil, fmt.Errorf("rewriter: %w", err)
	}

	oldLen := len(doc.source)
	err = doc.applyEdits(edits, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// Delete 删除指定字节范围。使用 ReplaceRange(nil) 实现。
func (ed *Editor) Delete(uri string, startByte, endByte uint32) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	if startByte > endByte || endByte > uint32(len(doc.source)) {
		return nil, ErrInvalidPosition
	}

	rw := gotreesitter.NewRewriter(doc.source)
	rw.ReplaceRange(startByte, endByte, nil)
	newSource, edits, err := rw.Apply()
	if err != nil {
		return nil, fmt.Errorf("rewriter: %w", err)
	}

	oldLen := len(doc.source)
	err = doc.applyEdits(edits, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// DeleteNode 删除节点。
func (ed *Editor) DeleteNode(uri string, sel NodeSelector) (*EditResult, error) {
	doc, err := ed.getOrOpenDocument(uri)
	if err != nil {
		return nil, err
	}
	doc.Lock()
	defer doc.Unlock()

	node, err := doc.resolveNode(sel)
	if err != nil {
		return nil, err
	}

	rw := gotreesitter.NewRewriter(doc.source)
	rw.Delete(node)
	newSource, edits, err := rw.Apply()
	if err != nil {
		return nil, fmt.Errorf("rewriter: %w", err)
	}

	oldLen := len(doc.source)
	err = doc.applyEdits(edits, newSource)
	if err != nil {
		return nil, err
	}
	ed.saveAfterEdit(doc)
	return doc.successResult(newSource, oldLen), nil
}

// ─── 查询 ───

// GetDocument 返回文档。
// 不自动打开文档，仅查询已缓存的文档。
func (ed *Editor) GetDocument(uri string) (*Document, error) {
	doc, ok := ed.getDocument(uri)
	if !ok {
		return nil, ErrDocumentNotFound
	}
	return doc, nil
}

// IsDirty 检查文档是否有未保存的更改。
func (ed *Editor) IsDirty(uri string) (bool, error) {
	doc, ok := ed.getDocument(uri)
	if !ok {
		return false, ErrDocumentNotFound
	}
	doc.Lock()
	defer doc.Unlock()
	return doc.dirty, nil
}

// DirtyDocuments 返回所有有未保存更改的文档 URI 列表。
func (ed *Editor) DirtyDocuments() []string {
	var dirty []string
	ed.documents.Range(func(key, value interface{}) bool {
		doc := value.(*Document)
		doc.Lock()
		if doc.dirty {
			dirty = append(dirty, key.(string))
		}
		doc.Unlock()
		return true
	})
	return dirty
}

// ─── 内部方法 ───

// getOrOpenDocument 获取或自动打开文档。
// 如果文档已在缓存中，直接返回；否则尝试自动打开。
func (ed *Editor) getOrOpenDocument(uri string) (*Document, error) {
	if doc, ok := ed.getDocument(uri); ok {
		return doc, nil
	}
	// 尝试自动打开
	if err := ed.OpenDocument(uri, nil, ""); err != nil {
		return nil, err
	}
	doc, ok := ed.getDocument(uri)
	if !ok {
		return nil, ErrDocumentNotFound
	}
	return doc, nil
}

// getDocument 从 sync.Map 获取文档。
func (ed *Editor) getDocument(uri string) (*Document, bool) {
	val, ok := ed.documents.Load(uri)
	if !ok {
		return nil, false
	}
	return val.(*Document), true
}

// resolveDocumentPath 将 URI 解析为文件系统路径，
// 相对路径会基于 projectRoot 进行解析。
func (ed *Editor) resolveDocumentPath(uri string) (string, error) {
	path, err := resolveURI(uri)
	if err != nil {
		return "", err
	}
	// projectRoot 在创建时固定，无需加锁
	if !filepath.IsAbs(path) && ed.projectRoot != "" {
		path = filepath.Join(ed.projectRoot, path)
	}
	return path, nil
}

// checkFileExists 检查文件是否存在并返回文件信息。
func checkFileExists(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if info.IsDir() {
		return nil, false
	}
	return info, true
}
// saveAfterEdit 在编辑操作后自动保存文件（假设文档已锁定）。
// scratch 文档（无文件路径）静默跳过。
// 如果保存失败，将 dirty 标记为 true 并记录警告。
func (ed *Editor) saveAfterEdit(doc *Document) {
	if doc.origFilePath == "" {
		return // scratch 文档无文件可写
	}
	_, err := atomicWrite(doc.origFilePath, doc.source, doc.origFileMode)
	if err != nil {
		log.Printf("warning: auto-save failed for %s: %v", doc.uri, err)
		doc.dirty = true
		return
	}
	doc.dirty = false
	doc.origFileSize = int64(len(doc.source))
	if fi, err := os.Stat(doc.origFilePath); err == nil {
		doc.lastSavedMTime = fi.ModTime()
	}
}

// reloadIfExternalModified 检测外部修改并自动重载文档（假设文档已锁定）。
// 读取操作前调用，确保读取的是最新内容。
// 如果文件在外部被修改，重新从磁盘加载并全量解析。
func (ed *Editor) reloadIfExternalModified(doc *Document) {
	if doc.origFilePath == "" {
		return // scratch 文档
	}
	fi, err := os.Stat(doc.origFilePath)
	if err != nil {
		return // 文件已删除或无法访问
	}
	if fi.ModTime().Equal(doc.lastSavedMTime) && fi.Size() == doc.origFileSize {
		return // 无外部修改
	}
	// 外部有修改，重新加载
	source, err := os.ReadFile(doc.origFilePath)
	if err != nil {
		log.Printf("warning: auto-reload read failed for %s: %v", doc.uri, err)
		return
	}
	newTree, err := doc.parser.Parse(source)
	if err != nil {
		log.Printf("warning: auto-reload parse failed for %s: %v", doc.uri, err)
		return
	}
	if doc.tree != nil {
		doc.tree.Release()
	}
	doc.source = source
	doc.tree = newTree
	doc.bound = gotreesitter.Bind(newTree)
	doc.lineIdx = NewLineIndex(source)
	doc.dirty = false
	doc.version = 0
	doc.modifiedAt = time.Now()
	doc.origFileSize = fi.Size()
	doc.origModTime = fi.ModTime()
	doc.lastSavedMTime = fi.ModTime()
}

// extractVirtualPath 从非文件 URI 中提取路径部分，用于语言检测。
// 例如 "scratch:///test.go" → "/test.go"，"scratch:///snippet" → "/snippet"。
func extractVirtualPath(uri string) string {
	// 查找 "://" 后的第一个 '/'
	for i := 0; i < len(uri)-3; i++ {
		if uri[i] == ':' && uri[i+1] == '/' && uri[i+2] == '/' {
			pathStart := i + 3
			if pathStart < len(uri) && uri[pathStart] == '/' {
				pathStart++
			}
			if pathStart < len(uri) {
				return uri[pathStart-1:] // include leading /
			}
			return ""
		}
	}
	return ""
}
