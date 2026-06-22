package editor

import (
	"fmt"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// applyEdit 在 Document 上执行编辑并维护增量解析。
func (d *Document) applyEdit(inputEdit gotreesitter.InputEdit, newSource []byte) error {
	// 1. 更新旧树的位置信息
	d.tree.Edit(inputEdit)

	// 2. 增量解析
	result, err := d.parser.ParseWith(newSource,
		gotreesitter.WithOldTree(d.tree),
	)
	if err != nil {
		// 增量解析失败，回退到全量解析
		tree, fallbackErr := d.parser.Parse(newSource)
		if fallbackErr != nil {
			return fmt.Errorf("reparse failed: %w", fallbackErr)
		}
		d.tree.Release()
		d.tree = tree
	} else {
		// 增量解析成功
		d.tree.Release()
		d.tree = result.Tree
	}

	// 3. 更新状态
	d.source = newSource
	d.bound = gotreesitter.Bind(d.tree)
	d.lineIdx.ApplyEdit(inputEdit, newSource)
	d.dirty = true
	d.version++
	d.modifiedAt = time.Now()

	return nil
}

// applyEdits 批量应用多个 InputEdit（来自 Rewriter.Apply）。
func (d *Document) applyEdits(edits []gotreesitter.InputEdit, newSource []byte) error {
	// 应用所有 InputEdit 到旧树
	for _, edit := range edits {
		d.tree.Edit(edit)
	}

	// 增量解析
	result, err := d.parser.ParseWith(newSource,
		gotreesitter.WithOldTree(d.tree),
	)
	if err != nil {
		tree, fallbackErr := d.parser.Parse(newSource)
		if fallbackErr != nil {
			return fmt.Errorf("reparse failed: %w", fallbackErr)
		}
		d.tree.Release()
		d.tree = tree
	} else {
		d.tree.Release()
		d.tree = result.Tree
	}

	d.source = newSource
	d.bound = gotreesitter.Bind(d.tree)
	d.lineIdx = NewLineIndex(newSource)
	d.dirty = true
	d.version++
	d.modifiedAt = time.Now()

	return nil
}
