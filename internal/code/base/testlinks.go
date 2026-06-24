package base

import (
	"fmt"
	"path/filepath"
	"strings"
)

// LinkTests post-processes indexed data to create TESTS and TESTS_FILE edges.
//
// Heuristics:
//   - A function is a "test function" if its file path contains "_test" and
//     its name starts with "Test" (Go convention).
//   - For each CALLS edge from a test function to a non-test function in the
//     same project, a TESTS edge is created: test_function → tested_function.
//   - For each test file, a TESTS_FILE edge is created to the corresponding
//     source file (foo_test.go → foo.go).
//
// Returns the created edges (not yet saved to the database).
func (s *Store) LinkTests(projectID int64, project string) ([]Edge, error) {
	// 1. Find all test functions in this project
	rows, err := s.db.Query(`
		SELECT n.qualified_name, n.file_path
		FROM nodes n
		JOIN projects p ON p.id = n.project_id
		WHERE p.name = ?
		  AND n.kind IN ('function', 'method')
		  AND n.file_path LIKE '%\_test%' ESCAPE '\'
		  AND n.name LIKE 'Test%'
	`, project)
	if err != nil {
		return nil, fmt.Errorf("query test functions: %w", err)
	}
	defer rows.Close()

	type testFn struct {
		qn       string
		filePath string
	}
	var testFns []testFn
	for rows.Next() {
		var fn testFn
		if err := rows.Scan(&fn.qn, &fn.filePath); err != nil {
			return nil, fmt.Errorf("scan test function: %w", err)
		}
		testFns = append(testFns, fn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(testFns) == 0 {
		return nil, nil
	}

	// 2. For each test function, find CALLS edges to non-test functions
	var edges []Edge
	seenTESTS := make(map[string]bool) // dedup: "source_qn→target_qn"
	seenTESTSFile := make(map[string]bool)

	for _, tf := range testFns {
		// Find CALLS edges from this test function
		callRows, err := s.db.Query(`
			SELECT e.target_qn
			FROM edges e
			WHERE e.project_id = ? AND e.source_qn = ? AND e.edge_type = 'CALLS'
		`, projectID, tf.qn)
		if err != nil {
			return nil, fmt.Errorf("query calls for %s: %w", tf.qn, err)
		}

		for callRows.Next() {
			var targetQN string
			if err := callRows.Scan(&targetQN); err != nil {
				callRows.Close()
				return nil, fmt.Errorf("scan call target: %w", err)
			}

			// Check if the target is a non-test function in the same project
			var tgtIsTest bool
			var tgtFile string
			err := s.db.QueryRow(`
				SELECT INSTR(n.file_path, '_test.') > 0, COALESCE(n.file_path, '')
				FROM nodes n
				JOIN projects p ON p.id = n.project_id
				WHERE p.name = ? AND n.qualified_name = ?
			`, project, targetQN).Scan(&tgtIsTest, &tgtFile)
			if err != nil {
				// Target not found in project (e.g., external call) — skip
				continue
			}
			if tgtIsTest {
				// Target is also a test function — skip
				continue
			}

			// Create TESTS edge: test_function → tested_function
			key := tf.qn + "→" + targetQN
			if !seenTESTS[key] {
				seenTESTS[key] = true
				edges = append(edges, Edge{
					ProjectID: projectID,
					SourceQN:  tf.qn,
					TargetQN:  targetQN,
					EdgeType:  "TESTS",
				})
			}
		}
		callRows.Close()
		if err := callRows.Err(); err != nil {
			return nil, err
		}

		// 3. Create TESTS_FILE edge: test_file → source_file
		// Heuristic: foo_test.go → foo.go
		srcFile := testFileToSourceFile(tf.filePath)
		if srcFile != "" {
			key := tf.filePath + "→" + srcFile
			if !seenTESTSFile[key] {
				seenTESTSFile[key] = true
				edges = append(edges, Edge{
					ProjectID: projectID,
					SourceQN:  tf.filePath,
					TargetQN:  srcFile,
					EdgeType:  "TESTS_FILE",
				})
			}
		}
	}

	return edges, nil
}

// testFileToSourceFile converts a test file path to its corresponding source file.
// Examples:
//
//	"foo_test.go" → "foo.go"
//	"sub/bar_test.go" → "sub/bar.go"
//	"foo.test.go" → "foo.go"
func testFileToSourceFile(testPath string) string {
	dir := filepath.Dir(testPath)
	base := filepath.Base(testPath)
	ext := filepath.Ext(base)

	// Strip the extension (e.g., ".go")
	name := strings.TrimSuffix(base, ext)

	// Strip "_test" suffix
	if strings.HasSuffix(name, "_test") {
		name = strings.TrimSuffix(name, "_test")
	} else if strings.HasSuffix(name, ".test") {
		name = strings.TrimSuffix(name, ".test")
	} else {
		return ""
	}

	// Handle edge case: if the name is empty after stripping, skip
	if name == "" {
		return ""
	}

	return filepath.Join(dir, name+ext)
}
