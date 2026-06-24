package edit

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
)

// NodeSelector 用于定位代码中的位置或节点。
type NodeSelector struct {
	Pos   *uint32    `json:"pos,omitempty"`
	Point *[2]uint32 `json:"point,omitempty"` // [row, col]
	Range *[2]uint32 `json:"range,omitempty"` // [startByte, endByte]
	Path  []PathStep `json:"path,omitempty"`
}

// PathStep 描述路径中的一步。
type PathStep struct {
	Type       string `json:"type"`
	Field      string `json:"field,omitempty"`
	ChildIndex int    `json:"childIndex,omitempty"` // 1-based（AI 友好），>0 才参与比较
	NamedOnly  bool   `json:"namedOnly,omitempty"`
}

// resolveNode 根据 NodeSelector 解析出目标节点。
func (d *Document) resolveNode(sel NodeSelector) (*gotreesitter.Node, error) {
	root := d.tree.RootNode()

	switch {
	case sel.Pos != nil:
		node := root.DescendantForByteRange(*sel.Pos, *sel.Pos)
		if node == nil {
			return nil, ErrNodeNotFound
		}
		return node, nil

	case sel.Point != nil:
		p := gotreesitter.Point{
			Row:    (*sel.Point)[0],
			Column: (*sel.Point)[1],
		}
		node := root.DescendantForPointRange(p, p)
		if node == nil {
			return nil, ErrNodeNotFound
		}
		return node, nil

	case sel.Range != nil:
		start := (*sel.Range)[0]
		end := (*sel.Range)[1]
		node := root.DescendantForByteRange(start, end)
		if node == nil {
			return nil, ErrNodeNotFound
		}
		return node, nil

	case len(sel.Path) > 0:
		return d.resolvePath(sel.Path)

	default:
		return nil, fmt.Errorf("empty node selector")
	}
}

// resolvePath 按路径逐层匹配节点。
func (d *Document) resolvePath(path []PathStep) (*gotreesitter.Node, error) {
	node := d.tree.RootNode()
	for _, step := range path {
		found := false
		count := int(node.ChildCount())
		if step.NamedOnly {
			count = int(node.NamedChildCount())
		}
		for i := 0; i < count; i++ {
			var child *gotreesitter.Node
			if step.NamedOnly {
				child = node.NamedChild(i)
			} else {
				child = node.Child(i)
			}
			if child == nil {
				continue
			}

			if step.Type != "" && child.Type(d.language) != step.Type {
				continue
			}
			if step.Field != "" {
				fieldChild := node.ChildByFieldName(step.Field, d.language)
				if fieldChild != child {
					continue
				}
			}
			if step.ChildIndex > 0 && step.ChildIndex != i+1 {
				continue
			}
			node = child
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("%w: path step not found: %+v", ErrNodeNotFound, step)
		}
	}
	return node, nil
}
