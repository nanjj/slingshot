package editor

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewLineIndexEmpty(t *testing.T) {
	li := NewLineIndex([]byte{})
	if len(li.offsets) != 1 || li.offsets[0] != 0 {
		t.Errorf("empty source: offsets=%v, want [0]", li.offsets)
	}
}

func TestNewLineIndexSingleLine(t *testing.T) {
	li := NewLineIndex([]byte("hello"))
	if len(li.offsets) != 1 || li.offsets[0] != 0 {
		t.Errorf("single line: offsets=%v, want [0]", li.offsets)
	}
}

func TestNewLineIndexMultiLine(t *testing.T) {
	li := NewLineIndex([]byte("line1\nline2\nline3"))
	want := []uint32{0, 6, 12}
	if len(li.offsets) != len(want) {
		t.Fatalf("multi line: offsets=%v, want %v", li.offsets, want)
	}
	for i := range want {
		if li.offsets[i] != want[i] {
			t.Errorf("multi line: offsets[%d]=%d, want %d", i, li.offsets[i], want[i])
		}
	}
}

func TestNewLineIndexTrailingNewline(t *testing.T) {
	li := NewLineIndex([]byte("line1\nline2\n"))
	if len(li.offsets) != 3 {
		t.Fatalf("trailing newline: len=%d, want 3; offsets=%v", len(li.offsets), li.offsets)
	}
	if li.offsets[0] != 0 || li.offsets[1] != 6 || li.offsets[2] != 12 {
		t.Errorf("trailing newline: offsets=%v, want [0, 6, 12]", li.offsets)
	}
}

func TestNewLineIndexCRLF(t *testing.T) {
	li := NewLineIndex([]byte("line1\r\nline2\r\n"))
	want := []uint32{0, 7, 14}
	if len(li.offsets) != len(want) {
		t.Fatalf("CRLF: offsets=%v, want %v", li.offsets, want)
	}
	for i := range want {
		if li.offsets[i] != want[i] {
			t.Errorf("CRLF: offsets[%d]=%d, want %d", i, li.offsets[i], want[i])
		}
	}
}

func TestPointToByte(t *testing.T) {
	li := NewLineIndex([]byte("hello\nworld\nfoo"))
	tests := []struct {
		row, col, want uint32
	}{
		{0, 0, 0},
		{0, 3, 3},
		{1, 0, 6},
		{1, 2, 8},
		{2, 0, 12},
		{2, 2, 14},
	}
	for _, tc := range tests {
		got := li.PointToByte(tc.row, tc.col)
		if got != tc.want {
			t.Errorf("PointToByte(%d,%d)=%d, want %d", tc.row, tc.col, got, tc.want)
		}
	}
}

func TestPointToByteOutOfRangeRow(t *testing.T) {
	li := NewLineIndex([]byte("hello\nworld"))
	got := li.PointToByte(999, 0)
	if got != 6 {
		t.Errorf("PointToByte(999,0)=%d, want 6 (last line offset)", got)
	}
}

func TestPointToByteColumnPastLineEnd(t *testing.T) {
	li := NewLineIndex([]byte("hello\nworld"))
	// Column past line end is not validated
	got := li.PointToByte(0, 100)
	if got != 100 {
		t.Errorf("PointToByte(0,100)=%d, want 100", got)
	}
}

func TestByteToPoint(t *testing.T) {
	li := NewLineIndex([]byte("hello\nworld\nfoo"))
	tests := []struct {
		offset           uint32
		wantRow, wantCol uint32
	}{
		{0, 0, 0},
		{3, 0, 3},
		{5, 0, 5},
		{6, 1, 0},
		{8, 1, 2},
		{11, 1, 5},
		{12, 2, 0},
		{14, 2, 2},
	}
	for _, tc := range tests {
		row, col := li.ByteToPoint(tc.offset)
		if row != tc.wantRow || col != tc.wantCol {
			t.Errorf("ByteToPoint(%d)=(%d,%d), want (%d,%d)",
				tc.offset, row, col, tc.wantRow, tc.wantCol)
		}
	}
}

func TestByteToPointEmpty(t *testing.T) {
	li := NewLineIndex([]byte{})
	row, col := li.ByteToPoint(0)
	if row != 0 || col != 0 {
		t.Errorf("empty ByteToPoint(0)=(%d,%d), want (0,0)", row, col)
	}
}

func TestByteToPointOnNewline(t *testing.T) {
	source := []byte("ab\ncd\n")
	li := NewLineIndex(source)
	tests := []struct {
		offset           uint32
		wantRow, wantCol uint32
	}{
		{0, 0, 0},
		{1, 0, 1},
		{2, 0, 2}, // 'b'
		{3, 1, 0}, // start of line 1
		{5, 1, 2}, // 'd'
		{6, 2, 0}, // start of line 2 (past last \n)
	}
	for _, tc := range tests {
		row, col := li.ByteToPoint(tc.offset)
		if row != tc.wantRow || col != tc.wantCol {
			t.Errorf("ByteToPoint(%d)=(%d,%d), want (%d,%d)",
				tc.offset, row, col, tc.wantRow, tc.wantCol)
		}
	}
}

func TestByteToPointEmptySource(t *testing.T) {
	li := NewLineIndex(nil)
	row, col := li.ByteToPoint(0)
	if row != 0 || col != 0 {
		t.Errorf("nil source ByteToPoint(0)=%d,%d, want (0,0)", row, col)
	}
}

func TestApplyEditRebuildsIndex(t *testing.T) {
	li := NewLineIndex([]byte("hello\nworld"))
	// ApplyEdit ignores the InputEdit and rebuilds from newSource
	li.ApplyEdit(gotreesitter.InputEdit{}, []byte("Xhello\nworld"))
	if len(li.offsets) != 2 {
		t.Fatalf("after edit: got %d offsets, want 2; offsets=%v", len(li.offsets), li.offsets)
	}
	if li.offsets[0] != 0 || li.offsets[1] != 7 {
		t.Errorf("after edit: offsets=%v, want [0, 7]", li.offsets)
	}
}

func TestApplyEditWithEmptySource(t *testing.T) {
	li := NewLineIndex([]byte("hello\nworld"))
	li.ApplyEdit(gotreesitter.InputEdit{}, []byte{})
	if len(li.offsets) != 1 || li.offsets[0] != 0 {
		t.Errorf("empty after edit: offsets=%v, want [0]", li.offsets)
	}
}

func TestRoundTripPointBytePoint(t *testing.T) {
	source := []byte("hello\nworld\nfoo\nbar")
	li := NewLineIndex(source)
	// For any valid offset, ByteToPoint → PointToByte should round-trip
	offsets := []uint32{0, 3, 5, 6, 8, 11, 12, 14, 17}
	for _, off := range offsets {
		row, col := li.ByteToPoint(off)
		got := li.PointToByte(row, col)
		if got != off {
			t.Errorf("round-trip: ByteToPoint(%d)->(%d,%d)->PointToByte=%d, want %d",
				off, row, col, got, off)
		}
	}
}

func TestRoundTripPointBytePointMultiLine(t *testing.T) {
	source := []byte("line1\nline2\nline3\nline4\n")
	li := NewLineIndex(source)
	for off := uint32(0); off < uint32(len(source)); off++ {
		row, col := li.ByteToPoint(off)
		got := li.PointToByte(row, col)
		if got != off {
			t.Errorf("round-trip at %d: got %d", off, got)
			break
		}
	}
}
