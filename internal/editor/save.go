package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// SaveResult 保存操作结果。
type SaveResult struct {
	Success  bool   `json:"success"`
	Path     string `json:"path"`
	Bytes    int    `json:"bytes"`
	Version  int    `json:"version"`
	Conflict bool   `json:"conflict,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Save 保存文档到磁盘。
// 检测外部修改冲突，使用原子写入（临时文件 + rename）。
func (ed *Editor) Save(uri string) (*SaveResult, error) {
	return ed.save(uri, "", false)
}

// ForceSave 强制保存，跳过冲突检查。
func (ed *Editor) ForceSave(uri string) (*SaveResult, error) {
	return ed.save(uri, "", true)
}

// SaveAs 将文档保存到指定路径（不改变 Document.uri）。
func (ed *Editor) SaveAs(uri, newPath string) (*SaveResult, error) {
	return ed.save(uri, newPath, false)
}

// save 内部保存实现。
func (ed *Editor) save(uri, newPath string, force bool) (*SaveResult, error) {
	val, ok := ed.documents.Load(uri)
	if !ok {
		return nil, ErrDocumentNotFound
	}
	doc := val.(*Document)

	doc.Lock()
	defer doc.Unlock()

	targetPath := doc.origFilePath
	if newPath != "" {
		targetPath = newPath
	}
	if targetPath == "" {
		return nil, ErrNonFileURI
	}

	// 检查外部修改
	if !force && newPath == "" {
		info, err := os.Stat(targetPath)
		if err == nil && !info.ModTime().Equal(doc.lastSavedMTime) {
			return &SaveResult{
				Success:  false,
				Path:     targetPath,
				Conflict: true,
				Message:  "file modified externally",
			}, nil
		}
	}

	// 原子写入
	bytesWritten, err := atomicWrite(targetPath, doc.source, doc.origFileMode)
	if err != nil {
		return nil, fmt.Errorf("atomic write: %w", err)
	}

	// 更新文档状态
	doc.dirty = false
	doc.origFileSize = int64(len(doc.source))
	if newPath == "" {
		if fi, err := os.Stat(targetPath); err == nil {
			doc.lastSavedMTime = fi.ModTime()
		}
	}
	if newPath != "" {
		doc.origFilePath = newPath
		if fi, err := os.Stat(newPath); err == nil {
			doc.lastSavedMTime = fi.ModTime()
			doc.origFileMode = fi.Mode()
			doc.origModTime = fi.ModTime()
		}
	}

	return &SaveResult{
		Success: true,
		Path:    targetPath,
		Bytes:   bytesWritten,
		Version: doc.version,
	}, nil
}

// atomicWrite 原子写入文件。
func atomicWrite(targetPath string, data []byte, fileMode os.FileMode) (int, error) {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	tmpFile := filepath.Join(dir, "."+base+".tmp")

	// 新建文件默认 0644
	if fileMode == 0 {
		fileMode = 0644
	}

	if err := os.WriteFile(tmpFile, data, fileMode); err != nil {
		return 0, fmt.Errorf("write temp: %w", err)
	}
	if err := syncFile(tmpFile); err != nil {
		os.Remove(tmpFile)
		return 0, fmt.Errorf("sync temp: %w", err)
	}
	if err := os.Rename(tmpFile, targetPath); err != nil {
		os.Remove(tmpFile)
		return 0, fmt.Errorf("rename: %w", err)
	}
	if fileMode != 0644 {
		os.Chmod(targetPath, fileMode)
	}
	return len(data), nil
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// Reload 从磁盘重新加载文档。
func (ed *Editor) Reload(uri string) error {
	val, ok := ed.documents.Load(uri)
	if !ok {
		return ErrDocumentNotFound
	}
	doc := val.(*Document)

	doc.Lock()
	defer doc.Unlock()

	if doc.origFilePath == "" {
		return ErrNonFileURI
	}

	source, err := os.ReadFile(doc.origFilePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	newTree, err := doc.parser.Parse(source)
	if err != nil {
		return fmt.Errorf("reparse: %w", err)
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

	if fi, err := os.Stat(doc.origFilePath); err == nil {
		doc.origFileSize = fi.Size()
		doc.origModTime = fi.ModTime()
		doc.lastSavedMTime = fi.ModTime()
		doc.origFileMode = fi.Mode()
	}

	return nil
}
