package edit

// EditResult 是编辑操作的结果。
type EditResult struct {
	Success     bool     `json:"success"`
	NewSource   string   `json:"newSource,omitempty"`
	Error       string   `json:"error,omitempty"`
	ByteDiff    int      `json:"byteDiff"`
	ParseErrors []string `json:"parseErrors,omitempty"`
}
