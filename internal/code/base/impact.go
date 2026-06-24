package base

import (
	"fmt"
	"strings"
)

// ImpactedSymbol represents a code symbol affected by a set of changes.
type ImpactedSymbol struct {
	QualifiedName string `json:"qualifiedName"`
	Kind          string `json:"kind"`
	FilePath      string `json:"filePath"`
	ChangeDepth   int    `json:"changeDepth"` // 0 = directly in changed file, 1+ = propagated
	Changed       bool   `json:"changed"`     // true if directly in a changed file (depth=0)
}

// ImpactAnalysis finds all symbols affected by changes to the given files.
// It propagates along CALLS edges (both inbound and outbound) up to depth levels.
//
// Returns ImpactedSymbols sorted by change depth (direct changes first),
// then by qualified name.
func (s *Store) ImpactAnalysis(project string, changedFiles []string, depth int) ([]ImpactedSymbol, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if depth <= 0 {
		depth = 2
	}
	if depth > 10 {
		depth = 10 // safety limit
	}
	if len(changedFiles) == 0 {
		return nil, nil
	}

	// Step 1: find all nodes that are directly in changed files
	changedSet := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	nodeRows, err := s.db.Query(`
		SELECT n.qualified_name, n.kind, n.file_path
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = ?
	`, project)
	if err != nil {
		return nil, fmt.Errorf("impact: query nodes: %w", err)
	}
	defer nodeRows.Close()

	type qnInfo struct {
		qn   string
		kind string
		fp   string
	}

	var directQNs []string
	allNodes := make(map[string]qnInfo)

	for nodeRows.Next() {
		var qn, kind, fp string
		if err := nodeRows.Scan(&qn, &kind, &fp); err != nil {
			return nil, fmt.Errorf("impact: scan node: %w", err)
		}
		allNodes[qn] = qnInfo{qn: qn, kind: kind, fp: fp}
		if changedSet[fp] {
			directQNs = append(directQNs, qn)
		}
	}
	if err := nodeRows.Err(); err != nil {
		return nil, err
	}

	if len(directQNs) == 0 {
		return nil, nil
	}

	// Step 2: propagate along CALLS edges using recursive CTE.
	// Build seed VALUES from direct QNs: ('qn1',0), ('qn2',0), ...
	var seedParts []string
	for _, qn := range directQNs {
		seedParts = append(seedParts, fmt.Sprintf("(%q, 0)", qn))
	}
	seed := strings.Join(seedParts, ", ")

	query := fmt.Sprintf(`
		WITH RECURSIVE impact(qn, lvl) AS (
			VALUES %s
			UNION
			SELECT e.source_qn, impact.lvl + 1
			FROM edges e
			JOIN impact ON e.target_qn = impact.qn
			WHERE e.edge_type = 'CALLS' AND impact.lvl < ?
			UNION
			SELECT e.target_qn, impact.lvl + 1
			FROM edges e
			JOIN impact ON e.source_qn = impact.qn
			WHERE e.edge_type = 'CALLS' AND impact.lvl < ?
		)
		SELECT impact.qn, MIN(impact.lvl) as min_lvl
		FROM impact
		GROUP BY impact.qn
		ORDER BY min_lvl, impact.qn
	`, seed)

	result, err := s.db.Query(query, depth, depth)
	if err != nil {
		return nil, fmt.Errorf("impact: propagate: %w", err)
	}
	defer result.Close()

	// Map QN → min depth
	minDepth := make(map[string]int)
	for _, qn := range directQNs {
		minDepth[qn] = 0
	}

	for result.Next() {
		var qn string
		var lvl int
		if err := result.Scan(&qn, &lvl); err != nil {
			return nil, fmt.Errorf("impact: scan result: %w", err)
		}
		if existing, ok := minDepth[qn]; !ok || lvl < existing {
			minDepth[qn] = lvl
		}
	}
	if err := result.Err(); err != nil {
		return nil, err
	}

	// Step 3: build ordered result — direct changes first, then propagated
	impacted := make([]ImpactedSymbol, 0, len(minDepth))
	seen := make(map[string]bool)

	// Direct changes first (depth = 0)
	for _, qn := range directQNs {
		if seen[qn] {
			continue
		}
		seen[qn] = true
		info := allNodes[qn]
		impacted = append(impacted, ImpactedSymbol{
			QualifiedName: qn,
			Kind:          info.kind,
			FilePath:      info.fp,
			ChangeDepth:   0,
			Changed:       true,
		})
	}

	// Propagated symbols (depth > 0)
	for qn, lvl := range minDepth {
		if seen[qn] {
			continue
		}
		seen[qn] = true
		info := allNodes[qn]
		impacted = append(impacted, ImpactedSymbol{
			QualifiedName: qn,
			Kind:          info.kind,
			FilePath:      info.fp,
			ChangeDepth:   lvl,
			Changed:       false,
		})
	}

	return impacted, nil
}
