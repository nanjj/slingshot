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

// Cluster represents a structural community detected from the call/import graph.
type Cluster struct {
	Label          string   `json:"label"`          // auto-generated name (top package/representative)
	Size           int      `json:"size"`           // number of nodes in the cluster
	InternalEdges  int      `json:"internalEdges"`  // edges with both endpoints inside the cluster
	ExternalEdges  int      `json:"externalEdges"`  // edges crossing cluster boundary
	Cohesion       float64  `json:"cohesion"`       // internal / total edges ratio (0–1)
	TopNodes       []string `json:"topNodes"`       // highest-degree nodes in the cluster
	Packages       []string `json:"packages"`       // packages that dominate this cluster
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

// ─── Community Detection ───────────────────────────────────────────────────
//
// Clusters detects structural communities from the CALLS and IMPORTS edge
// graph using connected components.  Each component becomes a cluster with
// cohesion (internal-edge ratio), top nodes (highest degree), and dominant
// packages.
//
// Limits total clusters returned (default 20) to keep the response size
// manageable.  Use limit=0 for all.
func (s *Store) Clusters(project string, limit int) ([]Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	// 1. Fetch all CALLS and IMPORTS edges for the project.
	rows, err := s.db.Query(`
		SELECT e.source_qn, e.target_qn, e.edge_type
		FROM edges e
		JOIN projects p ON p.id = e.project_id
		WHERE p.name = ? AND e.edge_type IN ('CALLS', 'IMPORTS')
	`, project)
	if err != nil {
		return nil, fmt.Errorf("clusters: %w", err)
	}
	defer rows.Close()

	// adj[n] = neighbours of n
	adj := make(map[string]map[string]bool)
	// edgeSet stores "src→tgt:type" for dedup and internal/edge counting
	edgeSet := make(map[string]struct{ src, tgt, etype string })
	// nodeDegrees[n] = degree count
	nodeDegrees := make(map[string]int)

	for rows.Next() {
		var src, tgt, etype string
		if err := rows.Scan(&src, &tgt, &etype); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		key := src + "→" + tgt + ":" + etype
		if _, ok := edgeSet[key]; ok {
			continue // deduplicate
		}
		edgeSet[key] = struct{ src, tgt, etype string }{src, tgt, etype}

		// Undirected adjacency for clustering
		if adj[src] == nil {
			adj[src] = make(map[string]bool)
		}
		adj[src][tgt] = true
		if adj[tgt] == nil {
			adj[tgt] = make(map[string]bool)
		}
		adj[tgt][src] = true

		nodeDegrees[src]++
		nodeDegrees[tgt]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(adj) == 0 {
		return []Cluster{}, nil
	}

	// 2. BFS connected components
	visited := make(map[string]bool)
	var clusters []Cluster

	for node := range adj {
		if visited[node] {
			continue
		}
		// BFS this component
		component := make(map[string]bool)
		queue := []string{node}
		visited[node] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			component[cur] = true
			for nb := range adj[cur] {
				if !visited[nb] {
					visited[nb] = true
					queue = append(queue, nb)
				}
			}
		}

		// 3. Compute cluster metrics
		memberCount := len(component)
		internalEdges := 0
		externalEdges := 0

		// For each edge, check if both endpoints are in this component
		for _, e := range edgeSet {
			_, srcIn := component[e.src]
			_, tgtIn := component[e.tgt]
			if srcIn || tgtIn {
				// Edge involves this component
				if srcIn && tgtIn {
					internalEdges++
				} else {
					externalEdges++
				}
			}
		}

		// 4. Find top nodes (highest degree)
		type degreeItem struct {
			qn    string
			deg   int
		}
		var top []degreeItem
		for n := range component {
			top = append(top, degreeItem{n, nodeDegrees[n]})
		}
		// Sort by degree descending
		for i := 1; i < len(top); i++ {
			for j := i; j > 0 && top[j].deg > top[j-1].deg; j-- {
				top[j], top[j-1] = top[j-1], top[j]
			}
		}
		topCount := 5
		if len(top) < topCount {
			topCount = len(top)
		}
		topNodes := make([]string, topCount)
		for i := 0; i < topCount; i++ {
			topNodes[i] = top[i].qn
		}

		// 5. Determine dominant packages
		pkgSet := make(map[string]int)
		for n := range component {
			pkg := extractPackage(n, project)
			pkgSet[pkg]++
		}
		packages := make([]string, 0, len(pkgSet))
		for p := range pkgSet {
			packages = append(packages, p)
		}
		// Sort packages by frequency
		for i := 1; i < len(packages); i++ {
			for j := i; j > 0 && pkgSet[packages[j]] > pkgSet[packages[j-1]]; j-- {
				packages[j], packages[j-1] = packages[j-1], packages[j]
			}
		}
		if len(packages) > 5 {
			packages = packages[:5]
		}

		// 6. Generate label from top package or top node
		label := packages[0]
		if label == "" && len(topNodes) > 0 {
			label = topNodes[0]
		}

		totalEdges := internalEdges + externalEdges
		var cohesion float64
		if totalEdges > 0 {
			cohesion = float64(internalEdges) / float64(totalEdges)
		} else {
			cohesion = 1.0 // singleton cluster with no edges
		}

		clusters = append(clusters, Cluster{
			Label:          label,
			Size:           memberCount,
			InternalEdges:  internalEdges,
			ExternalEdges:  externalEdges,
			Cohesion:       cohesion,
			TopNodes:       topNodes,
			Packages:       packages,
		})
	}

	// 7. Sort clusters by size descending
	for i := 1; i < len(clusters); i++ {
		for j := i; j > 0 && clusters[j].Size > clusters[j-1].Size; j-- {
			clusters[j], clusters[j-1] = clusters[j-1], clusters[j]
		}
	}

	if len(clusters) > limit {
		clusters = clusters[:limit]
	}

	return clusters, nil
}
