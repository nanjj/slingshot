package editor

import (
	"github.com/odvcencio/gotreesitter"
)

// NodeInfo 是节点的结构化描述。
type NodeInfo struct {
	Type       string     `json:"type"`
	StartByte  uint32     `json:"startByte"`
	EndByte    uint32     `json:"endByte"`
	StartPoint [2]uint32 `json:"startPoint"`
	EndPoint   [2]uint32 `json:"endPoint"`
	Text       string     `json:"text,omitempty"`
	IsNamed    bool       `json:"isNamed"`
	IsError    bool       `json:"isError"`
	IsMissing  bool       `json:"isMissing"`
	FieldName  string     `json:"fieldName,omitempty"`
	Children   []NodeInfo `json:"children,omitempty"`
}

// QueryResult 是查询匹配结果。
type QueryResult struct {
	Pattern  int                    `json:"pattern"`
	Captures map[string][]NodeInfo `json:"captures"`
}

// buildStructure 递归构建节点结构。
func buildStructure(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte, depth, maxDepth, maxWidth int) NodeInfo {
	if node == nil {
		return NodeInfo{}
	}

	sp := node.StartPoint()
	ep := node.EndPoint()

	info := NodeInfo{
		Type:       node.Type(lang),
		StartByte:  node.StartByte(),
		EndByte:    node.EndByte(),
		StartPoint: [2]uint32{sp.Row, sp.Column},
		EndPoint:   [2]uint32{ep.Row, ep.Column},
		IsNamed:    node.IsNamed(),
		IsError:    node.HasError(),
		IsMissing:  node.IsMissing(),
	}

	childCount := int(node.ChildCount())
	if childCount == 0 || (maxDepth != -1 && depth >= maxDepth) {
		if node.IsNamed() || info.Type == "comment" {
			info.Text = node.Text(source)
			if len(info.Text) > 200 {
				info.Text = info.Text[:200] + "..."
			}
		}
		return info
	}

	if maxWidth != -1 && childCount > maxWidth {
		childCount = maxWidth
	}

	info.Children = make([]NodeInfo, 0, childCount)
	for i := 0; i < childCount; i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		childInfo := buildStructure(child, lang, source, depth+1, maxDepth, maxWidth)
		info.Children = append(info.Children, childInfo)
	}

	return info
}

// nodeToInfo 将 gotreesitter.Node 转换为 NodeInfo。
func nodeToInfo(node *gotreesitter.Node, lang *gotreesitter.Language, source []byte) NodeInfo {
	sp := node.StartPoint()
	ep := node.EndPoint()

	return NodeInfo{
		Type:       node.Type(lang),
		StartByte:  node.StartByte(),
		EndByte:    node.EndByte(),
		StartPoint: [2]uint32{sp.Row, sp.Column},
		EndPoint:   [2]uint32{ep.Row, ep.Column},
		Text:       node.Text(source),
		IsNamed:    node.IsNamed(),
		IsError:    node.HasError(),
		IsMissing:  node.IsMissing(),
	}
}
