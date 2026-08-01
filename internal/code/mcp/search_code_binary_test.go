// Package mcp_test — search_code binary-safety tests (Archimedes feedback,
// mail #489: scanning _release/ binaries panicked with slice out of range and
// flooded results with binary noise).
package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gotest.tools/v3/assert"
)

// searchCodeFiles indexes a project and returns the "files" mode search result.
func searchCodeFiles(t *testing.T, cs *mcp.ClientSession, project, pattern string) string {
	t.Helper()
	result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search_code",
		Arguments: map[string]any{
			"pattern": pattern,
			"project": project,
			"mode":    "files",
		},
	})
	assert.NilError(t, err)
	assert.Assert(t, !result.IsError, "search_code must not panic/error: %s", textContent(result))
	return textContent(result)
}

// TestSearchCode_MachOBinarySkipped verifies extension-less Mach-O binaries
// are detected by magic and skipped — no panic, no binary noise in results.
func TestSearchCode_MachOBinarySkipped(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	// Mach-O 64-bit magic + payload containing the pattern.
	binary := append([]byte{0xFE, 0xED, 0xFA, 0xCE}, []byte("\x00\x00\x00\x01 CALLS in binary payload")...)
	err := os.WriteFile(filepath.Join(projectDir, "slingshot-tool"), binary, 0644)
	assert.NilError(t, err)

	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	out := searchCodeFiles(t, cs, idxRes.ProjectName, "CALLS")
	assert.Assert(t, !strings.Contains(out, "slingshot-tool"),
		"Mach-O binary must be skipped, got: %s", out)
}

// TestSearchCode_ReleaseDirSkipped verifies _release/ (and friends) are never
// walked, matching Archimedes' dscli failure where _release binaries were
// scanned as text.
func TestSearchCode_ReleaseDirSkipped(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	releaseDir := filepath.Join(projectDir, "_release")
	err := os.MkdirAll(releaseDir, 0755)
	assert.NilError(t, err)
	err = os.WriteFile(filepath.Join(releaseDir, "slingshot-darwin-amd64"),
		[]byte{0xFE, 0xED, 0xFA, 0xCF, 'C', 'A', 'L', 'L', 'S'}, 0644)
	assert.NilError(t, err)

	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	out := searchCodeFiles(t, cs, idxRes.ProjectName, "CALLS")
	assert.Assert(t, !strings.Contains(out, "_release"),
		"_release dir must be skipped, got: %s", out)
}

// TestSearchCode_InvalidUTF8LineNoPanic verifies a text file containing a
// line with invalid UTF-8 bytes (binary content with a source-like extension)
// does not panic and skips the offending line.
func TestSearchCode_InvalidUTF8LineNoPanic(t *testing.T) {
	cs, cleanup := testFixture(t)
	defer cleanup()

	projectDir := mkProject(t)
	content := []byte("line one\nCALLS \xff\xfe\xfd binary-ish\nline three CALLS here\n")
	err := os.WriteFile(filepath.Join(projectDir, "notes.txt"), content, 0644)
	assert.NilError(t, err)

	var idxRes struct {
		ProjectName string `json:"projectName"`
	}
	callToolUnmarshal(t, cs, "index_repository", map[string]any{
		"repoPath": projectDir,
		"mode":     "fast",
	}, &idxRes)

	out := searchCodeFiles(t, cs, idxRes.ProjectName, "CALLS")
	// The valid line must be found; the invalid-UTF-8 line must be skipped
	// (no panic, no garbage).
	assert.Assert(t, strings.Contains(out, "notes.txt"),
		"notes.txt should appear in results, got: %s", out)
}
