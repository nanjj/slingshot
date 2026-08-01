package base

import (
	"fmt"
	"path"
	"strings"
)

// ResolveProject resolves a project identifier to its ProjectInfo.
//
// The identifier may be:
//  1. an exact project name ("dscli")
//  2. a full or trailing-slash root path ("/home/me/src/dscli", "/home/me/src/dscli/")
//  3. the root's basename ("dscli" when the root is ".../dscli")
//  4. a path suffix of the root ("me/src/dscli")
//  5. a case-insensitive name match
//  6. a unique substring of a name or root (only if exactly one project matches)
//
// This lets LLM agents pass whatever identifier they have on hand — short
// name, full path, or partial path — without guessing the indexed name.
func (s *Store) ResolveProject(identifier string) (*ProjectInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, fmt.Errorf("project is required")
	}

	projects, err := s.listProjectsLocked()
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects indexed")
	}

	norm := func(p string) string { return strings.TrimRight(p, "/") }
	id := norm(identifier)

	// 1. Exact name, exact root, normalized root.
	for _, p := range projects {
		if p.Name == identifier || p.Root == identifier || norm(p.Root) == id {
			return &p, nil
		}
	}

	// 2. Basename / path-suffix matches.
	for _, p := range projects {
		root := norm(p.Root)
		if path.Base(root) == path.Base(id) || strings.HasSuffix(root, id) {
			return &p, nil
		}
	}

	// 3. Case-insensitive name match.
	for _, p := range projects {
		if strings.EqualFold(p.Name, identifier) {
			return &p, nil
		}
	}

	// 4. Unique substring match on name or root.
	var matches []ProjectInfo
	for _, p := range projects {
		if strings.Contains(p.Name, identifier) || strings.Contains(norm(p.Root), identifier) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("project %q not found", identifier)
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s (%s)", m.Name, m.Root)
		}
		return nil, fmt.Errorf("project %q is ambiguous, matches: %s", identifier, strings.Join(names, ", "))
	}
}

// listProjectsLocked lists all projects; the caller must hold s.mu (read or write).
func (s *Store) listProjectsLocked() ([]ProjectInfo, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.root, p.indexed_at, p.status, p.meta,
		       (SELECT COUNT(*) FROM nodes WHERE project_id = p.id) AS node_count,
		       (SELECT COUNT(*) FROM edges WHERE project_id = p.id) AS edge_count
		FROM projects p
		ORDER BY p.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []ProjectInfo
	for rows.Next() {
		var p ProjectInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.Root, &p.IndexedAt, &p.Status, &p.Meta,
			&p.NodeCount, &p.EdgeCount); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
