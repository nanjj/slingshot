package base

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ─── Data types ──────────────────────────────────────────────────────────────

// Hotspot represents a function/method with high fan-in (inbound call count).
type Hotspot struct {
	QualifiedName string `json:"qualifiedName"`
	Kind          string `json:"kind"`
	File          string `json:"file"`
	FanIn         int    `json:"fanIn"`
	Complexity    int    `json:"complexity,omitempty"`
	Lines         int    `json:"lines,omitempty"`
}

// PackageDep represents a dependency between two packages, aggregated from
// CALLS edges.  Source → Target with a call count.
type PackageDep struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Count  int    `json:"count"`
}

// FileTreeEntry represents a directory in the project with file and node counts.
type FileTreeEntry struct {
	Path      string `json:"path"`
	FileCount int    `json:"fileCount"`
	NodeCount int    `json:"nodeCount"`
}

// ─── Hotspots (fan-in analysis) ──────────────────────────────────────────────

// Hotspots returns the top N functions/methods ranked by fan-in (number of
// inbound CALLS edges).  Only considers functions and methods, not variables,
// files or other node kinds.
func (s *Store) Hotspots(project string, limit int) ([]Hotspot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
		SELECT n.qualified_name, n.kind, n.file_path,
		       COUNT(e.id) AS fan_in,
		       COALESCE(n.complexity, 0),
		       COALESCE(n.end_line - n.line, 0) AS lines
		FROM nodes n
		JOIN edges e ON e.target_qn = n.qualified_name AND e.edge_type = 'CALLS'
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = ? AND n.kind IN ('function', 'method')
		GROUP BY n.qualified_name
		ORDER BY fan_in DESC
		LIMIT ?
	`, project, limit)
	if err != nil {
		return nil, fmt.Errorf("hotspots: %w", err)
	}
	defer rows.Close()

	var hotspots []Hotspot
	for rows.Next() {
		var h Hotspot
		if err := rows.Scan(&h.QualifiedName, &h.Kind, &h.File, &h.FanIn, &h.Complexity, &h.Lines); err != nil {
			return nil, fmt.Errorf("scan hotspot: %w", err)
		}
		hotspots = append(hotspots, h)
	}
	return hotspots, rows.Err()
}

// ─── Package-level dependency aggregation ────────────────────────────────────

// extractPackage derives the Go package path from a qualified name by stripping
// the project prefix and the final component (function/type name).
//
//	Input:  "project.internal.code.base.store.OpenStore"
//	Project: "project"
//	Output: "internal.code.base.store"
func extractPackage(qn, project string) string {
	trimmed := strings.TrimPrefix(qn, project+".")
	lastDot := strings.LastIndex(trimmed, ".")
	if lastDot < 0 {
		return trimmed
	}
	return trimmed[:lastDot]
}

// PackageDeps aggregates CALLS edges into package-level dependency pairs.
// Only cross-package calls are included (self-dependencies filtered out).
func (s *Store) PackageDeps(project string) ([]PackageDep, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT e.source_qn, e.target_qn
		FROM edges e
		JOIN projects p ON p.id = e.project_id
		WHERE p.name = ? AND e.edge_type = 'CALLS'
	`, project)
	if err != nil {
		return nil, fmt.Errorf("package deps: %w", err)
	}
	defer rows.Close()

	type pair struct{ src, tgt string }
	depCount := make(map[pair]int)

	for rows.Next() {
		var srcQN, tgtQN string
		if err := rows.Scan(&srcQN, &tgtQN); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		srcPkg := extractPackage(srcQN, project)
		tgtPkg := extractPackage(tgtQN, project)
		if srcPkg != tgtPkg {
			depCount[pair{srcPkg, tgtPkg}]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	deps := make([]PackageDep, 0, len(depCount))
	for p, cnt := range depCount {
		deps = append(deps, PackageDep{Source: p.src, Target: p.tgt, Count: cnt})
	}

	// Sort by count descending (simple insertion sort for small N)
	for i := 1; i < len(deps); i++ {
		for j := i; j > 0 && deps[j].Count > deps[j-1].Count; j-- {
			deps[j], deps[j-1] = deps[j-1], deps[j]
		}
	}
	return deps, nil
}

// ─── File tree (directory-grouped statistics) ────────────────────────────────

// FileTree returns per-directory counts of files and nodes, ordered by path.
func (s *Store) FileTree(project string) ([]FileTreeEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT n.file_path, COUNT(*) AS cnt
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = ?
		GROUP BY n.file_path
		ORDER BY n.file_path
	`, project)
	if err != nil {
		return nil, fmt.Errorf("file tree: %w", err)
	}
	defer rows.Close()

	dirMap := make(map[string]*FileTreeEntry)
	var order []string

	for rows.Next() {
		var fp string
		var cnt int
		if err := rows.Scan(&fp, &cnt); err != nil {
			return nil, fmt.Errorf("scan file tree row: %w", err)
		}
		dir := filepath.Dir(fp)
		if dir == "." {
			dir = "/"
		}
		if _, ok := dirMap[dir]; !ok {
			dirMap[dir] = &FileTreeEntry{Path: dir}
			order = append(order, dir)
		}
		dirMap[dir].FileCount++
		dirMap[dir].NodeCount += cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]FileTreeEntry, len(order))
	for i, d := range order {
		result[i] = *dirMap[d]
	}
	return result, nil
}
