// Package base implements SQLite-backed code graph storage and project indexing.
package base

// IndexMode controls how IndexProject processes files.
type IndexMode string

const (
	IndexModeFull     IndexMode = "full"     // all files + similarity/semantic edges
	IndexModeModerate IndexMode = "moderate" // filtered files + similarity
	IndexModeFast     IndexMode = "fast"     // filtered files, no similarity
)

// Node represents a code symbol in the knowledge graph.
type Node struct {
	ID                  int64  `json:"id"`
	ProjectID           int64  `json:"projectId"`
	QualifiedName       string `json:"qualifiedName"`
	Kind                string `json:"kind"`    // function, method, class, struct, interface, module, file, variable
	Name                string `json:"name"`    // short name (without package prefix)
	FilePath            string `json:"filePath"`
	Line                uint32 `json:"line"`
	Col                 uint32 `json:"col"`
	EndLine             uint32 `json:"endLine"`
	EndCol              uint32 `json:"endCol"`
	Signature           string `json:"signature,omitempty"`
	DocComment          string `json:"docComment,omitempty"`
	Complexity          int    `json:"complexity,omitempty"`          // cyclomatic complexity
	Cognitive           int    `json:"cognitive,omitempty"`           // cognitive complexity
	LoopDepth           int    `json:"loopDepth,omitempty"`           // max nested loop depth
	TransitiveLoopDepth int    `json:"transitiveLoopDepth,omitempty"` // propagated loop depth
	LoopCount           int    `json:"loopCount,omitempty"`
	Recursive           bool   `json:"recursive,omitempty"`
	ParamCount          int    `json:"paramCount,omitempty"`
	LinearScanInLoop    int    `json:"linearScanInLoop,omitempty"` // hidden O(n²) signals
	AllocInLoop         int    `json:"allocInLoop,omitempty"`
	RecursionInLoop     bool   `json:"recursionInLoop,omitempty"`
	UnguardedRecursion  bool   `json:"unguardedRecursion,omitempty"`
	MaxAccessDepth      int    `json:"maxAccessDepth,omitempty"`
}

// Edge represents a relationship between two code symbols.
type Edge struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"projectId"`
	SourceQN    string `json:"sourceQN"`
	TargetQN    string `json:"targetQN"`
	EdgeType    string `json:"edgeType"` // CALLS, IMPLEMENTS, CONTAINS, IMPORTS, REFERENCES, DATA_FLOWS
	Metadata    string `json:"metadata,omitempty"` // JSON blob for extra data (e.g., arg expressions for DATA_FLOWS)
	CreatedAt   string `json:"createdAt,omitempty"`
}

// ProjectInfo describes an indexed project.
type ProjectInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Root      string `json:"root"`
	IndexedAt string `json:"indexedAt"`
	Status    string `json:"status"` // indexing, ready, error
	Meta      string `json:"meta,omitempty"` // JSON blob
	NodeCount int    `json:"nodeCount,omitempty"`
	EdgeCount int    `json:"edgeCount,omitempty"`
}

// IndexResult is returned after project indexing.
type IndexResult struct {
	ProjectName string    `json:"projectName"`
	ProjectRoot string    `json:"projectRoot"`
	Mode        IndexMode `json:"mode"`
	FilesParsed int       `json:"filesParsed"`
	NodesStored int       `json:"nodesStored"`
	EdgesStored int       `json:"edgesStored"`
	Errors      int       `json:"errors,omitempty"`
}

// TraceHop represents a hop in a call chain trace.
type TraceHop struct {
	SourceQN string `json:"sourceQN"`
	TargetQN string `json:"targetQN"`
	EdgeType string `json:"edgeType"`
	Depth    int    `json:"depth"`
	File     string `json:"file,omitempty"`
	Line     uint32 `json:"line,omitempty"`
}

// ADR represents an Architecture Decision Record.
type ADR struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Status    string `json:"status"` // proposed, accepted, deprecated, superseded
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Memo represents a persistent memory entry.
type Memo struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Type      string `json:"type"` // decision, architecture, pattern, bugfix, learning, discovery, config
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
