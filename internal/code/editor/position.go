package editor

import (
	"sort"

	"github.com/odvcencio/gotreesitter"
)

// LineIndex 缓存每行的起始字节偏移，支持行列↔字节的高效转换。
type LineIndex struct {
	offsets []uint32 // 每行的起始字节偏移，offsets[0]=0
}

// NewLineIndex 从源码创建 LineIndex。
func NewLineIndex(source []byte) *LineIndex {
	li := &LineIndex{offsets: []uint32{0}}
	for i, b := range source {
		if b == '\n' {
			li.offsets = append(li.offsets, uint32(i+1))
		}
	}
	return li
}

// PointToByte 将行列坐标转换为字节偏移。
// 如果行号超出范围，返回最后一行的末尾偏移。
func (li *LineIndex) PointToByte(row, col uint32) uint32 {
	if int(row) < len(li.offsets) {
		return li.offsets[row] + col
	}
	// 行号超出范围：返回最后一行的末尾
	return li.offsets[len(li.offsets)-1]
}

// ByteToPoint 将字节偏移转换为行列坐标。
func (li *LineIndex) ByteToPoint(offset uint32) (uint32, uint32) {
	// 二分查找最后一个 offsets[i] <= offset 的行
	idx := sort.Search(len(li.offsets), func(i int) bool {
		return li.offsets[i] > offset
	})
	row := idx - 1
	if row < 0 {
		row = 0
	}
	col := offset - li.offsets[row]
	return uint32(row), col
}

// ApplyEdit 在编辑后更新 LineIndex。
// 简单的策略：全部重建。对于编辑器场景性能足够。
func (li *LineIndex) ApplyEdit(_ gotreesitter.InputEdit, newSource []byte) {
	*li = *NewLineIndex(newSource)
}
